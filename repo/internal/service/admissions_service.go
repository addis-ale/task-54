package service

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

var (
	ErrValidation          = errors.New("service: validation")
	ErrConflict            = errors.New("service: conflict")
	ErrVersionConflict     = errors.New("service: version conflict")
	ErrNotFound            = errors.New("service: not found")
	ErrSchedulingConflict  = errors.New("service: scheduling conflict")
	ErrIdempotencyConflict = errors.New("service: idempotency conflict")
)

type AdmissionsService struct {
	db         *sql.DB
	patients   repository.PatientRepository
	wards      repository.WardRepository
	beds       repository.BedRepository
	admissions repository.AdmissionRepository
}

func NewAdmissionsService(
	db *sql.DB,
	patients repository.PatientRepository,
	wards repository.WardRepository,
	beds repository.BedRepository,
	admissions repository.AdmissionRepository,
) *AdmissionsService {
	return &AdmissionsService{
		db:         db,
		patients:   patients,
		wards:      wards,
		beds:       beds,
		admissions: admissions,
	}
}

type CreateBedInput struct {
	WardID  int64
	BedCode string
	Status  string
}

type CreateWardInput struct {
	Name string
}

type CreatePatientInput struct {
	MRN  string
	Name string
	DOB  *string
}

type UpdateBedStatusInput struct {
	BedID           int64
	Status          string
	ExpectedVersion int64
}

type AssignAdmissionInput struct {
	PatientID int64
	BedID     int64
	ActorID   *int64
}

type TransferAdmissionInput struct {
	AdmissionID int64
	ToBedID     int64
	ActorID     *int64
}

type DischargeAdmissionInput struct {
	AdmissionID int64
	ActorID     *int64
}

func (s *AdmissionsService) ListBeds(ctx context.Context, filter repository.BedFilter) ([]domain.Bed, error) {
	if filter.Status != "" && !domain.IsValidBedStatus(filter.Status) {
		return nil, fmt.Errorf("%w: invalid bed status filter", ErrValidation)
	}
	return s.beds.List(ctx, filter)
}

func (s *AdmissionsService) ListWards(ctx context.Context) ([]domain.Ward, error) {
	return s.wards.List(ctx)
}

func (s *AdmissionsService) CreateWard(ctx context.Context, input CreateWardInput) (*domain.Ward, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: ward name is required", ErrValidation)
	}

	ward := &domain.Ward{Name: name}
	if err := s.wards.Create(ctx, ward); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: ward name already exists", ErrConflict)
		}
		return nil, err
	}

	return ward, nil
}

func (s *AdmissionsService) ListPatients(ctx context.Context) ([]domain.Patient, error) {
	return s.patients.List(ctx)
}

func (s *AdmissionsService) CreatePatient(ctx context.Context, input CreatePatientInput) (*domain.Patient, error) {
	mrn := strings.TrimSpace(input.MRN)
	if mrn == "" {
		return nil, fmt.Errorf("%w: mrn is required", ErrValidation)
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: patient name is required", ErrValidation)
	}

	var dob *string
	if input.DOB != nil {
		trimmed := strings.TrimSpace(*input.DOB)
		if trimmed != "" {
			dob = &trimmed
		}
	}

	patient := &domain.Patient{
		MRN:  mrn,
		Name: name,
		DOB:  dob,
	}

	if err := s.patients.Create(ctx, patient); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: mrn already exists", ErrConflict)
		}
		return nil, err
	}

	return patient, nil
}

