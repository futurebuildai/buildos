//go:build !prod

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// In header mode the Auth middleware trusts X-Dev-Auth instead of a JWT. This
// path is compiled out of prod builds (auth_prod.go), so the test is !prod.
func TestAuth_HeaderMode(t *testing.T) {
	t.Run("valid header passes claims", func(t *testing.T) {
		cap := &claimsCapture{}
		h := Auth(nil, "header", quietLogger())(cap)
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-Dev-Auth", "alice@buildos.dev,demo-org,owner")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", rec.Code)
		}
		if !cap.called {
			t.Fatal("next handler not called")
		}
		if cap.claims.Sub != "alice@buildos.dev" || cap.claims.OrgID != "demo-org" ||
			cap.claims.Role != "owner" || cap.claims.PlanTier != "enterprise" {
			t.Errorf("claims = %+v, want alice/demo-org/owner/enterprise", cap.claims)
		}
	})

	t.Run("malformed header 401", func(t *testing.T) {
		cap := &claimsCapture{}
		h := Auth(nil, "header", quietLogger())(cap)
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-Dev-Auth", "too,few") // only two fields
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("code = %d, want 401", rec.Code)
		}
		if cap.called {
			t.Error("next handler called on malformed header")
		}
		if !strings.Contains(rec.Body.String(), "X-Dev-Auth invalid") {
			t.Errorf("body = %q, want to contain %q", rec.Body.String(), "X-Dev-Auth invalid")
		}
	})
}
