package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type ExamTemplateService struct {
	db         *sql.DB
	schedules  repository.ExamScheduleRepository
	scheduling *SchedulingService
	audit      *AuditService
}

func NewExamTemplateService(db *sql.DB, schedules repository.ExamScheduleRepository, scheduling *SchedulingService, audit *AuditService) *ExamTemplateService {
	return &ExamTemplateService{db: db, schedules: schedules, scheduling: scheduling, audit: audit}
}

type CreateTemplateInput struct {
	Title           string
	Subject         string
	DurationMinutes int
	RoomID          int64
	ProctorID       int64
	CandidateIDs    []int64
	WindowLabel     string
	WindowStartAt   time.Time
	WindowEndAt     time.Time
	ActorID         *int64
	RequestID       string
}

func (s *ExamTemplateService) CreateTemplate(ctx context.Context, input CreateTemplateInput) (*domain.ExamTemplate, error) {
	title := strings.TrimSpace(input.Title)
	subject := strings.TrimSpace(input.Subject)
	if title == "" || subject == "" {
		return nil, fmt.Errorf("%w: title and subject are required", ErrValidation)
	}
	if input.DurationMinutes < 15 {
		return nil, fmt.Errorf("%w: duration_minutes must be at least 15", ErrValidation)
	}
	if input.RoomID <= 0 || input.ProctorID <= 0 {
		return nil, fmt.Errorf("%w: room_id and proctor_id must be positive", ErrValidation)
	}
	candidates := normalizeCandidateIDs(input.CandidateIDs)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: at least one candidate is required", ErrValidation)
	}
	start := input.WindowStartAt.UTC()
	end := input.WindowEndAt.UTC()
	if !end.After(start) {
		return nil, fmt.Errorf("%w: window_end_at must be after window_start_at", ErrValidation)
	}
	if start.Add(time.Duration(input.DurationMinutes) * time.Minute).After(end) {
		return nil, fmt.Errorf("%w: duration does not fit in allowed window", ErrValidation)
	}

	now := time.Now().UTC()
	candidateJSON, _ := json.Marshal(candidates)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin exam template tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
