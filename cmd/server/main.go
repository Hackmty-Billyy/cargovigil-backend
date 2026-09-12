package main

import (
	"context"
	"log"
	"time"

	"github.com/Hackmty-Billyy/cargovigil-backend/config"
	"github.com/Hackmty-Billyy/cargovigil-backend/database"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/company"
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

	userRepo := auth.NewPostgresUserRepository(pool)
	roleRepo := auth.NewPostgresRoleRepository(pool)
	recoveryRepo := auth.NewPostgresRecoveryCodeRepository(pool)
	refreshRepo := auth.NewPostgresRefreshTokenRepository(pool)

	authService := auth.NewService(userRepo, roleRepo, recoveryRepo, refreshRepo, auth.ServiceConfig{
		JWTAccessSecret:   cfg.JWTAccessSecret,
		JWTMFASecret:      cfg.JWTMFASecret,
		TOTPEncryptionKey: cfg.TOTPEncryptionKey,
		RecoveryPepper:    cfg.RecoveryPepper,
		AccessTokenTTL:    cfg.AccessTokenTTL,
		RefreshTokenTTL:   cfg.RefreshTokenTTL,
		MFAPendingTTL:     cfg.MFAPendingTTL,
	})

	companyRepo := company.NewPostgresCompanyRepository(pool)
	vehicleRepo := company.NewPostgresVehicleRepository(pool)
	routeRepo := company.NewPostgresRouteRepository(pool)
	clientRepo := company.NewPostgresClientRepository(pool)
	contractRepo := company.NewPostgresContractRepository(pool)
	companyService := company.NewService(companyRepo, vehicleRepo, routeRepo, clientRepo, contractRepo, authService)

	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	defer cancelCleanup()
	jobs.StartRefreshTokenCleanup(cleanupCtx, refreshRepo, time.Hour)

	app := server.New(cfg, authService, companyService)
	log.Fatal(app.Listen(":" + cfg.Port))
}
