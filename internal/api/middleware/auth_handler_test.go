package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/futurebuildai/buildos/internal/auth"
	"github.com/futurebuildai/buildos/internal/obs"
)

// claimsCapture is a terminal handler that records what the Auth middleware
// stamped onto the request context: the validated claims and the correlation
// request id. It writes 200 so a passing chain is observable by status.
type claimsCapture struct {
	called bool
	claims Claims
	reqID  string
}

func (c *claimsCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.called = true
	c.claims, _ = ClaimsFromContext(r.Context())
	c.reqID = obs.RequestIDFromContext(r.Context())
	w.WriteHeader(http.StatusOK)
}

// quietLogger discards Auth's log output so test runs stay clean.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newIssuerVerifier builds a matched issuer/verifier pair backed by a fresh
// RSA key. opts let a test shift the clock or TTL to forge an expired token.
func newIssuerVerifier(t *testing.T, opts ...auth.IssuerOption) (*auth.TokenIssuer, *auth.Verifier) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	iss, err := auth.NewTokenIssuer(key, "kid-1", "buildos", "buildos", opts...)
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}
	ver, err := auth.NewVerifier(&key.PublicKey, "buildos", "buildos")
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return iss, ver
}

func TestAuth_JWTMode_ValidTokenPassesClaims(t *testing.T) {
	iss, ver := newIssuerVerifier(t)
	token, _, err := iss.Mint("user-1", "org-1", RoleAdmin, "enterprise")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	cap := &claimsCapture{}
	h := Auth(ver, "", quietLogger())(cap)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if !cap.called {
		t.Fatal("next handler not called")
	}
	if cap.claims.Sub != "user-1" || cap.claims.OrgID != "org-1" ||
		cap.claims.Role != RoleAdmin || cap.claims.PlanTier != "enterprise" {
		t.Errorf("claims = %+v, want user-1/org-1/admin/enterprise", cap.claims)
	}
	// Without chi's RequestID middleware in the chain the correlation id is empty.
	if cap.reqID != "" {
		t.Errorf("reqID = %q, want empty (no RequestID middleware)", cap.reqID)
	}
}

func TestAuth_JWTMode_Rejections(t *testing.T) {
	iss, ver := newIssuerVerifier(t)
	goodToken, _, err := iss.Mint("user-1", "org-1", RoleOwner, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	// A token with no role passes signature/expiry validation but fails the
	// required-claims gate inside Auth.
	noRoleToken, _, err := iss.Mint("user-1", "org-1", "", "")
	if err != nil {
		t.Fatalf("mint no-role: %v", err)
	}

	tests := []struct {
		name     string
		header   string
		wantBody string
	}{
		{"missing header", "", "missing Authorization header"},
		{"no bearer prefix", "Token " + goodToken, "invalid Authorization header format"},
		{"single token only", goodToken, "invalid Authorization header format"},
		{"garbage token", "Bearer not-a-jwt", "token verification failed"},
		{"missing required claims", "Bearer " + noRoleToken, "missing required claims"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &claimsCapture{}
			h := Auth(ver, "", quietLogger())(cap)
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("code = %d, want 401", rec.Code)
			}
			if cap.called {
				t.Error("next handler was called on a rejected request")
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAuth_JWTMode_ExpiredToken(t *testing.T) {
	// Mint against a clock an hour in the past with a 1-minute TTL so the
	// token's exp is well behind real now — the verifier (real clock) rejects
	// it as expired, exercising the jwt.ErrExpired branch distinctly.
	past := time.Now().Add(-time.Hour)
	iss, ver := newIssuerVerifier(t,
		auth.WithClock(func() time.Time { return past }),
		auth.WithAccessTTL(time.Minute),
	)
	token, _, err := iss.Mint("user-1", "org-1", RoleOwner, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	cap := &claimsCapture{}
	h := Auth(ver, "", quietLogger())(cap)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
	if cap.called {
		t.Error("next handler was called on an expired token")
	}
	if !strings.Contains(rec.Body.String(), "token expired") {
		t.Errorf("body = %q, want to contain %q", rec.Body.String(), "token expired")
	}
}

func TestAuth_StampsRequestCorrelation(t *testing.T) {
	iss, ver := newIssuerVerifier(t)
	token, _, err := iss.Mint("user-1", "org-1", RoleAdmin, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	cap := &claimsCapture{}
	// chi's RequestID middleware stamps a request id that withRequestCorrelation
	// must propagate onto the claims context under the obs key.
	h := chimw.RequestID(Auth(ver, "", quietLogger())(cap))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if cap.reqID == "" {
		t.Error("reqID not propagated onto context by withRequestCorrelation")
	}
}

func TestMustClaimsFromContext(t *testing.T) {
	t.Run("returns claims when present", func(t *testing.T) {
		want := Claims{Sub: "u1", OrgID: "o1", Role: RoleOwner}
		ctx := ContextWithClaims(t.Context(), want)
		if got := MustClaimsFromContext(ctx); got.Sub != want.Sub || got.Role != want.Role {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("panics when absent", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic on missing claims, got none")
			}
		}()
		_ = MustClaimsFromContext(t.Context())
	})
}
