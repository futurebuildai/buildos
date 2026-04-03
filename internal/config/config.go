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

	// Physics Engine
	Physics PhysicsConfig
}

// PhysicsConfig holds tunable parameters for the CPM/DHSM physics engine.
// See CPM_RES_MODEL_SPEC.md Section 11.2.1
type PhysicsConfig struct {
	StandardHouseSizeSF    float64 // Default: 2000 sqft
	SizeAdjustmentExponent float64 // Default: 0.35
}

// WithDefaults returns a copy with zero values replaced by sensible defaults.
func (p PhysicsConfig) WithDefaults() PhysicsConfig {
	if p.StandardHouseSizeSF <= 0 {
		p.StandardHouseSizeSF = 2000.0
	}
	if p.SizeAdjustmentExponent <= 0 {
		p.SizeAdjustmentExponent = 0.35
	}
	return p
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

		Physics: PhysicsConfig{
			StandardHouseSizeSF:    getEnvFloat("PHYSICS_STANDARD_HOUSE_SF", 2000.0),
			SizeAdjustmentExponent: getEnvFloat("PHYSICS_SIZE_EXPONENT", 0.35),
		},
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

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
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
