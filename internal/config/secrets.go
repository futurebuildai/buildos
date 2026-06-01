package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// SecretSource is the indirection between operator-managed secret
// stores (HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager,
// kubernetes secret-mounted files, plain env vars) and the rest of
// BuildOS. Production deployments swap implementations via the
// CONFIG_SOURCE env var; dev rigs keep the default "env" source so
// existing workflows are unchanged.
//
// Why an abstraction at all (vs. just reading env): every customer
// fork deploys to a different cloud / enterprise account with
// different secret-management standards. Hardcoding env-vars-only
// forces every customer fork to write its own bootstrap glue (and
// usually leaks secrets into K8s manifests). The interface is the
// minimal surface that lets fork operators choose their store
// without forking the secret-loading logic.
type SecretSource interface {
	// LookupSecret resolves a secret by its logical key (e.g.
	// "DATABASE_URL", "VAULT_MASTER_KEY"). The key namespace is
	// flat — sources are responsible for mapping it to whatever
	// hierarchy their backend uses (Vault path, IAM resource ARN, …).
	//
	// Returns (value, true) on hit, ("", false) on miss. Errors are
	// reserved for transport failures (network down, permission
	// denied); a clean miss returns false with err==nil so callers
	// can fall back to a default without distinguishing missing
	// from unreachable.
	LookupSecret(ctx context.Context, key string) (string, bool, error)

	// Name reports the source kind for log/telemetry tagging
	// ("env", "vault", "aws-sm", "gcp-sm", "file"). Operators
	// reading boot logs should see exactly which source resolved
	// each secret.
	Name() string

	// Close releases any held resources (open HTTP connections to
	// Vault, etc.). Called once at process shutdown. Safe to call
	// on a zero-config source (env) — no-op.
	Close() error
}

// EnvSecretSource is the default — secrets come from os.Environ().
// Used in dev and as the fallback for any deployment that hasn't
// configured a richer source.
type EnvSecretSource struct{}

// NewEnvSecretSource returns the env-backed source.
func NewEnvSecretSource() *EnvSecretSource { return &EnvSecretSource{} }

// LookupSecret reads from os.Getenv. Empty string is treated as a
// miss — env vars set to "" are operator typos, not "explicit empty".
func (s *EnvSecretSource) LookupSecret(_ context.Context, key string) (string, bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", false, nil
	}
	return v, true, nil
}

// Name returns "env".
func (s *EnvSecretSource) Name() string { return "env" }

// Close is a no-op for env.
func (s *EnvSecretSource) Close() error { return nil }

// FileSecretSource reads each secret from a file at <dir>/<key>.
// Matches the kubernetes secret-mount convention: a Secret mounted
// at /run/secrets/buildos/ exposes each entry as a file named after
// the key. Whitespace + trailing newline are stripped on read so
// `kubectl create secret generic --from-literal=...` round-trips
// cleanly.
//
// Best-of-class enterprise alternative to env vars: file-mounted
// secrets never appear in `ps`, in process environment dumps, or in
// container exec snapshots. Rotation is observed via inotify / file
// timestamp polling (not yet implemented; rotation TODO is a
// follow-up).
type FileSecretSource struct {
	dir string
}

// NewFileSecretSource constructs a source that reads secrets from
// <dir>/<KEY>. Returns an error if dir doesn't exist; we want loud
// failure at boot rather than silent fallback to "every secret
// missing".
func NewFileSecretSource(dir string) (*FileSecretSource, error) {
	if dir == "" {
		return nil, errors.New("file secret source: dir is empty")
	}
	st, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("file secret source: stat %s: %w", dir, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("file secret source: %s is not a directory", dir)
	}
	return &FileSecretSource{dir: dir}, nil
}

// LookupSecret reads <dir>/<KEY>. Missing file = miss (no error).
// Other read errors propagate (permission denied, IO error).
func (s *FileSecretSource) LookupSecret(_ context.Context, key string) (string, bool, error) {
	path := s.dir + "/" + key
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("file secret source: read %s: %w", path, err)
	}
	v := strings.TrimRight(string(b), "\n\r\t ")
	if v == "" {
		return "", false, nil
	}
	return v, true, nil
}

