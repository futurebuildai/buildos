package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
)

// OnboardingChecker is the consumer-side surface SetupGate needs.
// Defined here so the middleware stays free of the service-layer
// import; cmd/server wires the concrete *service.SetupService.
type OnboardingChecker interface {
	IsOnboardingComplete(ctx context.Context, orgID uuid.UUID) (bool, error)
}

// SetupGateConfig is the SetupGate middleware configuration. Paths
// listed in ExemptPrefixes bypass the gate even when onboarding is
// incomplete — these are the wizard routes themselves plus probes
// and the A2A webhook (signed; doesn't carry a JWT claims context).
type SetupGateConfig struct {
	Checker        OnboardingChecker
	ExemptPrefixes []string
}

// DefaultSetupGateExemptPrefixes is the canonical exempt list. Mounted
// into SetupGateConfig by NewSetupGate when ExemptPrefixes is empty.
//
//   - /api/v1/setup       wizard routes (the whole point of the gate)
//   - /health, /ready     liveness + readiness probes
//   - /metrics            Prometheus scrape (Prometheus convention)
//   - /api/v1/a2a/webhook JWS-authenticated, no JWT claims, sender is Brain
var DefaultSetupGateExemptPrefixes = []string{
	"/api/v1/setup",
	"/health",
	"/ready",
	"/metrics",
	"/api/v1/a2a/webhook",
}

// SetupGate returns middleware that 403s any request whose
// authenticated org_id has not yet flipped onboarding_complete=true.
// Requests whose path matches one of the exempt prefixes are passed
// through regardless. Auth middleware MUST run BEFORE SetupGate so
// the JWT claims are available; unauthenticated requests already
// hit 401 in Auth and never reach the gate.
//
// Failure modes:
//   - missing Claims (auth bypassed) → 401 UNAUTHORIZED (defense in depth)
//   - malformed org_id claim         → 401 UNAUTHORIZED
//   - org row not found              → 403 SETUP_INCOMPLETE (a fresh fork
//     before migration 010 seed is treated
//     identically to "not yet onboarded")
//   - DB transient error             → 503 SERVICE_UNAVAILABLE (don't 5xx
//     a setup-state check; the request
//     is unrelated to DB writes)
//   - onboarding_complete=false      → 403 SETUP_INCOMPLETE
//   - onboarding_complete=true       → next.ServeHTTP
func SetupGate(cfg SetupGateConfig) func(http.Handler) http.Handler {
	prefixes := cfg.ExemptPrefixes
	if len(prefixes) == 0 {
		prefixes = DefaultSetupGateExemptPrefixes
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isExempt(r.URL.Path, prefixes) {
				next.ServeHTTP(w, r)
				return
			}

			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}
			orgID, err := uuid.Parse(claims.OrgID)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid org_id claim")
				return
			}

			complete, err := cfg.Checker.IsOnboardingComplete(r.Context(), orgID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeError(w, http.StatusForbidden, "SETUP_INCOMPLETE", "tenant onboarding not yet complete")
					return
				}
				// Don't leak the DB error to the client; ops sees it in logs.
				writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "setup-state check failed")
				return
			}
			if !complete {
				writeError(w, http.StatusForbidden, "SETUP_INCOMPLETE", "tenant onboarding not yet complete")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isExempt returns true when path equals an exempt prefix exactly or
// is a sub-path of one (prefix followed by "/"). Strict match so
// "/api/v1/setup" exempts itself and "/api/v1/setup/state" but NOT a
// hypothetical "/api/v1/setup-impostor".
func isExempt(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}
