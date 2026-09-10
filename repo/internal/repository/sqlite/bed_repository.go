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

type BedRepository struct {
	db *sql.DB
}

func NewBedRepository(db *sql.DB) *BedRepository {
	return &BedRepository{db: db}
}

func (r *BedRepository) List(ctx context.Context, filter repository.BedFilter) ([]domain.Bed, error) {
	query := `
SELECT b.id, b.ward_id, b.bed_code, b.status, b.version, b.updated_at, b.created_at, w.name
FROM beds b
JOIN wards w ON w.id = b.ward_id
WHERE 1=1`

	args := make([]any, 0, 2)
	if filter.WardID != nil {
		query += ` AND b.ward_id = ?`
		args = append(args, *filter.WardID)
	}
	if filter.Status != "" {
		query += ` AND b.status = ?`
		args = append(args, filter.Status)
	}
	query += ` ORDER BY w.name ASC, b.bed_code ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list beds: %w", err)
	}
	defer rows.Close()

	beds := make([]domain.Bed, 0)
	for rows.Next() {
		bed, err := scanBed(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bed row: %w", err)
		}
		beds = append(beds, *bed)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate beds: %w", err)
	}

	return beds, nil
}

func (r *BedRepository) ListOccupancy(ctx context.Context) ([]domain.BedOccupancy, error) {
	const q = `
SELECT
    b.id,
    b.ward_id,
    w.name,
    b.bed_code,
    b.status,
    b.version,
    a.id,
    p.id,
    p.name
FROM beds b
JOIN wards w ON w.id = b.ward_id
LEFT JOIN admissions a ON a.bed_id = b.id AND a.status = 'active' AND a.discharged_at IS NULL
LEFT JOIN patients p ON p.id = a.patient_id
ORDER BY w.name ASC, b.bed_code ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list occupancy board: %w", err)
	}
	defer rows.Close()

	items := make([]domain.BedOccupancy, 0)
	for rows.Next() {
		var (
			item        domain.BedOccupancy
			admissionID sql.NullInt64
			patientID   sql.NullInt64
			patientName sql.NullString
		)

		if err := rows.Scan(
			&item.BedID,
			&item.WardID,
			&item.WardName,
			&item.BedCode,
			&item.Status,
			&item.Version,
			&admissionID,
			&patientID,
			&patientName,
		); err != nil {
			return nil, fmt.Errorf("scan occupancy row: %w", err)
		}

		if admissionID.Valid {
			v := admissionID.Int64
			item.AdmissionID = &v
		}
		if patientID.Valid {
			v := patientID.Int64
			item.PatientID = &v
		}
		if patientName.Valid {
			v := patientName.String
			item.PatientName = &v
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate occupancy rows: %w", err)
	}

	return items, nil
}

func (r *BedRepository) GetByID(ctx context.Context, id int64) (*domain.Bed, error) {
	return r.getByID(ctx, r.db, id)
}

func (r *BedRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.Bed, error) {
	return r.getByID(ctx, tx, id)
}

func (r *BedRepository) getByID(ctx context.Context, runner queryRowRunner, id int64) (*domain.Bed, error) {
	const q = `
SELECT b.id, b.ward_id, b.bed_code, b.status, b.version, b.updated_at, b.created_at, w.name
FROM beds b
JOIN wards w ON w.id = b.ward_id
WHERE b.id = ?`

	bed, err := scanBed(runner.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get bed by id: %w", err)
	}
	return bed, nil
}

func (r *BedRepository) Create(ctx context.Context, bed *domain.Bed) error {
	now := time.Now().UTC()
	status := strings.TrimSpace(bed.Status)
	if status == "" {
		status = domain.BedStatusAvailable
	}

	const q = `
INSERT INTO beds(ward_id, bed_code, status, version, updated_at, created_at)
VALUES(?, ?, ?, 1, ?, ?)`

	result, err := r.db.ExecContext(ctx, q, bed.WardID, strings.TrimSpace(bed.BedCode), status, now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("create bed: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create bed last insert id: %w", err)
	}

	bed.ID = id
	bed.Status = status
	bed.Version = 1
	bed.CreatedAt = now
	bed.UpdatedAt = now
	return nil
}

func (r *BedRepository) UpdateStatusWithVersion(ctx context.Context, id int64, status string, expectedVersion int64) (bool, error) {
	return r.updateStatusWithVersion(ctx, r.db, id, status, expectedVersion)
}

func (r *BedRepository) UpdateStatusWithVersionTx(ctx context.Context, tx *sql.Tx, id int64, status string, expectedVersion int64) (bool, error) {
	return r.updateStatusWithVersion(ctx, tx, id, status, expectedVersion)
}

func (r *BedRepository) updateStatusWithVersion(ctx context.Context, runner execRunner, id int64, status string, expectedVersion int64) (bool, error) {
	const q = `
UPDATE beds
SET status = ?, version = version + 1, updated_at = ?
WHERE id = ? AND version = ?`

	result, err := runner.ExecContext(ctx, q, status, time.Now().UTC().Unix(), id, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("update bed status with version: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("bed status rows affected: %w", err)
	}

	return affected == 1, nil
}

func scanBed(scanner rowScanner) (*domain.Bed, error) {
	var (
		bed       domain.Bed
		updatedAt int64
		createdAt int64
	)

	err := scanner.Scan(
		&bed.ID,
		&bed.WardID,
		&bed.BedCode,
		&bed.Status,
		&bed.Version,
		&updatedAt,
		&createdAt,
		&bed.WardName,
	)
	if err != nil {
		return nil, err
	}

	bed.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	bed.CreatedAt = time.Unix(createdAt, 0).UTC()

	return &bed, nil
}

var _ repository.BedRepository = (*BedRepository)(nil)
