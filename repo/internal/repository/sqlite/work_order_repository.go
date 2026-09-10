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

type WorkOrderRepository struct {
	db *sql.DB
}

func NewWorkOrderRepository(db *sql.DB) *WorkOrderRepository {
	return &WorkOrderRepository{db: db}
}

func (r *WorkOrderRepository) List(ctx context.Context, filter repository.WorkOrderFilter) ([]domain.WorkOrder, error) {
	query := `
SELECT id, service_type, priority, scheduled_start, created_at, started_at, completed_at, status, assignee_id, patient_id, version
FROM work_orders
WHERE 1=1`
	args := make([]any, 0, 4)

	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.AssignedTo != nil {
		query += ` AND assignee_id = ?`
		args = append(args, *filter.AssignedTo)
	}
	if filter.Priority != "" {
		query += ` AND priority = ?`
		args = append(args, filter.Priority)
	}
	if filter.ServiceType != "" {
		query += ` AND service_type = ?`
		args = append(args, filter.ServiceType)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list work orders: %w", err)
	}
	defer rows.Close()

	items := make([]domain.WorkOrder, 0)
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan work order row: %w", err)
		}
		items = append(items, *wo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate work order rows: %w", err)
	}

	return items, nil
}

func (r *WorkOrderRepository) GetByID(ctx context.Context, id int64) (*domain.WorkOrder, error) {
	return r.getByID(ctx, r.db, id)
}

func (r *WorkOrderRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.WorkOrder, error) {
	return r.getByID(ctx, tx, id)
}

func (r *WorkOrderRepository) getByID(ctx context.Context, runner queryRowRunner, id int64) (*domain.WorkOrder, error) {
	const q = `
SELECT id, service_type, priority, scheduled_start, created_at, started_at, completed_at, status, assignee_id, patient_id, version
FROM work_orders
WHERE id = ?`

	wo, err := scanWorkOrder(runner.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get work order by id: %w", err)
	}

	return wo, nil
}

func (r *WorkOrderRepository) Create(ctx context.Context, workOrder *domain.WorkOrder) error {
	const q = `
INSERT INTO work_orders(service_type, priority, scheduled_start, created_at, started_at, completed_at, status, assignee_id, patient_id, version)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`

	now := time.Now().UTC()
	workOrder.CreatedAt = now

	var scheduledStart sql.NullInt64
	if workOrder.ScheduledStart != nil {
		scheduledStart = sql.NullInt64{Int64: workOrder.ScheduledStart.UTC().Unix(), Valid: true}
	}

	var startedAt sql.NullInt64
	if workOrder.StartedAt != nil {
		startedAt = sql.NullInt64{Int64: workOrder.StartedAt.UTC().Unix(), Valid: true}
	}

	var completedAt sql.NullInt64
	if workOrder.CompletedAt != nil {
		completedAt = sql.NullInt64{Int64: workOrder.CompletedAt.UTC().Unix(), Valid: true}
	}

	var assigneeID sql.NullInt64
	if workOrder.AssigneeID != nil {
		assigneeID = sql.NullInt64{Int64: *workOrder.AssigneeID, Valid: true}
	}

	var patientID sql.NullInt64
	if workOrder.PatientID != nil {
		patientID = sql.NullInt64{Int64: *workOrder.PatientID, Valid: true}
	}

	result, err := r.db.ExecContext(ctx, q, workOrder.ServiceType, workOrder.Priority, scheduledStart, workOrder.CreatedAt.Unix(), startedAt, completedAt, workOrder.Status, assigneeID, patientID)
	if err != nil {
		return fmt.Errorf("create work order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create work order last insert id: %w", err)
	}

	workOrder.ID = id
	workOrder.Version = 1
	return nil
}

