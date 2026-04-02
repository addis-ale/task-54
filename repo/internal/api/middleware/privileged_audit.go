package middleware

import (
	"fmt"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

func PrivilegedAudit(audit *service.AuditService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if audit == nil {
			return c.Next()
		}

		method := strings.ToUpper(strings.TrimSpace(c.Method()))
		if method != fiber.MethodPost && method != fiber.MethodPatch && method != fiber.MethodPut && method != fiber.MethodDelete {
			return c.Next()
		}

		path := c.Path()
		before := map[string]any{
			"method":       method,
			"path":         path,
			"query":        c.Context().QueryArgs().String(),
			"request_body": truncateForAudit(string(c.Body()), 2000),
		}

		err := c.Next()

		authContext, ok := CurrentAuth(c)
		var actorID *int64
		if ok && authContext.User != nil {
			id := authContext.User.ID
			actorID = &id
		}

		after := map[string]any{
			"status_code":      c.Response().StatusCode(),
			"response_preview": truncateForAudit(string(c.Response().Body()), 2000),
		}

		_ = audit.LogEvent(c.UserContext(), service.AuditLogInput{
			ActorID:      actorID,
			Action:       buildAuditAction(method, path),
			ResourceType: inferResourceType(path),
			ResourceID:   inferResourceID(c),
			Before:       before,
			After:        after,
			RequestID:    httpx.RequestID(c),
		})

		return err
	}
}

func buildAuditAction(method, path string) string {
	normalized := strings.Trim(path, "/")
	normalized = strings.ReplaceAll(normalized, "/", ".")
	if normalized == "" {
		normalized = "root"
	}
	return fmt.Sprintf("privileged.%s.%s", strings.ToLower(method), normalized)
}

func inferResourceType(path string) string {
	clean := strings.Trim(path, "/")
	parts := strings.Split(clean, "/")
	if len(parts) == 0 {
		return "unknown"
	}
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v1" {
		return parts[2]
	}
	if len(parts) >= 2 && parts[0] == "ui" {
		return parts[1]
	}
	return parts[len(parts)-1]
}

func inferResourceID(c *fiber.Ctx) string {
	for _, key := range []string{
		"admission_id",
		"work_order_id",
		"exercise_id",
		"media_id",
		"schedule_id",
		"payment_id",
		"template_id",
		"draft_id",
		"id",
	} {
		if v := strings.TrimSpace(c.Params(key)); v != "" {
			return v
		}
	}
	return ""
}

func truncateForAudit(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...(truncated)"
}
