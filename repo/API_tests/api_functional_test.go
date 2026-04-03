package api_tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"clinic-admin-suite/internal/api"
	"clinic-admin-suite/internal/config"
	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository/migrations"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type apiEnv struct {
	app *fiber.App
	db  *sql.DB
	cfg config.Config
}

type httpResult struct {
	status int
	body   []byte
	env    responseEnvelope
}

type responseEnvelope struct {
	Data  map[string]any `json:"data"`
	Meta  map[string]any `json:"meta"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
}

func TestAuthLoginFunctionalAndMalformedJSON(t *testing.T) {
	env := setupAPIEnv(t)

	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":`))
	badReq.Header.Set("Content-Type", "application/json")
	badRes, err := env.app.Test(badReq, -1)
	if err != nil {
		t.Fatalf("bad login request failed: %v", err)
	}
	defer badRes.Body.Close()

	badBody := readAll(t, badRes.Body)
	badEnv := decodeEnvelope(t, badBody)
	if badRes.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for malformed json, got %d body=%s", badRes.StatusCode, string(badBody))
	}
	if badEnv.Error == nil || badEnv.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR envelope, got %+v", badEnv.Error)
	}

	cookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")
	if cookie == "" {
		t.Fatalf("expected non-empty admin session cookie")
	}
}

func TestAdmissionsEndpointMutatesState(t *testing.T) {
	env := setupAPIEnv(t)
	adminCookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")

	wardRes := doJSON(t, env.app, http.MethodPost, "/api/v1/wards", map[string]any{"name": "Emergency"}, map[string]string{"Cookie": adminCookie})
	if wardRes.status != http.StatusCreated {
		t.Fatalf("create ward failed status=%d body=%s", wardRes.status, string(wardRes.body))
	}
	wardID := int64FromData(t, wardRes.env.Data["ward"], "id")

	patientRes := doJSON(t, env.app, http.MethodPost, "/api/v1/patients", map[string]any{"mrn": "MRN-API-1", "name": "API Patient"}, map[string]string{"Cookie": adminCookie})
	if patientRes.status != http.StatusCreated {
		t.Fatalf("create patient failed status=%d body=%s", patientRes.status, string(patientRes.body))
	}
	patientID := int64FromData(t, patientRes.env.Data["patient"], "id")

	bedRes := doJSON(t, env.app, http.MethodPost, "/api/v1/beds", map[string]any{"ward_id": wardID, "bed_code": "E-01"}, map[string]string{"Cookie": adminCookie})
	if bedRes.status != http.StatusCreated {
		t.Fatalf("create bed failed status=%d body=%s", bedRes.status, string(bedRes.body))
	}
	bedID := int64FromData(t, bedRes.env.Data["bed"], "id")

	before := countRows(t, env.db, "admissions")

	admissionRes := doJSON(t, env.app, http.MethodPost, "/api/v1/admissions", map[string]any{"patient_id": patientID, "bed_id": bedID}, map[string]string{"Cookie": adminCookie})
	if admissionRes.status != http.StatusCreated {
		t.Fatalf("create admission failed status=%d body=%s", admissionRes.status, string(admissionRes.body))
	}

	after := countRows(t, env.db, "admissions")
	if after != before+1 {
		t.Fatalf("expected admissions count to increase by 1, before=%d after=%d", before, after)
	}

	bedsRes := doJSON(t, env.app, http.MethodGet, "/api/v1/beds?ward_id="+strconv.FormatInt(wardID, 10), nil, map[string]string{"Cookie": adminCookie})
	if bedsRes.status != http.StatusOK {
		t.Fatalf("list beds failed status=%d body=%s", bedsRes.status, string(bedsRes.body))
	}
	beds := bedsRes.env.Data["beds"].([]any)
	if len(beds) != 1 {
		t.Fatalf("expected one bed, got %d", len(beds))
	}
	bed := beds[0].(map[string]any)
	if status, _ := bed["status"].(string); status != domain.BedStatusOccupied {
		t.Fatalf("expected bed occupied after admission, got %v", bed["status"])
	}
}

