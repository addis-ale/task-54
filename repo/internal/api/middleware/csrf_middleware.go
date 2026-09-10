package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"strings"

	"clinic-admin-suite/internal/api/httpx"

	"github.com/gofiber/fiber/v2"
)

// CSRFProtect generates and validates a per-session CSRF token for state-changing requests.
// Tokens are stored in a cookie and must be echoed back via the X-CSRF-Token header or _csrf form field.
func CSRFProtect(cookieSecure bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		method := strings.ToUpper(c.Method())
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			ensureCSRFCookie(c, cookieSecure)
			return c.Next()
		}

		cookieToken := c.Cookies("clinic_csrf")
		if cookieToken == "" {
			return httpx.Error(c, fiber.StatusForbidden, "CSRF_MISSING", "CSRF token cookie not found", nil)
		}

		headerToken := strings.TrimSpace(c.Get("X-CSRF-Token"))
		if headerToken == "" {
			headerToken = strings.TrimSpace(c.FormValue("_csrf"))
		}

		if headerToken == "" || headerToken != cookieToken {
			return httpx.Error(c, fiber.StatusForbidden, "CSRF_INVALID", "CSRF token mismatch", nil)
		}

		return c.Next()
	}
}

// CSRFToken returns the current CSRF token for the request, reading from
// the incoming cookie or from Locals (set when a new cookie is generated).
func CSRFToken(c *fiber.Ctx) string {
	if token, ok := c.Locals("csrf_token").(string); ok && token != "" {
		return token
	}
	return c.Cookies("clinic_csrf")
}

func ensureCSRFCookie(c *fiber.Ctx, secure bool) {
	existing := c.Cookies("clinic_csrf")
	if existing != "" {
		c.Locals("csrf_token", existing)
		return
	}
	token := generateCSRFToken()
	c.Cookie(&fiber.Cookie{
		Name:     "clinic_csrf",
		Value:    token,
		Path:     "/",
		HTTPOnly: false,
		Secure:   secure,
		SameSite: "Strict",
	})
	c.Locals("csrf_token", token)
}

func generateCSRFToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("csrf: crypto/rand failure — system random source unavailable: %v", err)
	}
	return hex.EncodeToString(b)
}
