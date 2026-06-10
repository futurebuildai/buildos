package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Per-IP token-bucket rate limit — coarse protection against
// brute-force / runaway scripts.
//
// Defaults are intentionally permissive — a real human user with a
// busy frontend dashboard easily bursts 50 req/sec when refreshing
// multiple panes. The point is to stop a runaway script, not to
// throttle normal traffic.

// DefaultRateLimitRPS is the steady-state rate (requests per second)
// per remote IP. 50 rps × 60s = 3000 req/min, well above real
// dashboard load.
const DefaultRateLimitRPS = 50

// DefaultRateLimitBurst is the size of the token bucket — caps the
// instantaneous burst before the steady-state limit kicks in. 100
// gives users headroom for tab open / page refresh storms.
const DefaultRateLimitBurst = 100

// rateLimiterEvictAfter is how long a per-IP bucket lingers without
// activity before we GC it. Long enough that a returning user
// doesn't get a fresh empty bucket; short enough that bots cycling
// IPs don't bloat memory.
const rateLimiterEvictAfter = 10 * time.Minute

// IPRateLimiter is the per-IP bucket map. One instance per process —
// constructed in cmd/server, mounted as middleware on the global
// router stack.
type IPRateLimiter struct {
	rps   rate.Limit
	burst int

	mu      sync.Mutex
	buckets map[string]*ipBucket
}

type ipBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPRateLimiter constructs a limiter with the given steady-state
// rate (requests per second) and burst size. The eviction goroutine
// auto-starts; cancelling ctx stops it.
func NewIPRateLimiter(rps int, burst int) *IPRateLimiter {
	if rps <= 0 {
		rps = DefaultRateLimitRPS
	}
	if burst <= 0 {
		burst = DefaultRateLimitBurst
	}
	l := &IPRateLimiter{
		rps:     rate.Limit(rps),
		burst:   burst,
		buckets: make(map[string]*ipBucket),
	}
	go l.gcLoop()
	return l
}

// Middleware returns the chi-compatible HTTP middleware. Allowed
// requests pass through; rejected ones get 429 Too Many Requests with
// a Retry-After header (the SDK sets this from the bucket reservation
// — clients can use it to back off intelligently).
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !l.allow(ip) {
			// Retry-After "1": at the configured rps a single token refills in
			// well under a second, so the RFC 7231 integer-seconds floor is 1.
			w.Header().Set("Retry-After", "1")
			// Use the shared slim error writer (consistent envelope + the
			// {code, status} metrics observer) instead of a hand-rolled literal.
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *IPRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &ipBucket{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[ip] = b
	}
	b.lastSeen = time.Now()
	return b.limiter.Allow()
}

// gcLoop evicts buckets idle longer than rateLimiterEvictAfter. Runs
// every minute — bucket count is bounded by active IP count, which is
// small for a typical deployment but unbounded if abused, so the GC
// is non-optional.
func (l *IPRateLimiter) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.gcOnce()
	}
}

func (l *IPRateLimiter) gcOnce() {
	cutoff := time.Now().Add(-rateLimiterEvictAfter)
	l.mu.Lock()
	defer l.mu.Unlock()
	for ip, b := range l.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(l.buckets, ip)
		}
	}
}

// clientIP extracts the caller's IP from the request. chi.RealIP has
// already been mounted in the router stack, so r.RemoteAddr is the
// X-Forwarded-For-resolved value when behind a trusted proxy.
//
// SplitHostPort because RemoteAddr is "ip:port" form; on parse
// failure we fall back to the raw value rather than dropping
// requests on the floor.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	// IPv6 addresses can come in with surrounding brackets that
	// SplitHostPort already strips; defensively re-strip in case the
	// fallback path returned them.
	host = strings.Trim(host, "[]")
	return host
}
