package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func okHandler200() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func TestIPRateLimiter_AllowsBurst(t *testing.T) {
	// 100 burst, 1 rps steady — fire 100 immediate requests, all
	// should pass; the 101st gets rejected.
	rl := NewIPRateLimiter(1, 100)
	mw := rl.Middleware(okHandler200())

	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("burst request %d: status = %d, want 200", i+1, rec.Code)
		}
	}

	// 101st should hit 429.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("post-burst status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 429")
	}
}

func TestIPRateLimiter_PerIPIsolation(t *testing.T) {
	// 5 burst — exhaust IP A's bucket, IP B should still pass.
	rl := NewIPRateLimiter(1, 5)
	mw := rl.Middleware(okHandler200())

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("IP A request %d: status = %d", i+1, rec.Code)
		}
	}

	// IP B should still have a fresh bucket.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("IP B status = %d, want 200 (separate bucket)", rec.Code)
	}
}

func TestIPRateLimiter_DefaultsApplied(t *testing.T) {
	rl := NewIPRateLimiter(0, 0)
	if rl.rps != DefaultRateLimitRPS {
		t.Errorf("rps = %v, want %d (default)", rl.rps, DefaultRateLimitRPS)
	}
	if rl.burst != DefaultRateLimitBurst {
		t.Errorf("burst = %d, want %d (default)", rl.burst, DefaultRateLimitBurst)
	}
}

func TestIPRateLimiter_GCEvictsIdleBuckets(t *testing.T) {
	rl := NewIPRateLimiter(1, 1)
	// Seed two buckets.
	rl.allow("10.0.0.1")
	rl.allow("10.0.0.2")

	// Backdate one as idle.
	rl.mu.Lock()
	rl.buckets["10.0.0.1"].lastSeen = time.Now().Add(-2 * rateLimiterEvictAfter)
	rl.mu.Unlock()

	rl.gcOnce()

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if _, ok := rl.buckets["10.0.0.1"]; ok {
		t.Error("idle bucket should have been evicted")
	}
	if _, ok := rl.buckets["10.0.0.2"]; !ok {
		t.Error("active bucket should still be present")
	}
}

func TestClientIP_StripsPort(t *testing.T) {
	cases := map[string]string{
		"10.0.0.1:1234":     "10.0.0.1",
		"[::1]:443":         "::1",
		"192.168.1.5":       "192.168.1.5", // no port — return as-is
	}
	for raw, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = raw
		if got := clientIP(req); got != want {
			t.Errorf("clientIP(%q) = %q, want %q", raw, got, want)
		}
	}
}
