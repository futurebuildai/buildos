package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// VaultSecretSource resolves secrets from a HashiCorp Vault KV v2 mount.
//
// Why Vault is the production-default secret store for customer forks:
// every fork deploys to a different cloud account / k8s cluster, but
// most enterprise customers already have Vault as their secret-of-record.
// Putting BuildOS secrets in Vault lets the operator reuse their existing
// rotation, audit, and break-glass procedures rather than inventing a
// per-fork key-management story.
//
// Configuration model:
//
//   - Spec format: "vault://<mount>/data/<path-prefix>". For example
//     "vault://kv/data/buildos/acme" tells the source to read each
//     secret from "kv/data/buildos/acme/<KEY>" via the KV v2 HTTP API.
//     The "/data/" segment is the KV v2 API marker; KV v1 mounts
//     (without /data/) are explicitly rejected — operators on a fresh
//     deployment should provision KV v2, which gives them versioning,
//     check-and-set, and metadata-only deletes.
//
//   - Address: VAULT_ADDR env var. The Go SDK's DefaultConfig()
//     resolves this and TLS settings (VAULT_CACERT, VAULT_CAPATH,
//     VAULT_CLIENT_CERT, VAULT_CLIENT_KEY, VAULT_SKIP_VERIFY); we don't
//     reimplement the lookup.
//
//   - Auth: VAULT_TOKEN env var, or the file referenced by
//     VAULT_TOKEN_FILE, or ~/.vault-token (SDK default). Operators on
//     a Kubernetes fork typically project a service-account token and
//     trade it for a Vault token via Vault's k8s auth method; that
//     trade lives in the operator's wrapping init container, not here.
//     An empty/expired token at construction is permitted — fail-loud
//     on the first lookup is the right ergonomic for operators who
//     haven't fully provisioned auth yet.
//
// Secret-shape convention: one Vault secret per logical key, with a
// single field named "value". Operators provision via:
//
//	vault kv put kv/buildos/acme/DATABASE_URL value=postgres://...
//
// (The CLI hides the "data" segment; the HTTP API and our spec
// reference it explicitly so the spec format and the underlying API
// path agree literally.) Multi-field secrets can be modeled by chaining
// multiple paths in a ChainSecretSource.
//
// Per the SecretSource contract:
//
//   - Hit  → (val, true, nil)
//   - Miss → ("", false, nil) (404 from Vault, or empty "value" field)
//   - Transport / auth / permission errors propagate so a chain
//     short-circuits. "Vault is down" must NOT silently fall back to
//     env — that would downgrade security on a transient outage.
type VaultSecretSource struct {
	client   *vault.Client
	basePath string // e.g. "kv/data/buildos/acme"; raw HTTP API path
}

// NewVaultSecretSource constructs a Vault-backed source from a spec
// of the form "vault://<mount>/data/<path-prefix>". The Vault client
// is configured from environment variables (VAULT_ADDR, VAULT_TOKEN,
// VAULT_TOKEN_FILE, VAULT_CACERT, …) per the SDK's DefaultConfig.
func NewVaultSecretSource(spec string) (*VaultSecretSource, error) {
	if !strings.HasPrefix(spec, "vault://") {
		return nil, fmt.Errorf("vault secret source: spec must start with vault://, got %q", spec)
	}
	basePath := strings.TrimPrefix(spec, "vault://")
	basePath = strings.Trim(basePath, "/")
	if basePath == "" {
		return nil, errors.New("vault secret source: empty path; spec must be vault://<mount>/data/<path-prefix>")
	}
	// KV v2 marker check. Require explicit "/data/" so operators
	// don't accidentally aim a KV v1 mount at this source and hit
	// "missing data field" on every read.
	if !strings.Contains(basePath, "/data/") && !strings.HasSuffix(basePath, "/data") {
		return nil, fmt.Errorf("vault secret source: spec %q missing /data/ segment (KV v2 only; legacy KV v1 not supported)", spec)
	}

	cfg := vault.DefaultConfig()
	if cfg == nil {
		// Defensive — SDK contract says DefaultConfig never returns
		// nil, but we don't want a silent panic-on-nil if a future
		// SDK version changes that.
		return nil, errors.New("vault secret source: nil default config")
	}
	if cfg.Error != nil {
		return nil, fmt.Errorf("vault secret source: default config: %w", cfg.Error)
	}
	if strings.TrimSpace(cfg.Address) == "" {
		// DefaultConfig() always sets Address to "https://127.0.0.1:8200"
		// when VAULT_ADDR is unset, so this branch is mostly defensive.
		return nil, errors.New("vault secret source: VAULT_ADDR not set and no SDK default")
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("vault secret source: new client: %w", err)
	}

	// VAULT_TOKEN_FILE is BuildOS-specific glue on top of the SDK —
	// the standard SDK reads VAULT_TOKEN env or ~/.vault-token, but
	// k8s/secrets-mounted token files are the production norm and
	// don't fit either. Honor it without forcing operators into a
	// wrapper script.
	if tokFile := strings.TrimSpace(os.Getenv("VAULT_TOKEN_FILE")); tokFile != "" {
		b, err := os.ReadFile(tokFile)
		if err != nil {
			return nil, fmt.Errorf("vault secret source: read VAULT_TOKEN_FILE %s: %w", tokFile, err)
		}
		client.SetToken(strings.TrimSpace(string(b)))
	}

	return &VaultSecretSource{client: client, basePath: basePath}, nil
}

