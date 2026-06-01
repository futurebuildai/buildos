package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvSecretSource_HitMiss(t *testing.T) {
	t.Setenv("BUILDOS_TEST_SECRET", "value-1")
	t.Setenv("BUILDOS_TEST_EMPTY", "")

	s := NewEnvSecretSource()
	ctx := context.Background()

	t.Run("hit", func(t *testing.T) {
		v, ok, err := s.LookupSecret(ctx, "BUILDOS_TEST_SECRET")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !ok || v != "value-1" {
			t.Errorf("got (%q, %v), want (\"value-1\", true)", v, ok)
		}
	})

	t.Run("missing", func(t *testing.T) {
		_, ok, err := s.LookupSecret(ctx, "BUILDOS_TEST_NOT_SET")
		if err != nil {
			t.Errorf("missing key should not error: %v", err)
		}
		if ok {
			t.Errorf("missing key reported hit")
		}
	})

	t.Run("empty treated as miss", func(t *testing.T) {
		// Env vars set to "" are operator typos. Treat as miss so
		// callers fall through to defaults instead of injecting an
		// empty string.
		_, ok, _ := s.LookupSecret(ctx, "BUILDOS_TEST_EMPTY")
		if ok {
			t.Errorf("empty env value should miss, not hit")
		}
	})

	if s.Name() != "env" {
		t.Errorf("Name = %q, want env", s.Name())
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}

func TestFileSecretSource_HitMiss(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "BRAIN_DSN"), []byte("postgres://hidden\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "EMPTY_KEY"), []byte("\n\n"), 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}

	s, err := NewFileSecretSource(dir)
	if err != nil {
		t.Fatalf("NewFileSecretSource: %v", err)
	}
	ctx := context.Background()

	t.Run("hit strips trailing whitespace", func(t *testing.T) {
		v, ok, err := s.LookupSecret(ctx, "BRAIN_DSN")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !ok || v != "postgres://hidden" {
			t.Errorf("got (%q, %v), want (\"postgres://hidden\", true)", v, ok)
		}
	})

	t.Run("missing file is a clean miss", func(t *testing.T) {
		_, ok, err := s.LookupSecret(ctx, "DOES_NOT_EXIST")
		if err != nil {
			t.Errorf("missing file should not error: %v", err)
		}
		if ok {
			t.Errorf("missing file reported hit")
		}
	})

	t.Run("whitespace-only file is treated as miss", func(t *testing.T) {
		_, ok, _ := s.LookupSecret(ctx, "EMPTY_KEY")
		if ok {
			t.Errorf("whitespace-only file should miss, not hit")
		}
	})

	if s.Name() != "file" {
		t.Errorf("Name = %q, want file", s.Name())
	}
}

func TestNewFileSecretSource_RejectsBadInputs(t *testing.T) {
	if _, err := NewFileSecretSource(""); err == nil {
		t.Error("empty dir should error")
	}
	if _, err := NewFileSecretSource("/nonexistent-path-d3e4-test"); err == nil {
		t.Error("nonexistent dir should error")
	}

	// Path that exists but is a file, not a directory.
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "regular-file")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := NewFileSecretSource(filePath); err == nil {
		t.Error("file (not dir) should error")
	}
}

func TestChainSecretSource_FirstHitWins(t *testing.T) {
	t.Setenv("OVERLAP_KEY", "from-env")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "OVERLAP_KEY"), []byte("from-file"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "FILE_ONLY"), []byte("from-file-only"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("ENV_ONLY", "from-env-only")

	file, err := NewFileSecretSource(dir)
	if err != nil {
		t.Fatalf("file source: %v", err)
	}
	chain := NewChainSecretSource(file, NewEnvSecretSource())
	ctx := context.Background()

	t.Run("file wins over env when both have key", func(t *testing.T) {
		v, ok, _ := chain.LookupSecret(ctx, "OVERLAP_KEY")
		if !ok || v != "from-file" {
			t.Errorf("got (%q, %v), want (\"from-file\", true)", v, ok)
		}
	})

	t.Run("env-only key resolves through env", func(t *testing.T) {
		v, ok, _ := chain.LookupSecret(ctx, "ENV_ONLY")
		if !ok || v != "from-env-only" {
			t.Errorf("got (%q, %v), want (\"from-env-only\", true)", v, ok)
		}
	})

	t.Run("file-only key resolves through file", func(t *testing.T) {
		v, ok, _ := chain.LookupSecret(ctx, "FILE_ONLY")
		if !ok || v != "from-file-only" {
			t.Errorf("got (%q, %v), want (\"from-file-only\", true)", v, ok)
		}
	})

	t.Run("missing in all sources is clean miss", func(t *testing.T) {
		_, ok, err := chain.LookupSecret(ctx, "TOTALLY_ABSENT")
		if err != nil {
			t.Errorf("err = %v", err)
		}
		if ok {
			t.Errorf("got hit for absent key")
		}
	})

	if !strings.Contains(chain.Name(), "chain(file,env)") {
		t.Errorf("chain Name = %q, want it to contain chain(file,env)", chain.Name())
	}
}