INSERT INTO exam_templates(title, subject, duration_minutes, room_id, proctor_id, candidate_ids_json, created_by, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, title, subject, input.DurationMinutes, input.RoomID, input.ProctorID, string(candidateJSON), nullableInt64(input.ActorID), now.Unix(), now.Unix())
	if err != nil {
		return nil, fmt.Errorf("create exam template: %w", err)
	}
	templateID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create exam template last insert id: %w", err)
	}

	windowResult, err := tx.ExecContext(ctx, `
INSERT INTO exam_template_windows(template_id, label, window_start_at, window_end_at, created_at)
VALUES(?, ?, ?, ?, ?)`, templateID, strings.TrimSpace(input.WindowLabel), start.Unix(), end.Unix(), now.Unix())
	if err != nil {
		return nil, fmt.Errorf("create exam template window: %w", err)
	}
	windowID, _ := windowResult.LastInsertId()

	if s.audit != nil {
		_ = s.audit.LogEventTx(ctx, tx, AuditLogInput{
			ActorID:      input.ActorID,
			Action:       "exam_template.create",
			ResourceType: "exam_template",
			ResourceID:   fmt.Sprintf("%d", templateID),
			After: map[string]any{
				"template_id":        templateID,
				"title":              title,
				"subject":            subject,
				"duration_minutes":   input.DurationMinutes,
				"window_id":          windowID,
				"window_start_at":    start.Format(time.RFC3339),
				"window_end_at":      end.Format(time.RFC3339),
				"candidate_ids":      candidates,
				"allowable_time_box": start.Format("01/02/2006 15:04") + "-" + end.Format("15:04"),
			},
			RequestID: input.RequestID,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit exam template tx: %w", err)
	}

	item := &domain.ExamTemplate{
		ID:              templateID,
		Title:           title,
		Subject:         subject,
		DurationMinutes: input.DurationMinutes,
		RoomID:          input.RoomID,
		ProctorID:       input.ProctorID,
		CandidateIDs:    candidates,
		Windows: []domain.ExamTemplateWindow{{
			ID:            windowID,
			TemplateID:    templateID,
			Label:         strings.TrimSpace(input.WindowLabel),
			WindowStartAt: start,
			WindowEndAt:   end,
			CreatedAt:     now,
		}},
		CreatedBy: input.ActorID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return item, nil
}

func (s *ExamTemplateService) ListTemplates(ctx context.Context) ([]domain.ExamTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, subject, duration_minutes, room_id, proctor_id, candidate_ids_json, created_by, created_at, updated_at
FROM exam_templates
ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list exam templates: %w", err)
	}
	defer rows.Close()

	templateItems := make([]domain.ExamTemplate, 0)
	for rows.Next() {
		var (
			item      domain.ExamTemplate
			candidate string
			createdBy sql.NullInt64
			createdAt int64
			updatedAt int64
		)
		if err := rows.Scan(&item.ID, &item.Title, &item.Subject, &item.DurationMinutes, &item.RoomID, &item.ProctorID, &candidate, &createdBy, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan exam template row: %w", err)
		}
		item.CandidateIDs = parseCandidateJSON(candidate)
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		if createdBy.Valid {
			v := createdBy.Int64
			item.CreatedBy = &v
		}
		templateItems = append(templateItems, item)
	}
	rows.Close() // Release the connection immediately

	for i := range templateItems {
		windows, err := s.listTemplateWindows(ctx, templateItems[i].ID)
		if err != nil {
			return nil, err
		}
		templateItems[i].Windows = windows
	}

	return templateItems, nil
}

func (s *ExamTemplateService) listTemplateWindows(ctx context.Context, templateID int64) ([]domain.ExamTemplateWindow, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, template_id, label, window_start_at, window_end_at, created_at
FROM exam_template_windows
WHERE template_id = ?
ORDER BY window_start_at ASC`, templateID)
	if err != nil {
		return nil, fmt.Errorf("list template windows: %w", err)
	}
	defer rows.Close()

	out := make([]domain.ExamTemplateWindow, 0)
	for rows.Next() {
		var (
			item      domain.ExamTemplateWindow
			startUnix int64
			endUnix   int64
			createdAt int64
		)
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.Label, &startUnix, &endUnix, &createdAt); err != nil {
			return nil, fmt.Errorf("scan template window row: %w", err)
		}
		item.WindowStartAt = time.Unix(startUnix, 0).UTC()
		item.WindowEndAt = time.Unix(endUnix, 0).UTC()
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate template windows rows: %w", err)
	}
	return out, nil
}

type GenerateDraftInput struct {
	TemplateID int64
	WindowID   int64
	StartAt    *time.Time
	ActorID    *int64
	RequestID  string
}

func (s *ExamTemplateService) GenerateDraft(ctx context.Context, input GenerateDraftInput) (*domain.ExamSessionDraft, error) {
	if input.TemplateID <= 0 || input.WindowID <= 0 {
		return nil, fmt.Errorf("%w: template_id and window_id must be positive", ErrValidation)
	}

	templateItem, window, err := s.loadTemplateWindow(ctx, input.TemplateID, input.WindowID)
	if err != nil {
		return nil, err
	}

	start := window.WindowStartAt
	if input.StartAt != nil {
		start = input.StartAt.UTC()
	}
	end := start.Add(time.Duration(templateItem.DurationMinutes) * time.Minute)
	if start.Before(window.WindowStartAt) || end.After(window.WindowEndAt) {
		return nil, fmt.Errorf("%w: session must stay inside allowable window %s-%s", ErrValidation, window.WindowStartAt.Format(time.RFC3339), window.WindowEndAt.Format(time.RFC3339))
	}

	conflicts, err := s.schedules.DetectConflicts(ctx, nil, templateItem.RoomID, templateItem.ProctorID, templateItem.CandidateIDs, start, end)
	if err != nil {
		return nil, err
	}
	conflictJSON, _ := json.Marshal(conflicts)
	candidateJSON, _ := json.Marshal(templateItem.CandidateIDs)

	now := time.Now().UTC()
	status := "draft"
	result, err := s.db.ExecContext(ctx, `
INSERT INTO exam_session_drafts(template_id, subject, room_id, proctor_id, candidate_ids_json, start_at, end_at, status, conflict_details_json, created_by, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, input.TemplateID, templateItem.Subject, templateItem.RoomID, templateItem.ProctorID, string(candidateJSON), start.Unix(), end.Unix(), status, string(conflictJSON), nullableInt64(input.ActorID), now.Unix(), now.Unix())
	if err != nil {
		return nil, fmt.Errorf("create exam session draft: %w", err)
	}
	id, _ := result.LastInsertId()

	if s.audit != nil {
		_ = s.audit.LogEvent(ctx, AuditLogInput{
			ActorID:      input.ActorID,
			Action:       "exam_session_draft.generate",
			ResourceType: "exam_session_draft",
			ResourceID:   fmt.Sprintf("%d", id),
			After: map[string]any{
				"template_id": input.TemplateID,
				"start_at":    start.Format(time.RFC3339),
				"end_at":      end.Format(time.RFC3339),
				"conflicts":   conflicts,
			},
			RequestID: input.RequestID,
		})
	}

	draft := &domain.ExamSessionDraft{
		ID:           id,
		TemplateID:   input.TemplateID,
		Subject:      templateItem.Subject,
		RoomID:       templateItem.RoomID,
		ProctorID:    templateItem.ProctorID,
		CandidateIDs: templateItem.CandidateIDs,
		StartAt:      start,
		EndAt:        end,
		Status:       status,
		Conflicts:    conflicts,
		CreatedBy:    input.ActorID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return draft, nil
}

func (s *ExamTemplateService) loadTemplateWindow(ctx context.Context, templateID, windowID int64) (*domain.ExamTemplate, *domain.ExamTemplateWindow, error) {
	var (
		templateItem domain.ExamTemplate
		candidate    string
	)
	err := s.db.QueryRowContext(ctx, `
SELECT id, title, subject, duration_minutes, room_id, proctor_id, candidate_ids_json
FROM exam_templates
WHERE id = ?`, templateID).Scan(&templateItem.ID, &templateItem.Title, &templateItem.Subject, &templateItem.DurationMinutes, &templateItem.RoomID, &templateItem.ProctorID, &candidate)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("%w: exam template not found", ErrNotFound)
		}
		return nil, nil, fmt.Errorf("get exam template: %w", err)
	}
	templateItem.CandidateIDs = parseCandidateJSON(candidate)

	var (
		window    domain.ExamTemplateWindow
		startUnix int64
		endUnix   int64
		createdAt int64
	)
	err = s.db.QueryRowContext(ctx, `
SELECT id, template_id, label, window_start_at, window_end_at, created_at
FROM exam_template_windows
WHERE id = ? AND template_id = ?`, windowID, templateID).Scan(&window.ID, &window.TemplateID, &window.Label, &startUnix, &endUnix, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("%w: template window not found", ErrNotFound)
		}
		return nil, nil, fmt.Errorf("get template window: %w", err)
	}
	window.WindowStartAt = time.Unix(startUnix, 0).UTC()
	window.WindowEndAt = time.Unix(endUnix, 0).UTC()
	window.CreatedAt = time.Unix(createdAt, 0).UTC()

	return &templateItem, &window, nil
}

