// Command buildos-fork-init generates the cryptographic identity for
// a new customer fork. BuildOS is self-contained: it mints and
// validates its own RS256 JWTs against a per-fork RSA keypair, and
// encrypts the BYOK credential vault with a per-fork AES-256 master
// key. This tool provisions both, plus the one-shot bootstrap token
// the onboarding wizard needs to claim the first owner.
//
// Outputs (all in OUT_DIR):
//
//	private.pem        PKCS#8 RSA private key — the JWT signing key.
//	                   Operator must move this into their secret store
//	                   (Vault / AWS Secrets Manager / kubernetes Secret)
//	                   and reference it via the SecretSource as
//	                   JWT_PRIVATE_KEY_PEM. NEVER commit this file.
//
//	public.pem         PKIX/SPKI RSA public key — the JWT verification
//	                   key. Configure as JWT_PUBLIC_KEY_PEM. Safe to
//	                   commit.
//
//	vault_master_key.txt  Standard-base64 32-byte AES-256 key for the
//	                   encrypted credential vault. Configure as
//	                   VAULT_MASTER_KEY. NEVER commit — losing or
//	                   rotating it makes existing sealed credentials
//	                   undecryptable.
//
//	bootstrap_token.txt One-shot cleartext for the onboarding wizard's
//	                   first-owner claim. 32 bytes of CSPRNG, base64url-
//	                   encoded. The operator copies this into deploy
//	                   secrets as BUILDOS_BOOTSTRAP_TOKEN; cmd/server
//	                   seeds the hash into setup_bootstrap_tokens on
//	                   first boot, and the first owner presents the
//	                   cleartext to POST /api/v1/auth/claim to claim
//	                   owner role. NEVER commit. Disable with
//	                   --skip-bootstrap-token when rotating only the
//	                   JWT keypair.
//
//	fork.yaml          Operator-readable summary: kid, creation
//	                   timestamp, public-key fingerprint, and the
//	                   env-var names BuildOS expects to find each value
//	                   under at runtime. Committable.
//
// Usage:
//
//	go run ./cmd/buildos-fork-init \
//	  --out ./forks/acme-construction/secrets \
//	  --kid acme-2026-q2
//
// The operator copies private.pem + vault_master_key.txt +
// bootstrap_token.txt into their secret store, commits public.pem +
// fork.yaml to the customer's BuildOS fork repo, and sets
// JWT_KEY_ID=<kid> in the deployment environment.
//
// Reproducibility: pass --seed to derive the keypair from a stable
// seed (testing only — production deployments must use the default
// rand.Reader).
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Default key size. 2048-bit RSA is the floor enterprise procurement
// teams expect today; 3072-bit adds CPU cost on every JWS sign with
// no practical security benefit before 2030. Operators who need a
// larger key can pass --bits.
const defaultKeyBits = 2048

