//go:build integration

package config

import (
	"context"
	"testing"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestVaultSecretSource_RoundTripWithRealVault stands up a Vault dev
// server in a container, writes a secret via the SDK, then reads it
// back through VaultSecretSource. Confirms the spec format
// (vault://<mount>/data/<path-prefix>) maps cleanly to the KV v2 API
// and that the "value" field convention round-trips through a real
// Vault.
//
// Build tag: integration. Runs under `make test-integration`; the
// default `go test ./...` ignores this file so we don't need Docker
// for unit tests.
func TestVaultSecretSource_RoundTripWithRealVault(t *testing.T) {
	startCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const rootToken = "buildos-test-root"
	req := testcontainers.ContainerRequest{
		Image:        "hashicorp/vault:1.18",
		ExposedPorts: []string{"8200/tcp"},
		Env: map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID":  rootToken,
			"VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200",
		},
		// Vault dev mode binds dev-only KV v2 at "secret/" by default
		// AND prints a startup banner including "Root Token". Wait for
		// that line to be sure the API is up.
		WaitingFor: wait.ForLog("Root Token:").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(startCtx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("vault integration: Docker unavailable or container start failed: %v", err)
		return
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.Terminate(ctx); err != nil {
			t.Logf("vault integration: container terminate failed: %v", err)
		}
	})

	host, err := c.Host(startCtx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := c.MappedPort(startCtx, "8200/tcp")
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}
	addr := "http://" + host + ":" + port.Port()

	// Provision: write a secret via the SDK at secret/buildos/acme/DATABASE_URL.
	// Dev mode mounts KV v2 at "secret/" — so the API path becomes
	// secret/data/buildos/acme/DATABASE_URL and the spec we hand to
	// VaultSecretSource is vault://secret/data/buildos/acme.
	cfg := vault.DefaultConfig()
	cfg.Address = addr
	provClient, err := vault.NewClient(cfg)
	if err != nil {
		t.Fatalf("provision client: %v", err)
	}
	provClient.SetToken(rootToken)

	const secretValue = "postgres://hidden/db"
	if _, err := provClient.KVv2("secret").Put(startCtx, "buildos/acme/DATABASE_URL", map[string]any{
		"value": secretValue,
	}); err != nil {
		t.Fatalf("kv put DATABASE_URL: %v", err)
	}
	if _, err := provClient.KVv2("secret").Put(startCtx, "buildos/acme/EMPTY_VAL", map[string]any{
		"value": "",
	}); err != nil {
		t.Fatalf("kv put EMPTY_VAL: %v", err)
	}

	// Wire VaultSecretSource through the public spec format.
	t.Setenv("VAULT_ADDR", addr)
	t.Setenv("VAULT_TOKEN", rootToken)
	src, err := NewVaultSecretSource("vault://secret/data/buildos/acme")
	if err != nil {
		t.Fatalf("NewVaultSecretSource: %v", err)
	}
	t.Cleanup(func() { _ = src.Close() })

	t.Run("hit", func(t *testing.T) {
		v, ok, err := src.LookupSecret(startCtx, "DATABASE_URL")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !ok || v != secretValue {
			t.Errorf("got (%q, %v), want (%q, true)", v, ok, secretValue)
		}
	})

	t.Run("missing secret is clean miss", func(t *testing.T) {
		_, ok, err := src.LookupSecret(startCtx, "DOES_NOT_EXIST")
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
		if ok {
			t.Errorf("missing reported hit")
		}
	})

	t.Run("empty value is treated as miss", func(t *testing.T) {
		_, ok, err := src.LookupSecret(startCtx, "EMPTY_VAL")
		if err != nil {
			t.Errorf("err = %v", err)
		}
		if ok {
			t.Errorf("empty value reported hit")
		}
	})

	t.Run("LoadWithSource consumes vault values", func(t *testing.T) {
		// Provision the rest of LoadWithSource's required fields so
		// the integration covers the full Load() path, not just one
		// LookupSecret. Mirrors TestLoadWithSource_RoutesSecretsThroughSource
		// for the file source.
		mustPut := func(key, val string) {
			if _, err := provClient.KVv2("secret").Put(startCtx, "buildos/acme/"+key, map[string]any{
				"value": val,
			}); err != nil {
				t.Fatalf("kv put %s: %v", key, err)
			}
		}
		mustPut("BRAIN_JWKS_URL", "https://brain.example/jwks")
		mustPut("BRAIN_ISSUER_URL", "https://brain.example")

		cfg, err := LoadWithSource(startCtx, src)
		if err != nil {
			t.Fatalf("LoadWithSource: %v", err)
		}
		if cfg.DatabaseURL != secretValue {
			t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, secretValue)
		}
		if cfg.BrainJWKSURL != "https://brain.example/jwks" {
			t.Errorf("BrainJWKSURL = %q", cfg.BrainJWKSURL)
		}
		if cfg.BrainIssuerURL != "https://brain.example" {
			t.Errorf("BrainIssuerURL = %q", cfg.BrainIssuerURL)
		}
	})
}
