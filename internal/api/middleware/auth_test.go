package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// =============================================================================
// Helpers
// =============================================================================

func mustTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test RSA key: %v", err)
	}
	return key
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testCustomClaims is a flat struct combining standard JWT claims and custom
// FB-Brain claims for test token generation. Using a struct (not a map) avoids
// the go-jose "expected claims to be value convertible into JSON object" error
// that occurs when chaining multiple .Claims() calls.
type testCustomClaims struct {
	Sub      string `json:"sub"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
	PlanTier string `json:"plan_tier"`
}

// signTestToken creates a signed RS256 JWT string using the given key and claims.
func signTestToken(t *testing.T, key *rsa.PrivateKey, kid string, claims Claims) string {
	t.Helper()

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithHeader("kid", kid),
	)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	custom := testCustomClaims{
		Sub:      claims.Sub,
		OrgID:    claims.OrgID,
		Role:     claims.Role,
		PlanTier: claims.PlanTier,
	}

	token, err := jwt.Signed(signer).Claims(claims.Claims).Claims(custom).Serialize()
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return token
}

// newTestJWKS creates a JWKSProvider that serves a static JWKS endpoint with the given public key.
func newTestJWKS(t *testing.T, key *rsa.PrivateKey, kid string) (*JWKSProvider, *httptest.Server) {
	t.Helper()

	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				Key:       &key.PublicKey,
				KeyID:     kid,
				Algorithm: string(jose.RS256),
				Use:       "sig",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))

	provider := NewJWKSProvider(srv.URL, discardLogger())
	return provider, srv
}

// validClaims returns test Claims with valid standard JWT fields.
func validClaims(issuer string) Claims {
	now := time.Now()
	return Claims{
		Sub:      "user-abc-123",
		OrgID:    "org-xyz-456",
		Role:     "owner",
		PlanTier: "enterprise",
		Claims: jwt.Claims{
			Issuer:    issuer,
			Audience:  jwt.Audience{"fb-os"},
			IssuedAt:  jwt.NewNumericDate(now.Add(-1 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Minute)),
			Expiry:    jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
	}
}

// echoHandler returns the claims as JSON for inspection.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"no claims in context"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"sub":       claims.Sub,
			"org_id":    claims.OrgID,
			"role":      claims.Role,
			"plan_tier": claims.PlanTier,
		})
	})
}

// parseErrorResp extracts the error body from a response.
func parseErrorResp(t *testing.T, resp *http.Response) errorBody {
	t.Helper()
	var errResp errorResponse
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("parse error response: %v; body: %s", err, body)
	}
	return errResp.Error
}

// =============================================================================
// ClaimsFromContext / ContextWithClaims Tests
// =============================================================================

func TestClaimsFromContext_Present(t *testing.T) {
	c := Claims{Sub: "user-1", OrgID: "org-1", Role: "admin", PlanTier: "pro"}
	ctx := ContextWithClaims(context.Background(), c)

	got, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("expected claims to be present in context")
	}
	if got.Sub != "user-1" {
		t.Errorf("Sub = %q, want %q", got.Sub, "user-1")
	}
	if got.OrgID != "org-1" {
		t.Errorf("OrgID = %q, want %q", got.OrgID, "org-1")
	}
	if got.Role != "admin" {
		t.Errorf("Role = %q, want %q", got.Role, "admin")
	}
	if got.PlanTier != "pro" {
		t.Errorf("PlanTier = %q, want %q", got.PlanTier, "pro")
	}
}

func TestClaimsFromContext_Absent(t *testing.T) {
	_, ok := ClaimsFromContext(context.Background())
	if ok {
		t.Error("expected claims to be absent from empty context")
	}
}

func TestMustClaimsFromContext_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when claims are absent")
		}
	}()
	MustClaimsFromContext(context.Background())
}

func TestMustClaimsFromContext_Returns(t *testing.T) {
	c := Claims{Sub: "user-2", OrgID: "org-2", Role: "owner"}
	ctx := ContextWithClaims(context.Background(), c)

	got := MustClaimsFromContext(ctx)
	if got.Sub != "user-2" {
		t.Errorf("Sub = %q, want %q", got.Sub, "user-2")
	}
}

// =============================================================================
// Auth Middleware — Valid Token
// =============================================================================

func TestAuth_ValidToken(t *testing.T) {
	key := mustTestKey(t)
	kid := "test-key-1"
	issuer := "https://brain.futurebuild.ai"

	jwksProvider, srv := newTestJWKS(t, key, kid)
	defer srv.Close()

	claims := validClaims(issuer)
	token := signTestToken(t, key, kid, claims)

	handler := Auth(jwksProvider, issuer, false, discardLogger())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var result map[string]string
	json.NewDecoder(rec.Body).Decode(&result)

	if result["sub"] != "user-abc-123" {
		t.Errorf("sub = %q, want %q", result["sub"], "user-abc-123")
	}
	if result["org_id"] != "org-xyz-456" {
		t.Errorf("org_id = %q, want %q", result["org_id"], "org-xyz-456")
	}
	if result["role"] != "owner" {
		t.Errorf("role = %q, want %q", result["role"], "owner")
	}
	if result["plan_tier"] != "enterprise" {
		t.Errorf("plan_tier = %q, want %q", result["plan_tier"], "enterprise")
	}
}

// =============================================================================
// Auth Middleware — Missing Authorization Header
// =============================================================================

func TestAuth_MissingAuthHeader(t *testing.T) {
	key := mustTestKey(t)
	jwksProvider, srv := newTestJWKS(t, key, "kid")
	defer srv.Close()

	handler := Auth(jwksProvider, "https://brain.test", false, discardLogger())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	// No Authorization header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	errBody := parseErrorResp(t, rec.Result())
	if errBody.Code != "UNAUTHORIZED" {
		t.Errorf("error code = %q, want %q", errBody.Code, "UNAUTHORIZED")
	}
	if !strings.Contains(errBody.Message, "missing Authorization header") {
		t.Errorf("unexpected message: %q", errBody.Message)
	}
}

// =============================================================================
// Auth Middleware — Invalid Token Format
// =============================================================================

func TestAuth_InvalidTokenFormat(t *testing.T) {
	key := mustTestKey(t)
	jwksProvider, srv := newTestJWKS(t, key, "kid")
	defer srv.Close()

	handler := Auth(jwksProvider, "https://brain.test", false, discardLogger())(echoHandler())

	tests := []struct {
		name   string
		header string
		errMsg string
	}{
		{
			name:   "no bearer prefix",
			header: "Token abc123",
			errMsg: "invalid Authorization header format",
		},
		{
			name:   "empty bearer",
			header: "Bearer ",
			errMsg: "invalid token format",
		},
		{
			name:   "garbage token",
			header: "Bearer not-a-jwt-token",
			errMsg: "invalid token format",
		},
		{
			name:   "just the word bearer",
			header: "Bearer",
			errMsg: "invalid Authorization header format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tc.header)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}

			errBody := parseErrorResp(t, rec.Result())
			if errBody.Code != "UNAUTHORIZED" {
				t.Errorf("error code = %q, want UNAUTHORIZED", errBody.Code)
			}
		})
	}
}

// =============================================================================
// Auth Middleware — Wrong Signing Key
// =============================================================================

func TestAuth_WrongSigningKey(t *testing.T) {
	serverKey := mustTestKey(t)
	wrongKey := mustTestKey(t)

	jwksProvider, srv := newTestJWKS(t, serverKey, "server-key")
	defer srv.Close()

	issuer := "https://brain.test"
	claims := validClaims(issuer)
	// Sign with wrong key
	token := signTestToken(t, wrongKey, "wrong-key", claims)

	handler := Auth(jwksProvider, issuer, false, discardLogger())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// =============================================================================
// Auth Middleware — Expired Token
// =============================================================================

func TestAuth_ExpiredToken(t *testing.T) {
	key := mustTestKey(t)
	kid := "test-key"
	issuer := "https://brain.test"

	jwksProvider, srv := newTestJWKS(t, key, kid)
	defer srv.Close()

	claims := validClaims(issuer)
	// Set expiry in the past
	claims.Claims.Expiry = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))

	token := signTestToken(t, key, kid, claims)

	handler := Auth(jwksProvider, issuer, false, discardLogger())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	errBody := parseErrorResp(t, rec.Result())
	if !strings.Contains(errBody.Message, "token expired") {
		t.Errorf("expected 'token expired' message, got %q", errBody.Message)
	}
}

// =============================================================================
// Auth Middleware — Wrong Issuer
// =============================================================================

func TestAuth_WrongIssuer(t *testing.T) {
	key := mustTestKey(t)
	kid := "test-key"

	jwksProvider, srv := newTestJWKS(t, key, kid)
	defer srv.Close()

	// Token signed with wrong issuer
	claims := validClaims("https://wrong-issuer.test")
	token := signTestToken(t, key, kid, claims)

	handler := Auth(jwksProvider, "https://brain.test", false, discardLogger())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// =============================================================================
// Auth Middleware — Missing Required Custom Claims
// =============================================================================

func TestAuth_MissingCustomClaims(t *testing.T) {
	key := mustTestKey(t)
	kid := "test-key"
	issuer := "https://brain.test"

	jwksProvider, srv := newTestJWKS(t, key, kid)
	defer srv.Close()

	tests := []struct {
		name string
		sub  string
		org  string
		role string
	}{
		{"missing sub", "", "org-1", "owner"},
		{"missing org_id", "user-1", "", "owner"},
		{"missing role", "user-1", "org-1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := validClaims(issuer)
			claims.Sub = tc.sub
			claims.OrgID = tc.org
			claims.Role = tc.role

			token := signTestToken(t, key, kid, claims)

			handler := Auth(jwksProvider, issuer, false, discardLogger())(echoHandler())

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}

			errBody := parseErrorResp(t, rec.Result())
			if !strings.Contains(errBody.Message, "missing required claims") {
				t.Errorf("expected 'missing required claims', got %q", errBody.Message)
			}
		})
	}
}

// =============================================================================
// Auth Middleware — Dev Bypass Mode
// =============================================================================

func TestAuth_DevBypass(t *testing.T) {
	// In dev bypass mode, no JWKS or token needed
	handler := Auth(nil, "", true, discardLogger())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	// No Authorization header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 in dev bypass mode", rec.Code)
	}

	var result map[string]string
	json.NewDecoder(rec.Body).Decode(&result)

	if result["sub"] != "dev-user-00000000-0000-0000-0000-000000000000" {
		t.Errorf("sub = %q, want dev user", result["sub"])
	}
	if result["org_id"] != "dev-org-00000000-0000-0000-0000-000000000000" {
		t.Errorf("org_id = %q, want dev org", result["org_id"])
	}
	if result["role"] != "owner" {
		t.Errorf("role = %q, want owner", result["role"])
	}
	if result["plan_tier"] != "enterprise" {
		t.Errorf("plan_tier = %q, want enterprise", result["plan_tier"])
	}
}

func TestAuth_DevBypass_ClaimsInContext(t *testing.T) {
	var extractedClaims Claims
	var found bool

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		extractedClaims, found = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(nil, "", true, discardLogger())(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !found {
		t.Fatal("claims should be in context in dev bypass mode")
	}
	if extractedClaims.Sub != "dev-user-00000000-0000-0000-0000-000000000000" {
		t.Errorf("Sub = %q", extractedClaims.Sub)
	}
}

// =============================================================================
// JWKSProvider Tests
// =============================================================================

func TestJWKSProvider_FetchAndCache(t *testing.T) {
	key := mustTestKey(t)
	fetchCount := 0

	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "key-1", Algorithm: "RS256", Use: "sig"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer srv.Close()

	provider := NewJWKSProvider(srv.URL, discardLogger())

	// First fetch
	ks, err := provider.GetKeySet(context.Background())
	if err != nil {
		t.Fatalf("GetKeySet: %v", err)
	}
	if len(ks.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(ks.Keys))
	}

	// Second fetch should use cache
	ks2, err := provider.GetKeySet(context.Background())
	if err != nil {
		t.Fatalf("GetKeySet (cached): %v", err)
	}
	if len(ks2.Keys) != 1 {
		t.Fatalf("expected 1 cached key, got %d", len(ks2.Keys))
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 HTTP fetch (cache hit), got %d", fetchCount)
	}
}

func TestJWKSProvider_StaleCache_OnError(t *testing.T) {
	key := mustTestKey(t)
	callCount := 0

	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "key-1", Algorithm: "RS256", Use: "sig"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount > 1 {
			// Simulate failure on subsequent requests
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer srv.Close()

	provider := NewJWKSProvider(srv.URL, discardLogger())

	// First fetch succeeds
	_, err := provider.GetKeySet(context.Background())
	if err != nil {
		t.Fatalf("first GetKeySet: %v", err)
	}

	// Expire the cache manually
	provider.mu.Lock()
	provider.fetchedAt = time.Now().Add(-10 * time.Minute)
	provider.mu.Unlock()

	// Second fetch fails at HTTP level but returns stale cache
	ks, err := provider.GetKeySet(context.Background())
	if err != nil {
		t.Fatalf("GetKeySet with stale cache: %v", err)
	}
	if len(ks.Keys) != 1 {
		t.Errorf("expected stale cache to still have 1 key, got %d", len(ks.Keys))
	}
}

func TestJWKSProvider_NoCache_FetchFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	provider := NewJWKSProvider(srv.URL, discardLogger())

	_, err := provider.GetKeySet(context.Background())
	if err == nil {
		t.Fatal("expected error when no cache and fetch fails")
	}
}

// =============================================================================
// RBAC Middleware — RequireRole
// =============================================================================

func TestRequireRole_Allowed(t *testing.T) {
	mw := RequireRole("owner", "admin")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "o", Role: role})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 for role %q", rec.Code, role)
			}
		})
	}
}

func TestRequireRole_Denied(t *testing.T) {
	mw := RequireRole("owner", "admin")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, role := range []string{"superintendent", "field_worker", "unknown"} {
		t.Run(role, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "o", Role: role})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403 for role %q", rec.Code, role)
			}
		})
	}
}

func TestRequireRole_NoClaims(t *testing.T) {
	mw := RequireRole("owner")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when no claims", rec.Code)
	}
}

// =============================================================================
// RBAC Middleware — RequireMinRole
// =============================================================================

func TestRequireMinRole_Hierarchy(t *testing.T) {
	tests := []struct {
		minRole    string
		userRole   string
		wantStatus int
	}{
		// field_worker(1) < superintendent(2) < admin(3) < owner(4)
		{"field_worker", "field_worker", http.StatusOK},
		{"field_worker", "superintendent", http.StatusOK},
		{"field_worker", "admin", http.StatusOK},
		{"field_worker", "owner", http.StatusOK},

		{"superintendent", "field_worker", http.StatusForbidden},
		{"superintendent", "superintendent", http.StatusOK},
		{"superintendent", "admin", http.StatusOK},
		{"superintendent", "owner", http.StatusOK},

		{"admin", "field_worker", http.StatusForbidden},
		{"admin", "superintendent", http.StatusForbidden},
		{"admin", "admin", http.StatusOK},
		{"admin", "owner", http.StatusOK},

		{"owner", "field_worker", http.StatusForbidden},
		{"owner", "superintendent", http.StatusForbidden},
		{"owner", "admin", http.StatusForbidden},
		{"owner", "owner", http.StatusOK},
	}

	for _, tc := range tests {
		name := tc.minRole + "_requires_" + tc.userRole
		t.Run(name, func(t *testing.T) {
			mw := RequireMinRole(tc.minRole)
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "o", Role: tc.userRole})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestRequireMinRole_UnknownMinRole_BlocksAll(t *testing.T) {
	mw := RequireMinRole("superadmin") // not in hierarchy
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "o", Role: "owner"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for unknown min role", rec.Code)
	}
}

func TestRequireMinRole_UnknownUserRole_Denied(t *testing.T) {
	mw := RequireMinRole("field_worker")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "o", Role: "viewer"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for unknown user role", rec.Code)
	}
}

func TestRequireMinRole_NoClaims(t *testing.T) {
	mw := RequireMinRole("field_worker")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// =============================================================================
// RBAC Middleware — RequireWriteRole
// =============================================================================

func TestRequireWriteRole_ReadMethodsPassThrough(t *testing.T) {
	handler := RequireWriteRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	readMethods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}

	for _, method := range readMethods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/", nil)
			// No claims needed for read methods
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 for %s", rec.Code, method)
			}
		})
	}
}

func TestRequireWriteRole_WriteMethods_OwnerAndAdmin(t *testing.T) {
	handler := RequireWriteRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	writeMethods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

	for _, role := range []string{"owner", "admin"} {
		for _, method := range writeMethods {
			t.Run(role+"_"+method, func(t *testing.T) {
				req := httptest.NewRequest(method, "/", nil)
				ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "o", Role: role})
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Errorf("status = %d, want 200 for %s %s", rec.Code, role, method)
				}
			})
		}
	}
}

func TestRequireWriteRole_WriteMethods_NonAdminDenied(t *testing.T) {
	handler := RequireWriteRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, role := range []string{"superintendent", "field_worker"} {
		t.Run(role+"_POST", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "o", Role: role})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}

func TestRequireWriteRole_WriteMethods_NoClaims(t *testing.T) {
	handler := RequireWriteRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// =============================================================================
// Role Constants
// =============================================================================

func TestRoleConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Owner", RoleOwner, "owner"},
		{"Admin", RoleAdmin, "admin"},
		{"Superintendent", RoleSuperintendent, "superintendent"},
		{"FieldWorker", RoleFieldWorker, "field_worker"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestRoleHierarchy_Ordering(t *testing.T) {
	// field_worker < superintendent < admin < owner
	if roleHierarchy[RoleFieldWorker] >= roleHierarchy[RoleSuperintendent] {
		t.Error("field_worker should be lower than superintendent")
	}
	if roleHierarchy[RoleSuperintendent] >= roleHierarchy[RoleAdmin] {
		t.Error("superintendent should be lower than admin")
	}
	if roleHierarchy[RoleAdmin] >= roleHierarchy[RoleOwner] {
		t.Error("admin should be lower than owner")
	}
}

// =============================================================================
// writeError Tests
// =============================================================================

func TestWriteError_Format(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusForbidden, "FORBIDDEN", "you shall not pass")

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "FORBIDDEN" {
		t.Errorf("error code = %q", resp.Error.Code)
	}
	if resp.Error.Message != "you shall not pass" {
		t.Errorf("error message = %q", resp.Error.Message)
	}
}
