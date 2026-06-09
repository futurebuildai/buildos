package api

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/auth"
)

// TestNewRouter_BootsWithSetupService is a regression guard for a chi
// boot panic: "all middlewares must be defined before routes on a mux".
//
// The authenticated route group installs SetupGate via r.Use. chi
// forbids r.Use after any route has been mounted on the same mux, so
// the gate MUST be registered before MountSetupRoutes (and every other
// route) in the group. SetupGate only mounts when SetupService is
// non-nil — the production path — so a fresh fork booting with the
// wizard wired is exactly the configuration that used to panic.
//
// NewRouter builds and mounts everything eagerly, so the panic fired at
// construction time. We assert NewRouter returns without panicking and
// that the resulting handler still serves the unauthenticated /health
// probe, proving the route tree was assembled.
func TestNewRouter_BootsWithSetupService(t *testing.T) {
	// DevAuthMode "header" lets the auth middleware build without a
	// Verifier; every service can be nil because NewRouter only
	// constructs handlers and mounts routes — it never invokes them.
	cfg := RouterConfig{
		DevAuthMode:  "header",
		SetupService: &mockSetupService{},
	}

	var handler http.Handler
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("NewRouter panicked with SetupService wired: %v", rec)
			}
		}()
		handler = NewRouter(cfg)
	}()

	if handler == nil {
		t.Fatal("NewRouter returned a nil handler")
	}

	// Sanity-check the route tree actually assembled by hitting the
	// unauthenticated liveness probe.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestNewRouter_BootsWithoutSetupService confirms the other branch — no
// wizard wired (SetupService nil) — also builds cleanly, so the gate
// skip path stays exercised.
func TestNewRouter_BootsWithoutSetupService(t *testing.T) {
	cfg := RouterConfig{DevAuthMode: "header"}

	var handler http.Handler
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("NewRouter panicked without SetupService: %v", rec)
			}
		}()
		handler = NewRouter(cfg)
	}()

	if handler == nil {
		t.Fatal("NewRouter returned a nil handler")
	}
}

// authedGet builds a GET request authenticated via the DEV_AUTH_MODE=header
// bypass. The X-Dev-Auth value is "<sub>,<org_id>,<role>" — the middleware
// trusts it directly (no JWT) when the router is built with
// DevAuthMode:"header", so a router test can exercise the RBAC wiring of any
// role without minting tokens. SetupService is left nil in these tests, so
// SetupGate is skipped and operational routes stay reachable.
func authedGet(target, role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("X-Dev-Auth", "router-sub,11111111-1111-1111-1111-111111111111,"+role)
	return req
}