func TestExamSchedulesIdempotencyAndConflictScenarios(t *testing.T) {
	env := setupAPIEnv(t)
	adminCookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")

	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	end := start.Add(90 * time.Minute)
	payload := map[string]any{
		"exam_id":       "exam-api-1",
		"room_id":       100,
		"proctor_id":    200,
		"candidate_ids": []int64{3001, 3002},
		"start_at":      start.Format(time.RFC3339),
		"end_at":        end.Format(time.RFC3339),
	}

	first := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-schedules", payload, map[string]string{
		"Cookie":          adminCookie,
		"Idempotency-Key": "sched-key-1",
	})
	if first.status != http.StatusCreated {
		t.Fatalf("create schedule failed status=%d body=%s", first.status, string(first.body))
	}
	scheduleID := int64FromData(t, first.env.Data["schedule"], "id")
	if countRows(t, env.db, "exam_schedules") != 1 {
		t.Fatalf("expected one exam_schedule after first create")
	}

	replay := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-schedules", payload, map[string]string{
		"Cookie":          adminCookie,
		"Idempotency-Key": "sched-key-1",
	})
	if replay.status != http.StatusCreated {
		t.Fatalf("idempotent replay failed status=%d body=%s", replay.status, string(replay.body))
	}
	if countRows(t, env.db, "exam_schedules") != 1 {
		t.Fatalf("expected schedule count unchanged on replay")
	}

	payloadDifferent := map[string]any{
		"exam_id":       "exam-api-2",
		"room_id":       101,
		"proctor_id":    201,
		"candidate_ids": []int64{3003},
		"start_at":      start.Add(3 * time.Hour).Format(time.RFC3339),
		"end_at":        end.Add(3 * time.Hour).Format(time.RFC3339),
	}
	idemConflict := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-schedules", payloadDifferent, map[string]string{
		"Cookie":          adminCookie,
		"Idempotency-Key": "sched-key-1",
	})
	if idemConflict.status != http.StatusConflict {
		t.Fatalf("expected idempotency conflict status=409 got=%d body=%s", idemConflict.status, string(idemConflict.body))
	}
	if idemConflict.env.Error == nil || idemConflict.env.Error.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("expected IDEMPOTENCY_CONFLICT, got %+v", idemConflict.env.Error)
	}

	overlap := map[string]any{
		"exam_id":       "exam-api-3",
		"room_id":       100,
		"proctor_id":    999,
		"candidate_ids": []int64{4000},
		"start_at":      start.Add(15 * time.Minute).Format(time.RFC3339),
		"end_at":        end.Add(15 * time.Minute).Format(time.RFC3339),
	}
	schedConflict := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-schedules", overlap, map[string]string{
		"Cookie":          adminCookie,
		"Idempotency-Key": "sched-key-2",
	})
	if schedConflict.status != http.StatusConflict {
		t.Fatalf("expected scheduling conflict status=409 got=%d body=%s", schedConflict.status, string(schedConflict.body))
	}
	if schedConflict.env.Error == nil || schedConflict.env.Error.Code != "SCHEDULING_CONFLICT" {
		t.Fatalf("expected SCHEDULING_CONFLICT, got %+v", schedConflict.env.Error)
	}

	validate := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-schedules/"+strconv.FormatInt(scheduleID, 10)+"/validate", nil, map[string]string{"Cookie": adminCookie})
	if validate.status != http.StatusOK {
		t.Fatalf("validate schedule failed status=%d body=%s", validate.status, string(validate.body))
	}
	if hasConflicts, _ := validate.env.Data["has_conflicts"].(bool); hasConflicts {
		t.Fatalf("expected no self-conflicts on validate, got has_conflicts=true")
	}
}

