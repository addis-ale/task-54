package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clinic-admin-suite/internal/api"
	"clinic-admin-suite/internal/config"
	"clinic-admin-suite/internal/repository/migrations"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
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

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	patientRepo := sqlite.NewPatientRepository(db)
	wardRepo := sqlite.NewWardRepository(db)
	bedRepo := sqlite.NewBedRepository(db)
	admissionRepo := sqlite.NewAdmissionRepository(db)
	workOrderRepo := sqlite.NewWorkOrderRepository(db)
	kpiRollupRepo := sqlite.NewKPIRollupRepository(db)
	jobRunRepo := sqlite.NewJobRunRepository(db)
	exerciseRepo := sqlite.NewExerciseRepository(db)
	mediaRepo := sqlite.NewMediaRepository(db)
	examScheduleRepo := sqlite.NewExamScheduleRepository(db)
	idempotencyRepo := sqlite.NewIdempotencyRepository(db)
	paymentRepo := sqlite.NewPaymentRepository(db)
	paymentEventRepo := sqlite.NewPaymentEventRepository(db)
	settlementRepo := sqlite.NewSettlementRepository(db)

	auditService := service.NewAuditService(auditRepo)
	structuredLogs := service.NewStructuredLogService(cfg.StructuredLogPath)
	jobRunService := service.NewJobRunService(jobRunRepo)
	authService := service.NewAuthService(userRepo, sessionRepo, auditService, cfg.BcryptCost, cfg.SessionTTL)
	admissionsService := service.NewAdmissionsService(db, patientRepo, wardRepo, bedRepo, admissionRepo)
	workOrderService := service.NewWorkOrderService(db, workOrderRepo, auditService, jobRunService)
	kpiService := service.NewKPIService(db, workOrderRepo, kpiRollupRepo, jobRunService)
	exerciseService := service.NewExerciseService(db, exerciseRepo, mediaRepo)
	mediaService := service.NewMediaService(cfg.MediaRoot, exerciseRepo, mediaRepo)
	schedulingService := service.NewSchedulingService(db, examScheduleRepo, idempotencyRepo, auditService)
	exerciseFavoriteService := service.NewExerciseFavoriteService(db)
	careService := service.NewCareService(db, auditService)
	examTemplateService := service.NewExamTemplateService(db, examScheduleRepo, schedulingService, auditService)

	var fieldCipher *service.FieldCipher
	if cfg.MasterKeyBase64 != "" {
		cipher, err := service.NewFieldCipherFromBase64(cfg.MasterKeyBase64)
		if err != nil {
			log.Fatalf("load field cipher: %v", err)
		}
		fieldCipher = cipher
	}

	paymentService := service.NewPaymentService(
		db,
		paymentRepo,
		paymentEventRepo,
		auditService,
		fieldCipher,
		1,
		[]service.GatewayAdapter{
			&service.CashGatewayAdapter{},
			&service.CheckGatewayAdapter{},
			&service.FacilityChargeGatewayAdapter{},
			&service.ImportedCardBatchGatewayAdapter{},
			&service.LocalCardGatewayAdapter{},
		},
		structuredLogs,
	)
	settlementService := service.NewSettlementService(db, paymentRepo, settlementRepo, auditService, jobRunService, structuredLogs)
	diagnosticsService := service.NewDiagnosticsService(db, cfg.StructuredLogPath, cfg.DiagnosticsRoot)
	reportService := service.NewReportService(db, paymentRepo)
	reportService.ConfigureScheduling(cfg.ReportsSharedRoot, jobRunService, structuredLogs, auditService)

	if err := authService.EnsureBootstrapAdmin(ctx, cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(recover.New())

	runnerCtx, runnerCancel := context.WithCancel(context.Background())
	defer runnerCancel()

	go kpiService.StartHourlyRollupTicker(runnerCtx)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-runnerCtx.Done():
				return
			case t := <-ticker.C:
				if err := reportService.RunDueSchedules(runnerCtx, t.UTC()); err != nil {
					_ = structuredLogs.Log("error", "report.schedule.tick", map[string]any{"error": err.Error()})
				}
			}
		}
	}()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		runnerCancel()
		_ = app.Shutdown()
	}()

	api.Register(app, api.Dependencies{
		Config:              cfg,
		AuthService:         authService,
		AdmissionsService:   admissionsService,
		WorkOrderService:    workOrderService,
		KPIService:          kpiService,
		ExerciseService:     exerciseService,
		ExerciseFavorite:    exerciseFavoriteService,
		CareService:         careService,
		ExamTemplateService: examTemplateService,
		MediaService:        mediaService,
		SchedulingService:   schedulingService,
		PaymentService:      paymentService,
		SettlementService:   settlementService,
		DiagnosticsService:  diagnosticsService,
		ReportService:       reportService,
		AuditService:        auditService,
		IdempotencyRepo:     idempotencyRepo,
	})

	log.Printf("listening on %s", cfg.Addr)
	if err := app.Listen(cfg.Addr); err != nil {
		log.Fatalf("fiber listen: %v", err)
	}
}
