// Command buildos-fork-init generates the cryptographic identity for
// a new customer fork. Each fork needs its own RSA keypair for
// signing outbound A2A webhooks; The Brain verifies signatures against
// the public half published in the fork's JWKS endpoint.
//
// Outputs (all in OUT_DIR):
//
//	private.pem   PKCS#8 RSA private key. Operator must move this
//	              into their secret store (Vault / AWS Secrets Manager
//	              / kubernetes Secret) and reference it via the
//	              SecretSource as A2A_SIGNING_KEY_PATH or
//	              A2A_SIGNING_KEY (depending on the source kind).
//	              NEVER commit this file.
//
//	public.pem    PEM-encoded public key. Safe to commit; useful for
//	              docs / external verifiers that don't speak JWKS.
//
//	jwks.json     JSON Web Key Set with the public key, ready to
//	              paste into Brain's per-fork registration. The `kid`
//	              header value matches what BuildOS will stamp on
//	              every JWS produced by the matching private key.
//
//	fork.yaml     Operator-readable summary of the keypair: kid,
//	              creation timestamp, fingerprints, and the env-var
//	              names BuildOS expects to find each value under at
//	              runtime. This is the artifact that lives in the
//	              customer fork's repo (committable).
//
// Usage:
//
//	go run ./cmd/buildos-fork-init \
//	  --out ./forks/acme-construction/secrets \
//	  --kid acme-2026-q2 \
//	  --org-id 11111111-1111-1111-1111-111111111111
//
// The operator copies private.pem into their secret store, copies
// jwks.json + the kid into Brain's per-fork registration UI / API,
// commits public.pem + fork.yaml to the customer's BuildOS fork repo,
// and sets A2A_KEY_ID=<kid> in the deployment environment.
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
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
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
		outDir = flag.String("out", "", "output directory for generated artifacts (required)")
		kid    = flag.String("kid", "", "JWS key id; defaults to a fresh uuid")
		orgID  = flag.String("org-id", "", "fork's default org id (uuid); written to fork.yaml")
		bits   = flag.Int("bits", defaultKeyBits, "RSA key size in bits (2048 minimum)")
		seed   = flag.Int64("seed", 0, "deterministic seed for testing; ZERO uses crypto/rand (production)")
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

	if err := run(*outDir, *kid, *orgID, *bits, *seed); err != nil {
		fmt.Fprintf(os.Stderr, "buildos-fork-init: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nfork identity written to %s\n", *outDir)
	fmt.Println("next steps:")
	fmt.Println("  1. move private.pem into your secret store; configure A2A_SIGNING_KEY_PATH")
	fmt.Println("  2. paste jwks.json into Brain's per-fork registration")
	fmt.Println("  3. set A2A_KEY_ID=" + *kid + " in the deployment environment")
	fmt.Println("  4. commit public.pem + fork.yaml to your customer's BuildOS fork repo")
	fmt.Println("  5. NEVER commit private.pem — verify it's in your fork's .gitignore")
}

// run is the testable entrypoint. seed is 0 in production and uses
// crypto/rand; non-zero seeds derive the keypair deterministically
// for unit tests.
func run(outDir, kid, orgID string, bits int, seed int64) error {
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	var randSrc io.Reader = rand.Reader
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
	if err := writeJWKS(filepath.Join(outDir, "jwks.json"), &priv.PublicKey, kid); err != nil {
		return err
	}
	if err := writeForkYAML(filepath.Join(outDir, "fork.yaml"), &priv.PublicKey, kid, orgID); err != nil {
		return err
	}
	return nil
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
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: der})
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
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// writeJWKS emits a JSON Web Key Set with one RSA public key. Format
// matches RFC 7517; the inbound JWS verifier in BuildOS already
// parses this shape. Field encoding follows go-jose's conventions
// (base64url, no padding) — the same library both sides use.
func writeJWKS(path string, key *rsa.PublicKey, kid string) error {
	jwk := jwkRSA{
		KeyType:   "RSA",
		KeyID:     kid,
		Use:       "sig",
		Algorithm: "RS256",
		N:         base64URLEncode(key.N),
		E:         base64URLEncode(big.NewInt(int64(key.E))),
	}
	out := jwksDoc{Keys: []jwkRSA{jwk}}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal jwks: %w", err)
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

// writeForkYAML emits an operator-readable artifact. YAML by hand
// rather than gopkg.in/yaml.v3 to avoid a dep just for this; the
// shape is small and stable.
func writeForkYAML(path string, key *rsa.PublicKey, kid, orgID string) error {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return fmt.Errorf("marshal spki: %w", err)
	}
	fingerprint := sha256.Sum256(der)
	fpHex := hexLower(fingerprint[:])
	if orgID == "" {
		orgID = "<set DEFAULT_ORG_ID at deploy time>"
	}
	content := fmt.Sprintf(`# BuildOS fork identity — generated by buildos-fork-init
#
# This file is committed to the customer's BuildOS fork repo. The
# matching private key (private.pem) is NOT — it lives in the
# fork's secret store and is referenced via A2A_SIGNING_KEY_PATH.
#
# To rotate: regenerate with a new --kid, register the new public
# key with Brain, deploy with the new A2A_KEY_ID. Old key remains
# valid until removed from Brain's JWKS.

kid:           %q
created_at:    %q
default_org_id: %q
public_key_sha256: %q
key_size_bits: %d

# Environment variables BuildOS expects at runtime:
#   A2A_KEY_ID:           %s
#   A2A_SIGNING_KEY_PATH: <path-from-secret-store>/private.pem
#   DEFAULT_ORG_ID:       %s
`, kid, time.Now().UTC().Format(time.RFC3339), orgID, fpHex, key.N.BitLen(), kid, orgID)
	return os.WriteFile(path, []byte(content), 0o644)
}

// jwksDoc / jwkRSA are the wire-format types for a JWKS containing
// one or more RSA public keys.
type jwksDoc struct {
	Keys []jwkRSA `json:"keys"`
}

type jwkRSA struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	N         string `json:"n"`
	E         string `json:"e"`
}

// base64URLEncode applies RFC 7515 base64url-no-pad encoding to a
// big-endian unsigned-int representation of n. Matches the wire
// format every JWK consumer expects for RSA modulus / exponent.
func base64URLEncode(n *big.Int) string {
	return base64.RawURLEncoding.EncodeToString(n.Bytes())
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
