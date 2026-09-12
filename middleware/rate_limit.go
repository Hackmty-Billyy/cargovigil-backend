package middleware

import (
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// LoginRateLimiter is an in-memory fixed-window limiter keyed by ip+email.
// It is a stopgap for a single-instance deployment: it does not coordinate
// across processes. Swap for a Redis-backed limiter before scaling
// horizontally to more than one server instance.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func NewLoginRateLimiter(limit int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (l *LoginRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.attempts[key] = kept
		return false
	}
	l.attempts[key] = append(kept, now)
	return true
}

func (l *LoginRateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			Email string `json:"email"`
		}
		_ = c.BodyParser(&body) // best-effort; falls back to an IP-only key

		key := c.IP() + "|" + strings.ToLower(body.Email)
		if !l.allow(key) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too many attempts, try again later"})
		}
		return c.Next()
	}
}
