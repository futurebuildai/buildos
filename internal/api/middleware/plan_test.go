package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func reqWithClaims(c Claims) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	return req.WithContext(ContextWithClaims(context.Background(), c))
}

func TestRequirePlanTier_AllowsExactTier(t *testing.T) {
	mw := RequirePlanTier(PlanTierPro)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, reqWithClaims(Claims{PlanTier: PlanTierPro}))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRequirePlanTier_AllowsHigherTier(t *testing.T) {
	mw := RequirePlanTier(PlanTierPro)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, reqWithClaims(Claims{PlanTier: PlanTierEnterprise}))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRequirePlanTier_BlocksLowerTier(t *testing.T) {
	mw := RequirePlanTier(PlanTierPro)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, reqWithClaims(Claims{PlanTier: PlanTierStarter}))

	if rec.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want 402", rec.Code)
	}
}

func TestRequirePlanTier_TreatsMissingTierAsFree(t *testing.T) {
	mw := RequirePlanTier(PlanTierPro)
	rec := httptest.NewRecorder()
	// Empty PlanTier — auth middleware would have left it blank if the
	// JWT didn't include a claim.
	mw(okHandler()).ServeHTTP(rec, reqWithClaims(Claims{Role: "owner"}))

	if rec.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want 402 (missing plan_tier should fail closed)", rec.Code)
	}
}

func TestRequirePlanTier_TreatsUnknownTierAsFree(t *testing.T) {
	mw := RequirePlanTier(PlanTierPro)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, reqWithClaims(Claims{PlanTier: "ultra-mega"}))

	if rec.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want 402 (unknown plan_tier should fail closed)", rec.Code)
	}
}

func TestRequirePlanTier_NoClaimsReturns401(t *testing.T) {
	mw := RequirePlanTier(PlanTierPro)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequirePlanTier_UnknownGateBlocksAll(t *testing.T) {
	// A typo in the gate (e.g. "platinum") should fail closed so the
	// misconfiguration is visible immediately.
	mw := RequirePlanTier("platinum")
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, reqWithClaims(Claims{PlanTier: PlanTierEnterprise}))

	if rec.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want 402 (unknown gate should block everyone)", rec.Code)
	}
}

func TestRequirePlanTier_FreeAllowsFreeRoute(t *testing.T) {
	mw := RequirePlanTier(PlanTierFree)
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, reqWithClaims(Claims{PlanTier: PlanTierFree}))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