func (s *AdmissionsService) CreateBed(ctx context.Context, input CreateBedInput) (*domain.Bed, error) {
	if input.WardID <= 0 {
		return nil, fmt.Errorf("%w: ward_id must be positive", ErrValidation)
	}
	if strings.TrimSpace(input.BedCode) == "" {
		return nil, fmt.Errorf("%w: bed_code is required", ErrValidation)
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = domain.BedStatusAvailable
	}
	if !domain.IsValidBedStatus(status) {
		return nil, fmt.Errorf("%w: invalid bed status", ErrValidation)
	}

	if _, err := s.wards.GetByID(ctx, input.WardID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: ward not found", ErrNotFound)
		}
		return nil, err
	}

	bed := &domain.Bed{
		WardID:  input.WardID,
		BedCode: strings.TrimSpace(input.BedCode),
		Status:  status,
	}
	if err := s.beds.Create(ctx, bed); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: bed code already exists in ward", ErrConflict)
		}
		return nil, err
	}

	created, err := s.beds.GetByID(ctx, bed.ID)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *AdmissionsService) UpdateBedStatus(ctx context.Context, input UpdateBedStatusInput) (*domain.Bed, error) {
	if input.BedID <= 0 {
		return nil, fmt.Errorf("%w: bed_id must be positive", ErrValidation)
	}
	if input.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: expected version must be positive", ErrValidation)
	}
	if !domain.IsValidBedStatus(strings.TrimSpace(input.Status)) {
		return nil, fmt.Errorf("%w: invalid bed status", ErrValidation)
	}

	if _, err := s.beds.GetByID(ctx, input.BedID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: bed not found", ErrNotFound)
		}
		return nil, err
	}

	updated, err := s.beds.UpdateStatusWithVersion(ctx, input.BedID, strings.TrimSpace(input.Status), input.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("%w: stale bed version", ErrVersionConflict)
	}

	bed, err := s.beds.GetByID(ctx, input.BedID)
	if err != nil {
		return nil, err
	}
	return bed, nil
}

func (s *AdmissionsService) ListAdmissions(ctx context.Context, filter repository.AdmissionFilter) ([]domain.Admission, error) {
	if filter.Status != "" && !domain.IsValidAdmissionStatus(filter.Status) {
		return nil, fmt.Errorf("%w: invalid admission status filter", ErrValidation)
	}
	return s.admissions.List(ctx, filter)
}

func (s *AdmissionsService) AssignAdmission(ctx context.Context, input AssignAdmissionInput) (*domain.Admission, error) {
	if input.PatientID <= 0 || input.BedID <= 0 {
		return nil, fmt.Errorf("%w: patient_id and bed_id must be positive", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin assign admission tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := s.patients.GetByIDTx(ctx, tx, input.PatientID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: patient not found", ErrNotFound)
		}
		return nil, err
	}

	bed, err := s.beds.GetByIDTx(ctx, tx, input.BedID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: bed not found", ErrNotFound)
		}
		return nil, err
	}

	if bed.Status != domain.BedStatusAvailable {
		return nil, fmt.Errorf("%w: bed is not available", ErrConflict)
	}

	if _, err := s.admissions.FindActiveByBedIDTx(ctx, tx, bed.ID); err == nil {
		return nil, fmt.Errorf("%w: bed already has active admission", ErrConflict)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	admission := &domain.Admission{
		PatientID:  input.PatientID,
		BedID:      input.BedID,
		AdmittedAt: now,
		Status:     domain.AdmissionStatusActive,
	}
	if err := s.admissions.CreateTx(ctx, tx, admission); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: admission conflict for bed", ErrConflict)
		}
		return nil, err
	}

	updated, err := s.beds.UpdateStatusWithVersionTx(ctx, tx, bed.ID, domain.BedStatusOccupied, bed.Version)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("%w: bed status changed by another request", ErrVersionConflict)
	}

	history := &domain.BedAssignmentHistory{
		AdmissionID: admission.ID,
		ToBedID:     &bed.ID,
		ChangedAt:   now,
		ActorID:     input.ActorID,
	}
	if err := s.admissions.AddAssignmentHistoryTx(ctx, tx, history); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit assign admission tx: %w", err)
	}

	items, err := s.admissions.List(ctx, repository.AdmissionFilter{BedID: &input.BedID, Status: domain.AdmissionStatusActive})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: admission not found after create", ErrNotFound)
	}
	return &items[0], nil
}

