package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type KPIRollupRepository struct {
	db *sql.DB
}

func NewKPIRollupRepository(db *sql.DB) *KPIRollupRepository {
	return &KPIRollupRepository{db: db}
}

func (r *KPIRollupRepository) Upsert(ctx context.Context, rollup *domain.KPIServiceRollup) error {
	return r.upsert(ctx, r.db, rollup)
}

func (r *KPIRollupRepository) UpsertTx(ctx context.Context, tx *sql.Tx, rollup *domain.KPIServiceRollup) error {
	return r.upsert(ctx, tx, rollup)
}

func (r *KPIRollupRepository) upsert(ctx context.Context, runner execRunner, rollup *domain.KPIServiceRollup) error {
	const q = `
INSERT INTO kpi_service_rollups(bucket_start, bucket_granularity, service_type, total, completed, on_time_15m, execution_rate)
VALUES(?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(bucket_start, bucket_granularity, service_type)
DO UPDATE SET
    total = excluded.total,
    completed = excluded.completed,
    on_time_15m = excluded.on_time_15m,
    execution_rate = excluded.execution_rate`

	_, err := runner.ExecContext(
		ctx,
		q,
		rollup.BucketStart.UTC().Unix(),
		rollup.BucketGranularity,
		rollup.ServiceType,
		rollup.Total,
		rollup.Completed,
		rollup.OnTime15m,
		rollup.ExecutionRate,
	)
	if err != nil {
		return fmt.Errorf("upsert kpi rollup: %w", err)
	}

	return nil
}

func (r *KPIRollupRepository) List(ctx context.Context, filter repository.KPIRollupFilter) ([]domain.KPIServiceRollup, error) {
	query := `
SELECT id, bucket_start, bucket_granularity, service_type, total, completed, on_time_15m, execution_rate
FROM kpi_service_rollups
WHERE 1=1`

	args := make([]any, 0, 3)
	if strings.TrimSpace(filter.BucketGranularity) != "" {
		query += ` AND bucket_granularity = ?`
		args = append(args, strings.TrimSpace(filter.BucketGranularity))
	}
	if filter.From != nil {
		query += ` AND bucket_start >= ?`
		args = append(args, filter.From.UTC().Unix())
	}
	if filter.To != nil {
		query += ` AND bucket_start < ?`
		args = append(args, filter.To.UTC().Unix())
	}

	query += ` ORDER BY bucket_start ASC, service_type ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list kpi rollups: %w", err)
	}
	defer rows.Close()

	items := make([]domain.KPIServiceRollup, 0)
	for rows.Next() {
		var (
			item        domain.KPIServiceRollup
			bucketStart int64
		)

		if err := rows.Scan(
			&item.ID,
			&bucketStart,
			&item.BucketGranularity,
			&item.ServiceType,
			&item.Total,
			&item.Completed,
			&item.OnTime15m,
			&item.ExecutionRate,
		); err != nil {
			return nil, fmt.Errorf("scan kpi rollup row: %w", err)
		}

		item.BucketStart = time.Unix(bucketStart, 0).UTC()
		item.TimelinessRate = timelinessRate(item.Completed, item.OnTime15m)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate kpi rollup rows: %w", err)
	}

	return items, nil
}

var _ repository.KPIRollupRepository = (*KPIRollupRepository)(nil)
