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
