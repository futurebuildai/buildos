package config

import (
	"context"
	"fmt"
	"net"
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

	// Native auth (WS1). BuildOS mints + validates its own RS256 JWTs
	// against a per-fork keypair; there is no external IdP. The PEM
	// material is secret-sourced; the issuer/audience are wire-protocol
	// constants defaulting to "buildos". Required unless
	// DevAuthMode=="header" (dev/CI rigs that inject claims directly).
	JWTPrivateKeyPEM string // PKCS#1 or PKCS#8 RSA private key PEM (secret)
	JWTPublicKeyPEM  string // PKIX or PKCS#1 RSA public key PEM (secret)
	JWTKeyID         string // JWS `kid` header; defaults to "buildos-1"
	JWTIssuer        string // iss claim; defaults to "buildos"
	JWTAudience      string // aud claim; defaults to "buildos"

	// AppBaseURL is the public base URL of the deployment, used to
	// build password-reset links in outbound email. Empty disables the
	// link (the reset token is still emailed bare for manual use).
	AppBaseURL string

	// Auth token lifetimes. 0 == use the AuthService defaults
	// (refresh 30d, reset 1h).
	AuthRefreshTTL time.Duration
	AuthResetTTL   time.Duration

	// Vault (WS3) — encrypted BYOK credential store. VaultMasterKey is
	// a standard-base64 32-byte AES-256 key (secret). Empty disables
	// the vault entirely: no AI, no mailer, no /integrations routes
	// (soft-fail — the server still boots and serves the core domain).
	VaultMasterKey  string
	VaultKeyVersion int // key-rotation version stamped on sealed rows; defaults to 1

	// Mailer (WS3) — Resend transactional email. The Resend API key is
	// set in-app by the owner via the vault (provider="resend"), NOT
	// here; these only configure the static sender identity.
	MailFrom     string // From: address (e.g. "noreply@acme.example")
	MailFromName string // From display name; defaults to "BuildOS"

	// DevAuthMode: "" = production (validate JWTs only), "header" = inject claims from X-Dev-Auth.
	// Never set to a non-empty value in production. Future hardening: build-tag gate the header path.
	DevAuthMode string

	// BootstrapToken is the one-shot owner-claim cleartext that
	// cmd/server materializes into setup_bootstrap_tokens at boot.
	// Empty == no seeding (fork operator may issue a token via
	// cmd/buildos-fork-init instead). Format: 43 chars base64url
	// (32 random bytes, no padding). See docs/fork-onboarding.md.
	BootstrapToken string

	// Sentry — error reporting. Empty SentryDSN disables initialization
	// entirely; ops can ship without a DSN configured and add it
	// later without a code change.
	SentryDSN         string  // e.g. "https://<key>@<org>.ingest.sentry.io/<project>"; "" disables
	SentryEnvironment string  // "production" / "staging" / "dev"; defaults to "dev"
	SentryRelease     string  // build SHA or version tag; empty uses Sentry's auto-release
	SentryTracesRate  float64 // 0.0 disables performance tracing; defaults to 0.0

	// Rate limiting — per-IP token bucket. Defaults are deliberately
	// permissive: 50 rps steady, 100 burst.
	RateLimitRPS   int // requests per second per IP; 0 = use default
	RateLimitBurst int // burst capacity per IP; 0 = use default

	// TrustedProxyCIDRs gates whether X-Forwarded-For is trusted (so the per-IP
	// rate limiter keys on a client-forgeable header). Empty (default) = ignore
	// XFF and use the real TCP peer. Set to the LB/ingress subnet(s) only when
	// an upstream you control terminates the connection. Parsed from the
	// comma-separated TRUSTED_PROXY_CIDRS env.
	TrustedProxyCIDRs []*net.IPNet

	// OpenTelemetry tracing. Empty Endpoint disables initialization;
	// the SDK falls back to a no-op tracer that's safe to call but
	// emits nothing. The W3C TraceContext propagator is set
	// regardless so trace_ids still flow over inbound headers and
	// outbound HTTP calls — useful for log correlation even when
	// not exporting spans.
	OTelEndpoint   string  // OTLP-HTTP collector URL; "" disables
	OTelInsecure   bool    // allow plaintext HTTP to collector (loopback / sidecar only)
	OTelSampleRate float64 // [0.0, 1.0]; defaults to 0.1 in InitTracing

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
// Secret-bearing fields (DATABASE_URL, JWT_*, VAULT_MASTER_KEY,
// SENTRY_DSN) resolve through the source. Non-sensitive scalars
// (DB_POOL_MAX, PORT, etc.) stay direct env reads; pushing those
// through a vault would add latency without security benefit.
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

		JWTPrivateKeyPEM: secret("JWT_PRIVATE_KEY_PEM"),
		JWTPublicKeyPEM:  secret("JWT_PUBLIC_KEY_PEM"),
		JWTKeyID:         getEnvStr("JWT_KEY_ID", "buildos-1"),
		JWTIssuer:        getEnvStr("JWT_ISSUER", "buildos"),
		JWTAudience:      getEnvStr("JWT_AUDIENCE", "buildos"),

		AppBaseURL: getEnvStr("APP_BASE_URL", ""),

		AuthRefreshTTL: getEnvDuration("AUTH_REFRESH_TTL", 0),
		AuthResetTTL:   getEnvDuration("AUTH_RESET_TTL", 0),

		VaultMasterKey:  secret("VAULT_MASTER_KEY"),
		VaultKeyVersion: getEnvInt("VAULT_KEY_VERSION", 1),

		MailFrom:     getEnvStr("MAIL_FROM", ""),
		MailFromName: getEnvStr("MAIL_FROM_NAME", "BuildOS"),

		DevAuthMode: getEnvStr("DEV_AUTH_MODE", ""),

		BootstrapToken: secret("BUILDOS_BOOTSTRAP_TOKEN"),

		SentryDSN:         secret("SENTRY_DSN"),
		SentryEnvironment: getEnvStr("SENTRY_ENVIRONMENT", "dev"),
		SentryRelease:     os.Getenv("SENTRY_RELEASE"),
		SentryTracesRate:  getEnvFloat("SENTRY_TRACES_SAMPLE_RATE", 0.0),

		RateLimitRPS:      getEnvInt("RATE_LIMIT_RPS", 0),   // 0 → middleware default
		RateLimitBurst:    getEnvInt("RATE_LIMIT_BURST", 0), // 0 → middleware default
		TrustedProxyCIDRs: parseCIDRs(getEnvStr("TRUSTED_PROXY_CIDRS", "")),

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

// parseCIDRs parses a comma-separated list of CIDRs (e.g. "10.0.0.0/8,::1/128").
// Invalid entries are skipped (a typo must not silently widen trust — it just
// drops that entry, leaving the fail-safe "ignore XFF" default narrower).
func parseCIDRs(s string) []*net.IPNet {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			out = append(out, n)
		}
	}
	return out
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