func main() {
	var (
		outDir             = flag.String("out", "", "output directory for generated artifacts (required)")
		kid                = flag.String("kid", "", "JWS key id; defaults to a fresh uuid")
		orgID              = flag.String("org-id", "", "fork's org id (uuid); informational, written to fork.yaml")
		bits               = flag.Int("bits", defaultKeyBits, "RSA key size in bits (2048 minimum)")
		seed               = flag.Int64("seed", 0, "deterministic seed for testing; ZERO uses crypto/rand (production)")
		skipBootstrapToken = flag.Bool("skip-bootstrap-token", false, "do NOT emit bootstrap_token.txt (use when rotating only the JWT keypair on a fork that has already finished onboarding)")
	)
	flag.Parse()

	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "buildos-fork-init: --out is required")
		flag.Usage()
		os.Exit(64) // EX_USAGE
	}
	if *bits < 2048 {
		fmt.Fprintf(os.Stderr, "buildos-fork-init: --bits must be >= 2048 (got %d)\n", *bits)
		os.Exit(64)
	}
	if *kid == "" {
		*kid = uuid.NewString()
	}
	if *orgID != "" {
		if _, err := uuid.Parse(*orgID); err != nil {
			fmt.Fprintf(os.Stderr, "buildos-fork-init: --org-id must be a uuid (got %q)\n", *orgID)
			os.Exit(64)
		}
	}

	if err := run(*outDir, *kid, *orgID, *bits, *seed, !*skipBootstrapToken); err != nil {
		fmt.Fprintf(os.Stderr, "buildos-fork-init: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nfork identity written to %s\n", *outDir)
	fmt.Println("next steps:")
	fmt.Println("  1. move private.pem into your secret store; configure JWT_PRIVATE_KEY_PEM")
	fmt.Println("  2. configure JWT_PUBLIC_KEY_PEM from public.pem and set JWT_KEY_ID=" + *kid)
	fmt.Println("  3. move vault_master_key.txt into your secret store; configure VAULT_MASTER_KEY")
	if !*skipBootstrapToken {
		fmt.Println("  4. move bootstrap_token.txt into your secret store; configure BUILDOS_BOOTSTRAP_TOKEN")
		fmt.Println("     (cmd/server seeds the hash into setup_bootstrap_tokens on first boot;")
		fmt.Println("      the first owner presents this cleartext at POST /api/v1/auth/claim)")
		fmt.Println("  5. commit public.pem + fork.yaml to your customer's BuildOS fork repo")
		fmt.Println("  6. NEVER commit private.pem, vault_master_key.txt, or bootstrap_token.txt")
	} else {
		fmt.Println("  4. commit public.pem + fork.yaml to your customer's BuildOS fork repo")
		fmt.Println("  5. NEVER commit private.pem or vault_master_key.txt — verify .gitignore")
	}
}

// run is the testable entrypoint. seed is 0 in production and uses
// crypto/rand; non-zero seeds derive the keypair deterministically
// for unit tests. emitBootstrapToken=true also emits the one-shot
// cleartext used by the onboarding wizard's first-owner claim.
func run(outDir, kid, orgID string, bits int, seed int64, emitBootstrapToken bool) error {
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	randSrc := io.Reader(rand.Reader)
	if seed != 0 {
		// Deterministic generation for tests. NEVER use this in
		// production — math/rand is not cryptographically secure.
		randSrc = mathrand.New(mathrand.NewSource(seed))
	}

	priv, err := rsa.GenerateKey(randSrc, bits)
	if err != nil {
		return fmt.Errorf("generate rsa key: %w", err)
	}

	if err := writePrivatePEM(filepath.Join(outDir, "private.pem"), priv); err != nil {
		return err
	}
	if err := writePublicPEM(filepath.Join(outDir, "public.pem"), &priv.PublicKey); err != nil {
		return err
	}
	if err := writeVaultMasterKey(filepath.Join(outDir, "vault_master_key.txt"), randSrc); err != nil {
		return err
	}
	if emitBootstrapToken {
		if err := writeBootstrapToken(filepath.Join(outDir, "bootstrap_token.txt"), randSrc); err != nil {
			return err
		}
	}
	if err := writeForkYAML(filepath.Join(outDir, "fork.yaml"), &priv.PublicKey, kid, orgID, emitBootstrapToken); err != nil {
		return err
	}
	return nil
}

// vaultMasterKeyByteLen is the AES-256 key length. cryptobox seals the
// credential vault with AES-256-GCM, which requires a 32-byte key.
const vaultMasterKeyByteLen = 32

// writeVaultMasterKey emits a standard-base64 (padded) 32-byte AES-256
// key at path with file mode 0600. The operator copies this into
// deploy secrets as VAULT_MASTER_KEY; config decodes it with
// base64.StdEncoding.
func writeVaultMasterKey(path string, randSrc io.Reader) error {
	buf := make([]byte, vaultMasterKeyByteLen)
	if _, err := io.ReadFull(randSrc, buf); err != nil {
		return fmt.Errorf("read csprng for vault master key: %w", err)
	}
	key := base64.StdEncoding.EncodeToString(buf)
	return os.WriteFile(path, []byte(key+"\n"), 0o600)
}

// bootstrapTokenByteLen MUST match
// internal/service/setup.bootstrapTokenByteLen. 32 bytes of CSPRNG
// (256 bits of entropy) makes offline brute-force infeasible
// regardless of hash speed, which is why the service hashes with
// SHA-256 rather than a slow KDF.
const bootstrapTokenByteLen = 32

