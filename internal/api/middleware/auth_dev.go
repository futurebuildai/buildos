//go:build !prod

package middleware

import (
	"errors"
	"strings"
)

// claimsFromDevHeader parses an X-Dev-Auth header value of the form
// "<sub>,<org_id>,<role>[,<plan_tier>]" into Claims. Whitespace around
// each field is trimmed. plan_tier defaults to "enterprise" if omitted.
//
// This file compiles ONLY in non-prod builds. The matching auth_prod.go
// stub always errors so a binary built with `-tags=prod` cannot honor
// DEV_AUTH_MODE=header even if the env var is set. Bypass is impossible
// at runtime; the only way to re-enable is to rebuild without the
// prod tag.
func claimsFromDevHeader(h string) (Claims, error) {
	if h == "" {
		return Claims{}, errors.New("missing X-Dev-Auth header")
	}
	parts := strings.Split(h, ",")
	if len(parts) < 3 || len(parts) > 4 {
		return Claims{}, errors.New("expected sub,org_id,role[,plan_tier]")
	}
	sub := strings.TrimSpace(parts[0])
	orgID := strings.TrimSpace(parts[1])
	role := strings.TrimSpace(parts[2])
	planTier := "enterprise"
	if len(parts) == 4 {
		if t := strings.TrimSpace(parts[3]); t != "" {
			planTier = t
		}
	}
	if sub == "" || orgID == "" || role == "" {
		return Claims{}, errors.New("sub, org_id, and role are required")
	}
	return Claims{Sub: sub, OrgID: orgID, Role: role, PlanTier: planTier}, nil
}