func TestRBACForbiddenAndValidationErrors(t *testing.T) {
	env := setupAPIEnv(t)
	clinicianCookie := loginAs(t, env.app, env.cfg, "clinician", "ClinicianPass1!")

	forbidden := doJSON(t, env.app, http.MethodPost, "/api/v1/admissions", map[string]any{"patient_id": 1, "bed_id": 1}, map[string]string{"Cookie": clinicianCookie})
	if forbidden.status != http.StatusForbidden {
		t.Fatalf("expected 403 for clinician admissions write, got %d body=%s", forbidden.status, string(forbidden.body))
	}
	if forbidden.env.Error == nil || forbidden.env.Error.Code != "AUTH_FORBIDDEN" {
		t.Fatalf("expected AUTH_FORBIDDEN, got %+v", forbidden.env.Error)
	}

	adminCookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")
	missingParams := doJSON(t, env.app, http.MethodPost, "/api/v1/admissions", map[string]any{}, map[string]string{"Cookie": adminCookie})
	if missingParams.status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing admission params, got %d body=%s", missingParams.status, string(missingParams.body))
	}
	if missingParams.env.Error == nil || missingParams.env.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %+v", missingParams.env.Error)
	}
}

func TestFrontendAssetsAndFinanceRefundExport(t *testing.T) {
	env := setupAPIEnv(t)
	adminCookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")

	uiReq := httptest.NewRequest(http.MethodGet, "/app", nil)
	uiReq.Header.Set("Cookie", adminCookie)
	uiRes, err := env.app.Test(uiReq, -1)
	if err != nil {
		t.Fatalf("request app shell failed: %v", err)
	}
	defer uiRes.Body.Close()
	if uiRes.StatusCode != http.StatusOK {
		t.Fatalf("expected /app status 200 got=%d", uiRes.StatusCode)
	}
	uiBody := string(readAll(t, uiRes.Body))
	if !strings.Contains(uiBody, "CareOps Clinic Administration Suite") {
		t.Fatalf("app shell did not include expected title")
	}

	cssReq := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	cssRes, err := env.app.Test(cssReq, -1)
	if err != nil {
		t.Fatalf("request css asset failed: %v", err)
	}
	defer cssRes.Body.Close()
	if cssRes.StatusCode != http.StatusOK {
		t.Fatalf("expected css asset status 200 got=%d", cssRes.StatusCode)
	}

	paymentCreate := doJSON(t, env.app, http.MethodPost, "/api/v1/payments", map[string]any{
		"method":       "cash",
		"gateway":      "check_local",
		"amount_cents": 24000,
		"currency":     "USD",
		"shift_id":     "shift-0700",
	}, map[string]string{"Cookie": adminCookie})
	if paymentCreate.status != http.StatusCreated {
		t.Fatalf("create payment failed status=%d body=%s", paymentCreate.status, string(paymentCreate.body))
	}
	paymentID := int64FromData(t, paymentCreate.env.Data["payment"], "id")

	refundRes := doJSON(t, env.app, http.MethodPost, "/api/v1/payments/"+strconv.FormatInt(paymentID, 10)+"/refunds", map[string]any{
		"amount_cents": 10000,
		"reason":       "patient_adjustment",
	}, map[string]string{"Cookie": adminCookie})
	if refundRes.status != http.StatusCreated {
		t.Fatalf("refund payment failed status=%d body=%s", refundRes.status, string(refundRes.body))
	}

	csvReq := httptest.NewRequest(http.MethodGet, "/api/v1/reports/finance/export?format=csv", nil)
	csvReq.Header.Set("Cookie", adminCookie)
	csvRes, err := env.app.Test(csvReq, -1)
	if err != nil {
		t.Fatalf("finance csv export request failed: %v", err)
	}
	defer csvRes.Body.Close()
	if csvRes.StatusCode != http.StatusOK {
		t.Fatalf("finance csv export status=%d", csvRes.StatusCode)
	}
	if !strings.Contains(strings.ToLower(csvRes.Header.Get("Content-Type")), "text/csv") {
		t.Fatalf("expected text/csv content type, got %s", csvRes.Header.Get("Content-Type"))
	}
	csvBody := string(readAll(t, csvRes.Body))
	if !strings.Contains(csvBody, "payment_id") {
		t.Fatalf("csv payload missing header")
	}

	xlsxReq := httptest.NewRequest(http.MethodGet, "/api/v1/reports/finance/export?format=xlsx", nil)
	xlsxReq.Header.Set("Cookie", adminCookie)
	xlsxRes, err := env.app.Test(xlsxReq, -1)
	if err != nil {
		t.Fatalf("finance xlsx export request failed: %v", err)
	}
	defer xlsxRes.Body.Close()
	if xlsxRes.StatusCode != http.StatusOK {
		t.Fatalf("finance xlsx export status=%d", xlsxRes.StatusCode)
	}
	xlsxBody := readAll(t, xlsxRes.Body)
	if len(xlsxBody) < 4 || !bytes.Equal(xlsxBody[:4], []byte{'P', 'K', 3, 4}) {
		t.Fatalf("expected xlsx zip signature")
	}
}

