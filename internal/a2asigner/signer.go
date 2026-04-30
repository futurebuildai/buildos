// Package a2asigner produces detached JWS RS256 signatures for outbound
// A2A webhook events. Mirrors the inbound verifier in
// internal/api/a2a.go: a2a payloads travel as raw JSON in the body, with
// the signature in an X-JWS-Signature header.
//
// Each customer-fork BuildOS deployment owns its own RSA keypair. The
// public half is published in the deployment's JWKS so The Brain can
// verify; the private half is loaded from disk via NewSignerFromFile.
// Generating keys at runtime is intentionally not supported — keys
// must be operator-provisioned to survive process restarts and to
// support out-of-band JWKS rotation.
package a2asigner

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/go-jose/go-jose/v4"
)

// Signer wraps a configured jose.Signer for the lifetime of the
// process. Reuse one instance per signing key — internally jose.Signer
// is safe for concurrent use.
type Signer struct {
	inner jose.Signer
	keyID string
}

// SignDetached signs the given payload with RS256 and returns the
// detached compact serialization Brain expects in the X-JWS-Signature
// header (form: `<protected>..<signature>` with the payload omitted).
//
// Detached form keeps the wire envelope clean: Brain reads the body as
// raw JSON without re-parsing through a JWS layer.
func (s *Signer) SignDetached(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("a2asigner: payload is empty")
	}
	jws, err := s.inner.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("a2asigner: sign payload: %w", err)
	}
	return jws.DetachedCompactSerialize()
}

// KeyID returns the JWS `kid` header value the signer stamps. Useful
// for diagnostics — log this on every dispatch so a Brain-side
// rejection can be correlated to a specific BuildOS key generation.
func (s *Signer) KeyID() string { return s.keyID }

// NewSignerFromFile loads an RSA private key from a PKCS#8 PEM file at
// path and returns a configured Signer with the given keyID stamped
// into every JWS header.
//
// Generation script for operators:
//
//	openssl genpkey -algorithm RSA -out priv.pem -pkeyopt rsa_keygen_bits:2048
//	openssl pkey -in priv.pem -pubout -out pub.pem  # publish via JWKS
//
// The private key never leaves the BuildOS deployment. The public key
// must be added to the deployment's JWKS for Brain to verify.
func NewSignerFromFile(path, keyID string) (*Signer, error) {
	if path == "" {
		return nil, errors.New("a2asigner: key path is empty")
	}
	if keyID == "" {
		return nil, errors.New("a2asigner: key id is empty")
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("a2asigner: read key file %q: %w", path, err)
	}
	return NewSignerFromPEM(pemBytes, keyID)
}

// NewSignerFromPEM is the in-memory variant useful in tests and in
// deployments that pull the key from a secret manager rather than the
// filesystem. Accepts PKCS#1 ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE
// KEY") PEM blocks.
func NewSignerFromPEM(pemBytes []byte, keyID string) (*Signer, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("a2asigner: no PEM block found")
	}

	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		// PKCS#1.
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("a2asigner: parse PKCS#1 key: %w", err)
		}
		key = k
	case "PRIVATE KEY":
		// PKCS#8.
		anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("a2asigner: parse PKCS#8 key: %w", err)
		}
		rsaKey, ok := anyKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("a2asigner: PKCS#8 key is %T, want *rsa.PrivateKey", anyKey)
		}
		key = rsaKey
	default:
		return nil, fmt.Errorf("a2asigner: unsupported PEM block type %q", block.Type)
	}

	opts := (&jose.SignerOptions{}).
		WithType("JWT"). // Inbound verifier doesn't enforce typ but Brain reads it
		WithHeader("kid", keyID)

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("a2asigner: build signer: %w", err)
	}
	return &Signer{inner: signer, keyID: keyID}, nil
}
