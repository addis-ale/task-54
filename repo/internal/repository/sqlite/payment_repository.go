package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type PaymentRepository struct {
	db *sql.DB
}

func NewPaymentRepository(db *sql.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) List(ctx context.Context, filter repository.PaymentFilter) ([]domain.Payment, error) {
	query := `
SELECT id, external_ref, method, gateway, amount_cents, currency, status, received_at, shift_id, idempotency_key, version, pii_reference_enc, pii_key_version, failure_reason
FROM payments
WHERE 1=1`
	args := make([]any, 0, 4)

	if status := strings.TrimSpace(filter.Status); status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	if method := strings.TrimSpace(filter.Method); method != "" {
		query += ` AND method = ?`
		args = append(args, method)
	}
	if gateway := strings.TrimSpace(filter.Gateway); gateway != "" {
		query += ` AND gateway = ?`
		args = append(args, gateway)
	}
	if shiftID := strings.TrimSpace(filter.ShiftID); shiftID != "" {
		query += ` AND shift_id = ?`
		args = append(args, shiftID)
	}

	query += ` ORDER BY received_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Payment, 0)
	for rows.Next() {
		item, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan payment row: %w", err)
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payment rows: %w", err)
	}

	return items, nil
}

func (r *PaymentRepository) CreateTx(ctx context.Context, tx *sql.Tx, payment *domain.Payment) error {
	const q = `
INSERT INTO payments(external_ref, method, gateway, amount_cents, currency, status, received_at, shift_id, idempotency_key, version, pii_reference_enc, pii_key_version, failure_reason)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`

	now := time.Now().UTC()
	if payment.ReceivedAt.IsZero() {
		payment.ReceivedAt = now
	}

	var externalRef sql.NullString
	if payment.ExternalRef != nil {
		externalRef = sql.NullString{String: *payment.ExternalRef, Valid: true}
	}

	var idem sql.NullString
	if payment.IdempotencyKey != nil {
		idem = sql.NullString{String: *payment.IdempotencyKey, Valid: true}
	}

	var piiRef sql.NullString
	if payment.PIIReferenceEnc != nil {
		piiRef = sql.NullString{String: *payment.PIIReferenceEnc, Valid: true}
	}

	var piiKeyVer sql.NullInt64
	if payment.PIIKeyVersion != nil {
		piiKeyVer = sql.NullInt64{Int64: int64(*payment.PIIKeyVersion), Valid: true}
	}

	var failure sql.NullString
	if payment.FailureReason != nil {
		failure = sql.NullString{String: *payment.FailureReason, Valid: true}
	}

	result, err := tx.ExecContext(
		ctx,
		q,
		externalRef,
		payment.Method,
		payment.Gateway,
		payment.AmountCents,
		payment.Currency,
		payment.Status,
		payment.ReceivedAt.UTC().Unix(),
		payment.ShiftID,
		idem,
		piiRef,
		piiKeyVer,
		failure,
	)
	if err != nil {
		return fmt.Errorf("create payment: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create payment last insert id: %w", err)
	}

	payment.ID = id
	payment.Version = 1
	return nil
}

func (r *PaymentRepository) GetByID(ctx context.Context, id int64) (*domain.Payment, error) {
	const q = `
SELECT id, external_ref, method, gateway, amount_cents, currency, status, received_at, shift_id, idempotency_key, version, pii_reference_enc, pii_key_version, failure_reason
FROM payments
WHERE id = ?`

	item, err := scanPayment(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get payment by id: %w", err)
	}

	return item, nil
}

func (r *PaymentRepository) ListSucceededByShiftTx(ctx context.Context, tx *sql.Tx, shiftID string) ([]domain.Payment, int64, error) {
	const q = `
SELECT id, external_ref, method, gateway, amount_cents, currency, status, received_at, shift_id, idempotency_key, version, pii_reference_enc, pii_key_version, failure_reason
FROM payments
WHERE shift_id = ? AND status = 'succeeded'
ORDER BY received_at ASC`

	rows, err := tx.QueryContext(ctx, q, strings.TrimSpace(shiftID))
	if err != nil {
		return nil, 0, fmt.Errorf("list succeeded payments by shift: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Payment, 0)
	var total int64
	for rows.Next() {
		item, err := scanPayment(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan succeeded payment row: %w", err)
		}
		items = append(items, *item)
		total += item.AmountCents
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate succeeded payment rows: %w", err)
	}

	return items, total, nil
}

func scanPayment(scanner rowScanner) (*domain.Payment, error) {
	var (
		item       domain.Payment
		external   sql.NullString
		receivedAt int64
		idem       sql.NullString
		piiRef     sql.NullString
		piiKeyVer  sql.NullInt64
		failure    sql.NullString
	)

	err := scanner.Scan(
		&item.ID,
		&external,
		&item.Method,
		&item.Gateway,
		&item.AmountCents,
		&item.Currency,
		&item.Status,
		&receivedAt,
		&item.ShiftID,
		&idem,
		&item.Version,
		&piiRef,
		&piiKeyVer,
		&failure,
	)
	if err != nil {
		return nil, err
	}

	if external.Valid {
		v := external.String
		item.ExternalRef = &v
	}
	if idem.Valid {
		v := idem.String
		item.IdempotencyKey = &v
	}
	if piiRef.Valid {
		v := piiRef.String
		item.PIIReferenceEnc = &v
	}
	if piiKeyVer.Valid {
		v := int(piiKeyVer.Int64)
		item.PIIKeyVersion = &v
	}
	if failure.Valid {
		v := failure.String
		item.FailureReason = &v
	}
	item.ReceivedAt = time.Unix(receivedAt, 0).UTC()

	return &item, nil
}

var _ repository.PaymentRepository = (*PaymentRepository)(nil)
