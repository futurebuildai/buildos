package middleware

import (
	"net"
	"net/http"
	"strings"
)

// RealIP sets r.RemoteAddr to the real client IP, honoring forwarding headers
// (X-Forwarded-For / X-Real-IP) ONLY when the immediate TCP peer is within
// trustedProxies. It replaces chi's RealIP, which trusts those headers
// UNCONDITIONALLY — and a per-IP rate limiter keyed on a header any client can
// forge is no limiter at all (an attacker rotates the header into a fresh
// bucket on every request).
//
// When trustedProxies is empty — the default, i.e. a fork exposed directly or
// one whose ingress is not in the allowlist — forwarding headers are IGNORED
// and the real TCP peer is used. Fail-safe: a spoofed header can never move the
// caller into a different rate-limit bucket. When the peer IS a trusted proxy,
// the forwarding chain is walked right-to-left and the first non-trusted entry
// (the real client across a multi-hop trusted chain) is used.
//
// Set the allowlist via TRUSTED_PROXY_CIDRS (e.g. the LB/ingress subnet) only
// when an upstream you control terminates the connection and appends an
// accurate X-Forwarded-For.
func RealIP(trustedProxies []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip := clientIPFromHeaders(r, trustedProxies); ip != "" {
				r.RemoteAddr = ip // bare IP; clientIP() tolerates the missing port
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIPFromHeaders returns the trusted client IP from the forwarding headers,
// or "" to signal "leave r.RemoteAddr as the real TCP peer".
func clientIPFromHeaders(r *http.Request, trusted []*net.IPNet) string {
	peerHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peerHost = r.RemoteAddr
	}
	peer := net.ParseIP(strings.Trim(peerHost, "[]"))
	// Unparsable peer, or a peer that is NOT a trusted proxy → ignore headers.
	if peer == nil || !ipInAny(peer, trusted) {
		return ""
	}
	// Trusted peer: walk X-Forwarded-For right-to-left, take the first entry
	// that isn't itself a trusted proxy (the real client).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			cand := net.ParseIP(strings.TrimSpace(parts[i]))
			if cand != nil && !ipInAny(cand, trusted) {
				return cand.String()
			}
		}
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		if net.ParseIP(xr) != nil {
			return xr
		}
	}
	return ""
}

func ipInAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}
