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

	var b strings.Builder
	b.WriteString(`<section class="view active"><div class="grid two"><article class="card"><h3>Record Care Quality Checkpoint</h3><form class="stack-form" hx-post="/ui/care/checkpoints" hx-target="#panel"><label>Resident ID<input name="resident_id" type="number" required></label><label>Checkpoint Type<input name="checkpoint_type" required></label><label>Status<select name="status"><option value="pass">pass</option><option value="watch">watch</option><option value="fail">fail</option></select></label><label>Notes<input name="notes"></label><button type="submit">Save Checkpoint</button></form></article><article class="card"><h3>Record Alert Event</h3><form class="stack-form" hx-post="/ui/care/alerts" hx-target="#panel"><label>Resident ID<input name="resident_id" type="number" required></label><label>Alert Type<input name="alert_type" required></label><label>Severity<select name="severity"><option>low</option><option>medium</option><option>high</option><option>critical</option></select></label><label>State<select name="state"><option>open</option><option>acknowledged</option><option>resolved</option></select></label><label>Message<input name="message" required></label><button type="submit">Save Alert</button></form></article></div>`)

	b.WriteString(`<article class="card"><h3>Checkpoint Log</h3><div class="stack">`)
	if len(checkpoints) == 0 {
		b.WriteString(`<p class="hint">No checkpoints found.</p>`)
	}
	for _, item := range checkpoints {
		b.WriteString(`<div class="exercise-row"><div><strong>Resident ` + strconv.FormatInt(item.ResidentID, 10) + `</strong><div>` + html.EscapeString(item.CheckpointType) + ` / ` + html.EscapeString(item.Status) + `</div></div><div>` + item.CreatedAt.Format(time.RFC3339) + `</div></div>`)
	}
	b.WriteString(`</div></article><article class="card"><h3>Alert Events</h3><div class="stack">`)
	if len(alerts) == 0 {
		b.WriteString(`<p class="hint">No alerts found.</p>`)
	}
	for _, item := range alerts {
		b.WriteString(`<div class="exercise-row"><div><strong>Resident ` + strconv.FormatInt(item.ResidentID, 10) + `</strong><div>` + html.EscapeString(item.AlertType) + ` / ` + html.EscapeString(item.Severity) + ` / ` + html.EscapeString(item.State) + `</div></div><div>` + html.EscapeString(item.Message) + `</div></div>`)
	}
	b.WriteString(`</div></article></section>`)
	return c.SendString(b.String())
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

	var b strings.Builder
	b.WriteString(`<section class="view active"><div class="grid two"><article class="card"><h3>Create Exam Template</h3><form class="stack-form" hx-post="/ui/scheduling/templates" hx-target="#panel"><label>Title<input name="title" required></label><label>Subject<input name="subject" required></label><label>Duration Minutes<input type="number" name="duration_minutes" value="90" required></label><label>Room ID<input type="number" name="room_id" required></label><label>Proctor ID<input type="number" name="proctor_id" required></label><label>Candidate IDs<input name="candidate_ids" placeholder="1001,1002" required></label><label>Window Label<input name="window_label" value="Morning"></label><label>Window Start<input type="datetime-local" name="window_start_at" required></label><label>Window End<input type="datetime-local" name="window_end_at" required></label><button type="submit">Save Template</button></form></article><article class="card"><h3>Generate Session Draft</h3><form class="stack-form" hx-post="/ui/scheduling/drafts/generate" hx-target="#panel"><label>Template ID<input type="number" name="template_id" required></label><label>Window ID<input type="number" name="window_id" required></label><label>Start At (optional RFC3339)<input name="start_at"></label><button type="submit">Generate Draft</button></form></article></div>`)

	b.WriteString(`<article class="card"><h3>Exam Templates</h3><div class="stack">`)
	if len(templates) == 0 {
		b.WriteString(`<p class="hint">No templates configured.</p>`)
	}
	for _, item := range templates {
		b.WriteString(`<div class="exercise-row"><div><strong>#` + strconv.FormatInt(item.ID, 10) + ` ` + html.EscapeString(item.Title) + `</strong><div>` + html.EscapeString(item.Subject) + ` (` + strconv.Itoa(item.DurationMinutes) + ` min)</div></div><div>room ` + strconv.FormatInt(item.RoomID, 10) + ` / proctor ` + strconv.FormatInt(item.ProctorID, 10) + `</div></div>`)
		for _, w := range item.Windows {
			b.WriteString(`<div class="hint">window #` + strconv.FormatInt(w.ID, 10) + ` ` + html.EscapeString(w.Label) + `: ` + w.WindowStartAt.Format("01/02/2006 15:04") + `-` + w.WindowEndAt.Format("15:04") + `</div>`)
		}
	}
	b.WriteString(`</div></article>`)

	// Visual timeline component
	b.WriteString(`<article class="card"><h3>Visual Timeline (Drag to Adjust)</h3>`)
	if len(drafts) > 0 {
		b.WriteString(`<div id="timeline-grid" style="position:relative;border:1px solid #ccc;border-radius:4px;min-height:80px;margin:8px 0;overflow-x:auto;background:repeating-linear-gradient(90deg,#f0f0f0,#f0f0f0 1px,transparent 1px,transparent 60px)">`)
		// Find time range
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
		gridWidth := 900.0
		for _, item := range drafts {
			if item.Status == "published" {
				continue
			}
			left := (item.StartAt.Sub(minT).Minutes() / rangeMinutes) * gridWidth
			width := (item.EndAt.Sub(item.StartAt).Minutes() / rangeMinutes) * gridWidth
			if width < 40 {
				width = 40
			}
			b.WriteString(`<div class="timeline-block" draggable="true" data-draft-id="` + strconv.FormatInt(item.ID, 10) + `" data-start="` + item.StartAt.Format(time.RFC3339) + `" data-end="` + item.EndAt.Format(time.RFC3339) + `" style="position:absolute;left:` + fmt.Sprintf("%.0f", left) + `px;top:10px;width:` + fmt.Sprintf("%.0f", width) + `px;height:50px;background:#4a90d9;color:#fff;border-radius:4px;cursor:grab;display:flex;align-items:center;justify-content:center;font-size:12px;user-select:none" title="Drag to adjust">Draft #` + strconv.FormatInt(item.ID, 10) + `</div>`)
		}
		b.WriteString(`</div>`)
		// Hour labels
		b.WriteString(`<div style="display:flex;justify-content:space-between;font-size:11px;color:#666">`)
		b.WriteString(`<span>` + minT.Format("15:04") + `</span>`)
		b.WriteString(`<span>` + maxT.Format("15:04") + `</span>`)
		b.WriteString(`</div>`)
		b.WriteString(`<script>
(function(){
  var grid = document.getElementById('timeline-grid');
  if(!grid) return;
  var blocks = grid.querySelectorAll('.timeline-block');
  var gridWidth = ` + fmt.Sprintf("%.0f", gridWidth) + `;
  var rangeMs = ` + fmt.Sprintf("%.0f", rangeMinutes*60*1000) + `;
  var minTime = new Date('` + minT.Format(time.RFC3339) + `').getTime();
  var dragBlock = null, dragStartX = 0, origLeft = 0;
  blocks.forEach(function(block){
    block.addEventListener('mousedown', function(e){
      dragBlock = block; dragStartX = e.clientX; origLeft = parseInt(block.style.left)||0; e.preventDefault();
    });
  });
  document.addEventListener('mousemove', function(e){
    if(!dragBlock) return;
    var dx = e.clientX - dragStartX;
    dragBlock.style.left = Math.max(0, Math.min(gridWidth - parseInt(dragBlock.style.width), origLeft + dx)) + 'px';
  });
  document.addEventListener('mouseup', function(){
    if(!dragBlock) return;
    var block = dragBlock; dragBlock = null;
    var newLeft = parseInt(block.style.left)||0;
    var blockWidth = parseInt(block.style.width)||0;
    var startMs = minTime + (newLeft / gridWidth) * rangeMs;
    var endMs = minTime + ((newLeft + blockWidth) / gridWidth) * rangeMs;
    var startISO = new Date(startMs).toISOString();
    var endISO = new Date(endMs).toISOString();
    var draftId = block.getAttribute('data-draft-id');
    var form = new FormData();
    form.append('start_at', startISO);
    form.append('end_at', endISO);
    var headers = {'Idempotency-Key': 'drag:' + draftId + ':' + Date.now()};
    fetch('/ui/scheduling/drafts/' + draftId + '/adjust', {method:'POST', body:form, credentials:'include', headers:headers})
      .then(function(r){ return r.text(); })
      .then(function(html){ document.getElementById('panel').innerHTML = html; });
  });
})();
</script>`)
	} else {
		b.WriteString(`<p class="hint">No drafts to display on timeline.</p>`)
	}
	b.WriteString(`</article>`)

	b.WriteString(`<article class="card"><h3>Session Drafts (Adjust Before Publish)</h3><div class="stack">`)
	if len(drafts) == 0 {
		b.WriteString(`<p class="hint">No session drafts yet.</p>`)
	}
	for _, item := range drafts {
		b.WriteString(`<div class="exercise-row"><div><strong>Draft #` + strconv.FormatInt(item.ID, 10) + `</strong><div>` + item.StartAt.Format(time.RFC3339) + ` to ` + item.EndAt.Format(time.RFC3339) + `</div></div><div>Status: ` + html.EscapeString(item.Status) + `</div></div>`)
		if len(item.Conflicts) > 0 {
			b.WriteString(`<pre class="output">conflicts: ` + html.EscapeString(fmt.Sprintf("%v", item.Conflicts)) + `</pre>`)
		}
		if item.Status != "published" {
			b.WriteString(`<form class="stack-form" hx-post="/ui/scheduling/drafts/` + strconv.FormatInt(item.ID, 10) + `/adjust" hx-target="#panel"><label>Start At<input type="datetime-local" name="start_at" value="` + item.StartAt.Format("2006-01-02T15:04") + `"></label><label>End At<input type="datetime-local" name="end_at" value="` + item.EndAt.Format("2006-01-02T15:04") + `"></label><button type="submit">Adjust Time Block</button></form><button hx-post="/ui/scheduling/drafts/` + strconv.FormatInt(item.ID, 10) + `/publish" hx-target="#panel">Publish Session</button>`)
		}
	}
	b.WriteString(`</div></article></section>`)
	return c.SendString(b.String())
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
	var b strings.Builder
	b.WriteString(`<section class="view active"><div class="grid two"><article class="card"><h3>Create Payment</h3><form class="stack-form" hx-post="/ui/finance/payments" hx-target="#panel"><label>Method<input name="method" value="cash"></label><label>Gateway<select name="gateway"><option>cash_local</option><option>check_local</option><option>facility_charge_local</option><option>imported_card_batch_local</option><option>card_local</option></select></label><label>Amount Cents<input type="number" name="amount_cents" required></label><label>Currency<input name="currency" value="USD"></label><label>Shift ID<select name="shift_id"><option value="shift-0700">07:00 close</option><option value="shift-1500">15:00 close</option><option value="shift-2300">23:00 close</option></select></label><button type="submit">Capture Payment</button></form></article><article class="card"><h3>Issue Refund</h3><form class="stack-form" hx-post="/ui/finance/refunds" hx-target="#panel"><label>Payment ID<input type="number" name="payment_id" required></label><label>Amount Cents<input type="number" name="amount_cents" required></label><label>Reason<input name="reason" value="patient_adjustment" required></label><button type="submit">Submit Refund</button></form></article></div><div class="grid two"><article class="card"><h3>Run Settlement</h3><form class="stack-form" hx-post="/ui/finance/settlements" hx-target="#panel"><label>Shift ID<select name="shift_id"><option value="shift-0700">07:00 close</option><option value="shift-1500">15:00 close</option><option value="shift-2300">23:00 close</option></select></label><label>Actual Total Cents<input type="number" name="actual_total_cents" required></label><button type="submit">Run</button></form></article><article class="card"><h3>On-Demand Export</h3><div class="row"><a href="/api/v1/reports/finance/export?format=csv">Finance CSV</a><a href="/api/v1/reports/finance/export?format=xlsx">Finance XLSX</a></div></article></div><article class="card"><h3>Recent Payments</h3><pre class="output">`)
	for _, item := range payments {
		b.WriteString(fmt.Sprintf("id=%d status=%s method=%s gateway=%s amount=%d shift=%s\n", item.ID, item.Status, item.Method, item.Gateway, item.AmountCents, item.ShiftID))
	}
	b.WriteString(`</pre></article></section>`)
	return c.SendString(b.String())
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

	var b strings.Builder
	b.WriteString(`<section class="view active"><div class="grid two"><article class="card"><h3>Audit Search</h3><form class="stack-form" hx-get="/ui/reports/audit-results" hx-target="#audit-results"><label>Resident ID<input name="resident_id"></label><label>Record Type<input name="record_type"></label><label>From (YYYY-MM-DD)<input name="from"></label><label>To (YYYY-MM-DD)<input name="to"></label><button type="submit">Search</button></form><div id="audit-results"><pre class="output">`)
	for _, item := range auditRows {
		b.WriteString(fmt.Sprintf("%s %s %s %s %s\n", item.OccurredAt.Format(time.RFC3339), item.ResourceType, item.ResourceID, item.OperatorName, item.LocalIP))
	}
	b.WriteString(`</pre></div><div class="row"><a href="/api/v1/reports/audit/export?format=csv">Audit CSV</a><a href="/api/v1/reports/audit/export?format=xlsx">Audit XLSX</a></div></article><article class="card"><h3>Schedule Local Reports</h3><form class="stack-form" hx-post="/ui/reports/schedules" hx-target="#panel"><label>Report Type<select name="report_type"><option value="audit">audit</option><option value="finance">finance</option></select></label><label>Format<select name="format"><option>csv</option><option>xlsx</option></select></label><label>Shared Folder<input name="shared_folder_path" placeholder="` + html.EscapeString(h.config.ReportsSharedRoot) + `"></label><label>Interval Minutes<input type="number" name="interval_minutes" value="60"></label><label>First Run At (RFC3339)<input name="first_run_at"></label><button type="submit">Save Schedule</button></form><button hx-post="/ui/reports/schedules/run-now" hx-target="#panel">Run Schedules Now</button><pre class="output">`)
	for _, item := range schedules {
		b.WriteString(fmt.Sprintf("id=%d type=%s format=%s next=%s path=%s\n", item.ID, item.ReportType, item.Format, item.NextRunAt.Format(time.RFC3339), item.SharedFolderPath))
	}
	b.WriteString(`</pre></article></div><article class="card"><h3>Config Versioning and Rollback</h3><form class="stack-form" hx-post="/ui/config/versions" hx-target="#panel"><label>Config Key<input name="config_key" value="reporting"></label><label>Payload JSON<textarea name="payload_json" rows="4">{"shared_root":"` + html.EscapeString(h.config.ReportsSharedRoot) + `"}</textarea></label><button type="submit">Create Version</button></form><div class="stack">`)
	for _, item := range configVersions {
		b.WriteString(`<div class="exercise-row"><div><strong>#` + strconv.FormatInt(item.ID, 10) + `</strong><div>` + html.EscapeString(item.ConfigKey) + ` active=` + strconv.FormatBool(item.IsActive) + `</div></div><button hx-post="/ui/config/versions/` + strconv.FormatInt(item.ID, 10) + `/rollback" hx-target="#panel">Rollback</button></div>`)
	}
	b.WriteString(`</div></article></section>`)
	return c.SendString(b.String())
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
