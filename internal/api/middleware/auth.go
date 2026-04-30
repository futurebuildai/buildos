package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/futurebuildai/buildos/internal/brain"
)

// Claims represents the JWT claims issued by The Brain OIDC Provider.
type Claims struct {
	Sub      string `json:"sub"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
	PlanTier string `json:"plan_tier"`
	jwt.Claims
}

type claimsContextKey struct{}

// ContextWithClaims returns a new context carrying the given Claims.
// Production middleware uses this; tests in other packages use this to
// install fake claims without exporting the unexported context key.
func ContextWithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

// ClaimsFromContext extracts the authenticated Claims from the request context.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsContextKey{}).(Claims)
	return c, ok
}

// MustClaimsFromContext extracts Claims and panics if absent.
// Only use in handlers already protected by Auth middleware.
func MustClaimsFromContext(ctx context.Context) Claims {
	c, ok := ClaimsFromContext(ctx)
	if !ok {
		panic("middleware: Claims not found in context — is Auth middleware applied?")
	}
	return c
}

// JWKSProvider fetches and caches the JSON Web Key Set from The Brain.
type JWKSProvider struct {
	jwksURL    string
	httpClient *http.Client
	logger     *slog.Logger

	mu        sync.RWMutex
	keySet    *jose.JSONWebKeySet
	fetchedAt time.Time
	cacheTTL  time.Duration
}

// NewJWKSProvider creates a provider that fetches JWKS from the given URL.
func NewJWKSProvider(jwksURL string, logger *slog.Logger) *JWKSProvider {
	return &JWKSProvider{
		jwksURL: jwksURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:   logger,
		cacheTTL: 5 * time.Minute,
	}
}

// GetKeySet returns the cached JWKS, refreshing if stale.
func (p *JWKSProvider) GetKeySet(ctx context.Context) (*jose.JSONWebKeySet, error) {
	p.mu.RLock()
	if p.keySet != nil && time.Since(p.fetchedAt) < p.cacheTTL {
		ks := p.keySet
		p.mu.RUnlock()
		return ks, nil
	}
	p.mu.RUnlock()

	return p.refresh(ctx)
}

func (p *JWKSProvider) refresh(ctx context.Context) (*jose.JSONWebKeySet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if p.keySet != nil && time.Since(p.fetchedAt) < p.cacheTTL {
		return p.keySet, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating JWKS request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		// On fetch failure, return stale cache if available
		if p.keySet != nil {
			p.logger.Warn("JWKS refresh failed, using stale cache", "error", err)
			return p.keySet, nil
		}
		return nil, fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if p.keySet != nil {
			p.logger.Warn("JWKS refresh returned non-200, using stale cache", "status", resp.StatusCode)
			return p.keySet, nil
		}
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var keySet jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&keySet); err != nil {
		return nil, fmt.Errorf("decoding JWKS: %w", err)
	}

	p.keySet = &keySet
	p.fetchedAt = time.Now()
	p.logger.Info("JWKS refreshed", "key_count", len(keySet.Keys))

	return p.keySet, nil
}

// Auth creates middleware that validates JWT Bearer tokens from The Brain.
//
// authMode controls the path:
//   - ""        production: validate JWT against jwks/issuerURL
//   - "header"  dev/CI: read claims from X-Dev-Auth: <sub>,<org_id>,<role>[,<plan_tier>]
//
// For staging or sales demos, leave authMode empty and point BRAIN_JWKS_URL/
// BRAIN_ISSUER_URL at the cmd/dev-idp mini-IdP — no middleware change needed.
func Auth(jwks *JWKSProvider, issuerURL, authMode string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authMode == "header" {
				claims, err := claimsFromDevHeader(r.Header.Get("X-Dev-Auth"))
				if err != nil {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "X-Dev-Auth invalid: "+err.Error())
					return
				}
				ctx := ContextWithClaims(r.Context(), claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Extract Bearer token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid Authorization header format")
				return
			}
			rawToken := parts[1]

			// Parse the token
			tok, err := jwt.ParseSigned(rawToken, []jose.SignatureAlgorithm{jose.RS256})
			if err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token format")
				return
			}

			// Fetch JWKS
			keySet, err := jwks.GetKeySet(r.Context())
			if err != nil {
				logger.Error("failed to fetch JWKS", "error", err)
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unable to validate token")
				return
			}

			// Find the matching key and verify signature
			var claims Claims
			if err := verifyToken(tok, keySet, &claims); err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token verification failed")
				return
			}

			// Validate standard claims
			expected := jwt.Expected{
				Issuer:      issuerURL,
				AnyAudience: jwt.Audience{"fb-os"},
				Time:        time.Now(),
			}

			if err := claims.Claims.Validate(expected); err != nil {
				if errors.Is(err, jwt.ErrExpired) {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token expired")
				} else {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token validation failed")
				}
				return
			}

			// Validate required custom claims
			if claims.Sub == "" || claims.OrgID == "" || claims.Role == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing required claims")
				return
			}

			// Inject claims AND raw token into context. Service-layer
			// code that calls Brain reads the token from ctx via
			// brain.TokenFromContext — no need to plumb it through every
			// service-method signature.
			ctx := ContextWithClaims(r.Context(), claims)
			ctx = brain.ContextWithToken(ctx, rawToken)
			// Propagate chi's request ID into the brain ctx so
			// outbound calls stamp X-Request-ID for end-to-end
			// correlation. Empty (e.g. RequestID middleware not
			// mounted) just disables the header — no harm.
			if reqID := chimw.GetReqID(r.Context()); reqID != "" {
				ctx = brain.ContextWithRequestID(ctx, reqID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// claimsFromDevHeader parses an X-Dev-Auth header value of the form
// "<sub>,<org_id>,<role>[,<plan_tier>]" into Claims. Whitespace around
// each field is trimmed. plan_tier defaults to "enterprise" if omitted.
func claimsFromDevHeader(h string) (Claims, error) {
	if h == "" {
		return Claims{}, errors.New("missing X-Dev-Auth header")
	}
	parts := strings.Split(h, ",")
	if len(parts) < 3 || len(parts) > 4 {
		return Claims{}, errors.New("expected sub,org_id,role[,plan_tier]")
	}
	sub := strings.TrimSpace(parts[0])
	orgID := strings.TrimSpace(parts[1])
	role := strings.TrimSpace(parts[2])
	planTier := "enterprise"
	if len(parts) == 4 {
		if t := strings.TrimSpace(parts[3]); t != "" {
			planTier = t
		}
	}
	if sub == "" || orgID == "" || role == "" {
		return Claims{}, errors.New("sub, org_id, and role are required")
	}
	return Claims{Sub: sub, OrgID: orgID, Role: role, PlanTier: planTier}, nil
}

// verifyToken attempts to verify the token against any key in the JWKS.
func verifyToken(tok *jwt.JSONWebToken, keySet *jose.JSONWebKeySet, claims *Claims) error {
	for _, key := range keySet.Keys {
		if key.Use == "sig" || key.Use == "" {
			pubKey, ok := key.Key.(*rsa.PublicKey)
			if !ok {
				continue
			}
			if err := tok.Claims(pubKey, claims); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("no matching key found in JWKS")
}