// erroringSource lets us assert that a transport error from a
// chained source short-circuits the chain — by design we treat
// "Vault is down" as a fatal config error, not a fall through.
type erroringSource struct{}

func (erroringSource) LookupSecret(_ context.Context, _ string) (string, bool, error) {
	return "", false, errors.New("simulated transport failure")
}
func (erroringSource) Name() string { return "erroring" }
func (erroringSource) Close() error { return nil }

func TestChainSecretSource_TransportErrorShortCircuits(t *testing.T) {
	// First source errors; second source has the key. The chain
	// must still error — "first transport failed" is more
	// important than "second source had it".
	t.Setenv("FALLBACK_KEY", "would-be-found")
	chain := NewChainSecretSource(erroringSource{}, NewEnvSecretSource())
	_, _, err := chain.LookupSecret(context.Background(), "FALLBACK_KEY")
	if err == nil {
		t.Fatal("expected error to short-circuit; got fall-through")
	}
	if !strings.Contains(err.Error(), "erroring") {
		t.Errorf("error should name the failing source: %v", err)
	}
}

func TestLoadWithSource_RoutesSecretsThroughSource(t *testing.T) {
	// Confirms the new wiring: secret-bearing fields read from the
	// SecretSource, non-sensitive scalars keep reading env directly.
	dir := t.TempDir()
	must := func(key, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, key), []byte(value), 0o600); err != nil {
			t.Fatalf("write %s: %v", key, err)
		}
	}
	must("DATABASE_URL", "postgres://from-file/db")
	must("VAULT_MASTER_KEY", "bWFzdGVyLWtleS1mcm9tLWZpbGUtMzJieXRlcyEh")
	must("SENTRY_DSN", "https://sentry-from-file/proj")

	// Non-sensitive scalars stay direct env reads — verify they
	// still flow.
	t.Setenv("DB_POOL_MAX", "42")
	t.Setenv("PORT", "9090")
	t.Setenv("SENTRY_ENVIRONMENT", "staging")
	// And that a secret in env DOESN'T leak through when source has
	// the same key — file source MUST win when configured.
	t.Setenv("DATABASE_URL", "postgres://from-env-must-not-win")

	src, err := NewFileSecretSource(dir)
	if err != nil {
		t.Fatalf("file source: %v", err)
	}

	cfg, err := LoadWithSource(context.Background(), src)
	if err != nil {
		t.Fatalf("LoadWithSource: %v", err)
	}
	if cfg.DatabaseURL != "postgres://from-file/db" {
		t.Errorf("DATABASE_URL = %q, want from-file (file source must win over env)", cfg.DatabaseURL)
	}
	if cfg.VaultMasterKey != "bWFzdGVyLWtleS1mcm9tLWZpbGUtMzJieXRlcyEh" {
		t.Errorf("VaultMasterKey = %q", cfg.VaultMasterKey)
	}
	if cfg.SentryDSN != "https://sentry-from-file/proj" {
		t.Errorf("SentryDSN = %q", cfg.SentryDSN)
	}
	if cfg.DBPoolMax != 42 {
		t.Errorf("DBPoolMax = %d, want 42 (env-direct, not via source)", cfg.DBPoolMax)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.SentryEnvironment != "staging" {
		t.Errorf("SentryEnvironment = %q, want staging", cfg.SentryEnvironment)
	}
}

func TestLoadWithSource_RequiresDatabaseURL(t *testing.T) {
	// Empty source → no DATABASE_URL → must error. Covers both the
	// "operator forgot to set it" path and the "vault returned a
	// miss but config initialization continued" pitfall.
	dir := t.TempDir()
	src, err := NewFileSecretSource(dir)
	if err != nil {
		t.Fatalf("file source: %v", err)
	}
	// Defensive: clear any DATABASE_URL the test runner happens to
	// have inherited (CI), since LoadWithSource queries the source
	// not env, but a future maintainer might add a fallback that
	// uses env.
	t.Setenv("DATABASE_URL", "")
	_, err = LoadWithSource(context.Background(), src)
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing from source")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should name DATABASE_URL: %v", err)
	}
}

func TestLoadSecretSource_FactorySpecs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	cases := []struct {
		name string
		spec string
		want string // substring of returned source's Name()
		err  bool
	}{
		{"empty defaults to env", "", "env", false},
		{"explicit env", "env", "env", false},
		{"file source", "file:" + dir, "file", false},
		{"chain of file+env", "chain:file:" + dir + ",env", "chain(file,env)", false},
		{"vault source", "vault://kv/data/buildos/test", "vault", false},
		{"chain of vault+env", "chain:vault://kv/data/buildos/test,env", "chain(vault,env)", false},
		{"unknown spec rejected", "azure-kv://prod", "", true},
		{"vault missing /data/ rejected", "vault://kv/buildos", "", true},
		{"file with bad path", "file:/nonexistent-test-path-xyz", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := LoadSecretSource(ctx, c.spec)
			if c.err {
				if err == nil {
					t.Errorf("expected error for spec %q", c.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(s.Name(), c.want) {
				t.Errorf("Name = %q, want substring %q", s.Name(), c.want)
			}
			_ = s.Close()
		})
	}
}
