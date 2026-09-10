package unit_tests

import (
	"context"
	"errors"
	"testing"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestAdmissionsServiceLifecycleTransitions(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	svc := service.NewAdmissionsService(
		db,
		sqlite.NewPatientRepository(db),
		sqlite.NewWardRepository(db),
		sqlite.NewBedRepository(db),
		sqlite.NewAdmissionRepository(db),
	)

	ward, err := svc.CreateWard(ctx, service.CreateWardInput{Name: "General"})
	if err != nil {
		t.Fatalf("create ward: %v", err)
	}

	patient, err := svc.CreatePatient(ctx, service.CreatePatientInput{MRN: "MRN-1001", Name: "Jane Doe"})
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}

	bed, err := svc.CreateBed(ctx, service.CreateBedInput{WardID: ward.ID, BedCode: "G-01"})
	if err != nil {
		t.Fatalf("create bed: %v", err)
	}

	admission, err := svc.AssignAdmission(ctx, service.AssignAdmissionInput{PatientID: patient.ID, BedID: bed.ID})
	if err != nil {
		t.Fatalf("assign admission: %v", err)
	}
	if admission.Status != domain.AdmissionStatusActive {
		t.Fatalf("expected active admission, got %s", admission.Status)
	}

	beds, err := svc.ListBeds(ctx, repository.BedFilter{WardID: &ward.ID})
	if err != nil {
		t.Fatalf("list beds: %v", err)
	}
	if len(beds) != 1 || beds[0].Status != domain.BedStatusOccupied {
		t.Fatalf("expected occupied bed after assignment, got %+v", beds)
	}

	discharged, err := svc.DischargeAdmission(ctx, service.DischargeAdmissionInput{AdmissionID: admission.ID})
	if err != nil {
		t.Fatalf("discharge admission: %v", err)
	}
	if discharged.Status != domain.AdmissionStatusDischarged || discharged.DischargedAt == nil {
		t.Fatalf("expected discharged admission with timestamp, got %+v", discharged)
	}

	bedsAfter, err := svc.ListBeds(ctx, repository.BedFilter{WardID: &ward.ID})
	if err != nil {
		t.Fatalf("list beds after discharge: %v", err)
	}
	if len(bedsAfter) != 1 || bedsAfter[0].Status != domain.BedStatusCleaning {
		t.Fatalf("expected cleaning bed after discharge, got %+v", bedsAfter)
	}
}

func TestAdmissionsServiceValidationAndConflictHandling(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	svc := service.NewAdmissionsService(
		db,
		sqlite.NewPatientRepository(db),
		sqlite.NewWardRepository(db),
		sqlite.NewBedRepository(db),
		sqlite.NewAdmissionRepository(db),
	)

	if _, err := svc.AssignAdmission(ctx, service.AssignAdmissionInput{PatientID: 0, BedID: 0}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for zero ids, got: %v", err)
	}

	ward, err := svc.CreateWard(ctx, service.CreateWardInput{Name: "ICU"})
	if err != nil {
		t.Fatalf("create ward: %v", err)
	}

	patientA, err := svc.CreatePatient(ctx, service.CreatePatientInput{MRN: "MRN-2001", Name: "Patient A"})
	if err != nil {
		t.Fatalf("create patient A: %v", err)
	}
	patientB, err := svc.CreatePatient(ctx, service.CreatePatientInput{MRN: "MRN-2002", Name: "Patient B"})
	if err != nil {
		t.Fatalf("create patient B: %v", err)
	}

	bed1, err := svc.CreateBed(ctx, service.CreateBedInput{WardID: ward.ID, BedCode: "I-01"})
	if err != nil {
		t.Fatalf("create bed 1: %v", err)
	}
	bed2, err := svc.CreateBed(ctx, service.CreateBedInput{WardID: ward.ID, BedCode: "I-02"})
	if err != nil {
		t.Fatalf("create bed 2: %v", err)
	}

	admission, err := svc.AssignAdmission(ctx, service.AssignAdmissionInput{PatientID: patientA.ID, BedID: bed1.ID})
	if err != nil {
		t.Fatalf("assign admission A: %v", err)
	}

	if _, err := svc.AssignAdmission(ctx, service.AssignAdmissionInput{PatientID: patientB.ID, BedID: bed1.ID}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected conflict assigning to occupied bed, got: %v", err)
	}

	if _, err := svc.TransferAdmission(ctx, service.TransferAdmissionInput{AdmissionID: admission.ID, ToBedID: bed1.ID}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error transferring to same bed, got: %v", err)
	}

	if _, err := svc.TransferAdmission(ctx, service.TransferAdmissionInput{AdmissionID: admission.ID, ToBedID: bed2.ID}); err != nil {
		t.Fatalf("transfer admission to different bed: %v", err)
	}
}
