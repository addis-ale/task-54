package handler

import (
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type ExamTemplatesHandler struct {
	templates *service.ExamTemplateService
}

func NewExamTemplatesHandler(templates *service.ExamTemplateService) *ExamTemplatesHandler {
	return &ExamTemplatesHandler{templates: templates}
}

type createTemplateRequest struct {
	Title           string  `json:"title"`
	Subject         string  `json:"subject"`
	DurationMinutes int     `json:"duration_minutes"`
	RoomID          int64   `json:"room_id"`
	ProctorID       int64   `json:"proctor_id"`
	CandidateIDs    []int64 `json:"candidate_ids"`
	WindowLabel     string  `json:"window_label"`
	WindowStartAt   string  `json:"window_start_at"`
	WindowEndAt     string  `json:"window_end_at"`
}

type generateDraftRequest struct {
	TemplateID int64  `json:"template_id"`
	WindowID   int64  `json:"window_id"`
	StartAt    string `json:"start_at"`
}

type adjustDraftRequest struct {
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

func (h *ExamTemplatesHandler) CreateTemplate(c *fiber.Ctx) error {
	var req createTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}
	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.WindowStartAt))
	if err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "window_start_at must be RFC3339", nil)
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.WindowEndAt))
	if err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "window_end_at must be RFC3339", nil)
	}

	item, err := h.templates.CreateTemplate(c.UserContext(), service.CreateTemplateInput{
		Title:           req.Title,
		Subject:         req.Subject,
		DurationMinutes: req.DurationMinutes,
		RoomID:          req.RoomID,
		ProctorID:       req.ProctorID,
		CandidateIDs:    req.CandidateIDs,
		WindowLabel:     req.WindowLabel,
		WindowStartAt:   startAt,
		WindowEndAt:     endAt,
		ActorID:         currentActorIDFromContext(c),
		RequestID:       httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create exam template")
	}
	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"exam_template": item})
}

func (h *ExamTemplatesHandler) ListTemplates(c *fiber.Ctx) error {
	items, err := h.templates.ListTemplates(c.UserContext())
	if err != nil {
		return handleServiceError(c, err, "Failed to list exam templates")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exam_templates": items})
}

func (h *ExamTemplatesHandler) GenerateDraft(c *fiber.Ctx) error {
	var req generateDraftRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}
	var startAt *time.Time
	if strings.TrimSpace(req.StartAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartAt))
		if err != nil {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "start_at must be RFC3339", nil)
		}
		v := parsed.UTC()
		startAt = &v
	}

	item, err := h.templates.GenerateDraft(c.UserContext(), service.GenerateDraftInput{
		TemplateID: req.TemplateID,
		WindowID:   req.WindowID,
		StartAt:    startAt,
		ActorID:    currentActorIDFromContext(c),
		RequestID:  httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to generate session draft")
	}
	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"exam_session_draft": item})
}

func (h *ExamTemplatesHandler) ListDrafts(c *fiber.Ctx) error {
	var templateID *int64
	if raw := strings.TrimSpace(c.Query("template_id")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "template_id must be positive", nil)
		}
		templateID = &v
	}
	items, err := h.templates.ListDrafts(c.UserContext(), templateID)
	if err != nil {
		return handleServiceError(c, err, "Failed to list session drafts")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exam_session_drafts": items})
}

func (h *ExamTemplatesHandler) AdjustDraft(c *fiber.Ctx) error {
	draftID, err := strconv.ParseInt(c.Params("draft_id"), 10, 64)
	if err != nil || draftID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "draft_id must be positive", nil)
	}
	var req adjustDraftRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}
	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartAt))
	if err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "start_at must be RFC3339", nil)
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.EndAt))
	if err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "end_at must be RFC3339", nil)
	}

	item, err := h.templates.AdjustDraft(c.UserContext(), service.AdjustDraftInput{
		DraftID:   draftID,
		StartAt:   startAt,
		EndAt:     endAt,
		ActorID:   currentActorIDFromContext(c),
		RequestID: httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to adjust session draft")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exam_session_draft": item})
}

func (h *ExamTemplatesHandler) PublishDraft(c *fiber.Ctx) error {
	draftID, err := strconv.ParseInt(c.Params("draft_id"), 10, 64)
	if err != nil || draftID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "draft_id must be positive", nil)
	}
	actorID := currentActorIDFromContext(c)
	if actorID == nil {
		return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
	}

	idempotencyKey := strings.TrimSpace(c.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Idempotency-Key header is required", nil)
	}

	item, err := h.templates.PublishDraft(c.UserContext(), service.PublishDraftInput{
		DraftID:        draftID,
		ActorID:        *actorID,
		IdempotencyKey: idempotencyKey,
		RequestID:      httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to publish session draft")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exam_session_draft": item})
}
