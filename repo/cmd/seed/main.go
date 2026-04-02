package main

import (
	"context"
	"log"

	"clinic-admin-suite/internal/config"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/repository/migrations"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func main() {
	cfg := config.Load()

	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := migrations.Run(ctx, db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	patientRepo := sqlite.NewPatientRepository(db)
	wardRepo := sqlite.NewWardRepository(db)
	bedRepo := sqlite.NewBedRepository(db)
	admissionRepo := sqlite.NewAdmissionRepository(db)

	admissions := service.NewAdmissionsService(db, patientRepo, wardRepo, bedRepo, admissionRepo)

	wardIDs, err := ensureWards(ctx, admissions)
	if err != nil {
		log.Fatalf("seed wards: %v", err)
	}

	if err := ensurePatients(ctx, admissions); err != nil {
		log.Fatalf("seed patients: %v", err)
	}

	if err := ensureBeds(ctx, admissions, wardIDs); err != nil {
		log.Fatalf("seed beds: %v", err)
	}

	log.Printf("seed complete")
}

func ensureWards(ctx context.Context, admissions *service.AdmissionsService) (map[string]int64, error) {
	definitions := []string{"General", "ICU", "Pediatrics"}

	existing, err := admissions.ListWards(ctx)
	if err != nil {
		return nil, err
	}

	wardIDs := make(map[string]int64, len(existing))
	for _, ward := range existing {
		wardIDs[ward.Name] = ward.ID
	}

	for _, name := range definitions {
		if _, ok := wardIDs[name]; ok {
			continue
		}

		created, err := admissions.CreateWard(ctx, service.CreateWardInput{Name: name})
		if err != nil {
			return nil, err
		}
		wardIDs[created.Name] = created.ID
	}

	return wardIDs, nil
}

func ensurePatients(ctx context.Context, admissions *service.AdmissionsService) error {
	definitions := []service.CreatePatientInput{
		{MRN: "MRN-1001", Name: "Jane Doe"},
		{MRN: "MRN-1002", Name: "John Smith"},
		{MRN: "MRN-1003", Name: "Alex Brown"},
	}

	existing, err := admissions.ListPatients(ctx)
	if err != nil {
		return err
	}

	byMRN := make(map[string]struct{}, len(existing))
	for _, patient := range existing {
		byMRN[patient.MRN] = struct{}{}
	}

	for _, candidate := range definitions {
		if _, ok := byMRN[candidate.MRN]; ok {
			continue
		}

		if _, err := admissions.CreatePatient(ctx, candidate); err != nil {
			return err
		}
	}

	return nil
}

func ensureBeds(ctx context.Context, admissions *service.AdmissionsService, wardIDs map[string]int64) error {
	definitions := map[string][]string{
		"General":    {"G-01", "G-02", "G-03"},
		"ICU":        {"I-01", "I-02"},
		"Pediatrics": {"P-01", "P-02"},
	}

	for wardName, bedCodes := range definitions {
		wardID, ok := wardIDs[wardName]
		if !ok {
			continue
		}

		filterWardID := wardID
		existing, err := admissions.ListBeds(ctx, repository.BedFilter{WardID: &filterWardID})
		if err != nil {
			return err
		}

		byCode := make(map[string]struct{}, len(existing))
		for _, bed := range existing {
			byCode[bed.BedCode] = struct{}{}
		}

		for _, code := range bedCodes {
			if _, ok := byCode[code]; ok {
				continue
			}

			if _, err := admissions.CreateBed(ctx, service.CreateBedInput{WardID: wardID, BedCode: code, Status: "available"}); err != nil {
				return err
			}
		}
	}

	return nil
}
