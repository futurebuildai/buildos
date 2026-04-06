package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// Security
	CORSAllowedOrigins []string
	RateLimitRPS       float64
	RateLimitBurst     int

	// AI (Anthropic Claude)
	AnthropicAPIKey string // ANTHROPIC_API_KEY env var (required for AI features)

	// FutureShade (autonomous execution engine)
	FutureShadeEnabled bool // FUTURESHADE_ENABLED env var (default: false)

	// A2A (OS -> Brain reverse webhooks)
	A2ATargetURL     string // A2A_TARGET_URL env var (Brain's webhook endpoint)
	A2ASigningKeyPath string // A2A_SIGNING_KEY_PATH env var (path to RS256 private key PEM)
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

		CORSAllowedOrigins: getEnvStringSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		RateLimitRPS:       getEnvFloat64("RATE_LIMIT_RPS", 100),
		RateLimitBurst:     getEnvInt("RATE_LIMIT_BURST", 200),

		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		FutureShadeEnabled: getEnvBool("FUTURESHADE_ENABLED", false),

		A2ATargetURL:      getEnvStr("A2A_TARGET_URL", "http://localhost:8082/api/v1/a2a/webhook"),
		A2ASigningKeyPath: os.Getenv("A2A_SIGNING_KEY_PATH"),
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

func getEnvFloat64(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// =============================================================================
// Physics Engine Configuration
// =============================================================================

// PhysicsConfig holds tunable physics engine parameters.
// These control the DHSM (Duration & House Size Model) calculations.
// See CPM_RES_MODEL_SPEC.md Section 11.2.1
type PhysicsConfig struct {
	// StandardHouseSizeSF is the baseline GSF where SAF = 1.0.
	// Default: 2250.0 square feet.
	StandardHouseSizeSF float64

	// SizeAdjustmentExponent is the power curve for duration scaling.
	// SAF = (GSF / StandardHouseSizeSF) ^ SizeAdjustmentExponent
	// Default: 0.75
	SizeAdjustmentExponent float64

	// ConfigVersion for audit traceability.
	// Logged when schedules are calculated to track which config was used.
	ConfigVersion string
}

// DefaultPhysicsConfig returns a PhysicsConfig with safe production defaults.
// FAANG Threshold: Zero-value safety - if config is unset, use sensible defaults.
func DefaultPhysicsConfig() PhysicsConfig {
	return PhysicsConfig{
		StandardHouseSizeSF:    2250.0,
		SizeAdjustmentExponent: 0.75,
		ConfigVersion:          "default-v1",
	}
}

// WithDefaults returns a PhysicsConfig with zero values replaced by defaults.
func (c PhysicsConfig) WithDefaults() PhysicsConfig {
	defaults := DefaultPhysicsConfig()
	if c.StandardHouseSizeSF <= 0 {
		c.StandardHouseSizeSF = defaults.StandardHouseSizeSF
	}
	if c.SizeAdjustmentExponent <= 0 {
		c.SizeAdjustmentExponent = defaults.SizeAdjustmentExponent
	}
	if c.ConfigVersion == "" {
		c.ConfigVersion = defaults.ConfigVersion
	}
	return c
}

func getEnvStringSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return fallback
}
