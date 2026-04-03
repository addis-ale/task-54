package api

import (
	"time"

	"clinic-admin-suite/internal/api/handler"
	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/api/middleware"
	"clinic-admin-suite/internal/config"
	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type Dependencies struct {
	Config              config.Config
	AuthService         *service.AuthService
	AdmissionsService   *service.AdmissionsService
	WorkOrderService    *service.WorkOrderService
	KPIService          *service.KPIService
	ExerciseService     *service.ExerciseService
	ExerciseFavorite    *service.ExerciseFavoriteService
	CareService         *service.CareService
	ExamTemplateService *service.ExamTemplateService
	MediaService        *service.MediaService
	SchedulingService   *service.SchedulingService
	PaymentService      *service.PaymentService
	SettlementService   *service.SettlementService
	DiagnosticsService  *service.DiagnosticsService
	ReportService       *service.ReportService
	AuditService        *service.AuditService
	IdempotencyRepo     repository.IdempotencyRepository
}

func Register(app *fiber.App, deps Dependencies) {
	app.Use(middleware.RequestID())

	authHandler := handler.NewAuthHandler(deps.AuthService, deps.Config)
	adminHandler := handler.NewAdminHandler()
	uiShellHandler := handler.NewUIShellHandler()
	appPagesHandler := handler.NewAppPagesHandler(deps.Config, deps.AuthService, deps.AdmissionsService, deps.ExerciseService, deps.ExerciseFavorite, deps.CareService, deps.ExamTemplateService, deps.PaymentService, deps.SettlementService, deps.ReportService)
	wardsHandler := handler.NewWardsHandler(deps.AdmissionsService)
	patientsHandler := handler.NewPatientsHandler(deps.AdmissionsService)
	bedsHandler := handler.NewBedsHandler(deps.AdmissionsService)
	admissionsHandler := handler.NewAdmissionsHandler(deps.AdmissionsService)
	occupancyHandler := handler.NewOccupancyHandler(deps.AdmissionsService)
	workOrdersHandler := handler.NewWorkOrdersHandler(deps.WorkOrderService)
	kpiHandler := handler.NewKPIHandler(deps.KPIService)
	exerciseHandler := handler.NewExerciseHandler(deps.ExerciseService)
	exerciseFavoritesHandler := handler.NewExerciseFavoritesHandler(deps.ExerciseFavorite)
	mediaHandler := handler.NewMediaHandler(deps.MediaService)
	cacheUIHandler := handler.NewCacheUIHandler()
	schedulingHandler := handler.NewSchedulingHandler(deps.SchedulingService)
	examTemplatesHandler := handler.NewExamTemplatesHandler(deps.ExamTemplateService)
	paymentsHandler := handler.NewPaymentsHandler(deps.PaymentService)
	settlementHandler := handler.NewSettlementHandler(deps.SettlementService)
	diagnosticsHandler := handler.NewDiagnosticsHandler(deps.DiagnosticsService)
	reportsHandler := handler.NewReportsHandler(deps.ReportService)
	careHandler := handler.NewCareHandler(deps.CareService)
	governanceHandler := handler.NewGovernanceHandler(deps.ReportService)

	app.Get("/", uiShellHandler.IndexRedirect)
	loginLimiter := middleware.IPRateLimiter(10, 1*time.Minute)
	csrfMiddleware := middleware.CSRFProtect(deps.Config.CookieSecure)
	app.Get("/login", csrfMiddleware, appPagesHandler.LoginPage)
	app.Post("/login", loginLimiter, csrfMiddleware, appPagesHandler.LoginSubmit)
	app.Post("/logout", appPagesHandler.Logout)
	app.Get("/app", appPagesHandler.AppShell)
	app.Get("/assets/*", uiShellHandler.Asset)

	v1 := app.Group("/api/v1")
	v1.Get("/health", func(c *fiber.Ctx) error {
		return httpx.OK(c, fiber.StatusOK, fiber.Map{"status": "ok"})
	})

	authRoutes := v1.Group("/auth")
	authRoutes.Post("/login", loginLimiter, authHandler.Login)
	authRoutes.Post("/logout", authHandler.Logout)

	protected := v1.Group("", middleware.RequireAuth(deps.AuthService, deps.Config.SessionCookieName))
	protected.Use(middleware.RequireIdempotency(deps.IdempotencyRepo))
	protected.Use(middleware.PrivilegedAudit(deps.AuditService))
	protected.Get("/auth/me", authHandler.Me)
	protected.Get("/wards", middleware.RequirePermissions(domain.PermissionWardsRead), wardsHandler.List)
	protected.Post("/wards", middleware.RequirePermissions(domain.PermissionWardsWrite), wardsHandler.Create)
	protected.Get("/patients", middleware.RequirePermissions(domain.PermissionPatientsRead), patientsHandler.List)
	protected.Post("/patients", middleware.RequirePermissions(domain.PermissionPatientsWrite), patientsHandler.Create)
	protected.Get("/beds", middleware.RequirePermissions(domain.PermissionBedsRead), bedsHandler.List)
	protected.Post("/beds", middleware.RequirePermissions(domain.PermissionBedsWrite), bedsHandler.Create)
	protected.Patch("/beds/:bed_id", middleware.RequirePermissions(domain.PermissionBedsWrite), bedsHandler.PatchStatus)

	protected.Get("/admissions", middleware.RequirePermissions(domain.PermissionAdmissionsRead), admissionsHandler.List)
	protected.Post("/admissions", middleware.RequirePermissions(domain.PermissionAdmissionsWrite), admissionsHandler.Create)
	protected.Post("/admissions/:admission_id/transfer", middleware.RequirePermissions(domain.PermissionAdmissionsWrite), admissionsHandler.Transfer)
	protected.Post("/admissions/:admission_id/discharge", middleware.RequirePermissions(domain.PermissionAdmissionsWrite), admissionsHandler.Discharge)
	protected.Get("/work-orders", middleware.RequirePermissions(domain.PermissionWorkOrdersRead), workOrdersHandler.List)
	protected.Post("/work-orders", middleware.RequirePermissions(domain.PermissionWorkOrdersWrite), workOrdersHandler.Create)
	protected.Post("/work-orders/:work_order_id/start", middleware.RequirePermissions(domain.PermissionWorkOrdersWrite), workOrdersHandler.Start)
	protected.Post("/work-orders/:work_order_id/complete", middleware.RequirePermissions(domain.PermissionWorkOrdersWrite), workOrdersHandler.Complete)
	protected.Get("/kpis/service-delivery", middleware.RequirePermissions(domain.PermissionKPIRead), kpiHandler.ServiceDelivery)
	protected.Get("/exercises", middleware.RequirePermissions(domain.PermissionExercisesRead), exerciseHandler.List)
	protected.Post("/exercises", middleware.RequirePermissions(domain.PermissionExercisesWrite), exerciseHandler.Create)
	protected.Post("/exercises/:exercise_id/favorite", middleware.RequirePermissions(domain.PermissionExercisesRead), exerciseFavoritesHandler.Toggle)
	protected.Patch("/exercises/:exercise_id", middleware.RequirePermissions(domain.PermissionExercisesWrite), exerciseHandler.Patch)
	protected.Get("/exercises/:exercise_id", middleware.RequirePermissions(domain.PermissionExercisesRead), exerciseHandler.Get)
	protected.Get("/tags", middleware.RequirePermissions(domain.PermissionExercisesRead), exerciseHandler.ListTags)
	protected.Post("/exercises/:exercise_id/tags", middleware.RequirePermissions(domain.PermissionExercisesWrite), exerciseHandler.UpdateTags)
	protected.Post("/media", middleware.RequirePermissions(domain.PermissionMediaWrite), mediaHandler.Upload)
	protected.Get("/media/:media_id", middleware.RequirePermissions(domain.PermissionMediaRead), mediaHandler.Get)
	protected.Get("/media/:media_id/stream", middleware.RequirePermissions(domain.PermissionMediaRead), mediaHandler.Stream)
	protected.Get("/exam-schedules", middleware.RequirePermissions(domain.PermissionSchedulingRead), schedulingHandler.List)
	protected.Post("/exam-schedules", middleware.RequirePermissions(domain.PermissionSchedulingWrite), schedulingHandler.Create)
	protected.Post("/exam-schedules/:schedule_id/validate", middleware.RequirePermissions(domain.PermissionSchedulingRead), schedulingHandler.Validate)
	protected.Get("/exam-templates", middleware.RequirePermissions(domain.PermissionSchedulingRead), examTemplatesHandler.ListTemplates)
	protected.Post("/exam-templates", middleware.RequirePermissions(domain.PermissionSchedulingWrite), examTemplatesHandler.CreateTemplate)
	protected.Get("/exam-session-drafts", middleware.RequirePermissions(domain.PermissionSchedulingRead), examTemplatesHandler.ListDrafts)
	protected.Post("/exam-session-drafts/generate", middleware.RequirePermissions(domain.PermissionSchedulingWrite), examTemplatesHandler.GenerateDraft)
	protected.Post("/exam-session-drafts/:draft_id/adjust", middleware.RequirePermissions(domain.PermissionSchedulingWrite), examTemplatesHandler.AdjustDraft)
	protected.Post("/exam-session-drafts/:draft_id/publish", middleware.RequirePermissions(domain.PermissionSchedulingWrite), examTemplatesHandler.PublishDraft)
	protected.Get("/care-quality-checkpoints", middleware.RequirePermissions(domain.PermissionCareQualityRead), careHandler.ListCheckpoints)
	protected.Post("/care-quality-checkpoints", middleware.RequirePermissions(domain.PermissionCareQualityWrite), careHandler.CreateCheckpoint)
	protected.Get("/alert-events", middleware.RequirePermissions(domain.PermissionAlertsRead), careHandler.ListAlerts)
	protected.Post("/alert-events", middleware.RequirePermissions(domain.PermissionAlertsWrite), careHandler.CreateAlert)
	protected.Get("/care/dashboard", middleware.RequirePermissions(domain.PermissionCareQualityRead), careHandler.Dashboard)
	protected.Get("/payments", middleware.RequirePermissions(domain.PermissionPaymentsRead), paymentsHandler.List)
	protected.Post("/payments", middleware.RequirePermissions(domain.PermissionPaymentsWrite), paymentsHandler.Create)
	protected.Post("/payments/:payment_id/refunds", middleware.RequirePermissions(domain.PermissionPaymentsWrite), paymentsHandler.Refund)
	protected.Post("/settlements/run", middleware.RequirePermissions(domain.PermissionSettlementsRun), settlementHandler.Run)
	protected.Post("/diagnostics/export", middleware.RequirePermissions(domain.PermissionDiagnosticsExport), diagnosticsHandler.Export)
	protected.Get("/reports/ops/summary", middleware.RequirePermissions(domain.PermissionReportsRead), reportsHandler.OpsSummary)
	protected.Get("/reports/finance/export", middleware.RequirePermissions(domain.PermissionReportsExport), reportsHandler.ExportFinance)
	protected.Get("/reports/audit/search", middleware.RequirePermissions(domain.PermissionAuditRead), governanceHandler.AuditSearch)
	protected.Get("/reports/audit/export", middleware.RequirePermissions(domain.PermissionReportsExport), governanceHandler.AuditExport)
	protected.Get("/reports/schedules", middleware.RequirePermissions(domain.PermissionReportsRead), governanceHandler.ListReportSchedules)
	protected.Post("/reports/schedules", middleware.RequirePermissions(domain.PermissionReportsExport), governanceHandler.CreateReportSchedule)
	protected.Post("/reports/schedules/run-now", middleware.RequirePermissions(domain.PermissionReportsExport), governanceHandler.RunReportSchedulesNow)
	protected.Get("/config/versions", middleware.RequirePermissions(domain.PermissionConfigManage), governanceHandler.ListConfigVersions)
	protected.Post("/config/versions", middleware.RequirePermissions(domain.PermissionConfigManage), governanceHandler.CreateConfigVersion)
	protected.Post("/config/versions/:version_id/rollback", middleware.RequirePermissions(domain.PermissionConfigManage), governanceHandler.RollbackConfigVersion)

	admin := protected.Group("/admin", middleware.RequirePermissions(domain.PermissionAuditRead))
	admin.Get("/audit/ping", adminHandler.AuditPing)

	ui := app.Group("/ui", middleware.RequireAuth(deps.AuthService, deps.Config.SessionCookieName))
	ui.Use(csrfMiddleware)
	ui.Use(middleware.RequireIdempotency(deps.IdempotencyRepo))
	ui.Use(middleware.PrivilegedAudit(deps.AuditService))
	ui.Get("/panels/overview", middleware.RequirePermissions(domain.PermissionKPIRead), appPagesHandler.PanelOverview)
	ui.Get("/panels/occupancy", middleware.RequirePermissions(domain.PermissionOccupancyRead), appPagesHandler.PanelOccupancy)
	ui.Post("/occupancy/wards", middleware.RequirePermissions(domain.PermissionWardsWrite), appPagesHandler.CreateWard)
	ui.Post("/occupancy/patients", middleware.RequirePermissions(domain.PermissionPatientsWrite), appPagesHandler.CreatePatient)
	ui.Post("/occupancy/beds", middleware.RequirePermissions(domain.PermissionBedsWrite), appPagesHandler.CreateBed)
	ui.Post("/occupancy/admissions", middleware.RequirePermissions(domain.PermissionAdmissionsWrite), appPagesHandler.CreateAdmission)
	ui.Get("/panels/care", middleware.RequirePermissions(domain.PermissionCareQualityRead), appPagesHandler.PanelCare)
	ui.Post("/care/checkpoints", middleware.RequirePermissions(domain.PermissionCareQualityWrite), appPagesHandler.CreateCheckpoint)
	ui.Post("/care/alerts", middleware.RequirePermissions(domain.PermissionAlertsWrite), appPagesHandler.CreateAlert)
	ui.Get("/panels/exercises", middleware.RequirePermissions(domain.PermissionExercisesRead), appPagesHandler.PanelExercises)
	ui.Post("/exercises/:exercise_id/favorite", middleware.RequirePermissions(domain.PermissionExercisesRead), appPagesHandler.ToggleFavorite)
	ui.Get("/panels/scheduling", middleware.RequirePermissions(domain.PermissionSchedulingRead), appPagesHandler.PanelScheduling)
	ui.Post("/scheduling/templates", middleware.RequirePermissions(domain.PermissionSchedulingWrite), appPagesHandler.CreateTemplate)
	ui.Post("/scheduling/drafts/generate", middleware.RequirePermissions(domain.PermissionSchedulingWrite), appPagesHandler.GenerateDraft)
	ui.Post("/scheduling/drafts/:draft_id/adjust", middleware.RequirePermissions(domain.PermissionSchedulingWrite), appPagesHandler.AdjustDraft)
	ui.Post("/scheduling/drafts/:draft_id/publish", middleware.RequirePermissions(domain.PermissionSchedulingWrite), appPagesHandler.PublishDraft)
	ui.Get("/panels/finance", middleware.RequirePermissions(domain.PermissionPaymentsRead), appPagesHandler.PanelFinance)
	ui.Post("/finance/payments", middleware.RequirePermissions(domain.PermissionPaymentsWrite), appPagesHandler.CreatePayment)
	ui.Post("/finance/refunds", middleware.RequirePermissions(domain.PermissionPaymentsWrite), appPagesHandler.RefundPayment)
	ui.Post("/finance/settlements", middleware.RequirePermissions(domain.PermissionSettlementsRun), appPagesHandler.RunSettlement)
	ui.Get("/panels/reports", middleware.RequirePermissions(domain.PermissionReportsRead), appPagesHandler.PanelReports)
	ui.Get("/reports/audit-results", middleware.RequirePermissions(domain.PermissionAuditRead), appPagesHandler.AuditResults)
	ui.Post("/reports/schedules", middleware.RequirePermissions(domain.PermissionReportsExport), appPagesHandler.CreateReportSchedule)
	ui.Post("/reports/schedules/run-now", middleware.RequirePermissions(domain.PermissionReportsExport), appPagesHandler.RunReportSchedulesNow)
	ui.Post("/config/versions", middleware.RequirePermissions(domain.PermissionConfigManage), appPagesHandler.CreateConfigVersion)
	ui.Post("/config/versions/:version_id/rollback", middleware.RequirePermissions(domain.PermissionConfigManage), appPagesHandler.RollbackConfigVersion)
	ui.Get("/occupancy/board", middleware.RequirePermissions(domain.PermissionOccupancyRead), occupancyHandler.Board)
	ui.Get("/service-delivery/patient/:patient_id", middleware.RequirePermissions(domain.PermissionKPIRead), appPagesHandler.ServiceDeliveryDrillDown)
	ui.Get("/cache/lru", middleware.RequirePermissions(domain.PermissionUICacheRead), cacheUIHandler.LRUSimulator)
}
