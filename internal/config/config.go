package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Database
	DatabaseURL string
	DBPoolMax   int
	DBPoolMin   int
	DBTimeout   time.Duration

	// Server
	Port string

	// Auth (FB-Brain OIDC)
	BrainJWKSURL   string
	BrainIssuerURL string

	// Dev
	DevAuthBypass bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return &Config{
		DatabaseURL: dbURL,
		DBPoolMax:   getEnvInt("DB_POOL_MAX", 25),
		DBPoolMin:   getEnvInt("DB_POOL_MIN", 5),
		DBTimeout:   getEnvDuration("DB_TIMEOUT", 5*time.Second),

		Port: getEnvStr("PORT", "8080"),

		BrainJWKSURL:   os.Getenv("BRAIN_JWKS_URL"),
		BrainIssuerURL: os.Getenv("BRAIN_ISSUER_URL"),

		DevAuthBypass: getEnvBool("DEV_AUTH_BYPASS", false),
	}, nil
}

func getEnvStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
