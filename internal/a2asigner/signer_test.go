package a2asigner

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
)

// genTestKey returns an RSA-2048 keypair for tests. 2048-bit is fast
// enough on every CI runner; the production deployments use the same
// size.
func genTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return key
}

func keyToPKCS8PEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func keyToPKCS1PEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func TestNewSignerFromPEM_PKCS8RoundTrip(t *testing.T) {
	key := genTestKey(t)
	signer, err := NewSignerFromPEM(keyToPKCS8PEM(t, key), "buildos-test-1")
	if err != nil {
		t.Fatalf("NewSignerFromPEM: %v", err)
	}
	if signer.KeyID() != "buildos-test-1" {
		t.Errorf("KeyID = %q, want buildos-test-1", signer.KeyID())
	}

	payload := []byte(`{"event_type":"card_actioned","trace_id":"abc"}`)
	sig, err := signer.SignDetached(payload)
	if err != nil {
		t.Fatalf("SignDetached: %v", err)
	}
	// Detached compact form looks like "<protected>..<signature>" with
	// an empty middle segment.
	if !strings.Contains(sig, "..") {
		t.Errorf("signature missing detached marker: %s", sig)
	}

	// Round-trip: verify with the public half — confirms the wire
	// format is a real JWS that consumers (Brain) can parse.
	jws, err := jose.ParseDetached(sig, payload, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatalf("ParseDetached: %v", err)
	}
	if _, err := jws.Verify(&key.PublicKey); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestNewSignerFromPEM_PKCS1Accepted(t *testing.T) {
	key := genTestKey(t)
	signer, err := NewSignerFromPEM(keyToPKCS1PEM(t, key), "kid-1")
	if err != nil {
		t.Fatalf("NewSignerFromPEM (PKCS#1): %v", err)
	}
	if _, err := signer.SignDetached([]byte(`{"x":1}`)); err != nil {
		t.Errorf("SignDetached: %v", err)
	}
}

func TestNewSignerFromPEM_RejectsBadInputs(t *testing.T) {
	cases := []struct {
		name string
		pem  []byte
		kid  string
	}{
		{"empty pem", []byte(``), "kid"},
		{"non-pem garbage", []byte(`not a pem`), "kid"},
		{"empty kid", keyToPKCS1PEM(t, genTestKey(t)), ""},
		{"unsupported block type", []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"), "kid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.kid != "" {
				if _, err := NewSignerFromPEM(c.pem, c.kid); err == nil {
					t.Error("expected error")
				}
			} else {
				// Empty kid is rejected by NewSignerFromFile only; the
				// PEM variant doesn't pre-check kid. Skip.
				t.Skip("kid validation lives in NewSignerFromFile")
			}
		})
	}
}

func TestNewSignerFromFile_RejectsEmptyPath(t *testing.T) {
	if _, err := NewSignerFromFile("", "kid"); err == nil {
		t.Error("expected error for empty path")
	}
	if _, err := NewSignerFromFile("/tmp/x", ""); err == nil {
		t.Error("expected error for empty kid")
	}
}

func TestSignDetached_RejectsEmptyPayload(t *testing.T) {
	signer, err := NewSignerFromPEM(keyToPKCS8PEM(t, genTestKey(t)), "kid")
	if err != nil {
		t.Fatalf("NewSignerFromPEM: %v", err)
	}
	if _, err := signer.SignDetached(nil); err == nil {
		t.Error("expected error for nil payload")
	}
	if _, err := signer.SignDetached([]byte{}); err == nil {
		t.Error("expected error for empty payload")
	}
}

func TestSignDetached_DifferentPayloadsProduceDifferentSignatures(t *testing.T) {
	signer, err := NewSignerFromPEM(keyToPKCS8PEM(t, genTestKey(t)), "kid")
	if err != nil {
		t.Fatalf("NewSignerFromPEM: %v", err)
	}
	a, err := signer.SignDetached([]byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("SignDetached a: %v", err)
	}
	b, err := signer.SignDetached([]byte(`{"x":2}`))
	if err != nil {
		t.Fatalf("SignDetached b: %v", err)
	}
	if a == b {
		t.Error("identical signatures for different payloads")
	}
	// Both must remain detached.
	if !bytes.Contains([]byte(a), []byte("..")) || !bytes.Contains([]byte(b), []byte("..")) {
		t.Error("expected detached form (..)")
	}
}
