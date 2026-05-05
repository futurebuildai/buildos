package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	vault "github.com/hashicorp/vault/api"
)

// TestNewVaultSecretSource_RejectsBadSpecs verifies the spec-format
// gate runs before any Vault client is constructed. These are pure
// validation paths; no VAULT_ADDR / VAULT_TOKEN required.
func TestNewVaultSecretSource_RejectsBadSpecs(t *testing.T) {
	cases := []struct {
		name string
		spec string
	}{
		{"missing prefix", "kv/data/buildos/acme"},
		{"only prefix", "vault://"},
		{"only slashes", "vault:///"},
		{"missing /data/ marker (KV v1)", "vault://kv/buildos/acme"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewVaultSecretSource(c.spec)
			if err == nil {
				t.Errorf("expected error for spec %q", c.spec)
			}
		})
	}
}

// TestNewVaultSecretSource_AcceptsValidSpec ensures the happy-path
// constructor succeeds without making network calls. Vault SDK's
// DefaultConfig() always provides a fallback Address of
// https://127.0.0.1:8200 even if VAULT_ADDR is unset, so construction
// is purely local.
func TestNewVaultSecretSource_AcceptsValidSpec(t *testing.T) {
	t.Setenv("VAULT_ADDR", "https://vault.test:8200")
	t.Setenv("VAULT_TOKEN", "test-token")

	s, err := NewVaultSecretSource("vault://kv/data/buildos/acme")
	if err != nil {
		t.Fatalf("NewVaultSecretSource: %v", err)
	}
	if s.Name() != "vault" {
		t.Errorf("Name = %q, want vault", s.Name())
	}
	if s.basePath != "kv/data/buildos/acme" {
		t.Errorf("basePath = %q, want kv/data/buildos/acme", s.basePath)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}

// TestNewVaultSecretSource_HonorsTokenFile verifies the
// VAULT_TOKEN_FILE extension reads the token from disk and applies it
// to the client. K8s-mounted token-file deployments require this
// path; the standard SDK only knows VAULT_TOKEN env / ~/.vault-token.
func TestNewVaultSecretSource_HonorsTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokFile := filepath.Join(dir, "vault-token")
	const want = "tok-from-file"
	if err := os.WriteFile(tokFile, []byte(want+"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("VAULT_ADDR", "https://vault.test:8200")
	t.Setenv("VAULT_TOKEN_FILE", tokFile)
	t.Setenv("VAULT_TOKEN", "would-have-been-overridden")

	s, err := NewVaultSecretSource("vault://kv/data/buildos/acme")
	if err != nil {
		t.Fatalf("NewVaultSecretSource: %v", err)
	}
	if got := s.client.Token(); got != want {
		t.Errorf("token = %q, want %q (file should override env)", got, want)
	}
}

// TestNewVaultSecretSource_RejectsBadTokenFile surfaces a clear error
// if VAULT_TOKEN_FILE points at a nonexistent path. Operators have
// often misconfigured the projected-volume mount path, and we want a
// loud failure at boot, not at first secret read.
func TestNewVaultSecretSource_RejectsBadTokenFile(t *testing.T) {
	t.Setenv("VAULT_ADDR", "https://vault.test:8200")
	t.Setenv("VAULT_TOKEN_FILE", "/nonexistent/path/vault-token-xyz")
	_, err := NewVaultSecretSource("vault://kv/data/buildos/acme")
	if err == nil {
		t.Fatal("expected error for missing VAULT_TOKEN_FILE")
	}
	if !strings.Contains(err.Error(), "VAULT_TOKEN_FILE") {
		t.Errorf("error should reference VAULT_TOKEN_FILE: %v", err)
	}
}

// TestVaultSecretSource_LookupSecret_RoundTrips uses an httptest
// server impersonating Vault's KV v2 API to exercise LookupSecret
// without spinning up a real Vault. Covers the canonical KV v2
// response shape (data wrapped in {"data": {...}, "metadata": {...}}).
func TestVaultSecretSource_LookupSecret_RoundTrips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vault KV v2 path: /v1/<mount>/data/<path>
		// Our basePath is "kv/data/buildos/acme"; key "DATABASE_URL"
		// resolves to /v1/kv/data/buildos/acme/DATABASE_URL.
		switch r.URL.Path {
		case "/v1/kv/data/buildos/acme/DATABASE_URL":
			writeKVv2Response(t, w, map[string]any{"value": "postgres://hidden/db"})
		case "/v1/kv/data/buildos/acme/MISSING":
			// Vault returns JSON-shaped 404s: {"errors": []}.
			// http.NotFound writes plain text and the SDK then
			// fails to parse, which surfaces as an error rather
			// than the expected clean miss.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{}})
		case "/v1/kv/data/buildos/acme/EMPTY_VALUE":
			writeKVv2Response(t, w, map[string]any{"value": ""})
		case "/v1/kv/data/buildos/acme/WRONG_FIELD":
			writeKVv2Response(t, w, map[string]any{"secret": "should-have-used-value"})
		case "/v1/kv/data/buildos/acme/TOMBSTONE":
			// Versioned-secret tombstone (deleted version): inner
			// data is nil but metadata persists.
			writeKVv2Tombstone(t, w, nil, map[string]any{
				"deletion_time": "2026-01-01T00:00:00Z",
				"destroyed":     false,
				"version":       1,
			})
		case "/v1/kv/data/buildos/acme/NUMERIC_VALUE":
			writeKVv2Response(t, w, map[string]any{"value": 42})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	s, err := NewVaultSecretSource("vault://kv/data/buildos/acme")
	if err != nil {
		t.Fatalf("NewVaultSecretSource: %v", err)
	}
	ctx := context.Background()

	t.Run("hit", func(t *testing.T) {
		v, ok, err := s.LookupSecret(ctx, "DATABASE_URL")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !ok || v != "postgres://hidden/db" {
			t.Errorf("got (%q, %v), want (\"postgres://hidden/db\", true)", v, ok)
		}
	})

	t.Run("missing secret is clean miss", func(t *testing.T) {
		_, ok, err := s.LookupSecret(ctx, "MISSING")
		if err != nil {
			t.Errorf("err = %v, want nil (404 is a clean miss)", err)
		}
		if ok {
			t.Errorf("missing secret reported hit")
		}
	})

	t.Run("empty value is treated as miss", func(t *testing.T) {
		_, ok, err := s.LookupSecret(ctx, "EMPTY_VALUE")
		if err != nil {
			t.Errorf("err = %v", err)
		}
		if ok {
			t.Errorf("empty value reported hit")
		}
	})

	t.Run("missing 'value' field surfaces operator-friendly error", func(t *testing.T) {
		_, _, err := s.LookupSecret(ctx, "WRONG_FIELD")
		if err == nil {
			t.Fatal("expected error for missing value field")
		}
		if !strings.Contains(err.Error(), "vault kv put") {
			t.Errorf("error should hint at the kv put command: %v", err)
		}
	})

	t.Run("tombstoned version is clean miss", func(t *testing.T) {
		_, ok, err := s.LookupSecret(ctx, "TOMBSTONE")
		if err != nil {
			t.Errorf("err = %v, want nil (tombstone is a clean miss)", err)
		}
		if ok {
			t.Errorf("tombstone reported hit")
		}
	})

	t.Run("non-string value rejected", func(t *testing.T) {
		_, _, err := s.LookupSecret(ctx, "NUMERIC_VALUE")
		if err == nil {
			t.Fatal("expected error for non-string value")
		}
		if !strings.Contains(err.Error(), "want string") {
			t.Errorf("error should name the type mismatch: %v", err)
		}
	})

	t.Run("empty key rejected", func(t *testing.T) {
		_, _, err := s.LookupSecret(ctx, "")
		if err == nil {
			t.Error("expected error for empty key")
		}
	})
}

// TestVaultSecretSource_LookupSecret_PropagatesTransportErrors ensures
// a 500 / network failure surfaces as an error (not a clean miss) so
// the chain short-circuits per the documented contract: "Vault is
// down" must NOT silently fall back to env.
func TestVaultSecretSource_LookupSecret_PropagatesTransportErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "vault sealed", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	// Disable retries so the test is fast — by default the SDK
	// retries 5xx with backoff, which would inflate this test
	// unnecessarily.
	cfg := vault.DefaultConfig()
	cfg.MaxRetries = 0
	client, err := vault.NewClient(cfg)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	client.SetToken("test-token")
	s := &VaultSecretSource{client: client, basePath: "kv/data/buildos/acme"}

	_, _, err = s.LookupSecret(context.Background(), "DATABASE_URL")
	if err == nil {
		t.Fatal("expected transport error to propagate")
	}
}

// TestVaultSecretSource_Close_ClearsToken verifies Close drops the
// in-memory token, defending against leaked references.
func TestVaultSecretSource_Close_ClearsToken(t *testing.T) {
	t.Setenv("VAULT_ADDR", "https://vault.test:8200")
	t.Setenv("VAULT_TOKEN", "test-token")

	s, err := NewVaultSecretSource("vault://kv/data/buildos/acme")
	if err != nil {
		t.Fatalf("NewVaultSecretSource: %v", err)
	}
	if s.client.Token() == "" {
		t.Fatal("token should be set after construction")
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
	if got := s.client.Token(); got != "" {
		t.Errorf("token after Close = %q, want \"\"", got)
	}
}

// writeKVv2Response writes the canonical Vault KV v2 read response
// shape: outer "data" wraps an inner {"data": <fields>, "metadata": …}
// pair. The SDK unwraps the outer envelope into Secret.Data, leaving
// our LookupSecret to extract Secret.Data["data"].
func writeKVv2Response(t *testing.T, w http.ResponseWriter, fields map[string]any) {
	t.Helper()
	writeKVv2Tombstone(t, w, fields, map[string]any{"version": 1, "destroyed": false})
}

// writeKVv2Tombstone is the underlying writer; pass fields=nil to
// emulate a deleted version (data is nil but metadata persists).
func writeKVv2Tombstone(t *testing.T, w http.ResponseWriter, fields map[string]any, metadata map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	envelope := map[string]any{
		"request_id":     "test-req",
		"lease_id":       "",
		"renewable":      false,
		"lease_duration": 0,
		"data": map[string]any{
			"data":     fields,
			"metadata": metadata,
		},
	}
	if err := json.NewEncoder(w).Encode(envelope); err != nil {
		t.Fatalf("encode: %v", err)
	}
}
