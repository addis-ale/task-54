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

type GovernanceHandler struct {
	reports *service.ReportService
}

func NewGovernanceHandler(reports *service.ReportService) *GovernanceHandler {
	return &GovernanceHandler{reports: reports}
}

type createReportScheduleRequest struct {
	ReportType      string `json:"report_type"`
	Format          string `json:"format"`
	SharedFolder    string `json:"shared_folder_path"`
	FiltersJSON     string `json:"filters_json"`
	IntervalMinutes int    `json:"interval_minutes"`
	FirstRunAt      string `json:"first_run_at"`
}

type createConfigVersionRequest struct {
	ConfigKey   string `json:"config_key"`
	PayloadJSON string `json:"payload_json"`
}

func (h *GovernanceHandler) AuditSearch(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}

	filter := service.AuditSearchFilter{}
	if resident, ok := parseOptionalInt64(c.Query("resident_id")); ok {
		filter.ResidentID = &resident
	}
	filter.RecordType = strings.TrimSpace(c.Query("record_type"))
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
	if limit, ok := parseOptionalInt64(c.Query("limit")); ok {
		filter.Limit = int(limit)
	}

	items, err := h.reports.SearchAudit(c.UserContext(), filter)
	if err != nil {
		return handleServiceError(c, err, "Failed to search audit trail")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"audit_events": items})
}

func (h *GovernanceHandler) AuditExport(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}

	filter := service.AuditSearchFilter{}
	if resident, ok := parseOptionalInt64(c.Query("resident_id")); ok {
		filter.ResidentID = &resident
	}
	filter.RecordType = strings.TrimSpace(c.Query("record_type"))
	if from, ok, err := parseOptionalDateTime(c.Query("from")); err == nil && ok {
		filter.From = &from
	}
	if to, ok, err := parseOptionalDateTime(c.Query("to")); err == nil && ok {
		filter.To = &to
	}

	file, err := h.reports.ExportAudit(c.UserContext(), filter, strings.TrimSpace(c.Query("format")))
	if err != nil {
		return handleServiceError(c, err, "Failed to export audit report")
	}

	c.Set("Content-Type", file.ContentType)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.FileName))
	return c.Send(file.Body)
}

func (h *GovernanceHandler) CreateReportSchedule(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}

	var req createReportScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	var firstRunAt time.Time
	if strings.TrimSpace(req.FirstRunAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.FirstRunAt))
		if err != nil {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "first_run_at must be RFC3339", nil)
		}
		firstRunAt = parsed
	}

	item, err := h.reports.CreateSchedule(c.UserContext(), service.CreateReportScheduleInput{
		ReportType:      req.ReportType,
		Format:          req.Format,
		SharedFolder:    req.SharedFolder,
		FiltersJSON:     req.FiltersJSON,
		IntervalMinutes: req.IntervalMinutes,
		FirstRunAt:      firstRunAt,
		ActorID:         currentActorIDFromContext(c),
		RequestID:       httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create report schedule")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"report_schedule": item})
}

func (h *GovernanceHandler) ListReportSchedules(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}
	items, err := h.reports.ListSchedules(c.UserContext())
	if err != nil {
		return handleServiceError(c, err, "Failed to list report schedules")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"report_schedules": items})
}

func (h *GovernanceHandler) RunReportSchedulesNow(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}
	if err := h.reports.RunDueSchedules(c.UserContext(), time.Now().UTC().Add(24*time.Hour)); err != nil {
		return handleServiceError(c, err, "Failed to run report schedules")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"status": "ok"})
}

func (h *GovernanceHandler) CreateConfigVersion(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}
	var req createConfigVersionRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}
	item, err := h.reports.CreateConfigVersion(c.UserContext(), service.CreateConfigVersionInput{
		ConfigKey:   req.ConfigKey,
		PayloadJSON: req.PayloadJSON,
		ActorID:     currentActorIDFromContext(c),
		RequestID:   httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create config version")
	}
	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"config_version": item})
}

func (h *GovernanceHandler) ListConfigVersions(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}
	items, err := h.reports.ListConfigVersions(c.UserContext(), strings.TrimSpace(c.Query("config_key")))
	if err != nil {
		return handleServiceError(c, err, "Failed to list config versions")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"config_versions": items})
}

func (h *GovernanceHandler) RollbackConfigVersion(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}
	versionID, err := strconv.ParseInt(c.Params("version_id"), 10, 64)
	if err != nil || versionID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "version_id must be a positive integer", nil)
	}
	item, err := h.reports.RollbackConfigVersion(c.UserContext(), versionID, currentActorIDFromContext(c), httpx.RequestID(c))
	if err != nil {
		return handleServiceError(c, err, "Failed to rollback config version")
	}
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"config_version": item})
}
