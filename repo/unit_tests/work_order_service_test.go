package unit_tests

import (
	"context"
	"errors"
	"testing"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestWorkOrderCompletionCriticalTransition(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditService := service.NewAuditService(sqlite.NewAuditRepository(db))
	jobRuns := service.NewJobRunService(sqlite.NewJobRunRepository(db))
	workOrders := service.NewWorkOrderService(db, sqlite.NewWorkOrderRepository(db), auditService, jobRuns)

	queued, err := workOrders.Queue(ctx, service.QueueWorkOrderInput{ServiceType: "lab", Priority: domain.WorkOrderPriorityHigh})
	if err != nil {
		t.Fatalf("queue work order: %v", err)
	}

	started, err := workOrders.Start(ctx, service.StartWorkOrderInput{WorkOrderID: queued.ID})
	if err != nil {
		t.Fatalf("start work order: %v", err)
	}
	if started.Status != domain.WorkOrderStatusInProgress || started.StartedAt == nil {
		t.Fatalf("expected in_progress with started_at, got %+v", started)
	}

	completed, err := workOrders.Complete(ctx, service.CompleteWorkOrderInput{WorkOrderID: queued.ID, RequestID: "req_unit_workorder"})
	if err != nil {
		t.Fatalf("complete work order: %v", err)
	}
	if completed.WorkOrder.Status != domain.WorkOrderStatusCompleted {
		t.Fatalf("expected completed status, got %+v", completed.WorkOrder)
	}

	items, err := workOrders.List(ctx, repository.WorkOrderFilter{Status: domain.WorkOrderStatusCompleted})
	if err != nil {
		t.Fatalf("list completed work orders: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly one completed work order, got %d", len(items))
	}
}

func TestWorkOrderCompleteWithoutStartFails(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditService := service.NewAuditService(sqlite.NewAuditRepository(db))
	jobRuns := service.NewJobRunService(sqlite.NewJobRunRepository(db))
	workOrders := service.NewWorkOrderService(db, sqlite.NewWorkOrderRepository(db), auditService, jobRuns)

	queued, err := workOrders.Queue(ctx, service.QueueWorkOrderInput{ServiceType: "radiology", Priority: domain.WorkOrderPriorityNormal})
	if err != nil {
		t.Fatalf("queue work order: %v", err)
	}

	if _, err := workOrders.Complete(ctx, service.CompleteWorkOrderInput{WorkOrderID: queued.ID}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected conflict completing queued work order, got: %v", err)
	}
}
