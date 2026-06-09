package middleware

import (
	"net/http"

	"github.com/futurebuildai/buildos/internal/authz"
)

// Role constants matching the RBAC role claim values. These alias the canonical
// ladder in internal/authz so the middleware and the service-layer tool
// executors share a single source of truth (no privilege-ladder drift).
const (
	RoleOwner          = authz.RoleOwner
	RoleAdmin          = authz.RoleAdmin
	RoleSuperintendent = authz.RoleSuperintendent
	RoleFieldWorker    = authz.RoleFieldWorker
)

// RequireRole creates middleware that restricts access to the specified roles.
// The request must have been authenticated via Auth middleware first.
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			if !allowed[claims.Role] {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "insufficient role for this operation")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireMinRole creates middleware that requires at least the specified role level.
// Role hierarchy: field_worker < superintendent < admin < owner.
func RequireMinRole(minRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			// authz.RoleAtLeast fails closed on an unknown user role AND on an
			// unknown minRole, preserving the prior "unknown blocks everyone"
			// semantics via the single shared ladder.
			if !authz.RoleAtLeast(claims.Role, minRole) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "insufficient role for this operation")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireWriteRole restricts mutating operations to owner and admin roles.
// Read operations pass through for all authenticated users.
func RequireWriteRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
			return
		}

		if claims.Role != RoleOwner && claims.Role != RoleAdmin {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "write operations require owner or admin role")
			return
		}

		next.ServeHTTP(w, r)
	})
}
