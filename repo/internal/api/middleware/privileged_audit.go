package middleware

import (
	"encoding/json"
	"fmt"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

// sensitiveBodyFields lists JSON keys that must be redacted from request bodies
// before they are persisted in the immutable audit log.
var sensitiveBodyFields = map[string]bool{
	"password":        true,
	"pii_reference":   true,
	"card_number":     true,
	"payer_reference": true,
	"token":           true,
	"session_token":   true,
	"secret":          true,
	"master_key":      true,
}

// redactRequestBody parses the raw body as JSON and replaces sensitive field
// values with "***REDACTED***". If parsing fails, it returns the truncated
// raw body with a best-effort regex-style replacement of known keys.
func redactRequestBody(raw string, limit int) string {
	truncated := truncateForAudit(raw, limit)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// Not JSON (e.g. form-encoded) — do best-effort redaction of known keys
		result := truncated
		for key := range sensitiveBodyFields {
			// Redact form values like pii_reference=SENSITIVE_VALUE
			for {
				idx := strings.Index(strings.ToLower(result), key+"=")
				if idx == -1 {
					break
				}
				start := idx + len(key) + 1
				end := strings.IndexAny(result[start:], "&\n\r ")
				if end == -1 {
					result = result[:start] + "***REDACTED***"
				} else {
					result = result[:start] + "***REDACTED***" + result[start+end:]
				}
			}
		}
		return result
	}
	redactMap(parsed)
	redacted, err := json.Marshal(parsed)
	if err != nil {
		return truncated
	}
	return truncateForAudit(string(redacted), limit)
}

func redactMap(m map[string]any) {
	for key, val := range m {
		if sensitiveBodyFields[strings.ToLower(key)] {
			m[key] = "***REDACTED***"
			continue
		}
		switch v := val.(type) {
		case map[string]any:
			redactMap(v)
		case []any:
			for _, item := range v {
				if sub, ok := item.(map[string]any); ok {
					redactMap(sub)
				}
			}
		}
	}
}

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
			"request_body": redactRequestBody(string(c.Body()), 2000),
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
