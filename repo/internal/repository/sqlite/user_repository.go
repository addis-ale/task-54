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

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	const q = `
SELECT id, username, password_hash, role, failed_login_count, locked_until, created_at, updated_at
FROM users
WHERE username = ? COLLATE NOCASE`

	row := r.db.QueryRowContext(ctx, q, strings.TrimSpace(username))
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	const q = `
SELECT id, username, password_hash, role, failed_login_count, locked_until, created_at, updated_at
FROM users
WHERE id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	now := time.Now().UTC()
	const q = `
INSERT INTO users(username, password_hash, role, failed_login_count, locked_until, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?)`

	var lockedUntil sql.NullInt64
	if user.LockedUntil != nil {
		lockedUntil = sql.NullInt64{Int64: user.LockedUntil.UTC().Unix(), Valid: true}
	}

	result, err := r.db.ExecContext(
		ctx,
		q,
		strings.TrimSpace(user.Username),
		user.PasswordHash,
		user.Role,
		user.FailedLoginCount,
		lockedUntil,
		now.Unix(),
		now.Unix(),
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create user last insert id: %w", err)
	}

	user.ID = id
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

func (r *UserRepository) RecordFailedLogin(ctx context.Context, userID int64, failedCount int, lockedUntil *time.Time) error {
	const q = `
UPDATE users
SET failed_login_count = ?, locked_until = ?, updated_at = ?
WHERE id = ?`

	var lockedUntilValue sql.NullInt64
	if lockedUntil != nil {
		lockedUntilValue = sql.NullInt64{Int64: lockedUntil.UTC().Unix(), Valid: true}
	}

	if _, err := r.db.ExecContext(ctx, q, failedCount, lockedUntilValue, time.Now().UTC().Unix(), userID); err != nil {
		return fmt.Errorf("record failed login: %w", err)
	}
	return nil
}

func (r *UserRepository) ResetFailedLogins(ctx context.Context, userID int64) error {
	const q = `
UPDATE users
SET failed_login_count = 0, locked_until = NULL, updated_at = ?
WHERE id = ?`

	if _, err := r.db.ExecContext(ctx, q, time.Now().UTC().Unix(), userID); err != nil {
		return fmt.Errorf("reset failed logins: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(scanner rowScanner) (*domain.User, error) {
	var (
		user        domain.User
		lockedUntil sql.NullInt64
		createdAt   int64
		updatedAt   int64
	)

	err := scanner.Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.FailedLoginCount,
		&lockedUntil,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if lockedUntil.Valid {
		t := time.Unix(lockedUntil.Int64, 0).UTC()
		user.LockedUntil = &t
	}
	user.CreatedAt = time.Unix(createdAt, 0).UTC()
	user.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	return &user, nil
}