func (s *ExamTemplateService) ListDrafts(ctx context.Context, templateID *int64) ([]domain.ExamSessionDraft, error) {
	query := `SELECT id, template_id, subject, room_id, proctor_id, candidate_ids_json, start_at, end_at, status, conflict_details_json, published_schedule_id, created_by, version, created_at, updated_at FROM exam_session_drafts WHERE 1=1`
	args := make([]any, 0, 1)
	if templateID != nil {
		query += ` AND template_id = ?`
		args = append(args, *templateID)
	}
	query += ` ORDER BY created_at DESC LIMIT 200`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list exam session drafts: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ExamSessionDraft, 0)
	for rows.Next() {
		var (
			item          domain.ExamSessionDraft
			candidate     string
			conflictsJSON sql.NullString
			published     sql.NullInt64
			createdBy     sql.NullInt64
			startUnix     int64
			endUnix       int64
			createdAtUnix int64
			updatedAtUnix int64
		)
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.Subject, &item.RoomID, &item.ProctorID, &candidate, &startUnix, &endUnix, &item.Status, &conflictsJSON, &published, &createdBy, &item.Version, &createdAtUnix, &updatedAtUnix); err != nil {
			return nil, fmt.Errorf("scan exam draft row: %w", err)
		}
		item.CandidateIDs = parseCandidateJSON(candidate)
		item.StartAt = time.Unix(startUnix, 0).UTC()
		item.EndAt = time.Unix(endUnix, 0).UTC()
		item.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		item.UpdatedAt = time.Unix(updatedAtUnix, 0).UTC()
		if published.Valid {
			v := published.Int64
			item.PublishedSchedule = &v
		}
		if createdBy.Valid {
			v := createdBy.Int64
			item.CreatedBy = &v
		}
		if conflictsJSON.Valid {
			_ = json.Unmarshal([]byte(conflictsJSON.String), &item.Conflicts)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exam draft rows: %w", err)
	}
	return items, nil
}

type AdjustDraftInput struct {
	DraftID   int64
	StartAt   time.Time
	EndAt     time.Time
	ActorID   *int64
	RequestID string
}

