package handler

import (
	"bytes"
	"fmt"
	"html"
	htmltpl "html/template"
	"sort"
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/api/middleware"
	"clinic-admin-suite/internal/api/templates"
	"clinic-admin-suite/internal/config"
	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

var parsedTemplates = htmltpl.Must(htmltpl.ParseFS(templates.Files, "*.gohtml"))

func renderTemplate(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type AppPagesHandler struct {
	config      config.Config
	auth        *service.AuthService
	admissions  *service.AdmissionsService
	exercises   *service.ExerciseService
	favorites   *service.ExerciseFavoriteService
	care        *service.CareService
	templates   *service.ExamTemplateService
	payments    *service.PaymentService
	settlements *service.SettlementService
	reports     *service.ReportService
}

func NewAppPagesHandler(cfg config.Config, auth *service.AuthService, admissions *service.AdmissionsService, exercises *service.ExerciseService, favorites *service.ExerciseFavoriteService, care *service.CareService, templates *service.ExamTemplateService, payments *service.PaymentService, settlements *service.SettlementService, reports *service.ReportService) *AppPagesHandler {
	return &AppPagesHandler{
		config:      cfg,
		auth:        auth,
		admissions:  admissions,
		exercises:   exercises,
		favorites:   favorites,
		care:        care,
		templates:   templates,
		payments:    payments,
		settlements: settlements,
		reports:     reports,
	}
}

func (h *AppPagesHandler) LoginPage(c *fiber.Ctx) error {
	c.Type("html", "utf-8")
	result, err := renderTemplate("login.gohtml", nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("template error")
	}
	return c.SendString(result)
}

func (h *AppPagesHandler) LoginSubmit(c *fiber.Ctx) error {
	username := strings.TrimSpace(c.FormValue("username"))
	password := c.FormValue("password")
	result, err := h.auth.Login(c.UserContext(), service.LoginInput{
		Username:  username,
		Password:  password,
		RequestID: c.Get("X-Request-ID"),
		IP:        c.IP(),
		UserAgent: c.Get("User-Agent"),
	})
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("invalid credentials")
	}

	c.Cookie(&fiber.Cookie{Name: h.config.SessionCookieName, Value: result.Token, Path: "/", Expires: result.ExpiresAt, HTTPOnly: true, Secure: h.config.CookieSecure, SameSite: "Strict"})
	return c.Redirect("/app", fiber.StatusSeeOther)
}

func (h *AppPagesHandler) Logout(c *fiber.Ctx) error {
	rawToken := strings.TrimSpace(c.Cookies(h.config.SessionCookieName))
	if rawToken != "" {
		_ = h.auth.Logout(c.UserContext(), rawToken)
	}
	c.Cookie(&fiber.Cookie{Name: h.config.SessionCookieName, Value: "", Path: "/", Expires: time.Unix(0, 0), HTTPOnly: true, Secure: h.config.CookieSecure, SameSite: "Strict"})
	return c.Redirect("/login", fiber.StatusSeeOther)
}

func (h *AppPagesHandler) AppShell(c *fiber.Ctx) error {
	rawToken := strings.TrimSpace(c.Cookies(h.config.SessionCookieName))
	if rawToken == "" {
		return c.Redirect("/login", fiber.StatusSeeOther)
	}
	user, _, err := h.auth.AuthenticateToken(c.UserContext(), rawToken)
	if err != nil || user == nil {
		return c.Redirect("/login", fiber.StatusSeeOther)
	}
	username := user.Username

	result, err2 := renderTemplate("appshell.gohtml", struct{ Username string }{Username: username})
	if err2 != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("template error")
	}
	c.Type("html", "utf-8")
	return c.SendString(result)
}

func (h *AppPagesHandler) PanelOverview(c *fiber.Ctx) error {
	ops, _ := h.reports.OpsSummary(c.UserContext())
	careSummary, _ := h.care.Dashboard(c.UserContext())
	if ops == nil {
		ops = &service.OpsSummary{}
	}
	if careSummary == nil {
		careSummary = &domain.CareDashboardSummary{}
	}

	result, err := renderTemplate("panel_overview.gohtml", struct {
		Ops  *service.OpsSummary
		Care *domain.CareDashboardSummary
	}{Ops: ops, Care: careSummary})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("template error")
	}
	return c.SendString(result)
}

func (h *AppPagesHandler) PanelOccupancy(c *fiber.Ctx) error {
	items, err := h.admissions.OccupancyBoard(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("<div class='card'>Failed to load occupancy board</div>")
	}
	result, err := renderTemplate("panel_occupancy.gohtml", struct {
		Items []domain.BedOccupancy
	}{Items: items})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("template error")
	}
	return c.SendString(result)
}

