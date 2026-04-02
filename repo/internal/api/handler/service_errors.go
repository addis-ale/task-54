package handler

import (
	"errors"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

func handleServiceError(c *fiber.Ctx, err error, fallbackMessage string) error {
	switch {
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
