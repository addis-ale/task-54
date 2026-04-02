package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type PaymentEventRepository struct {
	db *sql.DB
}

func NewPaymentEventRepository(db *sql.DB) *PaymentEventRepository {
	return &PaymentEventRepository{db: db}
}

func (r *PaymentEventRepository) CreateTx(ctx context.Context, tx *sql.Tx, event *domain.PaymentEvent) error {
	const q = `
INSERT INTO payment_events(payment_id, event_type, payload_json, created_at)
VALUES(?, ?, ?, ?)`

	now := time.Now().UTC()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}

	result, err := tx.ExecContext(ctx, q, event.PaymentID, event.EventType, event.Payload, event.CreatedAt.UTC().Unix())
	if err != nil {
		return fmt.Errorf("create payment event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create payment event last insert id: %w", err)
	}
	event.ID = id

	return nil
}

var _ repository.PaymentEventRepository = (*PaymentEventRepository)(nil)
