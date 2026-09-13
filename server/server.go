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
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/fuel"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/logistics"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/routecost"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/treasury"
	appmw "github.com/Hackmty-Billyy/cargovigil-backend/middleware"
)

func New(cfg *config.Config, authService *auth.Service, companyService *company.Service,
	treasuryService *treasury.Service, routeCostService *routecost.Service, fuelService *fuel.Service,
	logisticsService *logistics.Service) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "Logistics Fintech API v1.0",
	})

	allowedOrigins := "https://cargovigil.tech,https://dev.cargovigil.tech,http://localhost:5173,http://localhost:3000"
	if cfg != nil && cfg.AllowedOrigins != "" {
		allowedOrigins = cfg.AllowedOrigins
	}

	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Requested-With",
		AllowMethods:     "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS",
		AllowCredentials: true,
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

	// Module 2 sits between the two worlds: any role of the company reads the
	// operational picture (trips, downtime, route risk), operations captures
	// it, and only admin+finance move the liquidity cushion.
	// platform_admin is excluded from the whole group: it has no company of
	// its own, so every tenant-scoped query here would be meaningless for it.
	financeGuard := appmw.RequireRole(auth.RoleAdminID, auth.RoleFinanceID)
	companyRoles := appmw.RequireRole(auth.RoleAdminID, auth.RoleOperationsID, auth.RoleFinanceID)
	routeCostHandler := routecost.NewHandler(routeCostService)
	routeCostGroup := app.Group("/routecost", authMiddleware, companyRoles)
	routeCostHandler.RegisterRoutes(routeCostGroup, writeGuard, financeGuard)

	// fuel_indexes has no company_id (shared global feed), so recording a
	// reading is platform_admin-only, same reasoning as company onboarding.
	fuelHandler := fuel.NewHandler(fuelService)
	fuelHandler.RegisterPlatformRoutes(platformGroup)

	// Reads (indexes, trip fuel logs) are open to any role of the company;
	// logging a purchase is operational (admin+operations); surcharge rules
	// and margin simulations are financial decisions (admin+finance) —
	// differentiated per-route inside RegisterRoutes, like the company group.
	fuelGroup := app.Group("/fuel", authMiddleware)
	fuelHandler.RegisterRoutes(fuelGroup, writeGuard, financeGuard)

	// Live radar: the read/orchestration layer the dashboard map talks to. It
	// creates nothing of its own — trips go through routecost, prices through
	// fuel — so it reuses the same company-role gate as module 2.
	logisticsHandler := logistics.NewHandler(logisticsService)
	logisticsGroup := app.Group("/logistics", authMiddleware, companyRoles)
	logisticsHandler.RegisterRoutes(logisticsGroup, writeGuard)

	return app
}
