package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// nextRecorder is a terminal handler that records whether it ran and writes 200.
type nextRecorder struct{ called bool }

func (n *nextRecorder) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	n.called = true
	w.WriteHeader(http.StatusOK)
}

// reqWithRole builds a request whose context carries claims with the given role.
// An empty role yields a request with NO claims (unauthenticated).
func reqWithRole(method, role string) *http.Request {
	r := httptest.NewRequest(method, "/x", nil)
	if role != "" {
		r = r.WithContext(ContextWithClaims(r.Context(), Claims{Sub: "u1", OrgID: "o1", Role: role}))
	}
	return r
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name     string
		allowed  []string
		role     string // "" => no claims
		wantCode int
		wantNext bool
	}{
		{"allowed role passes", []string{RoleOwner, RoleAdmin}, RoleAdmin, http.StatusOK, true},
		{"disallowed role 403", []string{RoleOwner}, RoleFieldWorker, http.StatusForbidden, false},
		{"no claims 401", []string{RoleOwner}, "", http.StatusUnauthorized, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &nextRecorder{}
			h := RequireRole(tt.allowed...)(next)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, reqWithRole(http.MethodPost, tt.role))
			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if next.called != tt.wantNext {
				t.Errorf("next.called = %v, want %v", next.called, tt.wantNext)
			}
		})
	}
}

func TestRequireMinRole(t *testing.T) {
	tests := []struct {
		name     string
		minRole  string
		role     string // "" => no claims
		wantCode int
		wantNext bool
	}{
		{"exact level passes", RoleAdmin, RoleAdmin, http.StatusOK, true},
		{"higher level passes", RoleSuperintendent, RoleOwner, http.StatusOK, true},
		{"lower level 403", RoleAdmin, RoleSuperintendent, http.StatusForbidden, false},
		{"no claims 401", RoleAdmin, "", http.StatusUnauthorized, false},
		{"unknown user role 403", RoleFieldWorker, "intern", http.StatusForbidden, false},
		{"unknown min role blocks owner", "superuser", RoleOwner, http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &nextRecorder{}
			h := RequireMinRole(tt.minRole)(next)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, reqWithRole(http.MethodPost, tt.role))
			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if next.called != tt.wantNext {
				t.Errorf("next.called = %v, want %v", next.called, tt.wantNext)
			}
		})
	}
}

func TestRequireWriteRole(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		role     string // "" => no claims
		wantCode int
		wantNext bool
	}{
		{"GET passes without claims", http.MethodGet, "", http.StatusOK, true},
		{"HEAD passes without claims", http.MethodHead, "", http.StatusOK, true},
		{"OPTIONS passes without claims", http.MethodOptions, "", http.StatusOK, true},
		{"POST owner passes", http.MethodPost, RoleOwner, http.StatusOK, true},
		{"PUT admin passes", http.MethodPut, RoleAdmin, http.StatusOK, true},
		{"POST field_worker 403", http.MethodPost, RoleFieldWorker, http.StatusForbidden, false},
		{"DELETE superintendent 403", http.MethodDelete, RoleSuperintendent, http.StatusForbidden, false},
		{"POST no claims 401", http.MethodPost, "", http.StatusUnauthorized, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &nextRecorder{}
			h := RequireWriteRole(next)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, reqWithRole(tt.method, tt.role))
			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if next.called != tt.wantNext {
				t.Errorf("next.called = %v, want %v", next.called, tt.wantNext)
			}
		})
	}
}
