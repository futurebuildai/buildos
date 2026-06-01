package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
)

// fakeChecker is the OnboardingChecker test double. Returns the
// preconfigured (complete, err) regardless of input.
type fakeChecker struct {
	complete bool
	err      error
	calls    int
}

func (f *fakeChecker) IsOnboardingComplete(_ context.Context, _ uuid.UUID) (bool, error) {
	f.calls++
	return f.complete, f.err
}

// nextHandler stamps a 200 with a sentinel body so tests can assert
// whether the request passed through the gate.
func nextHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

func TestSetupGate_ExemptPaths_BypassGate(t *testing.T) {
	chk := &fakeChecker{complete: false}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	for _, p := range []string{
		"/health",
		"/ready",
		"/metrics",
		"/api/v1/setup",
		"/api/v1/setup/state",
		"/api/v1/setup/company-info",
	} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		gate(nextHandler()).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("path %q: status = %d, want 200 (exempt)", p, rr.Code)
		}
	}
	if chk.calls != 0 {
		t.Errorf("checker calls = %d, want 0 (exempt paths never hit DB)", chk.calls)
	}
}

func TestSetupGate_OnboardingComplete_PassesThrough(t *testing.T) {
	chk := &fakeChecker{complete: true}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: uuid.NewString(), Role: RoleOwner})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	gate(nextHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestSetupGate_OnboardingIncomplete_403(t *testing.T) {
	chk := &fakeChecker{complete: false}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: uuid.NewString(), Role: RoleOwner})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	gate(nextHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
	if !contains(rr.Body.String(), "SETUP_INCOMPLETE") {
		t.Errorf("body = %q, want SETUP_INCOMPLETE", rr.Body.String())
	}
}

func TestSetupGate_OrgNotFound_403(t *testing.T) {
	chk := &fakeChecker{err: store.ErrNotFound}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: uuid.NewString(), Role: RoleOwner})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	gate(nextHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
	if !contains(rr.Body.String(), "SETUP_INCOMPLETE") {
		t.Errorf("body = %q, want SETUP_INCOMPLETE", rr.Body.String())
	}
}

func TestSetupGate_DBError_503(t *testing.T) {
	chk := &fakeChecker{err: errors.New("connection refused")}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: uuid.NewString(), Role: RoleOwner})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	gate(nextHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestSetupGate_NoClaims_401(t *testing.T) {
	// Defense-in-depth: if Auth middleware is misconfigured, the
	// gate still refuses to grant access.
	chk := &fakeChecker{complete: true}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rr := httptest.NewRecorder()
	gate(nextHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if chk.calls != 0 {
		t.Errorf("checker calls = %d, want 0 (no DB hit without claims)", chk.calls)
	}
}

func TestSetupGate_BadOrgIDClaim_401(t *testing.T) {
	chk := &fakeChecker{complete: true}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: "not-a-uuid", Role: RoleOwner})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	gate(nextHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestSetupGate_ImpostorPath_NotExempt(t *testing.T) {
	// "/api/v1/setup-impostor" must NOT be exempted by the
	// "/api/v1/setup" prefix entry.
	chk := &fakeChecker{complete: false}
	gate := SetupGate(SetupGateConfig{Checker: chk})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup-impostor", nil)
	ctx := ContextWithClaims(req.Context(), Claims{Sub: "u", OrgID: uuid.NewString(), Role: RoleOwner})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	gate(nextHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("impostor path got status %d, want 403 (not exempt)", rr.Code)
	}
}

// contains is a tiny substring helper so the test file doesn't need
// strings imported just for this.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
