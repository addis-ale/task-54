package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type AuditRepository struct {
	db *sql.DB
}

func NewAuditRepository(db *sql.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) LastHash(ctx context.Context) (*string, error) {
	return r.lastHash(ctx, r.db)
}

func (r *AuditRepository) LastHashTx(ctx context.Context, tx *sql.Tx) (*string, error) {
	return r.lastHash(ctx, tx)
}

func (r *AuditRepository) lastHash(ctx context.Context, runner queryRowRunner) (*string, error) {
	const q = `SELECT hash_self FROM audit_events ORDER BY id DESC LIMIT 1`

	var hash string
	err := runner.QueryRowContext(ctx, q).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("audit last hash: %w", err)
	}

	return &hash, nil
}

func (r *AuditRepository) Append(ctx context.Context, event *domain.AuditEvent) error {
	return r.append(ctx, r.db, event)
}

func (r *AuditRepository) AppendTx(ctx context.Context, tx *sql.Tx, event *domain.AuditEvent) error {
	return r.append(ctx, tx, event)
}

func (r *AuditRepository) append(ctx context.Context, runner execRunner, event *domain.AuditEvent) error {
	const q = `
INSERT INTO audit_events(
    occurred_at,
    actor_id,
    operator_username,
    local_ip,
    action,
    resource_type,
    resource_id,
    before_json,
    after_json,
    request_id,
    hash_prev,
    hash_self
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var actorID sql.NullInt64
	if event.ActorID != nil {
		actorID = sql.NullInt64{Int64: *event.ActorID, Valid: true}
	}

	var beforeJSON sql.NullString
	if event.BeforeJSON != nil {
		beforeJSON = sql.NullString{String: *event.BeforeJSON, Valid: true}
	}

	var afterJSON sql.NullString
	if event.AfterJSON != nil {
		afterJSON = sql.NullString{String: *event.AfterJSON, Valid: true}
	}

	var hashPrev sql.NullString
	if event.HashPrev != nil {
		hashPrev = sql.NullString{String: *event.HashPrev, Valid: true}
	}

	result, err := runner.ExecContext(
		ctx,
		q,
		event.OccurredAt.UTC().Unix(),
		actorID,
		event.OperatorName,
		event.LocalIP,
		event.Action,
		event.ResourceType,
		event.ResourceID,
		beforeJSON,
		afterJSON,
		event.RequestID,
		hashPrev,
		event.HashSelf,
	)
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("append audit event last insert id: %w", err)
	}
	event.ID = id

	return nil
}

var _ repository.AuditRepository = (*AuditRepository)(nil)
