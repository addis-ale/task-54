package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type SettlementRepository struct {
	db *sql.DB
}

func NewSettlementRepository(db *sql.DB) *SettlementRepository {
	return &SettlementRepository{db: db}
}

func (r *SettlementRepository) CreateTx(ctx context.Context, tx *sql.Tx, settlement *domain.Settlement) error {
	const q = `
INSERT INTO settlements(shift_id, started_at, finished_at, status, expected_total_cents, actual_total_cents)
VALUES(?, ?, ?, ?, ?, ?)`

	result, err := tx.ExecContext(
		ctx,
		q,
		settlement.ShiftID,
		settlement.StartedAt.UTC().Unix(),
		settlement.FinishedAt.UTC().Unix(),
		settlement.Status,
		settlement.ExpectedTotalCents,
		settlement.ActualTotalCents,
	)
	if err != nil {
		return fmt.Errorf("create settlement: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create settlement last insert id: %w", err)
	}
	settlement.ID = id

	return nil
}

func (r *SettlementRepository) AddItemTx(ctx context.Context, tx *sql.Tx, item *domain.SettlementItem) error {
	const q = `
INSERT INTO settlement_items(settlement_id, payment_id, result, discrepancy_reason)
VALUES(?, ?, ?, ?)`

	var reason sql.NullString
	if item.DiscrepancyReason != nil {
		reason = sql.NullString{String: *item.DiscrepancyReason, Valid: true}
	}

	result, err := tx.ExecContext(ctx, q, item.SettlementID, item.PaymentID, item.Result, reason)
	if err != nil {
		return fmt.Errorf("create settlement item: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create settlement item last insert id: %w", err)
	}
	item.ID = id

	return nil
}

var _ repository.SettlementRepository = (*SettlementRepository)(nil)
