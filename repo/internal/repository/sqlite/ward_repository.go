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

type WardRepository struct {
	db *sql.DB
}

func NewWardRepository(db *sql.DB) *WardRepository {
	return &WardRepository{db: db}
}

func (r *WardRepository) List(ctx context.Context) ([]domain.Ward, error) {
	const q = `
SELECT id, name, created_at, updated_at
FROM wards
ORDER BY name ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list wards: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Ward, 0)
	for rows.Next() {
		ward, err := scanWard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan ward row: %w", err)
		}
		items = append(items, *ward)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ward rows: %w", err)
	}

	return items, nil
}

func (r *WardRepository) GetByID(ctx context.Context, id int64) (*domain.Ward, error) {
	const q = `
SELECT id, name, created_at, updated_at
FROM wards
WHERE id = ?`

	ward, err := scanWard(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get ward by id: %w", err)
	}

	return ward, nil
}

func (r *WardRepository) Create(ctx context.Context, ward *domain.Ward) error {
	now := time.Now().UTC()

	const q = `
INSERT INTO wards(name, created_at, updated_at)
VALUES(?, ?, ?)`

	result, err := r.db.ExecContext(ctx, q, strings.TrimSpace(ward.Name), now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("create ward: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create ward last insert id: %w", err)
	}

	ward.ID = id
	ward.CreatedAt = now
	ward.UpdatedAt = now
	return nil
}

func scanWard(scanner rowScanner) (*domain.Ward, error) {
	var (
		ward      domain.Ward
		createdAt int64
		updatedAt int64
	)

	err := scanner.Scan(
		&ward.ID,
		&ward.Name,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	ward.CreatedAt = time.Unix(createdAt, 0).UTC()
	ward.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	return &ward, nil
}

var _ repository.WardRepository = (*WardRepository)(nil)
