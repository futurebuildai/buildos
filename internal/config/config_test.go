package config

import (
	"context"
	"testing"
	"time"
)

func TestPhysicsConfig_WithDefaults(t *testing.T) {
	// Zero values get the documented CPM defaults...
	got := PhysicsConfig{}.WithDefaults()
	if got.StandardHouseSizeSF != 2000.0 {
		t.Errorf("StandardHouseSizeSF = %v, want 2000", got.StandardHouseSizeSF)
	}
	if got.SizeAdjustmentExponent != 0.35 {
		t.Errorf("SizeAdjustmentExponent = %v, want 0.35", got.SizeAdjustmentExponent)
	}

	// ...while explicit non-zero values are preserved.
	custom := PhysicsConfig{StandardHouseSizeSF: 3200, SizeAdjustmentExponent: 0.5}.WithDefaults()
	if custom.StandardHouseSizeSF != 3200 || custom.SizeAdjustmentExponent != 0.5 {
		t.Errorf("custom values clobbered: %+v", custom)
	}
}

func TestLoad_HappyPathFromEnv(t *testing.T) {
	// CONFIG_SOURCE unset → EnvSecretSource; DATABASE_URL present →
	// Load resolves and returns the full Config with scalar defaults.
	t.Setenv("CONFIG_SOURCE", "")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5433/buildos")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://u:p@localhost:5433/buildos" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	// Spot-check a couple of the documented defaults.
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.DBPoolMax != 25 {
		t.Errorf("DBPoolMax = %d, want 25", cfg.DBPoolMax)
	}
	// Physics defaults flow through getEnvFloat fallbacks.
	if cfg.Physics.StandardHouseSizeSF != 2000.0 {
		t.Errorf("Physics.StandardHouseSizeSF = %v, want 2000", cfg.Physics.StandardHouseSizeSF)
	}
}

func TestLoad_UnknownConfigSourceErrors(t *testing.T) {
	// A malformed CONFIG_SOURCE fails fast at boot (LoadSecretSource
	// rejects unknown prefixes) — Load surfaces that, never a partial
	// Config.
	t.Setenv("CONFIG_SOURCE", "bogus:nope")
	if _, err := Load(); err == nil {
		t.Fatal("Load with unknown CONFIG_SOURCE = nil error, want failure")
	}
}

func TestLoadWithSource_ParsesScalarEnv(t *testing.T) {
	// Drive the getEnvInt/Duration/Bool/Float success branches by
	// supplying valid non-default values through the env source.
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5433/buildos")
	t.Setenv("DB_POOL_MAX", "42")
	t.Setenv("DB_TIMEOUT", "9s")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	t.Setenv("SENTRY_TRACES_SAMPLE_RATE", "0.25")

	cfg, err := LoadWithSource(context.Background(), NewEnvSecretSource())
	if err != nil {
		t.Fatalf("LoadWithSource: %v", err)
	}
	if cfg.DBPoolMax != 42 {
		t.Errorf("DBPoolMax = %d, want 42", cfg.DBPoolMax)
	}
	if cfg.DBTimeout != 9*time.Second {
		t.Errorf("DBTimeout = %v, want 9s", cfg.DBTimeout)
	}
	if !cfg.OTelInsecure {
		t.Error("OTelInsecure = false, want true")
	}
	if cfg.SentryTracesRate != 0.25 {
		t.Errorf("SentryTracesRate = %v, want 0.25", cfg.SentryTracesRate)
	}
}

func TestLoadWithSource_MissingDatabaseURLErrors(t *testing.T) {
	// DATABASE_URL is the one hard-required secret; absent → error.
	t.Setenv("DATABASE_URL", "")
	if _, err := LoadWithSource(context.Background(), NewEnvSecretSource()); err == nil {
		t.Fatal("LoadWithSource with no DATABASE_URL = nil error, want failure")
	}
}
