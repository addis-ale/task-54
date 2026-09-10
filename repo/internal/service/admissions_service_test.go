package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/repository/migrations"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestUpdateBedStatusVersionConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, cleanup := setupAdmissionsService(t)
	defer cleanup()

	ward, err := svc.CreateWard(ctx, service.CreateWardInput{Name: "Ward A"})
	if err != nil {
		t.Fatalf("create ward: %v", err)
	}

	bed, err := svc.CreateBed(ctx, service.CreateBedInput{WardID: ward.ID, BedCode: "A-01"})
	if err != nil {
		t.Fatalf("create bed: %v", err)
	}

	updated, err := svc.UpdateBedStatus(ctx, service.UpdateBedStatusInput{
		BedID:           bed.ID,
		Status:          domain.BedStatusCleaning,
		ExpectedVersion: bed.Version,
	})
	if err != nil {
		t.Fatalf("initial status update: %v", err)
	}
	if updated.Version != bed.Version+1 {
		t.Fatalf("expected version %d, got %d", bed.Version+1, updated.Version)
	}

	_, err = svc.UpdateBedStatus(ctx, service.UpdateBedStatusInput{
		BedID:           bed.ID,
		Status:          domain.BedStatusAvailable,
		ExpectedVersion: bed.Version,
	})
	if !errors.Is(err, service.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got: %v", err)
	}
}

func TestAssignAdmissionRollbackOnHistoryInsertFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, cleanup := setupAdmissionsService(t)
	defer cleanup()

	ward, err := svc.CreateWard(ctx, service.CreateWardInput{Name: "Ward A"})
	if err != nil {
		t.Fatalf("create ward: %v", err)
	}

	patient, err := svc.CreatePatient(ctx, service.CreatePatientInput{MRN: "MRN-1001", Name: "John Doe"})
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}

	bed, err := svc.CreateBed(ctx, service.CreateBedInput{WardID: ward.ID, BedCode: "A-01"})
	if err != nil {
		t.Fatalf("create bed: %v", err)
	}

	invalidActorID := int64(999999)
	_, err = svc.AssignAdmission(ctx, service.AssignAdmissionInput{
		PatientID: patient.ID,
		BedID:     bed.ID,
		ActorID:   &invalidActorID,
	})
	if err == nil {
		t.Fatalf("expected assign admission to fail with invalid actor id")
	}

	beds, err := svc.ListBeds(ctx, repository.BedFilter{WardID: &ward.ID})
	if err != nil {
		t.Fatalf("list beds after rollback: %v", err)
	}
	if len(beds) != 1 {
		t.Fatalf("expected 1 bed, got %d", len(beds))
	}
	if beds[0].Status != domain.BedStatusAvailable {
		t.Fatalf("expected bed status available after rollback, got %s", beds[0].Status)
	}

	activeAdmissions, err := svc.ListAdmissions(ctx, repository.AdmissionFilter{Status: domain.AdmissionStatusActive, BedID: &bed.ID})
	if err != nil {
		t.Fatalf("list active admissions after rollback: %v", err)
	}
	if len(activeAdmissions) != 0 {
		t.Fatalf("expected 0 active admissions after rollback, got %d", len(activeAdmissions))
	}
}

func setupAdmissionsService(t *testing.T) (*service.AdmissionsService, func()) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	if err := migrations.Run(context.Background(), db); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}

	patients := sqlite.NewPatientRepository(db)
	wards := sqlite.NewWardRepository(db)
	beds := sqlite.NewBedRepository(db)
	admissions := sqlite.NewAdmissionRepository(db)

	svc := service.NewAdmissionsService(db, patients, wards, beds, admissions)

	return svc, func() {
		_ = db.Close()
	}
}
