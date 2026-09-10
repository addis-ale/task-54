package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type CareHandler struct {
	care *service.CareService
}

func NewCareHandler(care *service.CareService) *CareHandler {
	return &CareHandler{care: care}
}

type createCheckpointRequest struct {
	ResidentID     int64  `json:"resident_id"`
	CheckpointType string `json:"checkpoint_type"`
	Status         string `json:"status"`
	Notes          string `json:"notes"`
}

type createAlertRequest struct {
	ResidentID int64  `json:"resident_id"`
	AlertType  string `json:"alert_type"`
	Severity   string `json:"severity"`
	State      string `json:"state"`
	Message    string `json:"message"`
}

func (h *CareHandler) ListCheckpoints(c *fiber.Ctx) error {
	filter := service.CareCheckpointFilter{}
	if resident, ok := parseOptionalInt64(c.Query("resident_id")); ok {
		filter.ResidentID = &resident
	}
	filter.Status = strings.TrimSpace(c.Query("status"))
	if from, ok, err := parseOptionalDateTime(c.Query("from")); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error(), nil)
	} else if ok {
		filter.From = &from
	}
	if to, ok, err := parseOptionalDateTime(c.Query("to")); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error(), nil)
	} else if ok {
		filter.To = &to
	}

	items, err := h.care.ListCheckpoints(c.UserContext(), filter)
	if err != nil {
		return handleServiceError(c, err, "Failed to list care checkpoints")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"care_quality_checkpoints": items})
}

func (h *CareHandler) CreateCheckpoint(c *fiber.Ctx) error {
	var req createCheckpointRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	item, err := h.care.CreateCheckpoint(c.UserContext(), service.CreateCheckpointInput{
		ResidentID:     req.ResidentID,
		CheckpointType: req.CheckpointType,
		Status:         req.Status,
		Notes:          req.Notes,
		ActorID:        currentActorIDFromContext(c),
		RequestID:      httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create care checkpoint")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"care_quality_checkpoint": item})
}

func (h *CareHandler) ListAlerts(c *fiber.Ctx) error {
	filter := service.AlertEventFilter{}
	if resident, ok := parseOptionalInt64(c.Query("resident_id")); ok {
		filter.ResidentID = &resident
	}
	filter.Severity = strings.TrimSpace(c.Query("severity"))
	filter.State = strings.TrimSpace(c.Query("state"))
	if from, ok, err := parseOptionalDateTime(c.Query("from")); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error(), nil)
	} else if ok {
		filter.From = &from
	}
	if to, ok, err := parseOptionalDateTime(c.Query("to")); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error(), nil)
	} else if ok {
		filter.To = &to
	}

	items, err := h.care.ListAlerts(c.UserContext(), filter)
	if err != nil {
		return handleServiceError(c, err, "Failed to list alert events")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"alert_events": items})
}

func (h *CareHandler) CreateAlert(c *fiber.Ctx) error {
	var req createAlertRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	item, err := h.care.CreateAlert(c.UserContext(), service.CreateAlertInput{
		ResidentID: req.ResidentID,
		AlertType:  req.AlertType,
		Severity:   req.Severity,
		State:      req.State,
		Message:    req.Message,
		ActorID:    currentActorIDFromContext(c),
		RequestID:  httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create alert event")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"alert_event": item})
}

func (h *CareHandler) Dashboard(c *fiber.Ctx) error {
	summary, err := h.care.Dashboard(c.UserContext())
	if err != nil {
		return handleServiceError(c, err, "Failed to load care dashboard")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"summary": summary})
}

func parseOptionalDateTime(raw string) (time.Time, bool, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}, false, nil
	}
	if parsed, err := time.Parse(time.RFC3339, v); err == nil {
		return parsed.UTC(), true, nil
	}
	parsed, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("date filters must be RFC3339 or YYYY-MM-DD")
	}
	return parsed.UTC(), true, nil
}

func parseOptionalInt64(raw string) (int64, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
