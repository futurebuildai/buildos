// Package authz is the single source of truth for the BuildOS RBAC role ladder.
//
// It is a dependency-free leaf (stdlib only) imported by both
// internal/api/middleware (route gates) and internal/service (in-executor role
// re-checks for the conversational assistant tools). Keeping the ladder here
// eliminates the privilege-ladder duplication/drift risk that would otherwise
// exist between the HTTP middleware and the service-layer tool executors.
//
// It is deliberately NOT internal/agentic: the role ladder is a domain authz
// fact consumed by service, not by the harness leaf, and internal/agentic must
// import no internal/* package. authz is invisible to the isolation gate.
package authz

// Role constants — the canonical RBAC vocabulary. middleware aliases these.
const (
	RoleFieldWorker    = "field_worker"
	RoleSuperintendent = "superintendent"
	RoleAdmin          = "admin"
	RoleOwner          = "owner"
)

// rank is the privilege ladder. Higher = more privileged. Single source of truth.
var rank = map[string]int{
	RoleFieldWorker:    1,
	RoleSuperintendent: 2,
	RoleAdmin:          3,
	RoleOwner:          4,
}

// RoleRank returns the privilege rank for a role, or 0 if unknown.
func RoleRank(role string) int { return rank[role] }

// RoleAtLeast reports whether `role` meets or exceeds `min`. An unknown role or
// an unknown min returns false (fail closed).
func RoleAtLeast(role, min string) bool {
	r, ok1 := rank[role]
	m, ok2 := rank[min]
	if !ok1 || !ok2 {
		return false
	}
	return r >= m
}
