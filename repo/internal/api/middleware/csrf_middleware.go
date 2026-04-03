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
		// Only enforce on state-changing methods
		method := strings.ToUpper(c.Method())
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			ensureCSRFCookie(c, cookieSecure)
			return c.Next()
		}

		// Read token from cookie
		cookieToken := c.Cookies("clinic_csrf")
		if cookieToken == "" {
			return httpx.Error(c, fiber.StatusForbidden, "CSRF_MISSING", "CSRF token cookie not found", nil)
		}

		// Read token from header or form field
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

func ensureCSRFCookie(c *fiber.Ctx, secure bool) {
	if c.Cookies("clinic_csrf") != "" {
		return
	}
	token := generateCSRFToken()
	c.Cookie(&fiber.Cookie{
		Name:     "clinic_csrf",
		Value:    token,
		Path:     "/",
		HTTPOnly: false, // Needs to be readable by JavaScript for HTMX headers
		Secure:   secure,
		SameSite: "Strict",
	})
}

func generateCSRFToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("csrf: crypto/rand failure — system random source unavailable: %v", err)
	}
	return hex.EncodeToString(b)
}
