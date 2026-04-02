package httpx

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	RequestIDHeader   = "X-Request-ID"
	RequestIDLocalKey = "request_id"
)

type envelope struct {
	Data  any        `json:"data"`
	Meta  meta       `json:"meta"`
	Error *errorBody `json:"error"`
}

type meta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func OK(c *fiber.Ctx, status int, data any) error {
	return c.Status(status).JSON(envelope{
		Data: data,
		Meta: buildMeta(c),
	})
}

func Error(c *fiber.Ctx, status int, code, message string, details any) error {
	return c.Status(status).JSON(envelope{
		Data: nil,
		Meta: buildMeta(c),
		Error: &errorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func RequestID(c *fiber.Ctx) string {
	if v := c.Locals(RequestIDLocalKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func buildMeta(c *fiber.Ctx) meta {
	return meta{
		RequestID: RequestID(c),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
