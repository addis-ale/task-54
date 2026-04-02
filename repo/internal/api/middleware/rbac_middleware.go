package middleware

import (
	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/domain"

	"github.com/gofiber/fiber/v2"
)

func RequirePermissions(required ...domain.Permission) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authContext, ok := CurrentAuth(c)
		if !ok || authContext.User == nil {
			return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
		}

		if !domain.HasPermissions(authContext.User.Role, required...) {
			return httpx.Error(c, fiber.StatusForbidden, "AUTH_FORBIDDEN", "Insufficient permissions", fiber.Map{
				"required": required,
			})
		}

		return c.Next()
	}
}
