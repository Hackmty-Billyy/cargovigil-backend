package main

import (
	"context"
	"log"
	"time"

	"github.com/Hackmty-Billyy/cargovigil-backend/config"
	"github.com/Hackmty-Billyy/cargovigil-backend/database"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/company"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/fuel"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/logistics"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/routecost"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/treasury"
	"github.com/Hackmty-Billyy/cargovigil-backend/jobs"
	"github.com/Hackmty-Billyy/cargovigil-backend/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// Every domain package queries through Bun. bunDB shares this same
	// pgxpool.Pool (via stdlib.OpenDBFromPool) — it does not open a second
	// connection pool. Schema stays owned exclusively by golang-migrate
	// (RunMigrations above); Bun is a pure query/mapping layer on top of it.
	bunDB := database.NewBunDB(pool)

	userRepo := auth.NewBunUserRepository(bunDB)
	roleRepo := auth.NewBunRoleRepository(bunDB)
	recoveryRepo := auth.NewBunRecoveryCodeRepository(bunDB)
	refreshRepo := auth.NewBunRefreshTokenRepository(bunDB)

	authService := auth.NewService(userRepo, roleRepo, recoveryRepo, refreshRepo, auth.ServiceConfig{
		JWTAccessSecret:   cfg.JWTAccessSecret,
		JWTMFASecret:      cfg.JWTMFASecret,
		TOTPEncryptionKey: cfg.TOTPEncryptionKey,
		RecoveryPepper:    cfg.RecoveryPepper,
		AccessTokenTTL:    cfg.AccessTokenTTL,
		RefreshTokenTTL:   cfg.RefreshTokenTTL,
		MFAPendingTTL:     cfg.MFAPendingTTL,
	})

	companyRepo := company.NewBunCompanyRepository(bunDB)
	vehicleRepo := company.NewBunVehicleRepository(bunDB)
	routeRepo := company.NewBunRouteRepository(bunDB)
	clientRepo := company.NewBunClientRepository(bunDB)
	contractRepo := company.NewBunContractRepository(bunDB)
	companyService := company.NewService(companyRepo, vehicleRepo, routeRepo, clientRepo, contractRepo, authService)

	treasuryRepo := treasury.NewBunRepository(bunDB)
	treasuryService := treasury.NewService(treasuryRepo, treasuryRepo, treasuryRepo, treasuryRepo, treasuryRepo)

	routeCostRepo := routecost.NewPostgresRepository(pool)
	routeCostService := routecost.NewService(routeCostRepo, routeCostRepo, routeCostRepo, routeCostRepo, routeCostRepo,
		routecost.FXRates{USDToMXN: cfg.FXUSDToMXN})

	fuelRepo := fuel.NewBunRepository(bunDB)
	fuelService := fuel.NewService(fuelRepo, fuelRepo, fuelRepo, fuelRepo)

	logisticsRepo := logistics.NewBunRepository(bunDB)
	logisticsService := logistics.NewService(logisticsRepo, routeCostService, fuelService,
		routecost.FXRates{USDToMXN: cfg.FXUSDToMXN})

	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	defer cancelCleanup()
	jobs.StartRefreshTokenCleanup(cleanupCtx, refreshRepo, time.Hour)
	jobs.StartTreasuryForecastJob(cleanupCtx, companyService, treasuryService, 24*time.Hour)
	jobs.StartRouteRiskJob(cleanupCtx, companyService, routeCostService, 24*time.Hour)
	jobs.StartFuelSurchargeRecalcJob(cleanupCtx, companyService, fuelService, 24*time.Hour)

	app := server.New(cfg, authService, companyService, treasuryService, routeCostService, fuelService, logisticsService)
	log.Fatal(app.Listen(":" + cfg.Port))
}
