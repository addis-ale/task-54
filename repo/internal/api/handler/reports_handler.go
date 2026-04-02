package handler

import (
	"fmt"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type ReportsHandler struct {
	reports *service.ReportService
}

func NewReportsHandler(reports *service.ReportService) *ReportsHandler {
	return &ReportsHandler{reports: reports}
}

func (h *ReportsHandler) OpsSummary(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}

	summary, err := h.reports.OpsSummary(c.UserContext())
	if err != nil {
		return handleServiceError(c, err, "Failed to load ops summary")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"summary": summary})
}

func (h *ReportsHandler) ExportFinance(c *fiber.Ctx) error {
	if h.reports == nil {
		return httpx.Error(c, fiber.StatusNotImplemented, "NOT_IMPLEMENTED", "Reporting service is not configured", nil)
	}

	report, err := h.reports.ExportFinance(c.UserContext(), service.ExportFinanceInput{
		Format:  strings.TrimSpace(c.Query("format")),
		Status:  strings.TrimSpace(c.Query("status")),
		Method:  strings.TrimSpace(c.Query("method")),
		Gateway: strings.TrimSpace(c.Query("gateway")),
		ShiftID: strings.TrimSpace(c.Query("shift_id")),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to export finance report")
	}

	c.Set("Content-Type", report.ContentType)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", report.FileName))
	return c.Send(report.Body)
}
