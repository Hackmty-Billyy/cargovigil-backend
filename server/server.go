package server

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/Hackmty-Billyy/cargovigil-backend/config"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
	appmw "github.com/Hackmty-Billyy/cargovigil-backend/middleware"
)

func New(cfg *config.Config, authService *auth.Service) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "Logistics Fintech API v1.0",
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS",
	}))
	app.Use(recover.New())
	app.Use(logger.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "online",
			"message": "Engine is running smoothly",
		})
	})

	authMiddleware := appmw.RequireAuth(cfg.JWTAccessSecret)
	loginLimiter := appmw.NewLoginRateLimiter(10, time.Minute)

	authHandler := auth.NewHandler(authService)
	authGroup := app.Group("/auth")
	authHandler.RegisterRoutes(authGroup, authMiddleware, loginLimiter.Middleware())

	return app
}