func TestFinanceShiftPolicyValidation(t *testing.T) {
	env := setupAPIEnv(t)
	adminCookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")

	invalidPayment := doJSON(t, env.app, http.MethodPost, "/api/v1/payments", map[string]any{
		"method":       "cash",
		"gateway":      "check_local",
		"amount_cents": 1000,
		"currency":     "USD",
		"shift_id":     "night-shift",
	}, map[string]string{"Cookie": adminCookie})
	if invalidPayment.status != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid payment shift status=422 got=%d body=%s", invalidPayment.status, string(invalidPayment.body))
	}
	if invalidPayment.env.Error == nil || invalidPayment.env.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR for invalid payment shift, got %+v", invalidPayment.env.Error)
	}

	validPayment := doJSON(t, env.app, http.MethodPost, "/api/v1/payments", map[string]any{
		"method":       "cash",
		"gateway":      "check_local",
		"amount_cents": 2500,
		"currency":     "USD",
		"shift_id":     "07:00",
	}, map[string]string{"Cookie": adminCookie})
	if validPayment.status != http.StatusCreated {
		t.Fatalf("create payment with alias shift failed status=%d body=%s", validPayment.status, string(validPayment.body))
	}
	paymentShift, _ := validPayment.env.Data["payment"].(map[string]any)["shift_id"].(string)
	if paymentShift != "shift-0700" {
		t.Fatalf("expected canonical shift_id=shift-0700, got %q", paymentShift)
	}

	invalidSettlement := doJSON(t, env.app, http.MethodPost, "/api/v1/settlements/run", map[string]any{
		"shift_id":           "swing",
		"actual_total_cents": 0,
	}, map[string]string{"Cookie": adminCookie})
	if invalidSettlement.status != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid settlement shift status=422 got=%d body=%s", invalidSettlement.status, string(invalidSettlement.body))
	}
	if invalidSettlement.env.Error == nil || invalidSettlement.env.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR for invalid settlement shift, got %+v", invalidSettlement.env.Error)
	}
}