func (s *ExamTemplateService) AdjustDraft(ctx context.Context, input AdjustDraftInput) (*domain.ExamSessionDraft, error) {
	if input.DraftID <= 0 {
		return nil, fmt.Errorf("%w: draft_id must be positive", ErrValidation)
	}
	if !input.EndAt.After(input.StartAt) {
		return nil, fmt.Errorf("%w: end_at must be after start_at", ErrValidation)
	}

	draft, err := s.getDraft(ctx, input.DraftID)
	if err != nil {
		return nil, err
	}

	conflicts, err := s.schedules.DetectConflicts(ctx, nil, draft.RoomID, draft.ProctorID, draft.CandidateIDs, input.StartAt.UTC(), input.EndAt.UTC())
	if err != nil {
		return nil, err
	}
	conflictJSON, _ := json.Marshal(conflicts)

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE exam_session_drafts
SET start_at = ?, end_at = ?, conflict_details_json = ?, updated_at = ?, version = version + 1
WHERE id = ? AND version = ?`, input.StartAt.UTC().Unix(), input.EndAt.UTC().Unix(), string(conflictJSON), now.Unix(), input.DraftID, draft.Version)
	if err != nil {
		return nil, fmt.Errorf("update exam session draft: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("%w: draft was modified by another user", ErrVersionConflict)
	}

	if s.audit != nil {
		_ = s.audit.LogEvent(ctx, AuditLogInput{
			ActorID:      input.ActorID,
			Action:       "exam_session_draft.adjust",
			ResourceType: "exam_session_draft",
			ResourceID:   strconv.FormatInt(input.DraftID, 10),
			Before: map[string]any{
				"start_at": draft.StartAt.Format(time.RFC3339),
				"end_at":   draft.EndAt.Format(time.RFC3339),
			},
			After: map[string]any{
				"start_at":  input.StartAt.UTC().Format(time.RFC3339),
				"end_at":    input.EndAt.UTC().Format(time.RFC3339),
				"conflicts": conflicts,
			},
			RequestID: input.RequestID,
		})
	}

	draft.StartAt = input.StartAt.UTC()
	draft.EndAt = input.EndAt.UTC()
	draft.Conflicts = conflicts
	draft.UpdatedAt = now
	return draft, nil
}

func (s *ExamTemplateService) getDraft(ctx context.Context, draftID int64) (*domain.ExamSessionDraft, error) {
	items, err := s.ListDrafts(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == draftID {
			copyItem := item
			return &copyItem, nil
		}
	}
	return nil, fmt.Errorf("%w: draft not found", ErrNotFound)
}

type PublishDraftInput struct {
	DraftID        int64
	ActorID        int64
	IdempotencyKey string
	RequestID      string
}

func (s *ExamTemplateService) PublishDraft(ctx context.Context, input PublishDraftInput) (*domain.ExamSessionDraft, error) {
	if input.DraftID <= 0 {
		return nil, fmt.Errorf("%w: draft_id must be positive", ErrValidation)
	}
	draft, err := s.getDraft(ctx, input.DraftID)
	if err != nil {
		return nil, err
	}
	if len(draft.Conflicts) > 0 {
		return nil, fmt.Errorf("%w: cannot publish draft with conflicts", ErrSchedulingConflict)
	}
	if draft.Status == "published" {
		return draft, nil
	}

	result, err := s.scheduling.CreateIdempotent(ctx, CreateExamScheduleInput{
		ExamID:         draft.Subject,
		RoomID:         draft.RoomID,
		ProctorID:      draft.ProctorID,
		CandidateIDs:   draft.CandidateIDs,
		StartAt:        draft.StartAt,
		EndAt:          draft.EndAt,
		ActorID:        input.ActorID,
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		RouteKey:       "/api/v1/exam-session-drafts/" + strconv.FormatInt(input.DraftID, 10) + "/publish",
		RequestID:      input.RequestID,
	})
	if err != nil {
		return nil, err
	}
	if result.StatusCode != 201 {
		return nil, fmt.Errorf("%w: draft publish response status %d", ErrConflict, result.StatusCode)
	}

	now := time.Now().UTC()
	publishResult, err := s.db.ExecContext(ctx, `UPDATE exam_session_drafts SET status = 'published', updated_at = ?, version = version + 1 WHERE id = ? AND version = ? AND status = 'draft'`, now.Unix(), input.DraftID, draft.Version)
	if err != nil {
		return nil, fmt.Errorf("mark draft published: %w", err)
	}
	affected, _ := publishResult.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("%w: draft was modified or already published by another user", ErrVersionConflict)
	}
	draft.Status = "published"
	draft.Version++
	draft.UpdatedAt = now
	return draft, nil
}

func parseCandidateJSON(raw string) []int64 {
	values := make([]int64, 0)
	if strings.TrimSpace(raw) == "" {
		return values
	}
	_ = json.Unmarshal([]byte(raw), &values)
	return normalizeCandidateIDs(values)
}
