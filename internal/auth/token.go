package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// DefaultAccessTTL is the lifetime of a freshly minted access token. Kept
// short because refresh tokens (opaque, server-revocable) handle longevity;
// a stolen access token is only useful for this window.
const DefaultAccessTTL = 15 * time.Minute

// opaqueTokenBytes is the CSPRNG entropy (in bytes) behind a refresh or
// password-reset token. 32 bytes == 256 bits, base64url-encoded to a 43-char
// cleartext that is shown to the client exactly once; only its sha256 hash is
// stored.
const opaqueTokenBytes = 32

// TokenClaims is the wire shape of a BuildOS-issued access token. The custom
// fields mirror what handlers read from the request context; the embedded
// jwt.Claims carries the registered claims (iss/sub/aud/exp/iat/nbf).
type TokenClaims struct {
	Sub      string `json:"sub"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
	PlanTier string `json:"plan_tier,omitempty"`
	jwt.Claims
}

// TokenIssuer mints signed RS256 access tokens. BuildOS owns the signing key
// now (one keypair per fork); there is no external OIDC provider.
type TokenIssuer struct {
	signer    jose.Signer
	issuer    string
	audience  string
	accessTTL time.Duration
	now       func() time.Time
}

// IssuerOption customizes a TokenIssuer at construction.
type IssuerOption func(*TokenIssuer)

// WithAccessTTL overrides the default access-token lifetime.
func WithAccessTTL(ttl time.Duration) IssuerOption {
	return func(ti *TokenIssuer) { ti.accessTTL = ttl }
}

// WithClock overrides the time source (tests inject a fixed clock).
func WithClock(now func() time.Time) IssuerOption {
	return func(ti *TokenIssuer) { ti.now = now }
}

// NewTokenIssuer builds an issuer from an RSA private key. kid is stamped into
// the JWS header so a future multi-key rotation can select the right verifier;
// issuer/audience become the iss/aud claims.
func NewTokenIssuer(priv *rsa.PrivateKey, kid, issuer, audience string, opts ...IssuerOption) (*TokenIssuer, error) {
	if priv == nil {
		return nil, errors.New("auth: private key must not be nil")
	}
	if issuer == "" || audience == "" {
		return nil, errors.New("auth: issuer and audience are required")
	}
	signerOpts := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", kid)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: priv}, signerOpts)
	if err != nil {
		return nil, fmt.Errorf("auth: build signer: %w", err)
	}
	ti := &TokenIssuer{
		signer:    signer,
		issuer:    issuer,
		audience:  audience,
		accessTTL: DefaultAccessTTL,
		now:       time.Now,
	}
	for _, o := range opts {
		o(ti)
	}
	return ti, nil
}

// Mint signs an access token for the given subject/org/role and returns the
// compact JWT plus its expiry. plan tier is optional.
func (ti *TokenIssuer) Mint(sub, orgID, role, planTier string) (token string, expiresAt time.Time, err error) {
	now := ti.now()
	exp := now.Add(ti.accessTTL)
	claims := TokenClaims{
		Sub:      sub,
		OrgID:    orgID,
		Role:     role,
		PlanTier: planTier,
		Claims: jwt.Claims{
			Issuer:    ti.issuer,
			Subject:   sub,
			Audience:  jwt.Audience{ti.audience},
			Expiry:    jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	raw, err := jwt.Signed(ti.signer).Claims(claims).Serialize()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return raw, exp, nil
}

// AccessTTL reports the configured access-token lifetime (handlers echo it as
// expires_in on the login/refresh response).
func (ti *TokenIssuer) AccessTTL() time.Duration { return ti.accessTTL }

// Verifier validates BuildOS-issued access tokens against the public half of
// the signing keypair. It checks signature, issuer, audience, and expiry.
type Verifier struct {
	pub      *rsa.PublicKey
	issuer   string
	audience string
}

// NewVerifier builds a Verifier from an RSA public key plus the expected
// issuer/audience.
func NewVerifier(pub *rsa.PublicKey, issuer, audience string) (*Verifier, error) {
	if pub == nil {
		return nil, errors.New("auth: public key must not be nil")
	}
	if issuer == "" || audience == "" {
		return nil, errors.New("auth: issuer and audience are required")
	}
	return &Verifier{pub: pub, issuer: issuer, audience: audience}, nil
}

// Verify parses and validates a compact RS256 JWT. On success it returns the
// decoded claims. The returned error is the raw go-jose validation error so
// callers can distinguish jwt.ErrExpired from other failures via errors.Is.
func (v *Verifier) Verify(raw string, now time.Time) (*TokenClaims, error) {
	tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, fmt.Errorf("auth: parse token: %w", err)
	}
	var claims TokenClaims
	if err := tok.Claims(v.pub, &claims); err != nil {
		return nil, fmt.Errorf("auth: verify signature: %w", err)
	}
	expected := jwt.Expected{
		Issuer:      v.issuer,
		AnyAudience: jwt.Audience{v.audience},
		Time:        now,
	}
	if err := claims.Validate(expected); err != nil {
		return nil, err
	}
	return &claims, nil
}

// GenerateOpaqueToken returns a fresh CSPRNG token: the base64url cleartext
// (shown to the client once) and its sha256 hex hash (stored). Used for both
// refresh tokens and password-reset tokens.
func GenerateOpaqueToken() (cleartext, hash string, err error) {
	b := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	cleartext = base64.RawURLEncoding.EncodeToString(b)
	return cleartext, HashOpaqueToken(cleartext), nil
}

// HashOpaqueToken returns the hex-encoded sha256 of an opaque token cleartext.
// Lookups hash the presented cleartext and compare against the stored hash, so
// a database leak never exposes a usable token.
func HashOpaqueToken(cleartext string) string {
	sum := sha256.Sum256([]byte(cleartext))
	return hex.EncodeToString(sum[:])
}

// ParseRSAPrivateKeyPEM decodes a PKCS#1 ("RSA PRIVATE KEY") or PKCS#8
// ("PRIVATE KEY") PEM block into an *rsa.PrivateKey.
func ParseRSAPrivateKeyPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("auth: no PEM block found in private key")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parse PKCS#1 key: %w", err)
		}
		return key, nil
	case "PRIVATE KEY":
		anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parse PKCS#8 key: %w", err)
		}
		key, ok := anyKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("auth: PKCS#8 key is %T, want *rsa.PrivateKey", anyKey)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("auth: unsupported private-key PEM block type %q", block.Type)
	}
}

// ParseRSAPublicKeyPEM decodes a PKIX ("PUBLIC KEY") or PKCS#1 ("RSA PUBLIC
// KEY") PEM block into an *rsa.PublicKey.
func ParseRSAPublicKeyPEM(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("auth: no PEM block found in public key")
	}
	switch block.Type {
	case "PUBLIC KEY":
		anyKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parse PKIX public key: %w", err)
		}
		key, ok := anyKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("auth: PKIX key is %T, want *rsa.PublicKey", anyKey)
		}
		return key, nil
	case "RSA PUBLIC KEY":
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parse PKCS#1 public key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("auth: unsupported public-key PEM block type %q", block.Type)
	}
}
