package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"clinic-admin-suite/internal/api/httpx"

	"github.com/gofiber/fiber/v2"
)

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := strings.TrimSpace(c.Get(httpx.RequestIDHeader))
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Locals(httpx.RequestIDLocalKey, requestID)
		c.Set(httpx.RequestIDHeader, requestID)

		return c.Next()
	}
}

func generateRequestID() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(b)
}