type exerciseView struct {
	ID         int64
	Title      string
	Difficulty string
	Favored    bool
}

func (h *AppPagesHandler) PanelExercises(c *fiber.Ctx) error {
	authCtx, _ := middleware.CurrentAuth(c)
	items, err := h.exercises.List(c.UserContext(), repository.ExerciseFilter{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("<div class='card'>Failed to load exercises</div>")
	}
	favorites := map[int64]struct{}{}
	if authCtx != nil && authCtx.User != nil {
		favorites, _ = h.favorites.ListIDs(c.UserContext(), authCtx.User.ID)
	}
	views := make([]exerciseView, len(items))
	for i, item := range items {
		_, favored := favorites[item.ID]
		views[i] = exerciseView{ID: item.ID, Title: item.Title, Difficulty: item.Difficulty, Favored: favored}
	}
	result, err := renderTemplate("panel_exercises.gohtml", struct {
		Items []exerciseView
	}{Items: views})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("template error")
	}
	return c.SendString(result)
}

func (h *AppPagesHandler) ToggleFavorite(c *fiber.Ctx) error {
	authCtx, ok := middleware.CurrentAuth(c)
	if !ok || authCtx.User == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("authentication required")
	}
	exerciseID, err := strconv.ParseInt(c.Params("exercise_id"), 10, 64)
	if err != nil || exerciseID <= 0 {
		return c.Status(fiber.StatusUnprocessableEntity).SendString("invalid exercise_id")
	}
	_, err = h.favorites.Toggle(c.UserContext(), authCtx.User.ID, exerciseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to toggle favorite")
	}
	return h.PanelExercises(c)
}

