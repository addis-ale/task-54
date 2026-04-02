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

type AdmissionRepository struct {
	db *sql.DB
}

func NewAdmissionRepository(db *sql.DB) *AdmissionRepository {
	return &AdmissionRepository{db: db}
}

func (r *AdmissionRepository) List(ctx context.Context, filter repository.AdmissionFilter) ([]domain.Admission, error) {
	query := `
SELECT
    a.id,
    a.patient_id,
    a.bed_id,
    a.admitted_at,
    a.discharged_at,
    a.status,
    a.version,
    p.name,
    b.bed_code,
    w.name
FROM admissions a
JOIN patients p ON p.id = a.patient_id
JOIN beds b ON b.id = a.bed_id
JOIN wards w ON w.id = b.ward_id
WHERE 1=1`

	args := make([]any, 0, 3)
	if filter.Status != "" {
		query += ` AND a.status = ?`
		args = append(args, filter.Status)
	}
	if filter.PatientID != nil {
		query += ` AND a.patient_id = ?`
		args = append(args, *filter.PatientID)
	}
	if filter.BedID != nil {
		query += ` AND a.bed_id = ?`
		args = append(args, *filter.BedID)
	}
	query += ` ORDER BY a.admitted_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list admissions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Admission, 0)
	for rows.Next() {
		admission, err := scanAdmission(rows)
		if err != nil {
			return nil, fmt.Errorf("scan admission row: %w", err)
		}
		items = append(items, *admission)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admission rows: %w", err)
	}

	return items, nil
}

func (r *AdmissionRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.Admission, error) {
	const q = `
SELECT
    a.id,
    a.patient_id,
    a.bed_id,
    a.admitted_at,
    a.discharged_at,
    a.status,
    a.version,
    p.name,
    b.bed_code,
    w.name
FROM admissions a
JOIN patients p ON p.id = a.patient_id
JOIN beds b ON b.id = a.bed_id
JOIN wards w ON w.id = b.ward_id
WHERE a.id = ?`

	admission, err := scanAdmission(tx.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get admission by id: %w", err)
	}

	return admission, nil
}

func (r *AdmissionRepository) FindActiveByBedIDTx(ctx context.Context, tx *sql.Tx, bedID int64) (*domain.Admission, error) {
	const q = `
SELECT
    a.id,
    a.patient_id,
    a.bed_id,
    a.admitted_at,
    a.discharged_at,
    a.status,
    a.version,
    p.name,
    b.bed_code,
    w.name
FROM admissions a
JOIN patients p ON p.id = a.patient_id
JOIN beds b ON b.id = a.bed_id
JOIN wards w ON w.id = b.ward_id
WHERE a.bed_id = ? AND a.status = 'active' AND a.discharged_at IS NULL`

	admission, err := scanAdmission(tx.QueryRowContext(ctx, q, bedID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("find active admission by bed id: %w", err)
	}

	return admission, nil
}

func (r *AdmissionRepository) CreateTx(ctx context.Context, tx *sql.Tx, admission *domain.Admission) error {
	const q = `
INSERT INTO admissions(patient_id, bed_id, admitted_at, discharged_at, status, version)
VALUES(?, ?, ?, ?, ?, 1)`

	var dischargedAt sql.NullInt64
	if admission.DischargedAt != nil {
		dischargedAt = sql.NullInt64{Int64: admission.DischargedAt.UTC().Unix(), Valid: true}
	}

	result, err := tx.ExecContext(
		ctx,
		q,
		admission.PatientID,
		admission.BedID,
		admission.AdmittedAt.UTC().Unix(),
		dischargedAt,
		admission.Status,
	)
	if err != nil {
		return fmt.Errorf("create admission: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create admission last insert id: %w", err)
	}

	admission.ID = id
	admission.Version = 1
	return nil
}

func (r *AdmissionRepository) UpdateBedAndVersionTx(ctx context.Context, tx *sql.Tx, admissionID, toBedID, expectedVersion int64) (bool, error) {
	const q = `
UPDATE admissions
SET bed_id = ?, version = version + 1
WHERE id = ? AND status = 'active' AND version = ?`

	result, err := tx.ExecContext(ctx, q, toBedID, admissionID, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("update admission bed and version: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update admission rows affected: %w", err)
	}

	return affected == 1, nil
}

func (r *AdmissionRepository) DischargeTx(ctx context.Context, tx *sql.Tx, admissionID, expectedVersion int64, dischargedAt time.Time) (bool, error) {
	const q = `
UPDATE admissions
SET status = 'discharged', discharged_at = ?, version = version + 1
WHERE id = ? AND status = 'active' AND discharged_at IS NULL AND version = ?`

	result, err := tx.ExecContext(ctx, q, dischargedAt.UTC().Unix(), admissionID, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("discharge admission: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("discharge admission rows affected: %w", err)
	}

	return affected == 1, nil
}

func (r *AdmissionRepository) AddAssignmentHistoryTx(ctx context.Context, tx *sql.Tx, history *domain.BedAssignmentHistory) error {
	const q = `
INSERT INTO bed_assignment_history(admission_id, from_bed_id, to_bed_id, changed_at, actor_id)
VALUES(?, ?, ?, ?, ?)`

	var fromBedID sql.NullInt64
	if history.FromBedID != nil {
		fromBedID = sql.NullInt64{Int64: *history.FromBedID, Valid: true}
	}

	var toBedID sql.NullInt64
	if history.ToBedID != nil {
		toBedID = sql.NullInt64{Int64: *history.ToBedID, Valid: true}
	}

	var actorID sql.NullInt64
	if history.ActorID != nil {
		actorID = sql.NullInt64{Int64: *history.ActorID, Valid: true}
	}

	result, err := tx.ExecContext(ctx, q, history.AdmissionID, fromBedID, toBedID, history.ChangedAt.UTC().Unix(), actorID)
	if err != nil {
		return fmt.Errorf("insert bed assignment history: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("insert bed assignment history last insert id: %w", err)
	}
	history.ID = id

	return nil
}

func scanAdmission(scanner rowScanner) (*domain.Admission, error) {
	var (
		admission    domain.Admission
		admittedAt   int64
		dischargedAt sql.NullInt64
	)

	err := scanner.Scan(
		&admission.ID,
		&admission.PatientID,
		&admission.BedID,
		&admittedAt,
		&dischargedAt,
		&admission.Status,
		&admission.Version,
		&admission.PatientName,
		&admission.BedCode,
		&admission.WardName,
	)
	if err != nil {
		return nil, err
	}

	admission.AdmittedAt = time.Unix(admittedAt, 0).UTC()
	if dischargedAt.Valid {
		t := time.Unix(dischargedAt.Int64, 0).UTC()
		admission.DischargedAt = &t
	}

	return &admission, nil
}

var _ repository.AdmissionRepository = (*AdmissionRepository)(nil)
