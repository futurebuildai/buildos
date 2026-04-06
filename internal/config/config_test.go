package config

import (
	"os"
	"testing"
	"time"
)

// =============================================================================
// Load() Tests
// =============================================================================

func TestLoad_MissingDatabaseURL(t *testing.T) {
	// Ensure DATABASE_URL is unset
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
	if err.Error() != "DATABASE_URL is required" {
		t.Errorf("error = %q, want 'DATABASE_URL is required'", err.Error())
	}
}

func TestLoad_WithDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/testdb")
	// Clear all optional vars to test defaults
	clearOptionalEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost:5432/testdb" {
		t.Errorf("DatabaseURL = %q, want 'postgres://localhost:5432/testdb'", cfg.DatabaseURL)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/testdb")
	clearOptionalEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Database defaults
	if cfg.DBPoolMax != 25 {
		t.Errorf("DBPoolMax = %d, want 25", cfg.DBPoolMax)
	}
	if cfg.DBPoolMin != 5 {
		t.Errorf("DBPoolMin = %d, want 5", cfg.DBPoolMin)
	}
	if cfg.DBTimeout != 5*time.Second {
		t.Errorf("DBTimeout = %v, want 5s", cfg.DBTimeout)
	}

	// Server defaults
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want '8080'", cfg.Port)
	}

	// Auth defaults (empty strings)
	if cfg.BrainJWKSURL != "" {
		t.Errorf("BrainJWKSURL = %q, want empty", cfg.BrainJWKSURL)
	}
	if cfg.BrainIssuerURL != "" {
		t.Errorf("BrainIssuerURL = %q, want empty", cfg.BrainIssuerURL)
	}

	// Dev defaults
	if cfg.DevAuthBypass {
		t.Error("DevAuthBypass should default to false")
	}

	// Security defaults
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("CORSAllowedOrigins = %v, want [http://localhost:3000]", cfg.CORSAllowedOrigins)
	}
	if cfg.RateLimitRPS != 100 {
		t.Errorf("RateLimitRPS = %f, want 100", cfg.RateLimitRPS)
	}
	if cfg.RateLimitBurst != 200 {
		t.Errorf("RateLimitBurst = %d, want 200", cfg.RateLimitBurst)
	}

	// AI defaults
	if cfg.AnthropicAPIKey != "" {
		t.Errorf("AnthropicAPIKey = %q, want empty", cfg.AnthropicAPIKey)
	}

	// FutureShade default
	if cfg.FutureShadeEnabled {
		t.Error("FutureShadeEnabled should default to false")
	}

	// A2A defaults
	if cfg.A2ATargetURL != "http://localhost:8082/api/v1/a2a/webhook" {
		t.Errorf("A2ATargetURL = %q, want default URL", cfg.A2ATargetURL)
	}
	if cfg.A2ASigningKeyPath != "" {
		t.Errorf("A2ASigningKeyPath = %q, want empty", cfg.A2ASigningKeyPath)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://custom:5433/mydb")
	t.Setenv("DB_POOL_MAX", "50")
	t.Setenv("DB_POOL_MIN", "10")
	t.Setenv("DB_TIMEOUT", "10s")
	t.Setenv("PORT", "9090")
	t.Setenv("BRAIN_JWKS_URL", "https://brain.local/.well-known/jwks.json")
	t.Setenv("BRAIN_ISSUER_URL", "https://brain.local")
	t.Setenv("DEV_AUTH_BYPASS", "true")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")
	t.Setenv("RATE_LIMIT_RPS", "50.5")
	t.Setenv("RATE_LIMIT_BURST", "100")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("FUTURESHADE_ENABLED", "true")
	t.Setenv("A2A_TARGET_URL", "https://brain.prod/api/v1/a2a/webhook")
	t.Setenv("A2A_SIGNING_KEY_PATH", "/etc/keys/a2a.pem")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DBPoolMax != 50 {
		t.Errorf("DBPoolMax = %d, want 50", cfg.DBPoolMax)
	}
	if cfg.DBPoolMin != 10 {
		t.Errorf("DBPoolMin = %d, want 10", cfg.DBPoolMin)
	}
	if cfg.DBTimeout != 10*time.Second {
		t.Errorf("DBTimeout = %v, want 10s", cfg.DBTimeout)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want '9090'", cfg.Port)
	}
	if cfg.BrainJWKSURL != "https://brain.local/.well-known/jwks.json" {
		t.Errorf("BrainJWKSURL = %q", cfg.BrainJWKSURL)
	}
	if cfg.BrainIssuerURL != "https://brain.local" {
		t.Errorf("BrainIssuerURL = %q", cfg.BrainIssuerURL)
	}
	if !cfg.DevAuthBypass {
		t.Error("DevAuthBypass should be true")
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("CORSAllowedOrigins length = %d, want 2", len(cfg.CORSAllowedOrigins))
	}
	if cfg.CORSAllowedOrigins[0] != "https://app.example.com" {
		t.Errorf("CORSAllowedOrigins[0] = %q", cfg.CORSAllowedOrigins[0])
	}
	if cfg.CORSAllowedOrigins[1] != "https://admin.example.com" {
		t.Errorf("CORSAllowedOrigins[1] = %q", cfg.CORSAllowedOrigins[1])
	}
	if cfg.RateLimitRPS != 50.5 {
		t.Errorf("RateLimitRPS = %f, want 50.5", cfg.RateLimitRPS)
	}
	if cfg.RateLimitBurst != 100 {
		t.Errorf("RateLimitBurst = %d, want 100", cfg.RateLimitBurst)
	}
	if cfg.AnthropicAPIKey != "sk-ant-test-key" {
		t.Errorf("AnthropicAPIKey = %q, want 'sk-ant-test-key'", cfg.AnthropicAPIKey)
	}
	if !cfg.FutureShadeEnabled {
		t.Error("FutureShadeEnabled should be true")
	}
	if cfg.A2ATargetURL != "https://brain.prod/api/v1/a2a/webhook" {
		t.Errorf("A2ATargetURL = %q", cfg.A2ATargetURL)
	}
	if cfg.A2ASigningKeyPath != "/etc/keys/a2a.pem" {
		t.Errorf("A2ASigningKeyPath = %q", cfg.A2ASigningKeyPath)
	}
}

