package handler

import (
	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/api/middleware"

	"github.com/gofiber/fiber/v2"
)

type AdminHandler struct{}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
}

func (h *AdminHandler) AuditPing(c *fiber.Ctx) error {
	authContext, ok := middleware.CurrentAuth(c)
	if !ok {
		return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{
		"message": "rbac check passed",
		"actor": fiber.Map{
			"id":       authContext.User.ID,
			"username": authContext.User.Username,
			"role":     authContext.User.Role,
		},
		"permissions": authContext.Permissions,
	})
}
