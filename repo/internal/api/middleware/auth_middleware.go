package middleware

import (
	"errors"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

const authContextLocalKey = "auth_context"

type AuthContext struct {
	User        *domain.User
	Session     *domain.Session
	Permissions []domain.Permission
}

func RequireAuth(authService *service.AuthService, cookieName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rawToken := c.Cookies(cookieName)
		if rawToken == "" {
			return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
		}

		user, session, err := authService.AuthenticateToken(c.UserContext(), rawToken)
		if err != nil {
			if errors.Is(err, service.ErrUnauthenticated) {
				return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Invalid or expired session", nil)
			}
			return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Unable to authenticate session", nil)
		}

		authContext := &AuthContext{
			User:        user,
			Session:     session,
			Permissions: domain.PermissionsForRole(user.Role),
		}

		c.Locals(authContextLocalKey, authContext)
		c.Locals("permissions", authContext.Permissions)

		enriched := service.WithAuditContext(c.UserContext(), service.AuditContext{
			OperatorID:       &authContext.User.ID,
			OperatorUsername: authContext.User.Username,
			OperatorRole:     authContext.User.Role,
			LocalIP:          c.IP(),
			RequestID:        httpx.RequestID(c),
		})
		c.SetUserContext(enriched)

		return c.Next()
	}
}

func CurrentAuth(c *fiber.Ctx) (*AuthContext, bool) {
	v := c.Locals(authContextLocalKey)
	authContext, ok := v.(*AuthContext)
	if !ok || authContext == nil {
		return nil, false
	}
	return authContext, true
}
