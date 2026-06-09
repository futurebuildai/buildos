package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futurebuildai/buildos/internal/authz"
)

// TestRoleConstantsAliasAuthz asserts the middleware role constants are exactly
// the canonical authz vocabulary — they must not drift apart.
func TestRoleConstantsAliasAuthz(t *testing.T) {
	cases := []struct {
		mw, canon string
		name      string
	}{
		{RoleOwner, authz.RoleOwner, "owner"},
		{RoleAdmin, authz.RoleAdmin, "admin"},
		{RoleSuperintendent, authz.RoleSuperintendent, "superintendent"},
		{RoleFieldWorker, authz.RoleFieldWorker, "field_worker"},
	}
	for _, c := range cases {
		if c.mw != c.canon {
			t.Errorf("%s: middleware constant %q != authz constant %q", c.name, c.mw, c.canon)
		}
	}
}

// TestRequireMinRoleMatchesAuthzLadder asserts that RequireMinRole's gate
// decision (whether next is invoked) agrees with authz.RoleAtLeast for every
// role × min pair, including unknowns. This is the load-bearing guarantee that
// the HTTP gate and the service-layer in-executor re-checks share one ladder.
func TestRequireMinRoleMatchesAuthzLadder(t *testing.T) {
	roles := []string{
		RoleFieldWorker, RoleSuperintendent, RoleAdmin, RoleOwner,
		"intern", "", // unknown / empty user role
	}
	mins := []string{
		RoleFieldWorker, RoleSuperintendent, RoleAdmin, RoleOwner,
		"superuser", "", // unknown / empty min role
	}

	for _, role := range roles {
		for _, min := range mins {
			next := &nextRecorder{}
			h := RequireMinRole(min)(next)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, reqWithRole(http.MethodPost, role))

			gotPass := next.called
			// An empty role yields no claims (401, never passes); authz also
			// fails closed on it. For all non-empty roles the gate decision must
			// match the shared ladder exactly.
			wantPass := role != "" && authz.RoleAtLeast(role, min)

			if gotPass != wantPass {
				t.Errorf("RequireMinRole(min=%q) role=%q: next.called=%v (code %d), want pass=%v",
					min, role, gotPass, rec.Code, wantPass)
			}
		}
	}
}