func (r *WorkOrderRepository) StartTx(ctx context.Context, tx *sql.Tx, id, expectedVersion int64, startedAt time.Time) (bool, error) {
	const q = `
UPDATE work_orders
SET status = 'in_progress', started_at = ?, version = version + 1
WHERE id = ? AND version = ? AND status = 'queued' AND started_at IS NULL`

	result, err := tx.ExecContext(ctx, q, startedAt.UTC().Unix(), id, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("start work order: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("start work order rows affected: %w", err)
	}

	return affected == 1, nil
}

func (r *WorkOrderRepository) CompleteTx(ctx context.Context, tx *sql.Tx, id, expectedVersion int64, completedAt time.Time) (bool, error) {
	const q = `
UPDATE work_orders
SET status = 'completed', completed_at = ?, version = version + 1
WHERE id = ? AND version = ? AND status = 'in_progress' AND started_at IS NOT NULL AND completed_at IS NULL`

	result, err := tx.ExecContext(ctx, q, completedAt.UTC().Unix(), id, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("complete work order: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("complete work order rows affected: %w", err)
	}

	return affected == 1, nil
}

func (r *WorkOrderRepository) AggregateMetrics(ctx context.Context, from, to time.Time) ([]domain.KPIServiceRollup, error) {
	return r.aggregateMetrics(ctx, r.db, from, to)
}

func (r *WorkOrderRepository) AggregateMetricsTx(ctx context.Context, tx *sql.Tx, from, to time.Time) ([]domain.KPIServiceRollup, error) {
	return r.aggregateMetrics(ctx, tx, from, to)
}

func (r *WorkOrderRepository) aggregateMetrics(ctx context.Context, runner queryRunner, from, to time.Time) ([]domain.KPIServiceRollup, error) {
	const q = `
SELECT
    service_type,
    COUNT(1) AS total,
    SUM(CASE WHEN completed_at IS NOT NULL THEN 1 ELSE 0 END) AS completed,
    SUM(CASE WHEN completed_at IS NOT NULL AND scheduled_start IS NOT NULL AND (completed_at - scheduled_start) <= 900 THEN 1 ELSE 0 END) AS on_time_15m
FROM work_orders
WHERE started_at IS NOT NULL AND started_at >= ? AND started_at < ?
GROUP BY service_type`

	rows, err := runner.QueryContext(ctx, q, from.UTC().Unix(), to.UTC().Unix())
	if err != nil {
		return nil, fmt.Errorf("aggregate work order metrics: %w", err)
	}
	defer rows.Close()

	metrics := make([]domain.KPIServiceRollup, 0)
	for rows.Next() {
		var item domain.KPIServiceRollup
		if err := rows.Scan(&item.ServiceType, &item.Total, &item.Completed, &item.OnTime15m); err != nil {
			return nil, fmt.Errorf("scan work order metrics row: %w", err)
		}
		item.ExecutionRate = executionRate(item.Total, item.Completed)
		item.TimelinessRate = timelinessRate(item.Completed, item.OnTime15m)
		metrics = append(metrics, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate work order metrics rows: %w", err)
	}

	return metrics, nil
}

func scanWorkOrder(scanner rowScanner) (*domain.WorkOrder, error) {
	var (
		item           domain.WorkOrder
		scheduledStart sql.NullInt64
		createdAt      int64
		startedAt      sql.NullInt64
		completedAt    sql.NullInt64
		assigneeID     sql.NullInt64
		patientID      sql.NullInt64
	)

	err := scanner.Scan(
		&item.ID,
		&item.ServiceType,
		&item.Priority,
		&scheduledStart,
		&createdAt,
		&startedAt,
		&completedAt,
		&item.Status,
		&assigneeID,
		&patientID,
		&item.Version,
	)
	if err != nil {
		return nil, err
	}

	if scheduledStart.Valid {
		t := time.Unix(scheduledStart.Int64, 0).UTC()
		item.ScheduledStart = &t
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	if startedAt.Valid {
		t := time.Unix(startedAt.Int64, 0).UTC()
		item.StartedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0).UTC()
		item.CompletedAt = &t
	}
	if assigneeID.Valid {
		v := assigneeID.Int64
		item.AssigneeID = &v
	}
	if patientID.Valid {
		v := patientID.Int64
		item.PatientID = &v
	}

	return &item, nil
}

func executionRate(total, completed int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) * 100 / float64(total)
}

func timelinessRate(completed, onTime int64) float64 {
	if completed == 0 {
		return 0
	}
	return float64(onTime) * 100 / float64(completed)
}

var _ repository.WorkOrderRepository = (*WorkOrderRepository)(nil)
