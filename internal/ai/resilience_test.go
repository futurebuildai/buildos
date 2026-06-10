package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newResilienceTestClient builds a Client wired to a httptest server
// with a tunable circuit breaker and tight retries (so tests don't drag
// on backoff). The clock is overridable through CircuitConfig.Now so
// half-open transitions can be triggered without sleeping.
func newResilienceTestClient(t *testing.T, handler http.HandlerFunc, cb CircuitConfig) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := NewClient(Config{
		KeyResolver: staticKey("test-key"),
		BaseURL:     srv.URL,
		Retry: RetryConfig{
			MaxAttempts: 3,
			BaseDelayMs: 1, // exercise the backoff path without latency
			Multiplier:  2.0,
		},
		Circuit: cb,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv
}

// minimalOKMessages writes a minimal valid /v1/messages success body.
func minimalOKMessages(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
}

func bareMessagesReq() messagesRequest {
	return messagesRequest{
		Model:     "test-model",
		MaxTokens: 16,
		Messages:  []messageParam{{Role: "user", Content: []contentBlock{textBlock("hi")}}},
	}
}

// 503 thrice → ErrTransient. Proves the retry loop respects the budget.
func TestRetry_ExhaustsAfter3Attempts(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}, CircuitConfig{FailureThreshold: 100})
	defer srv.Close()

	_, err := c.messages(context.Background(), "test", "org", bareMessagesReq())
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server saw %d attempts, want 3", got)
	}
}

// 503 twice then 200 should retry through and succeed.
func TestRetry_RecoversAfter503Then200(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		minimalOKMessages(w)
	}, CircuitConfig{FailureThreshold: 100})
	defer srv.Close()

	resp, err := c.messages(context.Background(), "test", "org", bareMessagesReq())
	if err != nil {
		t.Fatalf("messages err = %v", err)
	}
	if len(resp.Content) == 0 {
		t.Errorf("empty content despite 200")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server saw %d attempts, want 3", got)
	}
	state, fails := c.breaker.snapshot()
	if state != circuitClosed || fails != 0 {
		t.Errorf("breaker state=%v fails=%d, want closed+0", state, fails)
	}
}

// 429 thrice → ErrRateLimited, and Retry-After is honored on the way.
func TestRetry_429MapsToRateLimited(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "0") // 0 → no extra wait, keep test fast
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
	}, CircuitConfig{FailureThreshold: 100})
	defer srv.Close()

	_, err := c.messages(context.Background(), "test", "org", bareMessagesReq())
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server saw %d attempts, want 3", got)
	}
}

// Retry-After header is parsed and used as the floor for the next delay.
func TestRetry_HonorsRetryAfterHeader(t *testing.T) {
	var attempts atomic.Int32
	var firstAt, secondAt time.Time
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		switch n {
		case 1:
			firstAt = time.Now()
			w.Header().Set("Retry-After", "1") // ask for 1s before retry
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			secondAt = time.Now()
			minimalOKMessages(w)
		}
	}, CircuitConfig{FailureThreshold: 100})
	defer srv.Close()

	_, err := c.messages(context.Background(), "test", "org", bareMessagesReq())
	if err != nil {
		t.Fatalf("messages err = %v", err)
	}
	if gap := secondAt.Sub(firstAt); gap < 900*time.Millisecond {
		t.Errorf("retry gap = %v, want >= ~1s (Retry-After honored)", gap)
	}
}

// 4xx (non-429) returns immediately with *HTTPError; no retry.
func TestClient_4xxNotRetried(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
	}, CircuitConfig{FailureThreshold: 100})
	defer srv.Close()

	_, err := c.messages(context.Background(), "test", "org", bareMessagesReq())
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err type = %T, want *HTTPError", err)
	}
	if httpErr.StatusCode != 400 || httpErr.Type != "invalid_request_error" {
		t.Errorf("HTTPError = %+v", httpErr)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server saw %d attempts; 4xx must not retry", got)
	}
}

