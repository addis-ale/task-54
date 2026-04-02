package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

var ErrOutsideSettlementWindow = fmt.Errorf("%w: settlement can only run within ±30 minutes of a shift-close time (07:00, 15:00, 23:00)", ErrValidation)

type SettlementService struct {
	db                    *sql.DB
	payments              repository.PaymentRepository
	settlements           repository.SettlementRepository
	audit                 *AuditService
	jobRuns               *JobRunService
	logs                  *StructuredLogService
	settlementWindowMins  int
	adminOverrideSettlement bool
}

func NewSettlementService(
	db *sql.DB,
	payments repository.PaymentRepository,
	settlements repository.SettlementRepository,
	audit *AuditService,
	jobRuns *JobRunService,
	logs *StructuredLogService,
) *SettlementService {
	return &SettlementService{
		db:                   db,
		payments:             payments,
		settlements:          settlements,
		audit:                audit,
		jobRuns:              jobRuns,
		logs:                 logs,
		settlementWindowMins: 30,
	}
}

// SetSettlementWindow sets the ±minutes window around shift-close times.
func (s *SettlementService) SetSettlementWindow(minutes int) {
	if minutes > 0 {
		s.settlementWindowMins = minutes
	}
}

// SetAdminOverride enables or disables the admin override for settlement window enforcement.
func (s *SettlementService) SetAdminOverride(override bool) {
	s.adminOverrideSettlement = override
}

// isWithinShiftCloseWindow checks if the given time is within ±windowMins of any shift-close time.
func isWithinShiftCloseWindow(now time.Time, windowMins int) bool {
	shiftCloseHours := []int{7, 15, 23}
	window := time.Duration(windowMins) * time.Minute

	for _, hour := range shiftCloseHours {
		shiftClose := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		if now.After(shiftClose.Add(-window)) && now.Before(shiftClose.Add(window)) {
			return true
		}
	}
	// Also check previous day's 23:00 for early-morning window
	prevDay23 := time.Date(now.Year(), now.Month(), now.Day()-1, 23, 0, 0, 0, now.Location())
	if now.After(prevDay23.Add(-window)) && now.Before(prevDay23.Add(window)) {
		return true
	}
	return false
}

type RunSettlementInput struct {
	ShiftID          string
	ActualTotalCents int64
	ActorID          *int64
	RequestID        string
}

type RunSettlementResult struct {
	Settlement  *domain.Settlement      `json:"settlement"`
	Items       []domain.SettlementItem `json:"items"`
	Discrepancy int64                   `json:"discrepancy_cents"`
}

func (s *SettlementService) RunShift(ctx context.Context, input RunSettlementInput) (*RunSettlementResult, error) {
	if !s.adminOverrideSettlement && !isWithinShiftCloseWindow(time.Now().UTC(), s.settlementWindowMins) {
		return nil, ErrOutsideSettlementWindow
	}

	shiftID, err := normalizeShiftID(input.ShiftID)
	if err != nil {
		return nil, err
	}
	if input.ActualTotalCents < 0 {
		return nil, fmt.Errorf("%w: actual_total_cents must be non-negative", ErrValidation)
	}

	startedAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin settlement tx: %w", err)
	}
	defer tx.Rollback()

	payments, expectedTotal, err := s.payments.ListSucceededByShiftTx(ctx, tx, shiftID)
	if err != nil {
		return nil, err
	}

	status := domain.SettlementStatusMatched
	discrepancy := input.ActualTotalCents - expectedTotal
	if discrepancy != 0 {
		status = domain.SettlementStatusDiscrepancy
	}

	finishedAt := time.Now().UTC()
	settlement := &domain.Settlement{
		ShiftID:            shiftID,
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
		Status:             status,
		ExpectedTotalCents: expectedTotal,
		ActualTotalCents:   input.ActualTotalCents,
	}

	if err := s.settlements.CreateTx(ctx, tx, settlement); err != nil {
		return nil, err
	}

	items := make([]domain.SettlementItem, 0, len(payments))
	for _, payment := range payments {
		item := domain.SettlementItem{
			SettlementID: settlement.ID,
			PaymentID:    payment.ID,
			Result:       "matched",
		}
		if status == domain.SettlementStatusDiscrepancy {
			reason := "shift_total_mismatch"
			item.Result = "discrepancy"
			item.DiscrepancyReason = &reason
		}
		if err := s.settlements.AddItemTx(ctx, tx, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := s.jobRuns.RecordTx(ctx, tx, JobRunInput{
		JobType:    "settlement_run",
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Status:     status,
		Summary: map[string]any{
			"settlement_id":        settlement.ID,
			"shift_id":             shiftID,
			"expected_total_cents": expectedTotal,
			"actual_total_cents":   input.ActualTotalCents,
			"discrepancy_cents":    discrepancy,
			"payment_count":        len(payments),
		},
	}); err != nil {
		return nil, err
	}

	if err := s.audit.LogEventTx(ctx, tx, AuditLogInput{
		ActorID:      input.ActorID,
		Action:       "settlement.run",
		ResourceType: "settlement",
		ResourceID:   fmt.Sprintf("%d", settlement.ID),
		After: map[string]any{
			"shift_id":             shiftID,
			"status":               status,
			"expected_total_cents": expectedTotal,
			"actual_total_cents":   input.ActualTotalCents,
			"discrepancy_cents":    discrepancy,
		},
		RequestID: fallbackRequestID(input.RequestID),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit settlement tx: %w", err)
	}

	if s.logs != nil {
		_ = s.logs.Log("info", "settlement.run", map[string]any{
			"settlement_id":        settlement.ID,
			"shift_id":             shiftID,
			"status":               status,
			"expected_total_cents": expectedTotal,
			"actual_total_cents":   input.ActualTotalCents,
			"discrepancy_cents":    discrepancy,
			"request_id":           fallbackRequestID(input.RequestID),
		})
	}

	return &RunSettlementResult{
		Settlement:  settlement,
		Items:       items,
		Discrepancy: discrepancy,
	}, nil
}
