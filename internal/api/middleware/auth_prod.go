//go:build prod

package middleware

import "errors"

// claimsFromDevHeader is the prod-build stub for the dev-auth bypass.
// Always returns an error so DEV_AUTH_MODE=header is a no-op in
// production binaries — the only way to re-enable is to rebuild
// without `-tags=prod`. See ADR-002 D8.
//
// The error message is intentionally explicit so a misconfigured
// staging that accidentally inherits a prod build sees a clear log
// line in middleware/auth.go's writeError 401 response, not silent
// authentication failures that look like JWT rejections.
func claimsFromDevHeader(_ string) (Claims, error) {
	return Claims{}, errors.New("DEV_AUTH_MODE=header is disabled in prod builds; rebuild without -tags=prod to re-enable")
}
