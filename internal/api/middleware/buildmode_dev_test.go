//go:build !prod

package middleware

import "testing"

// In a non-prod build IsProdBuild must report false — the dev-auth
// header bypass is compiled in. The prod counterpart is asserted by the
// //go:build prod test alongside the auth_prod stub.
func TestIsProdBuild_DevReportsFalse(t *testing.T) {
	if IsProdBuild() {
		t.Error("IsProdBuild() = true in a non-prod build, want false")
	}
}
