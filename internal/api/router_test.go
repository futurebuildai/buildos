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