// LookupSecret reads <basePath>/<key> via the KV v2 HTTP API and
// returns the "value" field of the secret. Missing secret → clean
// miss; auth/transport errors propagate.
func (s *VaultSecretSource) LookupSecret(ctx context.Context, key string) (string, bool, error) {
	if key == "" {
		return "", false, errors.New("vault secret source: empty key")
	}
	path := s.basePath + "/" + key
	sec, err := s.client.Logical().ReadWithContext(ctx, path)
	if err != nil {
		// SDK does not surface 404 as a typed error — a missing
		// secret returns (nil, nil). Errors here are transport
		// (timeout, connection refused), auth (403 token
		// revoked/expired), or server (5xx). All propagate.
		return "", false, fmt.Errorf("vault secret source: read %s: %w", path, err)
	}
	if sec == nil || sec.Data == nil {
		// Missing path. Clean miss so callers fall through.
		return "", false, nil
	}
	// KV v2 response shape: sec.Data = {"data": {field: value, ...},
	// "metadata": {...}}. Extract the inner data map.
	rawData, ok := sec.Data["data"]
	if !ok {
		return "", false, fmt.Errorf("vault secret source: %s response missing data field (KV v2 expected; check mount type)", path)
	}
	// Versioned-secret tombstone (kv put then kv metadata delete)
	// returns sec.Data with data=nil and a populated metadata block.
	// Treat as miss.
	if rawData == nil {
		return "", false, nil
	}
	data, ok := rawData.(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("vault secret source: %s data field is %T, want map[string]any", path, rawData)
	}
	rawVal, ok := data["value"]
	if !ok {
		// Field convention is fixed at "value". Surface a hint in
		// the error so an operator who provisioned with a different
		// field name (e.g. "secret=...") sees the fix immediately.
		cliPath := strings.ReplaceAll(s.basePath, "/data/", "/") + "/" + key
		return "", false, fmt.Errorf("vault secret source: %s missing 'value' field (provision via: vault kv put %s value=...)", path, cliPath)
	}
	v, ok := rawVal.(string)
	if !ok {
		return "", false, fmt.Errorf("vault secret source: %s 'value' field is %T, want string", path, rawVal)
	}
	if v == "" {
		// Same convention as env/file: empty value is a miss, not
		// an explicit empty.
		return "", false, nil
	}
	return v, true, nil
}

// Name returns "vault".
func (s *VaultSecretSource) Name() string { return "vault" }

// Close clears the in-memory token. The Vault Go SDK doesn't require
// an explicit close, but clearing the token defends against a leaked
// reference making authenticated calls after shutdown.
func (s *VaultSecretSource) Close() error {
	if s.client != nil {
		s.client.ClearToken()
	}
	return nil
}