func TestCareSchedulingGovernanceFlows(t *testing.T) {
	env := setupAPIEnv(t)
	adminCookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")

	patient := doJSON(t, env.app, http.MethodPost, "/api/v1/patients", map[string]any{"mrn": "MRN-CARE-1", "name": "Resident One"}, map[string]string{"Cookie": adminCookie})
	if patient.status != http.StatusCreated {
		t.Fatalf("create patient failed status=%d body=%s", patient.status, string(patient.body))
	}
	residentID := int64FromData(t, patient.env.Data["patient"], "id")

	checkpoint := doJSON(t, env.app, http.MethodPost, "/api/v1/care-quality-checkpoints", map[string]any{
		"resident_id":     residentID,
		"checkpoint_type": "hydration",
		"status":          "watch",
		"notes":           "needs recheck",
	}, map[string]string{"Cookie": adminCookie})
	if checkpoint.status != http.StatusCreated {
		t.Fatalf("create checkpoint failed status=%d body=%s", checkpoint.status, string(checkpoint.body))
	}

	alert := doJSON(t, env.app, http.MethodPost, "/api/v1/alert-events", map[string]any{
		"resident_id": residentID,
		"alert_type":  "fall_risk",
		"severity":    "high",
		"state":       "open",
		"message":     "bed exit alarm",
	}, map[string]string{"Cookie": adminCookie})
	if alert.status != http.StatusCreated {
		t.Fatalf("create alert failed status=%d body=%s", alert.status, string(alert.body))
	}

	tStart := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Minute)
	tEnd := tStart.Add(3 * time.Hour)
	template := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-templates", map[string]any{
		"title":            "Neuro Window",
		"subject":          "Neurology",
		"duration_minutes": 90,
		"room_id":          311,
		"proctor_id":       991,
		"candidate_ids":    []int64{701, 702},
		"window_label":     "AM Block",
		"window_start_at":  tStart.Format(time.RFC3339),
		"window_end_at":    tEnd.Format(time.RFC3339),
	}, map[string]string{"Cookie": adminCookie})
	if template.status != http.StatusCreated {
		t.Fatalf("create exam template failed status=%d body=%s", template.status, string(template.body))
	}
	templateID := int64FromData(t, template.env.Data["exam_template"], "id")
	windows := template.env.Data["exam_template"].(map[string]any)["windows"].([]any)
	windowID := int64FromData(t, windows[0], "id")

	draft := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-session-drafts/generate", map[string]any{
		"template_id": templateID,
		"window_id":   windowID,
	}, map[string]string{"Cookie": adminCookie})
	if draft.status != http.StatusCreated {
		t.Fatalf("generate exam draft failed status=%d body=%s", draft.status, string(draft.body))
	}
	draftID := int64FromData(t, draft.env.Data["exam_session_draft"], "id")

	adjust := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-session-drafts/"+strconv.FormatInt(draftID, 10)+"/adjust", map[string]any{
		"start_at": tStart.Add(10 * time.Minute).Format(time.RFC3339),
		"end_at":   tStart.Add(100 * time.Minute).Format(time.RFC3339),
	}, map[string]string{"Cookie": adminCookie})
	if adjust.status != http.StatusOK {
		t.Fatalf("adjust draft failed status=%d body=%s", adjust.status, string(adjust.body))
	}

	publish := doJSON(t, env.app, http.MethodPost, "/api/v1/exam-session-drafts/"+strconv.FormatInt(draftID, 10)+"/publish", nil, map[string]string{"Cookie": adminCookie})
	if publish.status != http.StatusOK {
		t.Fatalf("publish draft failed status=%d body=%s", publish.status, string(publish.body))
	}

	auditSearch := doJSON(t, env.app, http.MethodGet, "/api/v1/reports/audit/search?resident_id="+strconv.FormatInt(residentID, 10), nil, map[string]string{"Cookie": adminCookie})
	if auditSearch.status != http.StatusOK {
		t.Fatalf("audit search failed status=%d body=%s", auditSearch.status, string(auditSearch.body))
	}

	shared := filepath.Join(t.TempDir(), "scheduled_reports")
	schedule := doJSON(t, env.app, http.MethodPost, "/api/v1/reports/schedules", map[string]any{
		"report_type":        "audit",
		"format":             "csv",
		"shared_folder_path": shared,
		"interval_minutes":   5,
		"first_run_at":       time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	}, map[string]string{"Cookie": adminCookie})
	if schedule.status != http.StatusCreated {
		t.Fatalf("create report schedule failed status=%d body=%s", schedule.status, string(schedule.body))
	}

	runNow := doJSON(t, env.app, http.MethodPost, "/api/v1/reports/schedules/run-now", nil, map[string]string{"Cookie": adminCookie})
	if runNow.status != http.StatusOK {
		t.Fatalf("run report schedule failed status=%d body=%s", runNow.status, string(runNow.body))
	}
	entries, err := os.ReadDir(shared)
	if err != nil {
		t.Fatalf("read shared report path failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected scheduled report file in %s", shared)
	}

	configVersion := doJSON(t, env.app, http.MethodPost, "/api/v1/config/versions", map[string]any{
		"config_key":   "reporting",
		"payload_json": "{\"shared_root\":\"" + shared + "\"}",
	}, map[string]string{"Cookie": adminCookie})
	if configVersion.status != http.StatusCreated {
		t.Fatalf("create config version failed status=%d body=%s", configVersion.status, string(configVersion.body))
	}
	versionID := int64FromData(t, configVersion.env.Data["config_version"], "id")

	rollback := doJSON(t, env.app, http.MethodPost, "/api/v1/config/versions/"+strconv.FormatInt(versionID, 10)+"/rollback", nil, map[string]string{"Cookie": adminCookie})
	if rollback.status != http.StatusOK {
		t.Fatalf("rollback config version failed status=%d body=%s", rollback.status, string(rollback.body))
	}
}

