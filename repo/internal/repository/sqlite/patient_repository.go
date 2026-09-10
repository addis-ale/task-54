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

type PatientRepository struct {
	db *sql.DB
}

func NewPatientRepository(db *sql.DB) *PatientRepository {
	return &PatientRepository{db: db}
}

func (r *PatientRepository) List(ctx context.Context) ([]domain.Patient, error) {
	const q = `
SELECT id, mrn, name, dob, created_at, updated_at
FROM patients
ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list patients: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Patient, 0)
	for rows.Next() {
		patient, err := scanPatient(rows)
		if err != nil {
			return nil, fmt.Errorf("scan patient row: %w", err)
		}
		items = append(items, *patient)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate patient rows: %w", err)
	}

	return items, nil
}

func (r *PatientRepository) GetByID(ctx context.Context, id int64) (*domain.Patient, error) {
	return r.getByID(ctx, r.db, id)
}

func (r *PatientRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.Patient, error) {
	return r.getByID(ctx, tx, id)
}

func (r *PatientRepository) getByID(ctx context.Context, runner queryRowRunner, id int64) (*domain.Patient, error) {
	const q = `
SELECT id, mrn, name, dob, created_at, updated_at
FROM patients
WHERE id = ?`

	patient, err := scanPatient(runner.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get patient by id: %w", err)
	}

	return patient, nil
}

func (r *PatientRepository) Create(ctx context.Context, patient *domain.Patient) error {
	now := time.Now().UTC()

	var dob sql.NullString
	if patient.DOB != nil {
		trimmed := strings.TrimSpace(*patient.DOB)
		if trimmed != "" {
			dob = sql.NullString{String: trimmed, Valid: true}
		}
	}

	const q = `
INSERT INTO patients(mrn, name, dob, created_at, updated_at)
VALUES(?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(
		ctx,
		q,
		strings.TrimSpace(patient.MRN),
		strings.TrimSpace(patient.Name),
		dob,
		now.Unix(),
		now.Unix(),
	)
	if err != nil {
		return fmt.Errorf("create patient: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create patient last insert id: %w", err)
	}

	patient.ID = id
	patient.CreatedAt = now
	patient.UpdatedAt = now
	return nil
}

func scanPatient(scanner rowScanner) (*domain.Patient, error) {
	var (
		patient   domain.Patient
		dob       sql.NullString
		createdAt int64
		updatedAt int64
	)

	err := scanner.Scan(
		&patient.ID,
		&patient.MRN,
		&patient.Name,
		&dob,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if dob.Valid {
		v := strings.TrimSpace(dob.String)
		patient.DOB = &v
	}
	patient.CreatedAt = time.Unix(createdAt, 0).UTC()
	patient.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	return &patient, nil
}

var _ repository.PatientRepository = (*PatientRepository)(nil)
