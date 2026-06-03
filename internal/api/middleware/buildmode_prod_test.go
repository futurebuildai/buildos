//go:build prod

package middleware

import "testing"

// In a prod build IsProdBuild must report true — the dev-auth header
// bypass is stubbed out (D8 hardening). cmd/server/cmd/worker rely on
// this to refuse to start when a prod binary sees DEV_AUTH_MODE set.
func TestIsProdBuild_ProdReportsTrue(t *testing.T) {
	if !IsProdBuild() {
		t.Error("IsProdBuild() = false in a prod build, want true")
	}
}
