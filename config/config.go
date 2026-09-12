package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port string

	DatabaseURL string

	JWTAccessSecret   []byte
	JWTMFASecret      []byte
	TOTPEncryptionKey []byte
	RecoveryPepper    string

	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	MFAPendingTTL   time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load() // best-effort: real deployments set env vars directly

	dbURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	accessSecret, err := requireEnv("JWT_ACCESS_SECRET")
	if err != nil {
		return nil, err
	}
	mfaSecret, err := requireEnv("JWT_MFA_SECRET")
	if err != nil {
		return nil, err
	}
	totpKeyHex, err := requireEnv("TOTP_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	recoveryPepper, err := requireEnv("RECOVERY_CODE_PEPPER")
	if err != nil {
		return nil, err
	}

	totpKey, err := hex.DecodeString(totpKeyHex)
	if err != nil {
		return nil, fmt.Errorf("config: TOTP_ENCRYPTION_KEY must be hex-encoded: %w", err)
	}
	if len(totpKey) != 32 {
		return nil, fmt.Errorf("config: TOTP_ENCRYPTION_KEY must decode to 32 bytes for AES-256, got %d", len(totpKey))
	}

	return &Config{
		Port:              getEnvOrDefault("PORT", "8080"),
		DatabaseURL:       dbURL,
		JWTAccessSecret:   []byte(accessSecret),
		JWTMFASecret:      []byte(mfaSecret),
		TOTPEncryptionKey: totpKey,
		RecoveryPepper:    recoveryPepper,
		AccessTokenTTL:    durationOrDefault("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:   durationOrDefault("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		MFAPendingTTL:     durationOrDefault("MFA_PENDING_TTL", 5*time.Minute),
	}, nil
}

func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("config: required environment variable %s is not set", key)
	}
	return v, nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
