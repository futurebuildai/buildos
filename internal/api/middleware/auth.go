package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/futurebuildai/buildos/internal/auth"
	"github.com/futurebuildai/buildos/internal/obs"
)

// Claims represents the validated claims on a BuildOS-issued access token.
// BuildOS now mints and validates its own RS256 JWTs (no external OIDC
// provider); the wire shape is defined by auth.TokenClaims.
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

// Auth creates middleware that validates RS256 Bearer tokens minted by this
// BuildOS deployment's own TokenIssuer (see internal/auth).
//
// authMode controls the path:
//   - ""        production: validate the JWT against the local verifier
//   - "header"  dev/CI: read claims from X-Dev-Auth: <sub>,<org_id>,<role>[,<plan_tier>]
//
// The "header" path is compiled out of prod builds (see auth_dev.go /
// auth_prod.go, D8 hardening).
func Auth(verifier *auth.Verifier, authMode string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authMode == "header" {
				claims, err := claimsFromDevHeader(r.Header.Get("X-Dev-Auth"))
				if err != nil {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "X-Dev-Auth invalid: "+err.Error())
					return
				}
				next.ServeHTTP(w, r.WithContext(withRequestCorrelation(r, ContextWithClaims(r.Context(), claims))))
				return
			}

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

			tokClaims, err := verifier.Verify(rawToken, time.Now())
			if err != nil {
				if errors.Is(err, jwt.ErrExpired) {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token expired")
				} else {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token verification failed")
				}
				return
			}

			if tokClaims.Sub == "" || tokClaims.OrgID == "" || tokClaims.Role == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing required claims")
				return
			}

			claims := Claims{
				Sub:      tokClaims.Sub,
				OrgID:    tokClaims.OrgID,
				Role:     tokClaims.Role,
				PlanTier: tokClaims.PlanTier,
				Claims:   tokClaims.Claims,
			}
			next.ServeHTTP(w, r.WithContext(withRequestCorrelation(r, ContextWithClaims(r.Context(), claims))))
		})
	}
}

// withRequestCorrelation stamps chi's request id onto the context under the
// obs key so downstream egress (audit log, structured logs) carries the
// correlation id. Empty (RequestID middleware not mounted) is a harmless
// no-op.
func withRequestCorrelation(r *http.Request, ctx context.Context) context.Context {
	if reqID := chimw.GetReqID(r.Context()); reqID != "" {
		ctx = obs.ContextWithRequestID(ctx, reqID)
	}
	return ctx
}

// claimsFromDevHeader is defined in auth_dev.go (build !prod) and
// auth_prod.go (build prod). The non-prod implementation parses an
// X-Dev-Auth header into Claims; the prod stub always returns an
// error so DEV_AUTH_MODE=header is a no-op in prod binaries.
//
// This is the D8 build-tag hardening: prod images cannot reactivate
// the dev-auth bypass via env flip. See ADR-002.
