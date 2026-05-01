package config

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
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

	// Auth (The Brain OIDC)
	BrainJWKSURL   string
	BrainIssuerURL string

	// DevAuthMode: "" = production (validate JWTs only), "header" = inject claims from X-Dev-Auth.
	// Never set to a non-empty value in production. Future hardening: build-tag gate the header path.
	DevAuthMode string

	// DefaultOrgID is the fallback tenant for A2A webhooks that arrive
	// without an explicit org_id in the envelope. Set in single-tenant
	// fork deployments. Co-op multi-tenant variants leave this empty
	// and require Brain to populate org_id on every event.
	DefaultOrgID *uuid.UUID

	// Outbound A2A — JWS-signed POST to Brain's webhook receiver.
	// Empty A2ASigningKeyPath disables outbound dispatch; the worker
	// falls back to a no-op (queued events drain but discard with a
	// warning log). All three fields must be set together.
	BrainOutboundURL  string // e.g. "https://brain.example/api/v1/a2a/webhook"; "" defaults to BrainIssuerURL+"/api/v1/a2a/webhook"
	A2ASigningKeyPath string // path to PKCS#1 or PKCS#8 PEM RSA private key
	A2AKeyID          string // JWS `kid` header value; defaults to "buildos-1"

	// Sentry — error reporting. Empty SentryDSN disables initialization
	// entirely; ops can ship without a DSN configured and add it
	// later without a code change.
	SentryDSN         string  // e.g. "https://<key>@<org>.ingest.sentry.io/<project>"; "" disables
	SentryEnvironment string  // "production" / "staging" / "dev"; defaults to "dev"
	SentryRelease     string  // build SHA or version tag; empty uses Sentry's auto-release
	SentryTracesRate  float64 // 0.0 disables performance tracing; defaults to 0.0

	// Rate limiting — per-IP token bucket. Stopgap until the unified
	// per-tenant credit system (Maestro + Brain coordinated) lands.
	// Defaults are deliberately permissive: 50 rps steady, 100 burst.
	RateLimitRPS   int // requests per second per IP; 0 = use default
	RateLimitBurst int // burst capacity per IP; 0 = use default

	// OpenTelemetry tracing. Empty Endpoint disables initialization;
	// the SDK falls back to a no-op tracer that's safe to call but
	// emits nothing. The W3C TraceContext propagator is set
	// regardless so trace_ids still flow over inbound headers and
	// outbound Brain calls — useful for log correlation even when
	// not exporting spans.
	OTelEndpoint    string  // OTLP-HTTP collector URL; "" disables
	OTelInsecure    bool    // allow plaintext HTTP to collector (loopback / sidecar only)
	OTelSampleRate  float64 // [0.0, 1.0]; defaults to 0.1 in InitTracing

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

// Load reads configuration from environment variables with sensible
// defaults. It's a thin wrapper around LoadWithSource that uses the
// CONFIG_SOURCE env var to select the secret source — empty / "env"
// keeps the legacy behavior, "file:/path" or "chain:..." routes
// secrets through the operator's chosen store.
//
// Secret-bearing fields (DATABASE_URL, BRAIN_*, A2A_*, SENTRY_DSN)
// resolve through the source. Non-sensitive scalars (DB_POOL_MAX,
// PORT, etc.) stay direct env reads; pushing those through a vault
// would add latency without security benefit.
func Load() (*Config, error) {
	src, err := LoadSecretSource(context.Background(), os.Getenv("CONFIG_SOURCE"))
	if err != nil {
		return nil, fmt.Errorf("config: load secret source: %w", err)
	}
	return LoadWithSource(context.Background(), src)
}

// LoadWithSource constructs Config with the supplied SecretSource.
// Useful for tests + for operators wiring a pre-constructed source
// (e.g. one that's already authenticated to Vault). The source is
// not closed by Load; the caller's responsibility.
func LoadWithSource(ctx context.Context, src SecretSource) (*Config, error) {
	dbURL, ok, err := src.LookupSecret(ctx, "DATABASE_URL")
	if err != nil {
		return nil, fmt.Errorf("config: lookup DATABASE_URL: %w", err)
	}
	if !ok || dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	// secret resolves a key through the source, returning "" on miss.
	// Used for fields where empty == disabled.
	secret := func(key string) string {
		v, ok, err := src.LookupSecret(ctx, key)
		if err != nil || !ok {
			return ""
		}
		return v
	}

	return &Config{
		DatabaseURL: dbURL,
		DBPoolMax:   getEnvInt("DB_POOL_MAX", 25),
		DBPoolMin:   getEnvInt("DB_POOL_MIN", 5),
		DBTimeout:   getEnvDuration("DB_TIMEOUT", 5*time.Second),

		Port: getEnvStr("PORT", "8080"),

		BrainJWKSURL:   secret("BRAIN_JWKS_URL"),
		BrainIssuerURL: secret("BRAIN_ISSUER_URL"),

		DevAuthMode:  getEnvStr("DEV_AUTH_MODE", ""),
		DefaultOrgID: parseOptionalUUID(os.Getenv("DEFAULT_ORG_ID")),

		BrainOutboundURL:  secret("BRAIN_OUTBOUND_URL"),
		A2ASigningKeyPath: secret("A2A_SIGNING_KEY_PATH"),
		A2AKeyID:          getEnvStr("A2A_KEY_ID", "buildos-1"),

		SentryDSN:         secret("SENTRY_DSN"),
		SentryEnvironment: getEnvStr("SENTRY_ENVIRONMENT", "dev"),
		SentryRelease:     os.Getenv("SENTRY_RELEASE"),
		SentryTracesRate:  getEnvFloat("SENTRY_TRACES_SAMPLE_RATE", 0.0),

		RateLimitRPS:   getEnvInt("RATE_LIMIT_RPS", 0),   // 0 → middleware default
		RateLimitBurst: getEnvInt("RATE_LIMIT_BURST", 0), // 0 → middleware default

		OTelEndpoint:   secret("OTEL_EXPORTER_OTLP_ENDPOINT"),
		OTelInsecure:   getEnvBool("OTEL_EXPORTER_OTLP_INSECURE", false),
		OTelSampleRate: getEnvFloat("OTEL_TRACES_SAMPLE_RATE", 0.0),

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

// parseOptionalUUID returns nil for an empty string. Malformed UUIDs
// also return nil — config loads silently rather than failing startup;
// the resulting nil triggers an explicit ErrInvalidInput at the first
// A2A webhook arrival, which is loud enough.
func parseOptionalUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}
