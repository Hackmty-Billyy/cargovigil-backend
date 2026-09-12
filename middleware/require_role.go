package middleware

import "github.com/gofiber/fiber/v2"

// RequireRole must run after RequireAuth. Each user has exactly one role_id,
// set as a foreign key on users — no join table.
func RequireRole(allowed ...int16) fiber.Handler {
	allowedSet := make(map[int16]struct{}, len(allowed))
	for _, r := range allowed {
		allowedSet[r] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		roleID, ok := c.Locals("role_id").(int16)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		if _, permitted := allowedSet[roleID]; !permitted {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}
