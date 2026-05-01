package middleware

import (
	"net/http"
)

// Plan tier constants matching The Brain billing claim values.
//
// The hierarchy mirrors Brain's billing tiers — higher number =
// more entitlements. Free is the implicit default for any token
// missing a plan_tier claim, including legacy tokens issued before
// Brain started populating this claim.
const (
	PlanTierFree       = "free"
	PlanTierStarter    = "starter"
	PlanTierPro        = "pro"
	PlanTierEnterprise = "enterprise"
)

// planTierLevel ranks tiers for hierarchical "minimum tier" checks.
// Unknown values rank below "free" (zero) so an unrecognized value
// is treated as the most restrictive option — fail closed.
var planTierLevel = map[string]int{
	PlanTierFree:       1,
	PlanTierStarter:    2,
	PlanTierPro:        3,
	PlanTierEnterprise: 4,
}

// RequirePlanTier creates middleware that gates a route on a minimum
// plan tier. Lower-tier callers receive 402 Payment Required — that
// HTTP status is the conventional signal for "this works but your
// plan doesn't include it" and is what most billing-aware frontends
// look for to drive upgrade prompts.
//
// The plan tier is read from the JWT claim (`plan_tier`) — populated
// by Brain at token-issue time. There is intentionally NO Brain
// round-trip per request; that would put a billing system on the hot
// path of every authenticated call. Tier changes propagate when the
// user's token is refreshed.
//
// Auth middleware MUST run before this — RequirePlanTier requires a
// Claims object in context.
func RequirePlanTier(minTier string) func(http.Handler) http.Handler {
	minLevel, known := planTierLevel[minTier]
	if !known {
		// A nonsense gate would silently allow everyone; better to fail
		// closed at config time so the operator notices.
		minLevel = 99
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			callerLevel, present := planTierLevel[claims.PlanTier]
			if !present {
				// Token has no recognizable plan_tier — treat as the
				// lowest tier (free) by default. This keeps things safe
				// during the rollout window where some tokens may
				// predate the claim.
				callerLevel = planTierLevel[PlanTierFree]
			}
			if callerLevel < minLevel {
				writeError(w, http.StatusPaymentRequired, "UPGRADE_REQUIRED",
					"this feature requires the "+minTier+" plan or higher")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
