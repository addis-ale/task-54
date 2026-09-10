package handler

import (
	"errors"
	"strings"
	"time"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/api/middleware"
	"clinic-admin-suite/internal/config"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type AuthHandler struct {
	authService *service.AuthService
	config      config.Config
}

func NewAuthHandler(authService *service.AuthService, cfg config.Config) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		config:      cfg,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	result, err := h.authService.Login(c.UserContext(), service.LoginInput{
		Username:  req.Username,
		Password:  req.Password,
		RequestID: httpx.RequestID(c),
		IP:        c.IP(),
		UserAgent: c.Get("User-Agent"),
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Invalid username or password", nil)
		}

		var accountLocked *service.AccountLockedError
		if errors.As(err, &accountLocked) {
			return httpx.Error(c, fiber.StatusForbidden, "AUTH_FORBIDDEN", "Account is temporarily locked", fiber.Map{
				"locked_until": accountLocked.Until.Format("2006-01-02T15:04:05Z07:00"),
			})
		}

		return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process login", nil)
	}

	c.Cookie(&fiber.Cookie{
		Name:     h.config.SessionCookieName,
		Value:    result.Token,
		Path:     "/",
		Expires:  result.ExpiresAt,
		HTTPOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: "Strict",
	})

	return httpx.OK(c, fiber.StatusOK, fiber.Map{
		"user": fiber.Map{
			"id":       result.User.ID,
			"username": result.User.Username,
			"role":     result.User.Role,
		},
		"permissions":        result.Permission,
		"session_expires_at": result.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	rawToken := strings.TrimSpace(c.Cookies(h.config.SessionCookieName))
	if rawToken != "" {
		_ = h.authService.Logout(c.UserContext(), rawToken)
	}
	c.Cookie(&fiber.Cookie{
		Name:     h.config.SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: "Strict",
	})
	return httpx.OK(c, fiber.StatusOK, fiber.Map{"message": "logged out"})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	authContext, ok := middleware.CurrentAuth(c)
	if !ok {
		return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{
		"user": fiber.Map{
			"id":       authContext.User.ID,
			"username": authContext.User.Username,
			"role":     authContext.User.Role,
		},
		"permissions": authContext.Permissions,
	})
}