// =============================================================================
// getEnvInt Tests
// =============================================================================

func TestGetEnvInt_WithValidValue(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	got := getEnvInt("TEST_INT", 0)
	if got != 42 {
		t.Errorf("getEnvInt = %d, want 42", got)
	}
}

func TestGetEnvInt_WithInvalidValue(t *testing.T) {
	t.Setenv("TEST_INT", "not-a-number")
	got := getEnvInt("TEST_INT", 99)
	if got != 99 {
		t.Errorf("getEnvInt with invalid value = %d, want fallback 99", got)
	}
}

func TestGetEnvInt_WithEmptyValue(t *testing.T) {
	t.Setenv("TEST_INT", "")
	got := getEnvInt("TEST_INT", 77)
	if got != 77 {
		t.Errorf("getEnvInt with empty value = %d, want fallback 77", got)
	}
}

func TestGetEnvInt_WithUnsetVar(t *testing.T) {
	os.Unsetenv("TEST_INT_UNSET")
	got := getEnvInt("TEST_INT_UNSET", 55)
	if got != 55 {
		t.Errorf("getEnvInt with unset var = %d, want fallback 55", got)
	}
}

// =============================================================================
// getEnvBool Tests
// =============================================================================

func TestGetEnvBool_True(t *testing.T) {
	for _, v := range []string{"true", "1", "TRUE", "True"} {
		t.Setenv("TEST_BOOL", v)
		got := getEnvBool("TEST_BOOL", false)
		if !got {
			t.Errorf("getEnvBool(%q) = false, want true", v)
		}
	}
}

func TestGetEnvBool_False(t *testing.T) {
	for _, v := range []string{"false", "0", "FALSE", "False"} {
		t.Setenv("TEST_BOOL", v)
		got := getEnvBool("TEST_BOOL", true)
		if got {
			t.Errorf("getEnvBool(%q) = true, want false", v)
		}
	}
}

func TestGetEnvBool_Invalid(t *testing.T) {
	t.Setenv("TEST_BOOL", "maybe")
	got := getEnvBool("TEST_BOOL", true)
	if !got {
		t.Error("getEnvBool with invalid value should return fallback (true)")
	}
}

func TestGetEnvBool_Empty(t *testing.T) {
	t.Setenv("TEST_BOOL", "")
	got := getEnvBool("TEST_BOOL", true)
	if !got {
		t.Error("getEnvBool with empty value should return fallback (true)")
	}
}

// =============================================================================
// getEnvFloat64 Tests
// =============================================================================

func TestGetEnvFloat64_Valid(t *testing.T) {
	t.Setenv("TEST_FLOAT", "3.14")
	got := getEnvFloat64("TEST_FLOAT", 0)
	if got != 3.14 {
		t.Errorf("getEnvFloat64 = %f, want 3.14", got)
	}
}

func TestGetEnvFloat64_Integer(t *testing.T) {
	t.Setenv("TEST_FLOAT", "100")
	got := getEnvFloat64("TEST_FLOAT", 0)
	if got != 100.0 {
		t.Errorf("getEnvFloat64 = %f, want 100.0", got)
	}
}