func TestRateLimiterBlocksBruteForceLogin(t *testing.T) {
	env := setupAPIEnv(t)

	for i := 0; i < 12; i++ {
		username := "nonexistent_user_" + strconv.Itoa(i)
		payload, _ := json.Marshal(map[string]any{"username": username, "password": "WrongPass!"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		res, err := env.app.Test(req, -1)
		if err != nil {
			t.Fatalf("login request %d failed: %v", i, err)
		}
		res.Body.Close()
		if res.StatusCode == http.StatusTooManyRequests {
			return
		}
	}
	t.Fatalf("expected rate limiter to block after repeated login attempts, but all 12 requests succeeded")
}

func TestCSRFEnforcementOnUIFormSubmission(t *testing.T) {
	env := setupAPIEnv(t)
	adminCookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")

	// GET a panel page to receive the CSRF cookie
	panelReq := httptest.NewRequest(http.MethodGet, "/ui/panels/care", nil)
	panelReq.Header.Set("Cookie", adminCookie)
	panelRes, err := env.app.Test(panelReq, -1)
	if err != nil {
		t.Fatalf("panel request failed: %v", err)
	}
	panelRes.Body.Close()

	var csrfToken string
	for _, cookie := range panelRes.Cookies() {
		if cookie.Name == "clinic_csrf" {
			csrfToken = cookie.Value
			break
		}
	}
	if csrfToken == "" {
		t.Fatalf("expected clinic_csrf cookie from GET request")
	}

	// Create a patient first
	patientRes := doJSON(t, env.app, http.MethodPost, "/api/v1/patients", map[string]any{"mrn": "CSRF-001", "name": "Test Patient"}, map[string]string{"Cookie": adminCookie})
	if patientRes.status != http.StatusCreated {
		t.Fatalf("create patient failed status=%d body=%s", patientRes.status, string(patientRes.body))
	}
	patientID := int64FromData(t, patientRes.env.Data["patient"], "id")

	// POST without CSRF token should fail with 403
	noCSRFReq := httptest.NewRequest(http.MethodGet, "/ui/service-delivery/patient/"+strconv.FormatInt(patientID, 10), nil)
	noCSRFReq.Header.Set("Cookie", adminCookie+"; clinic_csrf="+csrfToken)
	noCSRFRes, err := env.app.Test(noCSRFReq, -1)
	if err != nil {
		t.Fatalf("service delivery request failed: %v", err)
	}
	noCSRFRes.Body.Close()
	if noCSRFRes.StatusCode != http.StatusOK {
		t.Fatalf("expected GET service delivery to succeed, got status=%d", noCSRFRes.StatusCode)
	}

	// POST without CSRF token should fail
	postNoCSRFReq := httptest.NewRequest(http.MethodPost, "/ui/care/checkpoints", strings.NewReader("resident_id="+strconv.FormatInt(patientID, 10)+"&checkpoint_type=hydration&status=pass&notes=test"))
	postNoCSRFReq.Header.Set("Cookie", adminCookie+"; clinic_csrf="+csrfToken)
	postNoCSRFReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postNoCSRFReq.Header.Set("Idempotency-Key", "csrf-test-no-token")
	postNoCSRFRes, err := env.app.Test(postNoCSRFReq, -1)
	if err != nil {
		t.Fatalf("POST without CSRF failed: %v", err)
	}
	postNoCSRFRes.Body.Close()
	if postNoCSRFRes.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for POST without CSRF token, got %d", postNoCSRFRes.StatusCode)
	}

	// POST with valid CSRF token should succeed
	postWithCSRFReq := httptest.NewRequest(http.MethodPost, "/ui/care/checkpoints", strings.NewReader("resident_id="+strconv.FormatInt(patientID, 10)+"&checkpoint_type=hydration&status=pass&notes=test"))
	postWithCSRFReq.Header.Set("Cookie", adminCookie+"; clinic_csrf="+csrfToken)
	postWithCSRFReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postWithCSRFReq.Header.Set("X-CSRF-Token", csrfToken)
	postWithCSRFReq.Header.Set("Idempotency-Key", "csrf-test-with-token")
	postWithCSRFRes, err := env.app.Test(postWithCSRFReq, -1)
	if err != nil {
		t.Fatalf("POST with CSRF failed: %v", err)
	}
	postWithCSRFRes.Body.Close()
	if postWithCSRFRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for POST with valid CSRF token, got %d", postWithCSRFRes.StatusCode)
	}
}

