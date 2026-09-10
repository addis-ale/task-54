package unit_tests

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestSettlementServiceMatchedAndDiscrepancyFlows(t *testing.T) {
	ctx := adminCtx()
	db := setupTestDB(t)

	paymentsRepo := sqlite.NewPaymentRepository(db)
	settlementsRepo := sqlite.NewSettlementRepository(db)
	auditService := service.NewAuditService(sqlite.NewAuditRepository(db))
	jobRuns := service.NewJobRunService(sqlite.NewJobRunRepository(db))
	settlementSvc := service.NewSettlementService(db, paymentsRepo, settlementsRepo, auditService, jobRuns, nil)
	settlementSvc.SetAdminOverride(true)

	shift := "shift-0700"
	insertPayment(t, db, domain.Payment{Method: "cash", Gateway: "cash_local", AmountCents: 1000, Currency: "USD", Status: domain.PaymentStatusSucceeded, ShiftID: shift, ReceivedAt: time.Now().UTC()})
	insertPayment(t, db, domain.Payment{Method: "card", Gateway: "card_local", AmountCents: 2500, Currency: "USD", Status: domain.PaymentStatusSucceeded, ShiftID: shift, ReceivedAt: time.Now().UTC()})
	insertPayment(t, db, domain.Payment{Method: "card", Gateway: "card_local", AmountCents: 9999, Currency: "USD", Status: domain.PaymentStatusFailed, ShiftID: shift, ReceivedAt: time.Now().UTC()})

	matched, err := settlementSvc.RunShift(ctx, service.RunSettlementInput{ShiftID: shift, ActualTotalCents: 3500, RequestID: "req_settle_match"})
	if err != nil {
		t.Fatalf("run matched settlement: %v", err)
	}
	if matched.Settlement.Status != domain.SettlementStatusMatched || matched.Discrepancy != 0 {
		t.Fatalf("expected matched settlement, got %+v", matched)
	}

	discrepant, err := settlementSvc.RunShift(ctx, service.RunSettlementInput{ShiftID: shift, ActualTotalCents: 3000, RequestID: "req_settle_disc"})
	if err != nil {
		t.Fatalf("run discrepancy settlement: %v", err)
	}
	if discrepant.Settlement.Status != domain.SettlementStatusDiscrepancy || discrepant.Discrepancy != -500 {
		t.Fatalf("expected discrepancy settlement, got %+v", discrepant)
	}
	if len(discrepant.Items) != 2 {
		t.Fatalf("expected settlement items for only succeeded payments, got %d", len(discrepant.Items))
	}
}

func TestSettlementServiceValidationErrors(t *testing.T) {
	ctx := adminCtx()
	db := setupTestDB(t)

	settlementSvc := service.NewSettlementService(
		db,
		sqlite.NewPaymentRepository(db),
		sqlite.NewSettlementRepository(db),
		service.NewAuditService(sqlite.NewAuditRepository(db)),
		service.NewJobRunService(sqlite.NewJobRunRepository(db)),
		nil,
	)
	settlementSvc.SetAdminOverride(true)

	if _, err := settlementSvc.RunShift(ctx, service.RunSettlementInput{ShiftID: "", ActualTotalCents: 100}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for missing shift_id, got: %v", err)
	}

	if _, err := settlementSvc.RunShift(ctx, service.RunSettlementInput{ShiftID: "shift-x", ActualTotalCents: -1}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for negative total, got: %v", err)
	}

	if _, err := settlementSvc.RunShift(ctx, service.RunSettlementInput{ShiftID: "swing", ActualTotalCents: 100}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for unsupported shift_id, got: %v", err)
	}
}

func insertPayment(t *testing.T, db *sql.DB, payment domain.Payment) {
	t.Helper()

	repo := sqlite.NewPaymentRepository(db)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin payment insert tx: %v", err)
	}
	defer tx.Rollback()

	if err := repo.CreateTx(context.Background(), tx, &payment); err != nil {
		t.Fatalf("insert payment: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit payment insert tx: %v", err)
	}
}
