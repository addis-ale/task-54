package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type ipEntry struct {
	count    int
	windowAt time.Time
}

// IPRateLimiter provides a simple in-memory IP-based rate limiter.
// It allows maxAttempts requests per IP within the given window duration.
func IPRateLimiter(maxAttempts int, window time.Duration) fiber.Handler {
	var mu sync.Mutex
	entries := make(map[string]*ipEntry)

	// Background cleanup every window duration to prevent memory leak
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			now := time.Now()
			for ip, e := range entries {
				if now.Sub(e.windowAt) > window {
					delete(entries, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *fiber.Ctx) error {
		ip := c.IP()
		now := time.Now()

		mu.Lock()
		e, ok := entries[ip]
		if !ok || now.Sub(e.windowAt) > window {
			entries[ip] = &ipEntry{count: 1, windowAt: now}
			mu.Unlock()
			return c.Next()
		}

		e.count++
		if e.count > maxAttempts {
			mu.Unlock()
			retryAfter := e.windowAt.Add(window).Sub(now)
			if retryAfter < 0 {
				retryAfter = 0
			}
			c.Set("Retry-After", retryAfter.Truncate(time.Second).String())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": "Too many login attempts. Please try again later.",
				},
			})
		}
		mu.Unlock()
		return c.Next()
	}
}