func setupAPIEnv(t *testing.T) *apiEnv {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "api-tests.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := migrations.Run(context.Background(), db); err != nil {
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
	jobRunService := service.NewJobRunService(jobRunRepo)
	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 12, 15*time.Minute)
	admissionsService := service.NewAdmissionsService(db, patientRepo, wardRepo, bedRepo, admissionRepo)
	workOrderService := service.NewWorkOrderService(db, workOrderRepo, auditService, jobRunService)
	kpiService := service.NewKPIService(db, workOrderRepo, kpiRollupRepo, jobRunService)
	exerciseService := service.NewExerciseService(db, exerciseRepo, mediaRepo)
	mediaService := service.NewMediaService(filepath.Join(t.TempDir(), "media"), exerciseRepo, mediaRepo)
	schedulingService := service.NewSchedulingService(db, examScheduleRepo, idempotencyRepo, auditService)
	exerciseFavoriteService := service.NewExerciseFavoriteService(db)
	careService := service.NewCareService(db, auditService)
	examTemplateService := service.NewExamTemplateService(db, examScheduleRepo, schedulingService, auditService)
	logs := service.NewStructuredLogService(filepath.Join(t.TempDir(), "logs", "structured.log"))
	testCipher, err := service.NewFieldCipherFromBase64("ZTJlX3Rlc3Rfa2V5X2Zvcl9hZXMyNTZfMzJieXRlc1g=")
	if err != nil {
		_ = db.Close()
		t.Fatalf("create test cipher: %v", err)
	}
	paymentService := service.NewPaymentService(db, paymentRepo, paymentEventRepo, auditService, testCipher, 1, []service.GatewayAdapter{&service.CashGatewayAdapter{}, &service.CheckGatewayAdapter{}, &service.FacilityChargeGatewayAdapter{}, &service.ImportedCardBatchGatewayAdapter{}, &service.LocalCardGatewayAdapter{}}, logs)
	settlementService := service.NewSettlementService(db, paymentRepo, settlementRepo, auditService, jobRunService, logs)
	diagnosticsService := service.NewDiagnosticsService(db, logs.Path(), filepath.Join(t.TempDir(), "diag"))
	reportService := service.NewReportService(db, paymentRepo)
	reportService.ConfigureScheduling(filepath.Join(t.TempDir(), "shared"), jobRunService, logs, auditService)

	if err := authService.EnsureBootstrapAdmin(context.Background(), "admin", "AdminPassword1!"); err != nil {
		_ = db.Close()
		t.Fatalf("bootstrap admin: %v", err)
	}

	clinicianHash, err := bcrypt.GenerateFromPassword([]byte("ClinicianPass1!"), 12)
	if err != nil {
		_ = db.Close()
		t.Fatalf("hash clinician password: %v", err)
	}
	if err := userRepo.Create(context.Background(), &domain.User{Username: "clinician", PasswordHash: string(clinicianHash), Role: string(domain.RoleClinician)}); err != nil {
		_ = db.Close()
		t.Fatalf("create clinician user: %v", err)
	}

	cfg := config.Config{SessionCookieName: "clinic_session", CookieSecure: false, MediaRoot: filepath.Join(t.TempDir(), "media")}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
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

	t.Cleanup(func() {
		_ = app.Shutdown()
		_ = db.Close()
	})

	return &apiEnv{app: app, db: db, cfg: cfg}
}