func TestGetEnvFloat64_Invalid(t *testing.T) {
	t.Setenv("TEST_FLOAT", "not-a-float")
	got := getEnvFloat64("TEST_FLOAT", 9.99)
	if got != 9.99 {
		t.Errorf("getEnvFloat64 with invalid value = %f, want fallback 9.99", got)
	}
}

func TestGetEnvFloat64_Empty(t *testing.T) {
	t.Setenv("TEST_FLOAT", "")
	got := getEnvFloat64("TEST_FLOAT", 1.5)
	if got != 1.5 {
		t.Errorf("getEnvFloat64 with empty value = %f, want fallback 1.5", got)
	}
}

// =============================================================================
// getEnvDuration Tests
// =============================================================================

func TestGetEnvDuration_Valid(t *testing.T) {
	t.Setenv("TEST_DUR", "30s")
	got := getEnvDuration("TEST_DUR", time.Second)
	if got != 30*time.Second {
		t.Errorf("getEnvDuration = %v, want 30s", got)
	}
}

func TestGetEnvDuration_Minutes(t *testing.T) {
	t.Setenv("TEST_DUR", "5m")
	got := getEnvDuration("TEST_DUR", time.Second)
	if got != 5*time.Minute {
		t.Errorf("getEnvDuration = %v, want 5m", got)
	}
}

func TestGetEnvDuration_Invalid(t *testing.T) {
	t.Setenv("TEST_DUR", "not-a-duration")
	got := getEnvDuration("TEST_DUR", 7*time.Second)
	if got != 7*time.Second {
		t.Errorf("getEnvDuration with invalid value = %v, want fallback 7s", got)
	}
}

func TestGetEnvDuration_Empty(t *testing.T) {
	t.Setenv("TEST_DUR", "")
	got := getEnvDuration("TEST_DUR", 3*time.Second)
	if got != 3*time.Second {
		t.Errorf("getEnvDuration with empty value = %v, want fallback 3s", got)
	}
}

// =============================================================================
// getEnvStringSlice Tests
// =============================================================================