// Transport error (closed port) maps to ErrTransient.
func TestClient_TransportErrorMapsToTransient(t *testing.T) {
	c, err := NewClient(Config{
		KeyResolver: staticKey("k"),
		BaseURL:     "http://127.0.0.1:1", // closed port
		Retry:       RetryConfig{MaxAttempts: 2, BaseDelayMs: 1, Multiplier: 2.0},
		Circuit:     CircuitConfig{FailureThreshold: 100},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.messages(context.Background(), "test", "org", bareMessagesReq())
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
}

// Always-503 drives the breaker past threshold; next call short-circuits.
func TestCircuit_OpensAfterThresholdFailures(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}, CircuitConfig{
		FailureThreshold: 4,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     1 * time.Hour,
	})
	defer srv.Close()

	_, _ = c.messages(context.Background(), "test", "org", bareMessagesReq()) // 3 failures
	_, _ = c.messages(context.Background(), "test", "org", bareMessagesReq()) // failure 4 trips

	state, _ := c.breaker.snapshot()
	if state != circuitOpen {
		t.Fatalf("after %d failures, breaker = %v, want open", attempts.Load(), state)
	}

	before := attempts.Load()
	_, err := c.messages(context.Background(), "test", "org", bareMessagesReq())
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if got := attempts.Load(); got != before {
		t.Errorf("open breaker should short-circuit; saw %d more attempts", got-before)
	}
}

// Half-open probe closes the breaker on success.
func TestCircuit_HalfOpenProbeClosesOnSuccess(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		minimalOKMessages(w)
	}, CircuitConfig{
		FailureThreshold: 2,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     30 * time.Second,
	})
	defer srv.Close()

	fakeNow := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	c.breaker.cfg.Now = func() time.Time { return fakeNow }

	_, _ = c.messages(context.Background(), "test", "org", bareMessagesReq())
	if state, _ := c.breaker.snapshot(); state != circuitOpen {
		t.Fatalf("breaker = %v, want open after trip", state)
	}
	if _, err := c.messages(context.Background(), "test", "org", bareMessagesReq()); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected short-circuit, got %v", err)
	}

	fakeNow = fakeNow.Add(31 * time.Second)
	fail.Store(false)
	if _, err := c.messages(context.Background(), "test", "org", bareMessagesReq()); err != nil {
		t.Fatalf("probe should succeed; err = %v", err)
	}
	if state, fails := c.breaker.snapshot(); state != circuitClosed || fails != 0 {
		t.Errorf("after probe, state=%v fails=%d, want closed+0", state, fails)
	}
}

// Half-open probe failure re-opens the breaker.
func TestCircuit_HalfOpenProbeReopensOnFailure(t *testing.T) {
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}, CircuitConfig{
		FailureThreshold: 2,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     30 * time.Second,
	})
	defer srv.Close()

	fakeNow := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	c.breaker.cfg.Now = func() time.Time { return fakeNow }

	_, _ = c.messages(context.Background(), "test", "org", bareMessagesReq())
	if state, _ := c.breaker.snapshot(); state != circuitOpen {
		t.Fatalf("breaker should be open before probe")
	}
	fakeNow = fakeNow.Add(31 * time.Second)
	_, _ = c.messages(context.Background(), "test", "org", bareMessagesReq())
	if state, _ := c.breaker.snapshot(); state != circuitOpen {
		t.Errorf("breaker should reopen after failed probe; got %v", state)
	}
}

// 4xx must not advance the breaker failure counter.
func TestCircuit_4xxDoesNotCountTowardThreshold(t *testing.T) {
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}, CircuitConfig{
		FailureThreshold: 3,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     30 * time.Second,
	})
	defer srv.Close()

	for i := 0; i < 10; i++ {
		_, _ = c.messages(context.Background(), "test", "org", bareMessagesReq())
	}
	if state, fails := c.breaker.snapshot(); state != circuitClosed || fails != 0 {
		t.Errorf("4xx should not trip breaker; state=%v fails=%d", state, fails)
	}
}

