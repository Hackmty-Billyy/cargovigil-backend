package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
)

// RequireAuth validates the access token and exposes user_id/role_id to
// downstream handlers via fiber locals.
func RequireAuth(secret []byte) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		tokenStr, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || tokenStr == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing bearer token"})
		}

		claims, err := auth.ParseAccessToken(secret, tokenStr)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired token"})
		}

		c.Locals("user_id", claims.UserID())
		c.Locals("role_id", claims.RoleID)
		return c.Next()
	}
}
