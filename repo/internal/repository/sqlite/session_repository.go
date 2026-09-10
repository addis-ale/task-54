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

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, session *domain.Session) error {
	const q = `
INSERT INTO sessions(user_id, token_hash, created_at, expires_at, last_seen_at)
VALUES(?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(
		ctx,
		q,
		session.UserID,
		session.TokenHash,
		session.CreatedAt.UTC().Unix(),
		session.ExpiresAt.UTC().Unix(),
		session.LastSeenAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create session last insert id: %w", err)
	}
	session.ID = id
	return nil
}

func (r *SessionRepository) GetActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (*domain.Session, *domain.User, error) {
	const q = `
SELECT
    s.id,
    s.user_id,
    s.token_hash,
    s.created_at,
    s.expires_at,
    s.last_seen_at,
    u.id,
    u.username,
    u.password_hash,
    u.role,
    u.failed_login_count,
    u.locked_until,
    u.created_at,
    u.updated_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.expires_at > ?`

	var (
		session     domain.Session
		user        domain.User
		sCreatedAt  int64
		sExpiresAt  int64
		sLastSeenAt int64
		uLocked     sql.NullInt64
		uCreatedAt  int64
		uUpdatedAt  int64
	)

	err := r.db.QueryRowContext(ctx, q, tokenHash, now.UTC().Unix()).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&sCreatedAt,
		&sExpiresAt,
		&sLastSeenAt,
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.FailedLoginCount,
		&uLocked,
		&uCreatedAt,
		&uUpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, repository.ErrNotFound
		}
		return nil, nil, fmt.Errorf("get active session by token hash: %w", err)
	}

	session.CreatedAt = time.Unix(sCreatedAt, 0).UTC()
	session.ExpiresAt = time.Unix(sExpiresAt, 0).UTC()
	session.LastSeenAt = time.Unix(sLastSeenAt, 0).UTC()

	if uLocked.Valid {
		t := time.Unix(uLocked.Int64, 0).UTC()
		user.LockedUntil = &t
	}
	user.CreatedAt = time.Unix(uCreatedAt, 0).UTC()
	user.UpdatedAt = time.Unix(uUpdatedAt, 0).UTC()

	return &session, &user, nil
}

func (r *SessionRepository) TouchActivity(ctx context.Context, sessionID int64, lastSeenAt, newExpiresAt time.Time) error {
	const q = `
UPDATE sessions
SET last_seen_at = ?, expires_at = ?
WHERE id = ?`

	if _, err := r.db.ExecContext(ctx, q, lastSeenAt.UTC().Unix(), newExpiresAt.UTC().Unix(), sessionID); err != nil {
		return fmt.Errorf("touch session activity: %w", err)
	}

	return nil
}

func (r *SessionRepository) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session by token hash: %w", err)
	}
	return nil
}