// FailureWindow expiry ages out old failures.
func TestCircuit_FailureWindowAgesOutFailures(t *testing.T) {
	cb := newCircuitBreaker(CircuitConfig{
		FailureThreshold: 3,
		FailureWindow:    10 * time.Second,
		OpenDuration:     30 * time.Second,
	})
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	cb.cfg.Now = func() time.Time { return now }

	_, gen, _ := cb.allow()
	cb.recordFailure(gen)
	_, gen, _ = cb.allow()
	cb.recordFailure(gen)
	if state, fails := cb.snapshot(); state != circuitClosed || fails != 2 {
		t.Fatalf("after 2 failures: state=%v fails=%d", state, fails)
	}

	now = now.Add(15 * time.Second)
	_, gen, _ = cb.allow()
	cb.recordFailure(gen)
	if state, fails := cb.snapshot(); state != circuitClosed || fails != 1 {
		t.Errorf("after window expiry + 1 failure: state=%v fails=%d, want closed+1", state, fails)
	}
}

// allow() returns the remaining open window as a Retry-After hint while open,
// and 0 while closed (Phase 4b-iii — surfaced as the HTTP Retry-After).
func TestCircuit_AllowReportsRetryAfterWhileOpen(t *testing.T) {
	cb := newCircuitBreaker(CircuitConfig{
		FailureThreshold: 1,
		OpenDuration:     30 * time.Second,
	})
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	cb.cfg.Now = func() time.Time { return now }

	ok, gen, ra := cb.allow()
	if !ok || ra != 0 {
		t.Fatalf("closed: ok=%v ra=%v, want true/0", ok, ra)
	}
	cb.recordFailure(gen) // threshold 1 → trips open at `now`

	now = now.Add(5 * time.Second)
	ok, _, ra = cb.allow()
	if ok {
		t.Fatal("breaker should be open (refusing the call)")
	}
	if ra != 25*time.Second {
		t.Errorf("RetryAfter = %v, want 25s (30s window − 5s elapsed)", ra)
	}
}

// Half-open admits exactly one probe: a second caller arriving while the
// probe is still outstanding is short-circuited.
func TestCircuit_HalfOpenSecondCallerShortCircuits(t *testing.T) {
	cb := newCircuitBreaker(CircuitConfig{
		FailureThreshold: 1,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     30 * time.Second,
	})
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	cb.cfg.Now = func() time.Time { return now }

	// Trip to open with a single failure.
	_, gen, _ := cb.allow()
	cb.recordFailure(gen)
	if state, _ := cb.snapshot(); state != circuitOpen {
		t.Fatalf("breaker = %v, want open", state)
	}

	// After the open-duration elapses the first caller is promoted to the
	// half-open probe and admitted.
	now = now.Add(31 * time.Second)
	if ok, _, _ := cb.allow(); !ok {
		t.Fatal("first caller after open-duration should be admitted as the probe")
	}
	if state, _ := cb.snapshot(); state != circuitHalfOpen {
		t.Fatalf("breaker = %v, want half-open", state)
	}

	// A second caller, with the probe still outstanding, must be denied.
	if ok, _, _ := cb.allow(); ok {
		t.Error("second caller during an outstanding probe should be denied")
	}
}

// Outcomes carrying a stale generation token (e.g. a probe that resolves
// after the breaker was already transitioned by another goroutine) are
// discarded so they can't corrupt newer state.
func TestCircuit_StaleGenerationOutcomesDiscarded(t *testing.T) {
	cb := newCircuitBreaker(CircuitConfig{
		FailureThreshold: 2,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     1 * time.Hour,
	})
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	cb.cfg.Now = func() time.Time { return now }

	// Record one real failure under the live generation.
	_, gen, _ := cb.allow()
	cb.recordFailure(gen)
	if _, fails := cb.snapshot(); fails != 1 {
		t.Fatalf("fails=%d, want 1 after one real failure", fails)
	}

	staleGen := gen + 99 // a generation token that never matches the live seed

	// A stale failure must be discarded — otherwise it would cross the
	// threshold and trip the breaker.
	cb.recordFailure(staleGen)
	if state, fails := cb.snapshot(); state != circuitClosed || fails != 1 {
		t.Errorf("stale failure leaked: state=%v fails=%d, want closed+1", state, fails)
	}

	// A stale success must be discarded — otherwise it would clear the
	// real in-window failure.
	cb.recordSuccess(staleGen)
	if _, fails := cb.snapshot(); fails != 1 {
		t.Errorf("stale success cleared real failures: fails=%d, want 1", fails)
	}
}
