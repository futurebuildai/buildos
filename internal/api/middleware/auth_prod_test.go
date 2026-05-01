//go:build prod

package middleware

import (
	"strings"
	"testing"
)

// In prod builds claimsFromDevHeader must always reject — D8 hardening
// per ADR-002. Even a perfectly-formatted X-Dev-Auth header that the
// non-prod build would accept gets a clear error message that names
// the build-tag posture so a misconfigured deploy is loud, not silent.
func TestClaimsFromDevHeader_ProdAlwaysRejects(t *testing.T) {
	cases := []string{
		"",                                       // empty
		"alice@buildos.dev,demo-org,owner",       // would be valid in dev
		"bob@buildos.dev,demo-org,admin,starter", // would be valid in dev
	}
	for _, h := range cases {
		_, err := claimsFromDevHeader(h)
		if err == nil {
			t.Errorf("prod build accepted header %q; expected unconditional reject", h)
		}
		if !strings.Contains(err.Error(), "prod") {
			t.Errorf("prod-build error should name the build tag: %v", err)
		}
	}
}
