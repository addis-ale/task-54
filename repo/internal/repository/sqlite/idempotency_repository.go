package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type IdempotencyRepository struct {
	db *sql.DB
}

func NewIdempotencyRepository(db *sql.DB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

func (r *IdempotencyRepository) GetActive(ctx context.Context, actorID int64, routeKey, key string, now time.Time) (*domain.IdempotencyKeyRecord, error) {
	return r.getActive(ctx, r.db, actorID, routeKey, key, now)
}

func (r *IdempotencyRepository) GetActiveTx(ctx context.Context, tx *sql.Tx, actorID int64, routeKey, key string, now time.Time) (*domain.IdempotencyKeyRecord, error) {
	return r.getActive(ctx, tx, actorID, routeKey, key, now)
}

func (r *IdempotencyRepository) getActive(ctx context.Context, runner queryRowRunner, actorID int64, routeKey, key string, now time.Time) (*domain.IdempotencyKeyRecord, error) {
	const q = `
SELECT id, actor_id, route_key, key, request_hash, response_code, response_body, expires_at, created_at
FROM idempotency_keys
WHERE actor_id = ? AND route_key = ? AND key = ? AND expires_at > ?`

	record, err := scanIdempotencyRecord(runner.QueryRowContext(ctx, q, actorID, routeKey, key, now.UTC().Unix()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get active idempotency key: %w", err)
	}

	return record, nil
}

func (r *IdempotencyRepository) Create(ctx context.Context, record *domain.IdempotencyKeyRecord) error {
	return r.create(ctx, r.db, record)
}

func (r *IdempotencyRepository) CreateTx(ctx context.Context, tx *sql.Tx, record *domain.IdempotencyKeyRecord) error {
	return r.create(ctx, tx, record)
}

func (r *IdempotencyRepository) create(ctx context.Context, runner execRunner, record *domain.IdempotencyKeyRecord) error {
	const q = `
INSERT INTO idempotency_keys(actor_id, route_key, key, request_hash, response_code, response_body, expires_at, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)`

	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	if _, err := runner.ExecContext(
		ctx,
		`DELETE FROM idempotency_keys WHERE actor_id = ? AND route_key = ? AND key = ? AND expires_at <= ?`,
		record.ActorID,
		record.RouteKey,
		record.Key,
		record.CreatedAt.UTC().Unix(),
	); err != nil {
		return fmt.Errorf("cleanup expired idempotency key: %w", err)
	}

	result, err := runner.ExecContext(
		ctx,
		q,
		record.ActorID,
		record.RouteKey,
		record.Key,
		record.RequestHash,
		record.ResponseCode,
		record.ResponseBody,
		record.ExpiresAt.UTC().Unix(),
		record.CreatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("create idempotency key: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create idempotency key last insert id: %w", err)
	}
	record.ID = id

	return nil
}

func scanIdempotencyRecord(scanner rowScanner) (*domain.IdempotencyKeyRecord, error) {
	var (
		record    domain.IdempotencyKeyRecord
		expiresAt int64
		createdAt int64
	)

	err := scanner.Scan(
		&record.ID,
		&record.ActorID,
		&record.RouteKey,
		&record.Key,
		&record.RequestHash,
		&record.ResponseCode,
		&record.ResponseBody,
		&expiresAt,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	record.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	record.CreatedAt = time.Unix(createdAt, 0).UTC()

	return &record, nil
}

var _ repository.IdempotencyRepository = (*IdempotencyRepository)(nil)