func doJSON(t *testing.T, app *fiber.App, method, path string, body any, headers map[string]string) httpResult {
	t.Helper()

	raw := []byte{}
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		raw = encoded
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if isMutationMethod(method) && req.Header.Get("Idempotency-Key") == "" {
		req.Header.Set("Idempotency-Key", method+"-"+path+"-"+strconv.FormatInt(time.Now().UTC().UnixNano(), 10))
	}

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	bodyBytes := readAll(t, res.Body)
	env := decodeEnvelope(t, bodyBytes)

	return httpResult{status: res.StatusCode, body: bodyBytes, env: env}
}

func loginAs(t *testing.T, app *fiber.App, cfg config.Config, username, password string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{"username": username, "password": password})
	if err != nil {
		t.Fatalf("marshal login payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body := readAll(t, res.Body)
		t.Fatalf("login failed status=%d body=%s", res.StatusCode, string(body))
	}

	for _, cookie := range res.Cookies() {
		if cookie.Name == cfg.SessionCookieName {
			return cookie.Name + "=" + cookie.Value
		}
	}
	t.Fatalf("session cookie %s not returned", cfg.SessionCookieName)
	return ""
}

func decodeEnvelope(t *testing.T, body []byte) responseEnvelope {
	t.Helper()
	var env responseEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope failed: %v body=%s", err, string(body))
	}
	return env
}

func readAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return raw
}

func int64FromData(t *testing.T, payload any, key string) int64 {
	t.Helper()
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", payload)
	}
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %s in payload", key)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64 for key %s, got %T", key, v)
	}
	return int64(f)
}

func countRows(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(1) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count rows for %s: %v", table, err)
	}
	return n
}

func isMutationMethod(method string) bool {
	m := strings.ToUpper(strings.TrimSpace(method))
	return m == http.MethodPost || m == http.MethodPatch || m == http.MethodPut || m == http.MethodDelete
}

func TestAllPanelsRenderSuccessfully(t *testing.T) {
	env := setupAPIEnv(t)
	cookie := loginAs(t, env.app, env.cfg, "admin", "AdminPassword1!")

	// Get CSRF cookie first
	getReq := httptest.NewRequest(http.MethodGet, "/ui/panels/overview", nil)
	getReq.Header.Set("Cookie", cookie)
	getRes, err := env.app.Test(getReq, -1)
	if err != nil {
		t.Fatalf("GET overview failed: %v", err)
	}
	getRes.Body.Close()

	var csrfCookie string
	for _, c := range getRes.Cookies() {
		if c.Name == "clinic_csrf" {
			csrfCookie = "; clinic_csrf=" + c.Value
		}
	}

	panels := []string{
		"/ui/panels/overview",
		"/ui/panels/occupancy",
		"/ui/panels/exercises",
		"/ui/panels/scheduling",
		"/ui/panels/finance",
		"/ui/panels/reports",
		"/ui/panels/care",
	}
	for _, panel := range panels {
		req := httptest.NewRequest(http.MethodGet, panel, nil)
		req.Header.Set("Cookie", cookie+csrfCookie)
		res, err := env.app.Test(req, 5000)
		if err != nil {
			t.Fatalf("panel %s request error: %v", panel, err)
		}
		body := readAll(t, res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("panel %s status=%d body=%s", panel, res.StatusCode, string(body))
		}
		if len(body) < 20 {
			t.Fatalf("panel %s returned suspiciously short body (len=%d): %s", panel, len(body), string(body))
		}
		t.Logf("OK %s status=%d len=%d", panel, res.StatusCode, len(body))
	}
}
