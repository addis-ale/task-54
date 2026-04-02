package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"

	"github.com/gofiber/fiber/v2"
)

func RequireIdempotency(idempotency repository.IdempotencyRepository, requiredPrefixes ...string) fiber.Handler {
	prefixes := make([]string, 0, len(requiredPrefixes))
	for _, p := range requiredPrefixes {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			prefixes = append(prefixes, trimmed)
		}
	}

	return func(c *fiber.Ctx) error {
		if idempotency == nil {
			return c.Next()
		}

		method := strings.ToUpper(strings.TrimSpace(c.Method()))
		if method != fiber.MethodPost && method != fiber.MethodPatch && method != fiber.MethodPut && method != fiber.MethodDelete {
			return c.Next()
		}

		path := c.Path()
		required := len(prefixes) == 0
		for _, prefix := range prefixes {
			if strings.HasPrefix(path, prefix) {
				required = true
				break
			}
		}
		if !required {
			return c.Next()
		}

		authContext, ok := CurrentAuth(c)
		if !ok || authContext.User == nil {
			return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
		}

		idempotencyKey := strings.TrimSpace(c.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Idempotency-Key header is required", nil)
		}

		routeKey := method + " " + path
		requestHash := hashRequest(method, path, c.Body())
		now := time.Now().UTC()

		existing, err := idempotency.GetActive(c.UserContext(), authContext.User.ID, routeKey, idempotencyKey, now)
		if err != nil {
			return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to check idempotency key", nil)
		}
		if existing != nil {
			if existing.RequestHash != requestHash {
				return httpx.Error(c, fiber.StatusConflict, "IDEMPOTENCY_CONFLICT", "Same key used with different payload", nil)
			}
			c.Set("X-Idempotent-Replay", "true")
			c.Type("json", "utf-8")
			return c.Status(existing.ResponseCode).SendString(existing.ResponseBody)
		}

		if err := c.Next(); err != nil {
			return err
		}

		responseCode := c.Response().StatusCode()
		responseBody := string(c.Response().Body())
		if responseBody == "" {
			responseBody = `{"data":null,"meta":{"request_id":"` + httpx.RequestID(c) + `","timestamp":"` + now.Format(time.RFC3339) + `"},"error":null}`
		}

		record := &domain.IdempotencyKeyRecord{
			ActorID:      authContext.User.ID,
			RouteKey:     routeKey,
			Key:          idempotencyKey,
			RequestHash:  requestHash,
			ResponseCode: responseCode,
			ResponseBody: responseBody,
			ExpiresAt:    now.Add(24 * time.Hour),
			CreatedAt:    now,
		}

		if err := idempotency.Create(c.UserContext(), record); err != nil {
			existingAgain, lookupErr := idempotency.GetActive(c.UserContext(), authContext.User.ID, routeKey, idempotencyKey, now)
			if lookupErr != nil {
				return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to persist idempotency result", nil)
			}
			if existingAgain != nil {
				if existingAgain.RequestHash != requestHash {
					return httpx.Error(c, fiber.StatusConflict, "IDEMPOTENCY_CONFLICT", "Same key used with different payload", nil)
				}
			}
		}

		return nil
	}
}

func hashRequest(method, path string, body []byte) string {
	sum := sha256.Sum256([]byte(method + "|" + path + "|" + string(body)))
	return hex.EncodeToString(sum[:])
}
