package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type WorkOrderService struct {
	db         *sql.DB
	workOrders repository.WorkOrderRepository
	audit      *AuditService
	jobRuns    *JobRunService
}

func NewWorkOrderService(db *sql.DB, workOrders repository.WorkOrderRepository, audit *AuditService, jobRuns *JobRunService) *WorkOrderService {
	return &WorkOrderService{
		db:         db,
		workOrders: workOrders,
		audit:      audit,
		jobRuns:    jobRuns,
	}
}

type QueueWorkOrderInput struct {
	ServiceType string
	Priority    string
	AssigneeID  *int64
}

type StartWorkOrderInput struct {
	WorkOrderID int64
}

type CompleteWorkOrderInput struct {
	WorkOrderID int64
	ActorID     *int64
	RequestID   string
}

type CompleteWorkOrderResult struct {
	WorkOrder      *domain.WorkOrder `json:"work_order"`
	LatencySeconds int64             `json:"latency_seconds"`
	OnTime15m      bool              `json:"on_time_15m"`
}

func (s *WorkOrderService) List(ctx context.Context, filter repository.WorkOrderFilter) ([]domain.WorkOrder, error) {
	if filter.Status != "" && !domain.IsValidWorkOrderStatus(filter.Status) {
		return nil, fmt.Errorf("%w: invalid work order status filter", ErrValidation)
	}
	if filter.Priority != "" && !domain.IsValidWorkOrderPriority(filter.Priority) {
		return nil, fmt.Errorf("%w: invalid work order priority filter", ErrValidation)
	}
	return s.workOrders.List(ctx, filter)
}

func (s *WorkOrderService) Queue(ctx context.Context, input QueueWorkOrderInput) (*domain.WorkOrder, error) {
	serviceType := strings.TrimSpace(input.ServiceType)
	if serviceType == "" {
		return nil, fmt.Errorf("%w: service_type is required", ErrValidation)
	}

	priority := strings.TrimSpace(input.Priority)
	if priority == "" {
		priority = domain.WorkOrderPriorityNormal
	}
	if !domain.IsValidWorkOrderPriority(priority) {
		return nil, fmt.Errorf("%w: invalid priority", ErrValidation)
	}

	workOrder := &domain.WorkOrder{
		ServiceType: serviceType,
		Priority:    priority,
		Status:      domain.WorkOrderStatusQueued,
		AssigneeID:  input.AssigneeID,
	}

	if err := s.workOrders.Create(ctx, workOrder); err != nil {
		return nil, err
	}

	created, err := s.workOrders.GetByID(ctx, workOrder.ID)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *WorkOrderService) Start(ctx context.Context, input StartWorkOrderInput) (*domain.WorkOrder, error) {
	if input.WorkOrderID <= 0 {
		return nil, fmt.Errorf("%w: work_order_id must be positive", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin start work order tx: %w", err)
	}
	defer tx.Rollback()

	current, err := s.workOrders.GetByIDTx(ctx, tx, input.WorkOrderID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, fmt.Errorf("%w: work order not found", ErrNotFound)
		}
		return nil, err
	}

	if current.Status != domain.WorkOrderStatusQueued {
		return nil, fmt.Errorf("%w: work order cannot be started from current status", ErrConflict)
	}

	now := time.Now().UTC()
	updated, err := s.workOrders.StartTx(ctx, tx, current.ID, current.Version, now)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("%w: work order version mismatch", ErrVersionConflict)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit start work order tx: %w", err)
	}

	started, err := s.workOrders.GetByID(ctx, input.WorkOrderID)
	if err != nil {
		return nil, err
	}
	return started, nil
}

func (s *WorkOrderService) Complete(ctx context.Context, input CompleteWorkOrderInput) (*CompleteWorkOrderResult, error) {
	if input.WorkOrderID <= 0 {
		return nil, fmt.Errorf("%w: work_order_id must be positive", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin complete work order tx: %w", err)
	}
	defer tx.Rollback()

	current, err := s.workOrders.GetByIDTx(ctx, tx, input.WorkOrderID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, fmt.Errorf("%w: work order not found", ErrNotFound)
		}
		return nil, err
	}

	if current.Status != domain.WorkOrderStatusInProgress || current.StartedAt == nil {
		return nil, fmt.Errorf("%w: work order cannot be completed from current status", ErrConflict)
	}

	now := time.Now().UTC()
	updated, err := s.workOrders.CompleteTx(ctx, tx, current.ID, current.Version, now)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("%w: work order version mismatch", ErrVersionConflict)
	}

	latencySeconds := int64(now.Sub(*current.StartedAt).Seconds())
	if latencySeconds < 0 {
		latencySeconds = 0
	}
	onTime15m := latencySeconds <= 900

	if err := s.jobRuns.RecordTx(ctx, tx, JobRunInput{
		JobType:    "work_order_completion",
		StartedAt:  now,
		FinishedAt: now,
		Status:     "success",
		Summary: map[string]any{
			"work_order_id":     current.ID,
			"service_type":      current.ServiceType,
			"latency_seconds":   latencySeconds,
			"on_time_15m":       onTime15m,
			"completed_at_unix": now.Unix(),
		},
	}); err != nil {
		return nil, err
	}

	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		requestID = "system"
	}

	if err := s.audit.LogEventTx(ctx, tx, AuditLogInput{
		ActorID:      input.ActorID,
		Action:       "work_order.complete",
		ResourceType: "work_order",
		ResourceID:   fmt.Sprintf("%d", current.ID),
		Before: map[string]any{
			"status":     current.Status,
			"started_at": current.StartedAt,
			"version":    current.Version,
		},
		After: map[string]any{
			"status":          domain.WorkOrderStatusCompleted,
			"completed_at":    now,
			"latency_seconds": latencySeconds,
			"on_time_15m":     onTime15m,
		},
		RequestID: requestID,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit complete work order tx: %w", err)
	}

	completed, err := s.workOrders.GetByID(ctx, current.ID)
	if err != nil {
		return nil, err
	}

	return &CompleteWorkOrderResult{
		WorkOrder:      completed,
		LatencySeconds: latencySeconds,
		OnTime15m:      onTime15m,
	}, nil
}
