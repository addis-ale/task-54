package handler

import (
	"errors"
	"html"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

func handleServiceError(c *fiber.Ctx, err error, fallbackMessage string) error {
	switch {
	case errors.Is(err, service.ErrForbidden):
		return httpx.Error(c, fiber.StatusForbidden, "AUTH_FORBIDDEN", err.Error(), nil)
	case errors.Is(err, service.ErrValidation):
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error(), nil)
	case errors.Is(err, service.ErrNotFound):
		return httpx.Error(c, fiber.StatusNotFound, "NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrVersionConflict):
		return httpx.Error(c, fiber.StatusConflict, "VERSION_CONFLICT", err.Error(), nil)
	case errors.Is(err, service.ErrSchedulingConflict):
		return httpx.Error(c, fiber.StatusConflict, "SCHEDULING_CONFLICT", err.Error(), nil)
	case errors.Is(err, service.ErrIdempotencyConflict):
		return httpx.Error(c, fiber.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error(), nil)
	case errors.Is(err, service.ErrConflict):
		return httpx.Error(c, fiber.StatusConflict, "CONFLICT", err.Error(), nil)
	default:
		return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", fallbackMessage, nil)
	}
}

// handleUIError returns HTML with the correct HTTP status for HTMX consumers.
// Conflicts (version/scheduling/idempotency) return 409 so htmx-lite.js shows the "Record Changed" prompt.
// Validation errors return 422, and unknown errors return 500.
func handleUIError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrForbidden):
		return c.Status(fiber.StatusForbidden).SendString(`<div class="card">Access denied: ` + html.EscapeString(err.Error()) + `</div>`)
	case errors.Is(err, service.ErrVersionConflict),
		errors.Is(err, service.ErrSchedulingConflict),
		errors.Is(err, service.ErrIdempotencyConflict),
		errors.Is(err, service.ErrConflict):
		return c.Status(fiber.StatusConflict).SendString(`<div class="card" style="border-left:4px solid #b04f2d;padding:1rem"><h4>Record Changed</h4><p>` + html.EscapeString(err.Error()) + `</p><p>Please review the latest state below and retry your action.</p></div>`)
	case errors.Is(err, service.ErrNotFound):
		return c.Status(fiber.StatusNotFound).SendString(`<div class="card">Not found: ` + html.EscapeString(err.Error()) + `</div>`)
	case errors.Is(err, service.ErrValidation):
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	default:
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + html.EscapeString(err.Error()) + `</div>`)
	}
}