// Name returns "file".
func (s *FileSecretSource) Name() string { return "file" }

// Close is a no-op for file (each Read opens + closes its own fd).
func (s *FileSecretSource) Close() error { return nil }

// ChainSecretSource queries each underlying source in order, returning
// the first hit. Lets a deployment combine sources: file-mounted
// secrets for the high-value items, env for the low-value rest.
//
// The order matters — earlier sources win. Typical chain:
//
//	chain := NewChainSecretSource(file, env)
//
// A secret in the file mount overrides any env var of the same name.
type ChainSecretSource struct {
	sources []SecretSource
}

// NewChainSecretSource composes sources in priority order.
func NewChainSecretSource(sources ...SecretSource) *ChainSecretSource {
	return &ChainSecretSource{sources: sources}
}

// LookupSecret tries each source in order. First non-error hit wins.
// A transport error from any source short-circuits the chain — we
// treat "Vault is down" as a fatal config-load failure, not a fall
// through to env (which would silently downgrade security on a
// transient outage).
func (s *ChainSecretSource) LookupSecret(ctx context.Context, key string) (string, bool, error) {
	for _, src := range s.sources {
		v, ok, err := src.LookupSecret(ctx, key)
		if err != nil {
			return "", false, fmt.Errorf("chain[%s]: %w", src.Name(), err)
		}
		if ok {
			return v, true, nil
		}
	}
	return "", false, nil
}

// Name returns "chain(<src1>,<src2>,…)" so logs show the composition.
func (s *ChainSecretSource) Name() string {
	names := make([]string, 0, len(s.sources))
	for _, src := range s.sources {
		names = append(names, src.Name())
	}
	return "chain(" + strings.Join(names, ",") + ")"
}

// Close calls Close on each underlying source. Errors are joined; a
// single source's close failure doesn't stop the others from being
// closed.
func (s *ChainSecretSource) Close() error {
	var errs []error
	for _, src := range s.sources {
		if err := src.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", src.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// LoadSecretSource constructs the SecretSource based on the
// CONFIG_SOURCE env var:
//
//	""       (default) → EnvSecretSource
//	"env"             → EnvSecretSource
//	"file:/path"       → FileSecretSource at /path
//	"chain:<a>,<b>"    → ChainSecretSource with sub-sources resolved
//	                    recursively (e.g. "chain:file:/run/secrets,env")
//
// "vault://<mount>/data/<path-prefix>" → VaultSecretSource (KV v2).
// AWS-SM / GCP-SM implementations land in follow-up PRs; they'll
// register against the same prefix scheme ("aws-sm:...", "gcp-sm:...")
// so adding a backend is additive without changing the operator
// interface.
func LoadSecretSource(ctx context.Context, spec string) (SecretSource, error) {
	if spec == "" || spec == "env" {
		return NewEnvSecretSource(), nil
	}
	if strings.HasPrefix(spec, "file:") {
		return NewFileSecretSource(strings.TrimPrefix(spec, "file:"))
	}
	if strings.HasPrefix(spec, "vault://") {
		return NewVaultSecretSource(spec)
	}
	if strings.HasPrefix(spec, "chain:") {
		parts := strings.Split(strings.TrimPrefix(spec, "chain:"), ",")
		sources := make([]SecretSource, 0, len(parts))
		for _, p := range parts {
			sub, err := LoadSecretSource(ctx, strings.TrimSpace(p))
			if err != nil {
				return nil, fmt.Errorf("chain element %q: %w", p, err)
			}
			sources = append(sources, sub)
		}
		return NewChainSecretSource(sources...), nil
	}
	return nil, fmt.Errorf("unknown CONFIG_SOURCE spec %q (want \"\", \"env\", \"file:/path\", \"vault://<mount>/data/<path>\", or \"chain:...\")", spec)
}