func TestGetEnvStringSlice_CommaSeparated(t *testing.T) {
	t.Setenv("TEST_SLICE", "a,b,c")
	got := getEnvStringSlice("TEST_SLICE", nil)
	if len(got) != 3 {
		t.Fatalf("length = %d, want 3", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("got %v, want [a b c]", got)
	}
}

func TestGetEnvStringSlice_WithSpaces(t *testing.T) {
	t.Setenv("TEST_SLICE", " a , b , c ")
	got := getEnvStringSlice("TEST_SLICE", nil)
	if len(got) != 3 {
		t.Fatalf("length = %d, want 3", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("got %v, want [a b c] (trimmed)", got)
	}
}

func TestGetEnvStringSlice_SingleValue(t *testing.T) {
	t.Setenv("TEST_SLICE", "only-one")
	got := getEnvStringSlice("TEST_SLICE", nil)
	if len(got) != 1 {
		t.Fatalf("length = %d, want 1", len(got))
	}
	if got[0] != "only-one" {
		t.Errorf("got %v, want [only-one]", got)
	}
}

func TestGetEnvStringSlice_Empty(t *testing.T) {
	t.Setenv("TEST_SLICE", "")
	fallback := []string{"default"}
	got := getEnvStringSlice("TEST_SLICE", fallback)
	if len(got) != 1 || got[0] != "default" {
		t.Errorf("got %v, want fallback [default]", got)
	}
}

func TestGetEnvStringSlice_OnlyCommas(t *testing.T) {
	t.Setenv("TEST_SLICE", ",,")
	fallback := []string{"fallback"}
	got := getEnvStringSlice("TEST_SLICE", fallback)
	// All empty after trimming => fallback
	if len(got) != 1 || got[0] != "fallback" {
		t.Errorf("got %v, want fallback [fallback]", got)
	}
}

func TestGetEnvStringSlice_Unset(t *testing.T) {
	os.Unsetenv("TEST_SLICE_UNSET")
	fallback := []string{"a", "b"}
	got := getEnvStringSlice("TEST_SLICE_UNSET", fallback)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want fallback [a b]", got)
	}
}

// =============================================================================
// getEnvStr Tests
// =============================================================================

func TestGetEnvStr_WithValue(t *testing.T) {
	t.Setenv("TEST_STR", "custom")
	got := getEnvStr("TEST_STR", "default")
	if got != "custom" {
		t.Errorf("getEnvStr = %q, want 'custom'", got)
	}
}

func TestGetEnvStr_Empty(t *testing.T) {
	t.Setenv("TEST_STR", "")
	got := getEnvStr("TEST_STR", "default")
	if got != "default" {
		t.Errorf("getEnvStr with empty = %q, want 'default'", got)
	}
}

func TestGetEnvStr_Unset(t *testing.T) {
	os.Unsetenv("TEST_STR_UNSET")
	got := getEnvStr("TEST_STR_UNSET", "fallback")
	if got != "fallback" {
		t.Errorf("getEnvStr unset = %q, want 'fallback'", got)
	}
}

// =============================================================================
// PhysicsConfig Tests
// =============================================================================

func TestDefaultPhysicsConfig(t *testing.T) {
	cfg := DefaultPhysicsConfig()
	if cfg.StandardHouseSizeSF != 2250.0 {
		t.Errorf("StandardHouseSizeSF = %f, want 2250.0", cfg.StandardHouseSizeSF)
	}
	if cfg.SizeAdjustmentExponent != 0.75 {
		t.Errorf("SizeAdjustmentExponent = %f, want 0.75", cfg.SizeAdjustmentExponent)
	}
	if cfg.ConfigVersion != "default-v1" {
		t.Errorf("ConfigVersion = %q, want 'default-v1'", cfg.ConfigVersion)
	}
}

func TestPhysicsConfig_WithDefaults_ZeroValues(t *testing.T) {
	cfg := PhysicsConfig{} // All zero values
	result := cfg.WithDefaults()

	if result.StandardHouseSizeSF != 2250.0 {
		t.Errorf("StandardHouseSizeSF = %f, want 2250.0", result.StandardHouseSizeSF)
	}
	if result.SizeAdjustmentExponent != 0.75 {
		t.Errorf("SizeAdjustmentExponent = %f, want 0.75", result.SizeAdjustmentExponent)
	}
	if result.ConfigVersion != "default-v1" {
		t.Errorf("ConfigVersion = %q, want 'default-v1'", result.ConfigVersion)
	}
}

func TestPhysicsConfig_WithDefaults_CustomValues(t *testing.T) {
	cfg := PhysicsConfig{
		StandardHouseSizeSF:    3000.0,
		SizeAdjustmentExponent: 0.80,
		ConfigVersion:          "custom-v2",
	}
	result := cfg.WithDefaults()

	if result.StandardHouseSizeSF != 3000.0 {
		t.Errorf("StandardHouseSizeSF = %f, want 3000.0 (custom)", result.StandardHouseSizeSF)
	}
	if result.SizeAdjustmentExponent != 0.80 {
		t.Errorf("SizeAdjustmentExponent = %f, want 0.80 (custom)", result.SizeAdjustmentExponent)
	}
	if result.ConfigVersion != "custom-v2" {
		t.Errorf("ConfigVersion = %q, want 'custom-v2' (custom)", result.ConfigVersion)
	}
}

func TestPhysicsConfig_WithDefaults_PartialOverride(t *testing.T) {
	cfg := PhysicsConfig{
		StandardHouseSizeSF: 1800.0,
		// SizeAdjustmentExponent is 0 => should use default
		// ConfigVersion is "" => should use default
	}
	result := cfg.WithDefaults()

	if result.StandardHouseSizeSF != 1800.0 {
		t.Errorf("StandardHouseSizeSF = %f, want 1800.0 (custom)", result.StandardHouseSizeSF)
	}
	if result.SizeAdjustmentExponent != 0.75 {
		t.Errorf("SizeAdjustmentExponent = %f, want 0.75 (default)", result.SizeAdjustmentExponent)
	}
	if result.ConfigVersion != "default-v1" {
		t.Errorf("ConfigVersion = %q, want 'default-v1' (default)", result.ConfigVersion)
	}
}

func TestPhysicsConfig_WithDefaults_NegativeValues(t *testing.T) {
	cfg := PhysicsConfig{
		StandardHouseSizeSF:    -100.0,
		SizeAdjustmentExponent: -0.5,
	}
	result := cfg.WithDefaults()

	// Negative values should be replaced with defaults
	if result.StandardHouseSizeSF != 2250.0 {
		t.Errorf("StandardHouseSizeSF = %f, want 2250.0 (default for negative)", result.StandardHouseSizeSF)
	}
	if result.SizeAdjustmentExponent != 0.75 {
		t.Errorf("SizeAdjustmentExponent = %f, want 0.75 (default for negative)", result.SizeAdjustmentExponent)
	}
}

// =============================================================================
// Helper
// =============================================================================

func clearOptionalEnvVars(t *testing.T) {
	t.Helper()
	vars := []string{
		"DB_POOL_MAX", "DB_POOL_MIN", "DB_TIMEOUT",
		"PORT", "BRAIN_JWKS_URL", "BRAIN_ISSUER_URL",
		"DEV_AUTH_BYPASS", "CORS_ALLOWED_ORIGINS",
		"RATE_LIMIT_RPS", "RATE_LIMIT_BURST",
		"ANTHROPIC_API_KEY", "FUTURESHADE_ENABLED",
		"A2A_TARGET_URL", "A2A_SIGNING_KEY_PATH",
	}
	for _, v := range vars {
		t.Setenv(v, "")
	}
}
