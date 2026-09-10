package handler

import (
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type SchedulingHandler struct {
	scheduling *service.SchedulingService
}

func NewSchedulingHandler(scheduling *service.SchedulingService) *SchedulingHandler {
	return &SchedulingHandler{scheduling: scheduling}
}

type createExamScheduleRequest struct {
	ExamID       string  `json:"exam_id"`
	RoomID       int64   `json:"room_id"`
	ProctorID    int64   `json:"proctor_id"`
	CandidateIDs []int64 `json:"candidate_ids"`
	StartAt      string  `json:"start_at"`
	EndAt        string  `json:"end_at"`
}

func (h *SchedulingHandler) List(c *fiber.Ctx) error {
	filter := repository.ExamScheduleFilter{}

	if dateRaw := strings.TrimSpace(c.Query("date")); dateRaw != "" {
		date, err := time.Parse("2006-01-02", dateRaw)
		if err != nil {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "date must be in YYYY-MM-DD format", nil)
		}
		filter.Date = &date
	}

	if roomIDRaw := strings.TrimSpace(c.Query("room_id")); roomIDRaw != "" {
		roomID, err := strconv.ParseInt(roomIDRaw, 10, 64)
		if err != nil || roomID <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "room_id must be a positive integer", nil)
		}
		filter.RoomID = &roomID
	}

	if proctorIDRaw := strings.TrimSpace(c.Query("proctor_id")); proctorIDRaw != "" {
		proctorID, err := strconv.ParseInt(proctorIDRaw, 10, 64)
		if err != nil || proctorID <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "proctor_id must be a positive integer", nil)
		}
		filter.ProctorID = &proctorID
	}

	if candidateIDRaw := strings.TrimSpace(c.Query("candidate_id")); candidateIDRaw != "" {
		candidateID, err := strconv.ParseInt(candidateIDRaw, 10, 64)
		if err != nil || candidateID <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "candidate_id must be a positive integer", nil)
		}
		filter.CandidateID = &candidateID
	}

	items, err := h.scheduling.List(c.UserContext(), filter)
	if err != nil {
		return handleServiceError(c, err, "Failed to list exam schedules")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exam_schedules": items})
}

func (h *SchedulingHandler) Create(c *fiber.Ctx) error {
	actorID := currentActorIDFromContext(c)
	if actorID == nil {
		return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
	}

	idempotencyKey := strings.TrimSpace(c.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Idempotency-Key header is required", nil)
	}

	var req createExamScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartAt))
	if err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "start_at must be RFC3339 timestamp", nil)
	}

	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.EndAt))
	if err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "end_at must be RFC3339 timestamp", nil)
	}

	result, err := h.scheduling.CreateIdempotent(c.UserContext(), service.CreateExamScheduleInput{
		ExamID:         req.ExamID,
		RoomID:         req.RoomID,
		ProctorID:      req.ProctorID,
		CandidateIDs:   req.CandidateIDs,
		StartAt:        startAt,
		EndAt:          endAt,
		ActorID:        *actorID,
		IdempotencyKey: idempotencyKey,
		RouteKey:       "/api/v1/exam-schedules",
		RequestID:      httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create exam schedule")
	}

	c.Type("json", "utf-8")
	return c.Status(result.StatusCode).Send(result.Body)
}

func (h *SchedulingHandler) Validate(c *fiber.Ctx) error {
	scheduleID, err := strconv.ParseInt(c.Params("schedule_id"), 10, 64)
	if err != nil || scheduleID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "schedule_id must be a positive integer", nil)
	}

	result, err := h.scheduling.Validate(c.UserContext(), scheduleID)
	if err != nil {
		return handleServiceError(c, err, "Failed to validate schedule conflicts")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{
		"schedule_id":   result.ScheduleID,
		"has_conflicts": result.HasConflicts,
		"conflicts":     result.Conflicts,
	})
}
