package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
