package server

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/Hackmty-Billyy/cargovigil-backend/config"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/company"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/treasury"
	appmw "github.com/Hackmty-Billyy/cargovigil-backend/middleware"
)

func New(cfg *config.Config, authService *auth.Service, companyService *company.Service, treasuryService *treasury.Service) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "Logistics Fintech API v1.0",
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "https://cargovigil.tech, https://dev.cargovigil.tech, https://api.cargovigil.tech, https://apidev.cargovigil.tech, http://localhost:5173, http://localhost:3000",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Requested-With",
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

	companyHandler := company.NewHandler(companyService)

	companyGroup := app.Group("/company", authMiddleware)
	adminOnly := appmw.RequireRole(auth.RoleAdminID)
	writeGuard := appmw.RequireRole(auth.RoleAdminID, auth.RoleOperationsID)
	companyHandler.RegisterRoutes(companyGroup, writeGuard, adminOnly)

	// Client companies no longer self-signup: only CargoVigil's own platform
	// staff (role platform_admin) can onboard a new company.
	platformGroup := app.Group("/platform", authMiddleware, appmw.RequireRole(auth.RolePlatformAdminID))
	companyHandler.RegisterPlatformRoutes(platformGroup)

	// operations has no access here at all, not even read — unlike the
	// company catalog, cash position/liquidity stays finance+admin only.
	treasuryHandler := treasury.NewHandler(treasuryService)
	treasuryGroup := app.Group("/treasury", authMiddleware, appmw.RequireRole(auth.RoleAdminID, auth.RoleFinanceID))
	treasuryHandler.RegisterRoutes(treasuryGroup)

	return app
}
