package middleware

import "net/http"

// SecurityHeaders sets baseline response security headers appropriate for a
// JSON API + same-origin SPA:
//
//   - X-Content-Type-Options: nosniff   — stop MIME-sniffing of responses.
//   - X-Frame-Options: DENY             — clickjacking defense (the API and the
//     console are never meant to be framed).
//   - Referrer-Policy: no-referrer      — URLs embed org/project ids; don't leak
//     them in the Referer to any cross-origin resource.
//
// HSTS is intentionally NOT set here: the Go server listens on plain HTTP behind
// a TLS-terminating ingress, which is the correct place to emit
// Strict-Transport-Security (see docs/fork-onboarding.md). Setting it on a
// plain-HTTP response is a no-op at best and wrong if HTTP is ever served.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