// writeBootstrapToken emits a base64url-no-pad cleartext token at
// path with file mode 0600. The operator copies this into deploy
// secrets as BUILDOS_BOOTSTRAP_TOKEN; cmd/server seeds the SHA-256
// hash into setup_bootstrap_tokens on first boot. Format matches
// internal/service/setup.generateBootstrapTokenCleartext so the two
// paths produce indistinguishable tokens regardless of who emits.
func writeBootstrapToken(path string, randSrc io.Reader) error {
	buf := make([]byte, bootstrapTokenByteLen)
	if _, err := io.ReadFull(randSrc, buf); err != nil {
		return fmt.Errorf("read csprng for bootstrap token: %w", err)
	}
	cleartext := base64.RawURLEncoding.EncodeToString(buf)
	// Newline-terminated so `cat bootstrap_token.txt | tr -d $'\n'`
	// is unnecessary and copy-paste from an editor doesn't pick up
	// a trailing carriage return on Windows hosts.
	return os.WriteFile(path, []byte(cleartext+"\n"), 0o600)
}

// writePrivatePEM emits PKCS#8 PEM with file mode 0600 — readable
// only by the owning user. Any existing file at the path is
// overwritten. The umask still applies; operators running this in a
// shared session should chmod 0400 after.
func writePrivatePEM(path string, key *rsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal pkcs#8: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: der}); err != nil {
		_ = f.Close() // already returning the encode error
		return err
	}
	// Surface flush/close errors — a swallowed close on a key file can
	// silently truncate the PEM.
	return f.Close()
}

// writePublicPEM emits SPKI PEM with file mode 0644 — public, safe
// to commit.
func writePublicPEM(path string, key *rsa.PublicKey) error {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return fmt.Errorf("marshal spki: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "PUBLIC KEY", Bytes: der}); err != nil {
		_ = f.Close() // already returning the encode error
		return err
	}
	return f.Close()
}

// writeForkYAML emits an operator-readable artifact. YAML by hand
// rather than gopkg.in/yaml.v3 to avoid a dep just for this; the
// shape is small and stable.
func writeForkYAML(path string, key *rsa.PublicKey, kid, orgID string, hasBootstrapToken bool) error {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return fmt.Errorf("marshal spki: %w", err)
	}
	fingerprint := sha256.Sum256(der)
	fpHex := hexLower(fingerprint[:])
	if orgID == "" {
		orgID = "<assigned during onboarding>"
	}
	bootstrapStanza := ""
	if hasBootstrapToken {
		bootstrapStanza = `#   BUILDOS_BOOTSTRAP_TOKEN: <from bootstrap_token.txt; one-shot, claim via POST /api/v1/auth/claim>
`
	}
	content := fmt.Sprintf(`# BuildOS fork identity — generated by buildos-fork-init
#
# This file is committed to the customer's BuildOS fork repo. The
# matching private key (private.pem), the vault master key, and the
# bootstrap token are NOT — they live in the fork's secret store.
#
# To rotate the JWT keypair only (keep onboarding state):
#   buildos-fork-init --out … --kid <new> --skip-bootstrap-token
#
# To rotate everything (fresh tenant): regenerate without
# --skip-bootstrap-token; cmd/server will reseed setup_bootstrap_tokens
# on next boot if the org has not yet completed onboarding.

kid:               %q
created_at:        %q
org_id:            %q
public_key_sha256: %q
key_size_bits:     %d
bootstrap_token_emitted: %t

# Environment variables BuildOS expects at runtime:
#   JWT_KEY_ID:           %s
#   JWT_PRIVATE_KEY_PEM:  <contents of private.pem, from secret store>
#   JWT_PUBLIC_KEY_PEM:   <contents of public.pem>
#   VAULT_MASTER_KEY:     <from vault_master_key.txt, in secret store>
%s`, kid, time.Now().UTC().Format(time.RFC3339), orgID, fpHex, key.N.BitLen(), hasBootstrapToken, kid, bootstrapStanza)
	return os.WriteFile(path, []byte(content), 0o644)
}

// hexLower formats bytes as a fixed-width lowercase hex string.
// Standalone implementation so the binary stays free of fmt-import
// surprises around %x precision.
func hexLower(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}