func (s *AdmissionsService) TransferAdmission(ctx context.Context, input TransferAdmissionInput) (*domain.Admission, error) {
	if input.AdmissionID <= 0 || input.ToBedID <= 0 {
		return nil, fmt.Errorf("%w: admission_id and to_bed_id must be positive", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transfer admission tx: %w", err)
	}
	defer tx.Rollback()

	admission, err := s.admissions.GetByIDTx(ctx, tx, input.AdmissionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: admission not found", ErrNotFound)
		}
		return nil, err
	}

	if admission.Status != domain.AdmissionStatusActive || admission.DischargedAt != nil {
		return nil, fmt.Errorf("%w: admission is not active", ErrConflict)
	}

	if admission.BedID == input.ToBedID {
		return nil, fmt.Errorf("%w: transfer target must be a different bed", ErrValidation)
	}

	fromBed, err := s.beds.GetByIDTx(ctx, tx, admission.BedID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: current bed not found", ErrNotFound)
		}
		return nil, err
	}

	toBed, err := s.beds.GetByIDTx(ctx, tx, input.ToBedID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: target bed not found", ErrNotFound)
		}
		return nil, err
	}

	if toBed.Status != domain.BedStatusAvailable {
		return nil, fmt.Errorf("%w: target bed is not available", ErrConflict)
	}

	if _, err := s.admissions.FindActiveByBedIDTx(ctx, tx, toBed.ID); err == nil {
		return nil, fmt.Errorf("%w: target bed already occupied", ErrConflict)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	updatedFrom, err := s.beds.UpdateStatusWithVersionTx(ctx, tx, fromBed.ID, domain.BedStatusCleaning, fromBed.Version)
	if err != nil {
		return nil, err
	}
	if !updatedFrom {
		return nil, fmt.Errorf("%w: current bed version mismatch", ErrVersionConflict)
	}

	updatedTo, err := s.beds.UpdateStatusWithVersionTx(ctx, tx, toBed.ID, domain.BedStatusOccupied, toBed.Version)
	if err != nil {
		return nil, err
	}
	if !updatedTo {
		return nil, fmt.Errorf("%w: target bed version mismatch", ErrVersionConflict)
	}

	updatedAdmission, err := s.admissions.UpdateBedAndVersionTx(ctx, tx, admission.ID, toBed.ID, admission.Version)
	if err != nil {
		return nil, err
	}
	if !updatedAdmission {
		return nil, fmt.Errorf("%w: admission version mismatch", ErrVersionConflict)
	}

	now := time.Now().UTC()
	history := &domain.BedAssignmentHistory{
		AdmissionID: admission.ID,
		FromBedID:   &fromBed.ID,
		ToBedID:     &toBed.ID,
		ChangedAt:   now,
		ActorID:     input.ActorID,
	}
	if err := s.admissions.AddAssignmentHistoryTx(ctx, tx, history); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transfer admission tx: %w", err)
	}

	items, err := s.admissions.List(ctx, repository.AdmissionFilter{BedID: &toBed.ID, Status: domain.AdmissionStatusActive})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: admission not found after transfer", ErrNotFound)
	}

	return &items[0], nil
}

func (s *AdmissionsService) DischargeAdmission(ctx context.Context, input DischargeAdmissionInput) (*domain.Admission, error) {
	if input.AdmissionID <= 0 {
		return nil, fmt.Errorf("%w: admission_id must be positive", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin discharge admission tx: %w", err)
	}
	defer tx.Rollback()

	admission, err := s.admissions.GetByIDTx(ctx, tx, input.AdmissionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: admission not found", ErrNotFound)
		}
		return nil, err
	}

	if admission.Status != domain.AdmissionStatusActive || admission.DischargedAt != nil {
		return nil, fmt.Errorf("%w: admission is not active", ErrConflict)
	}

	bed, err := s.beds.GetByIDTx(ctx, tx, admission.BedID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: bed not found", ErrNotFound)
		}
		return nil, err
	}

	now := time.Now().UTC()
	discharged, err := s.admissions.DischargeTx(ctx, tx, admission.ID, admission.Version, now)
	if err != nil {
		return nil, err
	}
	if !discharged {
		return nil, fmt.Errorf("%w: admission version mismatch", ErrVersionConflict)
	}

	updatedBed, err := s.beds.UpdateStatusWithVersionTx(ctx, tx, bed.ID, domain.BedStatusCleaning, bed.Version)
	if err != nil {
		return nil, err
	}
	if !updatedBed {
		return nil, fmt.Errorf("%w: bed version mismatch", ErrVersionConflict)
	}

	history := &domain.BedAssignmentHistory{
		AdmissionID: admission.ID,
		FromBedID:   &bed.ID,
		ChangedAt:   now,
		ActorID:     input.ActorID,
	}
	if err := s.admissions.AddAssignmentHistoryTx(ctx, tx, history); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit discharge admission tx: %w", err)
	}

	items, err := s.admissions.List(ctx, repository.AdmissionFilter{BedID: &bed.ID})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == admission.ID {
			return &item, nil
		}
	}

	return nil, fmt.Errorf("%w: admission not found after discharge", ErrNotFound)
}

func (s *AdmissionsService) OccupancyBoard(ctx context.Context) ([]domain.BedOccupancy, error) {
	return s.beds.ListOccupancy(ctx)
}
