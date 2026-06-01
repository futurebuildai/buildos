package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func TestTokenIssuerVerifier_RoundTrip(t *testing.T) {
	key := testKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	iss, err := NewTokenIssuer(key, "kid-1", "buildos", "buildos", WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	raw, exp, err := iss.Mint("user-123", "org-abc", "owner", "pro")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if !exp.Equal(now.Add(DefaultAccessTTL)) {
		t.Errorf("expiry = %v, want %v", exp, now.Add(DefaultAccessTTL))
	}

	ver, err := NewVerifier(&key.PublicKey, "buildos", "buildos")
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	claims, err := ver.Verify(raw, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Sub != "user-123" || claims.OrgID != "org-abc" || claims.Role != "owner" || claims.PlanTier != "pro" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestVerify_ExpiredToken(t *testing.T) {
	key := testKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	iss, _ := NewTokenIssuer(key, "kid-1", "buildos", "buildos",
		WithClock(func() time.Time { return now }),
		WithAccessTTL(5*time.Minute),
	)
	raw, _, err := iss.Mint("u", "o", "admin", "")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	ver, _ := NewVerifier(&key.PublicKey, "buildos", "buildos")
	// 10 minutes later — past the 5-minute TTL.
	_, err = ver.Verify(raw, now.Add(10*time.Minute))
	if !errors.Is(err, jwt.ErrExpired) {
		t.Fatalf("err = %v, want jwt.ErrExpired", err)
	}
}

func TestVerify_WrongKey(t *testing.T) {
	key := testKey(t)
	other := testKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	iss, _ := NewTokenIssuer(key, "kid-1", "buildos", "buildos", WithClock(func() time.Time { return now }))
	raw, _, _ := iss.Mint("u", "o", "admin", "")

	ver, _ := NewVerifier(&other.PublicKey, "buildos", "buildos")
	if _, err := ver.Verify(raw, now); err == nil {
		t.Fatal("expected verification failure against wrong key")
	}
}

func TestVerify_WrongIssuerAudience(t *testing.T) {
	key := testKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	iss, _ := NewTokenIssuer(key, "kid-1", "buildos", "buildos", WithClock(func() time.Time { return now }))
	raw, _, _ := iss.Mint("u", "o", "admin", "")

	verBadIss, _ := NewVerifier(&key.PublicKey, "evil", "buildos")
	if _, err := verBadIss.Verify(raw, now); err == nil {
		t.Error("expected failure on issuer mismatch")
	}
	verBadAud, _ := NewVerifier(&key.PublicKey, "buildos", "evil")
	if _, err := verBadAud.Verify(raw, now); err == nil {
		t.Error("expected failure on audience mismatch")
	}
}

func TestNewTokenIssuer_Validation(t *testing.T) {
	key := testKey(t)
	if _, err := NewTokenIssuer(nil, "k", "buildos", "buildos"); err == nil {
		t.Error("expected error on nil key")
	}
	if _, err := NewTokenIssuer(key, "k", "", "buildos"); err == nil {
		t.Error("expected error on empty issuer")
	}
	if _, err := NewTokenIssuer(key, "k", "buildos", ""); err == nil {
		t.Error("expected error on empty audience")
	}
}

func TestGenerateOpaqueToken(t *testing.T) {
	ct1, h1, err := GenerateOpaqueToken()
	if err != nil {
		t.Fatalf("GenerateOpaqueToken: %v", err)
	}
	ct2, h2, _ := GenerateOpaqueToken()
	if ct1 == ct2 {
		t.Error("two opaque tokens collided")
	}
	if h1 == h2 {
		t.Error("two opaque token hashes collided")
	}
	if HashOpaqueToken(ct1) != h1 {
		t.Error("HashOpaqueToken not stable for the returned cleartext")
	}
	if len(ct1) != 43 { // 32 bytes raw-base64url
		t.Errorf("cleartext len = %d, want 43", len(ct1))
	}
}

func TestParseRSAKeyPEM_RoundTrip(t *testing.T) {
	key := testKey(t)
	privPEM := marshalPKCS8(t, key)
	pubPEM := marshalPKIX(t, &key.PublicKey)

	gotPriv, err := ParseRSAPrivateKeyPEM(privPEM)
	if err != nil {
		t.Fatalf("ParseRSAPrivateKeyPEM: %v", err)
	}
	if !gotPriv.Equal(key) {
		t.Error("parsed private key differs from original")
	}
	gotPub, err := ParseRSAPublicKeyPEM(pubPEM)
	if err != nil {
		t.Fatalf("ParseRSAPublicKeyPEM: %v", err)
	}
	if !gotPub.Equal(&key.PublicKey) {
		t.Error("parsed public key differs from original")
	}
}

func TestParseRSAKeyPEM_Garbage(t *testing.T) {
	if _, err := ParseRSAPrivateKeyPEM([]byte("not pem")); err == nil {
		t.Error("expected error on garbage private PEM")
	}
	if _, err := ParseRSAPublicKeyPEM([]byte("not pem")); err == nil {
		t.Error("expected error on garbage public PEM")
	}
}