func (h *AppPagesHandler) PanelCare(c *fiber.Ctx) error {
	checkpoints, _ := h.care.ListCheckpoints(c.UserContext(), service.CareCheckpointFilter{})
	alerts, _ := h.care.ListAlerts(c.UserContext(), service.AlertEventFilter{})

	htmlContent, err := renderTemplate("panel_care.gohtml", map[string]any{
		"Checkpoints": checkpoints,
		"Alerts":      alerts,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendString(htmlContent)
}

func (h *AppPagesHandler) CreateCheckpoint(c *fiber.Ctx) error {
	residentID, _ := strconv.ParseInt(strings.TrimSpace(c.FormValue("resident_id")), 10, 64)
	_, err := h.care.CreateCheckpoint(c.UserContext(), service.CreateCheckpointInput{
		ResidentID:     residentID,
		CheckpointType: c.FormValue("checkpoint_type"),
		Status:         c.FormValue("status"),
		Notes:          c.FormValue("notes"),
		ActorID:        currentActorIDFromContext(c),
		RequestID:      c.Get("X-Request-ID"),
	})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelCare(c)
}

func (h *AppPagesHandler) CreateAlert(c *fiber.Ctx) error {
	residentID, _ := strconv.ParseInt(strings.TrimSpace(c.FormValue("resident_id")), 10, 64)
	_, err := h.care.CreateAlert(c.UserContext(), service.CreateAlertInput{
		ResidentID: residentID,
		AlertType:  c.FormValue("alert_type"),
		Severity:   c.FormValue("severity"),
		State:      c.FormValue("state"),
		Message:    c.FormValue("message"),
		ActorID:    currentActorIDFromContext(c),
		RequestID:  c.Get("X-Request-ID"),
	})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelCare(c)
}

func (h *AppPagesHandler) PanelScheduling(c *fiber.Ctx) error {
	templates, _ := h.templates.ListTemplates(c.UserContext())
	drafts, _ := h.templates.ListDrafts(c.UserContext(), nil)

	sort.Slice(drafts, func(i, j int) bool { return drafts[i].CreatedAt.After(drafts[j].CreatedAt) })

	var minT, maxT time.Time
	for i, item := range drafts {
		if i == 0 || item.StartAt.Before(minT) {
			minT = item.StartAt
		}
		if i == 0 || item.EndAt.After(maxT) {
			maxT = item.EndAt
		}
	}
	rangeMinutes := maxT.Sub(minT).Minutes()
	if rangeMinutes < 60 {
		rangeMinutes = 60
	}

	type timelineDraft struct {
		domain.ExamSessionDraft
		LeftOffset float64
		Width      float64
		Conflicts  string
	}

	gridWidth := 900.0
	viewDrafts := make([]timelineDraft, len(drafts))
	for i, item := range drafts {
		left := (item.StartAt.Sub(minT).Minutes() / rangeMinutes) * gridWidth
		width := (item.EndAt.Sub(item.StartAt).Minutes() / rangeMinutes) * gridWidth
		if width < 40 {
			width = 40
		}
		viewDrafts[i] = timelineDraft{
			ExamSessionDraft: item,
			LeftOffset:       left,
			Width:            width,
			Conflicts:        fmt.Sprintf("%v", item.Conflicts),
		}
	}

	htmlContent, err := renderTemplate("panel_scheduling.gohtml", map[string]any{
		"Templates":    templates,
		"Drafts":       viewDrafts,
		"MinTime":      minT,
		"MaxTime":      maxT,
		"RangeMinutes": rangeMinutes,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendString(htmlContent)
}

func (h *AppPagesHandler) CreateTemplate(c *fiber.Ctx) error {
	roomID, _ := strconv.ParseInt(c.FormValue("room_id"), 10, 64)
	proctorID, _ := strconv.ParseInt(c.FormValue("proctor_id"), 10, 64)
	duration, _ := strconv.Atoi(c.FormValue("duration_minutes"))
	startAt := parseDateTimeInput(c.FormValue("window_start_at"))
	endAt := parseDateTimeInput(c.FormValue("window_end_at"))

	_, err := h.templates.CreateTemplate(c.UserContext(), service.CreateTemplateInput{
		Title:           c.FormValue("title"),
		Subject:         c.FormValue("subject"),
		DurationMinutes: duration,
		RoomID:          roomID,
		ProctorID:       proctorID,
		CandidateIDs:    parseIDCSV(c.FormValue("candidate_ids")),
		WindowLabel:     c.FormValue("window_label"),
		WindowStartAt:   startAt,
		WindowEndAt:     endAt,
		ActorID:         currentActorIDFromContext(c),
		RequestID:       c.Get("X-Request-ID"),
	})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelScheduling(c)
}

func (h *AppPagesHandler) GenerateDraft(c *fiber.Ctx) error {
	templateID, _ := strconv.ParseInt(c.FormValue("template_id"), 10, 64)
	windowID, _ := strconv.ParseInt(c.FormValue("window_id"), 10, 64)
	var startAt *time.Time
	if strings.TrimSpace(c.FormValue("start_at")) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(c.FormValue("start_at")))
		if err == nil {
			v := parsed.UTC()
			startAt = &v
		}
	}
	_, err := h.templates.GenerateDraft(c.UserContext(), service.GenerateDraftInput{TemplateID: templateID, WindowID: windowID, StartAt: startAt, ActorID: currentActorIDFromContext(c), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelScheduling(c)
}

func (h *AppPagesHandler) AdjustDraft(c *fiber.Ctx) error {
	draftID, _ := strconv.ParseInt(c.Params("draft_id"), 10, 64)
	startAt := parseDateTimeInput(c.FormValue("start_at"))
	endAt := parseDateTimeInput(c.FormValue("end_at"))
	_, err := h.templates.AdjustDraft(c.UserContext(), service.AdjustDraftInput{DraftID: draftID, StartAt: startAt, EndAt: endAt, ActorID: currentActorIDFromContext(c), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelScheduling(c)
}

func (h *AppPagesHandler) PublishDraft(c *fiber.Ctx) error {
	draftID, _ := strconv.ParseInt(c.Params("draft_id"), 10, 64)
	actorID := currentActorIDFromContext(c)
	if actorID == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("unauthorized")
	}
	_, err := h.templates.PublishDraft(c.UserContext(), service.PublishDraftInput{DraftID: draftID, ActorID: *actorID, IdempotencyKey: strings.TrimSpace(c.Get("Idempotency-Key")), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelScheduling(c)
}

func (h *AppPagesHandler) PanelFinance(c *fiber.Ctx) error {
	payments, _ := h.payments.List(c.UserContext(), repository.PaymentFilter{})
	if len(payments) > 20 {
		payments = payments[:20]
	}

	htmlContent, err := renderTemplate("panel_finance.gohtml", map[string]any{
		"Payments": payments,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendString(htmlContent)
}

func (h *AppPagesHandler) CreatePayment(c *fiber.Ctx) error {
	amount, _ := strconv.ParseInt(c.FormValue("amount_cents"), 10, 64)
	_, err := h.payments.Create(c.UserContext(), service.CreatePaymentInput{Method: c.FormValue("method"), Gateway: c.FormValue("gateway"), AmountCents: amount, Currency: c.FormValue("currency"), ShiftID: c.FormValue("shift_id"), ActorID: currentActorIDFromContext(c), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelFinance(c)
}

func (h *AppPagesHandler) RefundPayment(c *fiber.Ctx) error {
	paymentID, _ := strconv.ParseInt(c.FormValue("payment_id"), 10, 64)
	amount, _ := strconv.ParseInt(c.FormValue("amount_cents"), 10, 64)
	_, err := h.payments.Refund(c.UserContext(), service.RefundPaymentInput{PaymentID: paymentID, AmountCents: amount, Reason: c.FormValue("reason"), ActorID: currentActorIDFromContext(c), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelFinance(c)
}

func (h *AppPagesHandler) RunSettlement(c *fiber.Ctx) error {
	actual, _ := strconv.ParseInt(c.FormValue("actual_total_cents"), 10, 64)
	_, err := h.settlements.RunShift(c.UserContext(), service.RunSettlementInput{ShiftID: c.FormValue("shift_id"), ActualTotalCents: actual, ActorID: currentActorIDFromContext(c), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelFinance(c)
}

func (h *AppPagesHandler) PanelReports(c *fiber.Ctx) error {
	auditRows, _ := h.reports.SearchAudit(c.UserContext(), service.AuditSearchFilter{Limit: 30})
	schedules, _ := h.reports.ListSchedules(c.UserContext())
	configVersions, _ := h.reports.ListConfigVersions(c.UserContext(), "")

	htmlContent, err := renderTemplate("panel_reports.gohtml", map[string]any{
		"AuditRows":         auditRows,
		"Schedules":         schedules,
		"ConfigVersions":    configVersions,
		"ReportsSharedRoot": h.config.ReportsSharedRoot,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendString(htmlContent)
}

func (h *AppPagesHandler) AuditResults(c *fiber.Ctx) error {
	filter := service.AuditSearchFilter{Limit: 100}
	if v, ok := parseOptionalInt64(c.Query("resident_id")); ok {
		filter.ResidentID = &v
	}
	filter.RecordType = strings.TrimSpace(c.Query("record_type"))
	if from, ok, _ := parseOptionalDateTime(c.Query("from")); ok {
		filter.From = &from
	}
	if to, ok, _ := parseOptionalDateTime(c.Query("to")); ok {
		filter.To = &to
	}
	items, err := h.reports.SearchAudit(c.UserContext(), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("search failed")
	}
	var b strings.Builder
	b.WriteString(`<pre class="output">`)
	for _, item := range items {
		b.WriteString(fmt.Sprintf("%s | %s | %s | %s | %s\n", item.OccurredAt.Format(time.RFC3339), item.ResourceType, item.ResourceID, item.OperatorName, item.LocalIP))
	}
	b.WriteString(`</pre>`)
	return c.SendString(b.String())
}

func (h *AppPagesHandler) CreateReportSchedule(c *fiber.Ctx) error {
	interval, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("interval_minutes")))
	firstRun := time.Now().UTC().Add(5 * time.Minute)
	if v := strings.TrimSpace(c.FormValue("first_run_at")); v != "" {
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			firstRun = parsed.UTC()
		}
	}
	_, err := h.reports.CreateSchedule(c.UserContext(), service.CreateReportScheduleInput{ReportType: c.FormValue("report_type"), Format: c.FormValue("format"), SharedFolder: c.FormValue("shared_folder_path"), IntervalMinutes: interval, FirstRunAt: firstRun, ActorID: currentActorIDFromContext(c), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelReports(c)
}

func (h *AppPagesHandler) RunReportSchedulesNow(c *fiber.Ctx) error {
	if err := h.reports.RunDueSchedules(c.UserContext(), time.Now().UTC().Add(24*time.Hour)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelReports(c)
}

func (h *AppPagesHandler) CreateConfigVersion(c *fiber.Ctx) error {
	_, err := h.reports.CreateConfigVersion(c.UserContext(), service.CreateConfigVersionInput{ConfigKey: c.FormValue("config_key"), PayloadJSON: c.FormValue("payload_json"), ActorID: currentActorIDFromContext(c), RequestID: c.Get("X-Request-ID")})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelReports(c)
}

func (h *AppPagesHandler) RollbackConfigVersion(c *fiber.Ctx) error {
	versionID, _ := strconv.ParseInt(c.Params("version_id"), 10, 64)
	_, err := h.reports.RollbackConfigVersion(c.UserContext(), versionID, currentActorIDFromContext(c), c.Get("X-Request-ID"))
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
	return h.PanelReports(c)
}

func parseDateTimeInput(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02T15:04", raw); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02T15:04:05", raw); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func parseIDCSV(raw string) []int64 {
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && v > 0 {
			out = append(out, v)
		}
	}
	return out
}
