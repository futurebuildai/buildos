package middleware

import (
	"net/http"
)

// Role constants matching The Brain OIDC role claim values.
const (
	RoleOwner          = "owner"
	RoleAdmin          = "admin"
	RoleSuperintendent = "superintendent"
	RoleFieldWorker    = "field_worker"
)

// roleHierarchy defines the privilege level for each role.
// Higher number = more privileges.
var roleHierarchy = map[string]int{
	RoleFieldWorker:    1,
	RoleSuperintendent: 2,
	RoleAdmin:          3,
	RoleOwner:          4,
}

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
	minLevel, ok := roleHierarchy[minRole]
	if !ok {
		minLevel = 99 // Unknown role blocks everyone
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			userLevel, exists := roleHierarchy[claims.Role]
			if !exists || userLevel < minLevel {
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
