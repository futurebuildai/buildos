//go:build prod

package middleware

// IsProdBuild reports whether the binary was compiled with -tags=prod.
// In prod builds it returns true; the dev-auth header bypass is
// stubbed out and DEV_AUTH_MODE=header has no effect.
//
// Used by cmd/server / cmd/worker to fail-fast at startup if a prod
// build sees DEV_AUTH_MODE set — that's an operator mistake worth
// surfacing as a refusal-to-start rather than a flood of 401s.
func IsProdBuild() bool { return true }