// TestNewRouter_CapabilitiesRoute_MountsAuthOnly proves MountCapabilitiesRoutes
// is wired when the vault is configured AND that it is auth-only (NOT
// admin-gated): a field_worker — the lowest role — can read it. This is the
// contract the web/Flutter UIs depend on to gate AI/email affordances per role.
func TestNewRouter_CapabilitiesRoute_MountsAuthOnly(t *testing.T) {
	handler := NewRouter(RouterConfig{
		DevAuthMode:         "header",
		IntegrationsService: &mockIntegrationsService{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedGet("/api/v1/capabilities", "field_worker"))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/capabilities as field_worker = %d, want %d (body=%s)",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestNewRouter_IntegrationsRoute_AdminOK proves MountIntegrationRoutes is wired
// when the vault is configured and that an admin clears the RequireMinRole(admin)
// gate.
func TestNewRouter_IntegrationsRoute_AdminOK(t *testing.T) {
	handler := NewRouter(RouterConfig{
		DevAuthMode:         "header",
		IntegrationsService: &mockIntegrationsService{listResult: nil},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedGet("/api/v1/integrations", "admin"))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/integrations as admin = %d, want %d (body=%s)",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestNewRouter_IntegrationsRoute_NonAdminForbidden proves the vault surface's
// RequireMinRole(admin) gate is actually wired: a superintendent — one rung
// below admin — is 403'd before the handler runs. Pairs with the admin-OK case
// to prove the gate discriminates rather than allow-all / deny-all.
func TestNewRouter_IntegrationsRoute_NonAdminForbidden(t *testing.T) {
	handler := NewRouter(RouterConfig{
		DevAuthMode:         "header",
		IntegrationsService: &mockIntegrationsService{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedGet("/api/v1/integrations", "superintendent"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /api/v1/integrations as superintendent = %d, want %d (body=%s)",
			rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// TestNewRouter_VaultRoutesSkippedWhenNil proves the conditional mount: with
// IntegrationsService nil (vault disabled — no VAULT_MASTER_KEY) neither the
// admin vault surface nor /capabilities mounts, so both 404. This is the path
// that lets the frontend fall back to assume-on when the vault is unconfigured.
func TestNewRouter_VaultRoutesSkippedWhenNil(t *testing.T) {
	handler := NewRouter(RouterConfig{DevAuthMode: "header"})

	for _, target := range []string{"/api/v1/capabilities", "/api/v1/integrations"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, authedGet(target, "owner"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s with vault disabled = %d, want %d (body=%s)",
				target, rec.Code, http.StatusNotFound, rec.Body.String())
		}
	}
}

// realAuth builds a real RS256 issuer/verifier pair (one keypair) and returns
// the verifier plus a mint(role) that issues a PRODUCTION-SHAPE token: empty
// plan_tier (what internal/service/auth.go actually mints), the given role,
// iss/aud "buildos". Used to exercise the REAL JWT path — not the
// DEV_AUTH_MODE=header bypass, which defaults plan_tier to "enterprise" and so
// masks the (removed) pro gate.
func realAuth(t *testing.T) (*auth.Verifier, func(role string) string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	iss, err := auth.NewTokenIssuer(key, "kid-test", "buildos", "buildos")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	ver, err := auth.NewVerifier(&key.PublicKey, "buildos", "buildos")
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	mint := func(role string) string {
		tok, _, mErr := iss.Mint("real-sub", "11111111-1111-1111-1111-111111111111", role, "")
		if mErr != nil {
			t.Fatalf("Mint: %v", mErr)
		}
		return tok
	}
	return ver, mint
}

// TestNewRouter_AgentsSurface_RealTokenNotPlanWalled is the ESC-002 regression
// guard. Production tokens are minted with an EMPTY plan_tier; the removed
// RequirePlanTier(pro) gate ranked an empty tier as "free" and returned 402
// UPGRADE_REQUIRED for the entire /api/v1/agents/* surface — so no real caller
// could reach it. The other router tests use the DEV_AUTH_MODE=header bypass
// (defaults plan_tier "enterprise"), which MASKED the wall. This mints a REAL
// plan_tier="" token, runs it through real JWT verification, and proves the
// agents surface is reachable (NOT 402). Fails if the pro gate is re-added.
func TestNewRouter_AgentsSurface_RealTokenNotPlanWalled(t *testing.T) {
	ver, mint := realAuth(t)
	// AgentsService non-nil so /api/v1/agents/daily-briefing mounts; SetupService
	// nil so SetupGate is skipped. daily-briefing is any-authenticated post-ESC-002.
	handler := NewRouter(RouterConfig{Verifier: ver, AgentsService: &mockAgentsService{}})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/daily-briefing", nil)
	req.Header.Set("Authorization", "Bearer "+mint("field_worker"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusPaymentRequired {
		t.Fatalf("agents surface 402-walled a real plan_tier=\"\" token — ESC-002 regressed (body=%s)",
			rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/agents/daily-briefing with a real token = %d, want %d (body=%s)",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestNewRouter_ChatRoute_RoleGateKeptNoPlanWall pins that the chat route's gate
// change is exactly right: ESC-002 dropped the pro plan-tier gate (a real
// plan_tier="" superintendent token must NOT be 402-walled) while the
// RequireMinRole(superintendent) gate is RETAINED (a field_worker must still be
// 403'd — not 402, not 200). Guards against accidentally dropping the role gate
// alongside the pro gate.
func TestNewRouter_ChatRoute_RoleGateKeptNoPlanWall(t *testing.T) {
	ver, mint := realAuth(t)
	handler := NewRouter(RouterConfig{
		Verifier:  ver,
		Assistant: &mockAssistantConverser{result: agentic.ChatResult{Reply: "ok"}},
	})

	post := func(role string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/chat",
			strings.NewReader(`{"message":"hi"}`))
		req.Header.Set("Authorization", "Bearer "+mint(role))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// superintendent: pro gate gone → reaches the handler (mock → 200), NOT 402.
	if code := post("superintendent"); code != http.StatusOK {
		t.Fatalf("chat as superintendent (real plan_tier=\"\" token) = %d, want 200 — the pro gate must be gone", code)
	}
	// field_worker: the RequireMinRole(superintendent) gate is RETAINED → 403,
	// NOT 402 (no plan wall) and NOT 200 (role floor holds).
	if code := post("field_worker"); code != http.StatusForbidden {
		t.Fatalf("chat as field_worker = %d, want 403 — the role gate must survive the pro-gate removal", code)
	}
}
