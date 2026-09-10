package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"clinic-admin-suite/internal/api"
	"clinic-admin-suite/internal/config"
	"clinic-admin-suite/internal/repository/migrations"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type envelope struct {
	Data  map[string]any `json:"data"`
	Meta  map[string]any `json:"meta"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestBedsPatchVersionConflictEndpoint(t *testing.T) {
	t.Parallel()

	app, cfg, cleanup := setupTestApp(t)
	defer cleanup()

	cookieHeader := loginAndGetCookie(t, app, cfg)

	wardBody := map[string]any{"name": "General"}
	wardResp := doJSONRequest(t, app, http.MethodPost, "/api/v1/wards", wardBody, map[string]string{"Cookie": cookieHeader})
	if wardResp.status != http.StatusCreated {
		t.Fatalf("create ward status = %d", wardResp.status)
	}
	wardID := int64FromMap(t, wardResp.env.Data["ward"].(map[string]any), "id")

	bedBody := map[string]any{"ward_id": wardID, "bed_code": "G-01", "status": "available"}
	bedResp := doJSONRequest(t, app, http.MethodPost, "/api/v1/beds", bedBody, map[string]string{"Cookie": cookieHeader})
	if bedResp.status != http.StatusCreated {
		t.Fatalf("create bed status = %d", bedResp.status)
	}
	bed := bedResp.env.Data["bed"].(map[string]any)
	bedID := int64FromMap(t, bed, "id")
	version := int64FromMap(t, bed, "version")

	patchBody := map[string]any{"status": "cleaning"}
	firstPatch := doJSONRequest(t, app, http.MethodPatch, "/api/v1/beds/"+intToString(bedID), patchBody, map[string]string{
		"Cookie":           cookieHeader,
		"If-Match-Version": intToString(version),
	})
	if firstPatch.status != http.StatusOK {
		t.Fatalf("first patch status = %d", firstPatch.status)
	}

	secondPatch := doJSONRequest(t, app, http.MethodPatch, "/api/v1/beds/"+intToString(bedID), patchBody, map[string]string{
		"Cookie":           cookieHeader,
		"If-Match-Version": intToString(version),
	})
	if secondPatch.status != http.StatusConflict {
		t.Fatalf("second patch status = %d", secondPatch.status)
	}
	if secondPatch.env.Error == nil || secondPatch.env.Error.Code != "VERSION_CONFLICT" {
		t.Fatalf("expected VERSION_CONFLICT error, got %+v", secondPatch.env.Error)
	}
}

func TestAdmissionsLifecycleAndOccupancyFragment(t *testing.T) {
	t.Parallel()

	app, cfg, cleanup := setupTestApp(t)
	defer cleanup()

	cookieHeader := loginAndGetCookie(t, app, cfg)

	wardResp := doJSONRequest(t, app, http.MethodPost, "/api/v1/wards", map[string]any{"name": "General"}, map[string]string{"Cookie": cookieHeader})
	if wardResp.status != http.StatusCreated {
		t.Fatalf("create ward status = %d", wardResp.status)
	}
	wardID := int64FromMap(t, wardResp.env.Data["ward"].(map[string]any), "id")

	patientResp := doJSONRequest(t, app, http.MethodPost, "/api/v1/patients", map[string]any{"mrn": "MRN-1001", "name": "Jane Doe"}, map[string]string{"Cookie": cookieHeader})
	if patientResp.status != http.StatusCreated {
		t.Fatalf("create patient status = %d", patientResp.status)
	}
	patientID := int64FromMap(t, patientResp.env.Data["patient"].(map[string]any), "id")

	bed1Resp := doJSONRequest(t, app, http.MethodPost, "/api/v1/beds", map[string]any{"ward_id": wardID, "bed_code": "G-01"}, map[string]string{"Cookie": cookieHeader})
	if bed1Resp.status != http.StatusCreated {
		t.Fatalf("create bed1 status = %d", bed1Resp.status)
	}
	bed1ID := int64FromMap(t, bed1Resp.env.Data["bed"].(map[string]any), "id")

	bed2Resp := doJSONRequest(t, app, http.MethodPost, "/api/v1/beds", map[string]any{"ward_id": wardID, "bed_code": "G-02"}, map[string]string{"Cookie": cookieHeader})
	if bed2Resp.status != http.StatusCreated {
		t.Fatalf("create bed2 status = %d", bed2Resp.status)
	}
	bed2ID := int64FromMap(t, bed2Resp.env.Data["bed"].(map[string]any), "id")

	admissionResp := doJSONRequest(t, app, http.MethodPost, "/api/v1/admissions", map[string]any{"patient_id": patientID, "bed_id": bed1ID}, map[string]string{"Cookie": cookieHeader})
	if admissionResp.status != http.StatusCreated {
		t.Fatalf("create admission status = %d", admissionResp.status)
	}
	admissionID := int64FromMap(t, admissionResp.env.Data["admission"].(map[string]any), "id")

	transferResp := doJSONRequest(t, app, http.MethodPost, "/api/v1/admissions/"+intToString(admissionID)+"/transfer", map[string]any{"to_bed_id": bed2ID}, map[string]string{"Cookie": cookieHeader})
	if transferResp.status != http.StatusOK {
		t.Fatalf("transfer admission status = %d", transferResp.status)
	}

	dischargeResp := doJSONRequest(t, app, http.MethodPost, "/api/v1/admissions/"+intToString(admissionID)+"/discharge", nil, map[string]string{"Cookie": cookieHeader})
	if dischargeResp.status != http.StatusOK {
		t.Fatalf("discharge admission status = %d", dischargeResp.status)
	}

	htmlResp := doRawRequest(t, app, http.MethodGet, "/ui/occupancy/board", nil, map[string]string{"Cookie": cookieHeader})
	if htmlResp.status != http.StatusOK {
		t.Fatalf("occupancy board status = %d", htmlResp.status)
	}
	if !strings.Contains(htmlResp.body, "G-01") || !strings.Contains(htmlResp.body, "G-02") {
		t.Fatalf("occupancy board does not include expected bed codes: %s", htmlResp.body)
	}
	if !strings.Contains(htmlResp.body, "bed-status-cleaning") {
		t.Fatalf("occupancy board does not include cleaning status class: %s", htmlResp.body)
	}
}

type httpResult struct {
	status int
	body   string
	env    envelope
}

func doJSONRequest(t *testing.T, app *fiber.App, method, path string, body any, headers map[string]string) httpResult {
	t.Helper()

	rawBody := []byte{}
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rawBody = encoded
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if isMutationMethod(method) && req.Header.Get("Idempotency-Key") == "" {
		req.Header.Set("Idempotency-Key", method+"-"+path+"-"+strconv.FormatInt(time.Now().UTC().UnixNano(), 10))
	}

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app test request: %v", err)
	}
	defer res.Body.Close()

	var out bytes.Buffer
	if _, err := out.ReadFrom(res.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal json response: %v body=%s", err, out.String())
	}

	return httpResult{status: res.StatusCode, body: out.String(), env: env}
}

func doRawRequest(t *testing.T, app *fiber.App, method, path string, body []byte, headers map[string]string) httpResult {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app test request: %v", err)
	}
	defer res.Body.Close()

	var out bytes.Buffer
	if _, err := out.ReadFrom(res.Body); err != nil {
		t.Fatalf("read raw response body: %v", err)
	}

	return httpResult{status: res.StatusCode, body: out.String()}
}

func loginAndGetCookie(t *testing.T, app *fiber.App, cfg config.Config) string {
	t.Helper()

	reqBody, err := json.Marshal(map[string]any{"username": "admin", "password": "AdminPassword1!"})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(res.Body)
		t.Fatalf("login status=%d body=%s", res.StatusCode, body.String())
	}

	for _, cookie := range res.Cookies() {
		if cookie.Name == cfg.SessionCookieName {
			return cookie.Name + "=" + cookie.Value
		}
	}

	t.Fatalf("session cookie %s not found", cfg.SessionCookieName)
	return ""
}

func int64FromMap(t *testing.T, m map[string]any, key string) int64 {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("key %s not found in map", key)
	}
	n, ok := v.(float64)
	if !ok {
		t.Fatalf("key %s is not float64, got %T", key, v)
	}
	return int64(n)
}

func intToString(v int64) string {
	return strconv.FormatInt(v, 10)
}

func isMutationMethod(method string) bool {
	m := strings.ToUpper(strings.TrimSpace(method))
	return m == http.MethodPost || m == http.MethodPatch || m == http.MethodPut || m == http.MethodDelete
}

func setupTestApp(t *testing.T) (*fiber.App, config.Config, func()) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "api-integration.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	ctx := context.Background()
	if err := migrations.Run(ctx, db); err != nil {
		_ = db.Close()
		t.Fatalf("run migrations: %v", err)
	}

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	patientRepo := sqlite.NewPatientRepository(db)
	wardRepo := sqlite.NewWardRepository(db)
	bedRepo := sqlite.NewBedRepository(db)
	admissionRepo := sqlite.NewAdmissionRepository(db)
	examScheduleRepo := sqlite.NewExamScheduleRepository(db)
	paymentRepo := sqlite.NewPaymentRepository(db)

	auditService := service.NewAuditService(auditRepo)
	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 12, 15*time.Minute)
	admissionsService := service.NewAdmissionsService(db, patientRepo, wardRepo, bedRepo, admissionRepo)
	reportService := service.NewReportService(db, paymentRepo)
	exerciseFavoriteService := service.NewExerciseFavoriteService(db)
	careService := service.NewCareService(db, auditService)
	idempotencyRepo := sqlite.NewIdempotencyRepository(db)
	schedulingService := service.NewSchedulingService(db, examScheduleRepo, idempotencyRepo, auditService)
	examTemplateService := service.NewExamTemplateService(db, examScheduleRepo, schedulingService, auditService)

	if err := authService.EnsureBootstrapAdmin(ctx, "admin", "AdminPassword1!"); err != nil {
		_ = db.Close()
		t.Fatalf("bootstrap admin: %v", err)
	}

	cfg := config.Config{
		SessionCookieName: "clinic_session",
		CookieSecure:      false,
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	api.Register(app, api.Dependencies{
		Config:              cfg,
		AuthService:         authService,
		AdmissionsService:   admissionsService,
		ExerciseFavorite:    exerciseFavoriteService,
		CareService:         careService,
		ExamTemplateService: examTemplateService,
		ReportService:       reportService,
		AuditService:        auditService,
		IdempotencyRepo:     idempotencyRepo,
	})

	return app, cfg, func() {
		_ = db.Close()
		_ = app.Shutdown()
	}
}
