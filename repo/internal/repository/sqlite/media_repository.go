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

type MediaRepository struct {
	db *sql.DB
}

func NewMediaRepository(db *sql.DB) *MediaRepository {
	return &MediaRepository{db: db}
}

func (r *MediaRepository) Create(ctx context.Context, asset *domain.MediaAsset) error {
	const q = `
INSERT INTO media_assets(exercise_id, media_type, path, checksum_sha256, duration_ms, bytes, variant, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now().UTC()
	asset.CreatedAt = now

	var duration sql.NullInt64
	if asset.DurationMS != nil {
		duration = sql.NullInt64{Int64: *asset.DurationMS, Valid: true}
	}

	result, err := r.db.ExecContext(ctx, q, asset.ExerciseID, asset.MediaType, asset.Path, asset.ChecksumSHA256, duration, asset.Bytes, asset.Variant, now.Unix())
	if err != nil {
		return fmt.Errorf("create media asset: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create media asset last insert id: %w", err)
	}

	asset.ID = id
	return nil
}

func (r *MediaRepository) GetByID(ctx context.Context, id int64) (*domain.MediaAsset, error) {
	const q = `
SELECT id, exercise_id, media_type, path, checksum_sha256, duration_ms, bytes, variant, created_at
FROM media_assets
WHERE id = ?`

	item, err := scanMediaAsset(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get media asset by id: %w", err)
	}

	return item, nil
}

func (r *MediaRepository) ListByExerciseID(ctx context.Context, exerciseID int64) ([]domain.MediaAsset, error) {
	const q = `
SELECT id, exercise_id, media_type, path, checksum_sha256, duration_ms, bytes, variant, created_at
FROM media_assets
WHERE exercise_id = ?
ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, q, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("list media assets by exercise id: %w", err)
	}
	defer rows.Close()

	items := make([]domain.MediaAsset, 0)
	for rows.Next() {
		item, err := scanMediaAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("scan media asset row: %w", err)
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media asset rows: %w", err)
	}

	return items, nil
}

func scanMediaAsset(scanner rowScanner) (*domain.MediaAsset, error) {
	var (
		item      domain.MediaAsset
		duration  sql.NullInt64
		createdAt int64
	)

	err := scanner.Scan(
		&item.ID,
		&item.ExerciseID,
		&item.MediaType,
		&item.Path,
		&item.ChecksumSHA256,
		&duration,
		&item.Bytes,
		&item.Variant,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	if duration.Valid {
		v := duration.Int64
		item.DurationMS = &v
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()

	return &item, nil
}

var _ repository.MediaRepository = (*MediaRepository)(nil)
