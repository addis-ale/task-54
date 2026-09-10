package handler

import (
	"fmt"

	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type DiagnosticsHandler struct {
	diagnostics *service.DiagnosticsService
}

func NewDiagnosticsHandler(diagnostics *service.DiagnosticsService) *DiagnosticsHandler {
	return &DiagnosticsHandler{diagnostics: diagnostics}
}

func (h *DiagnosticsHandler) Export(c *fiber.Ctx) error {
	bundle, err := h.diagnostics.Export(c.UserContext())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to export diagnostics bundle")
	}

	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", bundle.FileName))
	return c.SendFile(bundle.BundlePath)
}

func (h *DiagnosticsHandler) ExportUI(c *fiber.Ctx) error {
	bundle, err := h.diagnostics.Export(c.UserContext())
	if err != nil {
		// Do not leak internal error details to the UI
		return c.Status(fiber.StatusInternalServerError).SendString(`<div class="card" style="border-left:4px solid #b04f2d;padding:1rem">Diagnostics export failed. Please check server logs for details.</div>`)
	}
	c.Type("html", "utf-8")
	return c.SendString(`<div class="card" style="border-left:4px solid #2d7b4a;padding:1rem"><strong>Diagnostics bundle exported successfully.</strong><p>File: ` + bundle.FileName + `</p><a href="/api/v1/diagnostics/export" target="_blank">Download Bundle</a></div>`)
}
