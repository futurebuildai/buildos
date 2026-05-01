//go:build !prod

package middleware

// IsProdBuild reports whether the binary was compiled with -tags=prod.
// In non-prod builds it returns false; the dev-auth header bypass is
// available and DEV_AUTH_MODE=header is honored.
//
// Used by cmd/server / cmd/worker to fail-fast at startup if a prod
// build sees DEV_AUTH_MODE set — that's an operator mistake worth
// surfacing as a refusal-to-start rather than a flood of 401s.
func IsProdBuild() bool { return false }
