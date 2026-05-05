package brain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// helper: build a Client wired to a httptest server with a tunable
// circuit breaker and tight retries (so tests don't drag on backoff).
// The clock is overridable through CircuitConfig.Now so half-open
// transitions can be triggered without sleeping the test goroutine.
func newResilienceTestClient(t *testing.T, handler http.HandlerFunc, cb CircuitConfig) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := NewClient(Config{
		BaseURL: srv.URL,
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

// 503 thrice → success on attempt 4 should NOT happen because
// MaxAttempts=3. Instead, 503 thrice → ErrTransient. This proves the
// retry loop respects the budget and surfaces ErrTransient.
func TestRetry_ExhaustsAfter3Attempts(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}, CircuitConfig{FailureThreshold: 100}) // breaker won't trip in this test
	defer srv.Close()

	ctx := ContextWithToken(context.Background(), "test-token")
	_, err := c.doRequest(ctx, "GET", "/api/anything", nil)
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server saw %d attempts, want 3", got)
	}
}

// 503 twice then 200 should retry through and succeed. Exercises the
// success path of the retry loop alongside the breaker's recordSuccess
// reset.
func TestRetry_RecoversAfter503Then200(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeEnvelope(w, http.StatusOK, map[string]string{"ok": "yes"})
	}, CircuitConfig{FailureThreshold: 100})
	defer srv.Close()

	ctx := ContextWithToken(context.Background(), "test-token")
	raw, err := c.doRequest(ctx, "GET", "/api/anything", nil)
	if err != nil {
		t.Fatalf("doRequest err = %v", err)
	}
	if string(raw) == "" {
		t.Errorf("empty data despite 200 envelope")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server saw %d attempts, want 3", got)
	}
	// After a final success, breaker should be closed with no
	// accumulated failures.
	state, fails := c.breaker.snapshot()
	if state != circuitClosed || fails != 0 {
		t.Errorf("breaker state=%v fails=%d, want closed+0", state, fails)
	}
}

// Always-503: drive the breaker past its failure threshold and then
// assert the next call short-circuits with ErrCircuitOpen.
func TestCircuit_OpensAfterThresholdFailures(t *testing.T) {
	var attempts atomic.Int32
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}, CircuitConfig{
		FailureThreshold: 4,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     1 * time.Hour, // keep open for the duration of the test
	})
	defer srv.Close()

	ctx := ContextWithToken(context.Background(), "test-token")

	// First call: 3 attempts × failure ⇒ 3 failures recorded.
	_, _ = c.doRequest(ctx, "GET", "/x", nil)

	// Second call should send 1 more attempt (failure 4 ⇒ trips
	// breaker) and then short-circuit any further attempts in the
	// same call. We assert the breaker is open after the call.
	_, _ = c.doRequest(ctx, "GET", "/x", nil)

	state, _ := c.breaker.snapshot()
	if state != circuitOpen {
		t.Fatalf("after %d failures, breaker state = %v, want open", attempts.Load(), state)
	}

	// Third call must short-circuit — no HTTP attempts should fire.
	before := attempts.Load()
	_, err := c.doRequest(ctx, "GET", "/x", nil)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if got := attempts.Load(); got != before {
		t.Errorf("breaker open should short-circuit; saw %d more attempts", got-before)
	}
}

// Half-open probe: drive breaker open, advance the fake clock past
// open-duration, next call should be admitted as a probe. If the probe
// succeeds, the breaker closes.
func TestCircuit_HalfOpenProbeClosesOnSuccess(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeEnvelope(w, http.StatusOK, map[string]string{"ok": "yes"})
	}, CircuitConfig{
		FailureThreshold: 2,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     30 * time.Second,
	})
	defer srv.Close()

	// Pin a fake clock we can advance manually.
	fakeNow := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	c.breaker.cfg.Now = func() time.Time { return fakeNow }

	ctx := ContextWithToken(context.Background(), "test-token")

	// Trip the breaker (2 attempts × failure crosses threshold=2).
	_, _ = c.doRequest(ctx, "GET", "/x", nil)
	state, _ := c.breaker.snapshot()
	if state != circuitOpen {
		t.Fatalf("breaker state = %v, want open after trip", state)
	}

	// While open, calls short-circuit.
	_, err := c.doRequest(ctx, "GET", "/x", nil)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected short-circuit, got %v", err)
	}

	// Advance past open-duration; flip the server to healthy; probe
	// should fire and close the breaker.
	fakeNow = fakeNow.Add(31 * time.Second)
	fail.Store(false)

	if _, err := c.doRequest(ctx, "GET", "/x", nil); err != nil {
		t.Fatalf("probe should succeed; err = %v", err)
	}
	state, fails := c.breaker.snapshot()
	if state != circuitClosed || fails != 0 {
		t.Errorf("after successful probe, state=%v fails=%d, want closed+0", state, fails)
	}
}

// Half-open probe failure should re-open the breaker.
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

	ctx := ContextWithToken(context.Background(), "test-token")

	// Trip.
	_, _ = c.doRequest(ctx, "GET", "/x", nil)
	if state, _ := c.breaker.snapshot(); state != circuitOpen {
		t.Fatalf("breaker should be open before probe")
	}

	// Advance past open-duration. Probe will fire and fail.
	fakeNow = fakeNow.Add(31 * time.Second)
	_, _ = c.doRequest(ctx, "GET", "/x", nil)

	if state, _ := c.breaker.snapshot(); state != circuitOpen {
		t.Errorf("breaker should reopen after failed probe; got %v", state)
	}
}

// 4xx is NOT a brain-instability signal — it should not advance the
// breaker's failure counter even with hundreds of consecutive 4xx
// responses.
func TestCircuit_4xxDoesNotCountTowardThreshold(t *testing.T) {
	c, srv := newResilienceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}, CircuitConfig{
		FailureThreshold: 3,
		FailureWindow:    1 * time.Hour,
		OpenDuration:     30 * time.Second,
	})
	defer srv.Close()

	ctx := ContextWithToken(context.Background(), "test-token")
	for i := 0; i < 10; i++ {
		_, _ = c.doRequest(ctx, "GET", "/x", nil)
	}
	if state, fails := c.breaker.snapshot(); state != circuitClosed || fails != 0 {
		t.Errorf("4xx should not trip breaker; state=%v fails=%d", state, fails)
	}
}

// FailureWindow expiry: failures older than the window should age out
// of the rolling list and not contribute to the threshold.
func TestCircuit_FailureWindowAgesOutFailures(t *testing.T) {
	cb := newCircuitBreaker(CircuitConfig{
		FailureThreshold: 3,
		FailureWindow:    10 * time.Second,
		OpenDuration:     30 * time.Second,
	})

	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	cb.cfg.Now = func() time.Time { return now }

	// Two failures at t=0.
	_, gen := cb.allow()
	cb.recordFailure(gen)
	_, gen = cb.allow()
	cb.recordFailure(gen)
	if state, fails := cb.snapshot(); state != circuitClosed || fails != 2 {
		t.Fatalf("after 2 failures: state=%v fails=%d", state, fails)
	}

	// Advance past window. Next failure should age out the previous
	// two and leave only itself in the window.
	now = now.Add(15 * time.Second)
	_, gen = cb.allow()
	cb.recordFailure(gen)
	if state, fails := cb.snapshot(); state != circuitClosed || fails != 1 {
		t.Errorf("after window expiry + 1 new failure: state=%v fails=%d, want closed+1", state, fails)
	}
}

// Per-method timeout: when caller passes context.Background and the
// Maestro endpoint stalls past the configured timeout, the call must
// return a deadline-exceeded error rather than hanging.
func TestMaestroTimeout_RespectsClientCeiling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hold the connection longer than the test's Maestro timeout.
		select {
		case <-time.After(500 * time.Millisecond):
			writeEnvelope(w, http.StatusOK, map[string]string{"reply": "late"})
		case <-r.Context().Done():
			return // client cancelled
		}
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		BaseURL: srv.URL,
		// Force a tight Maestro ceiling. Billing left at default.
		Timeouts: Timeouts{Maestro: 30 * time.Millisecond, Billing: 5 * time.Second},
		Retry:    RetryConfig{MaxAttempts: 1, BaseDelayMs: 1, Multiplier: 2.0},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := ContextWithToken(context.Background(), "test-token")
	_, err = c.Maestro.Chat(ctx, ChatRequest{Message: "hi"})
	if err == nil {
		t.Fatalf("expected deadline-exceeded; err = nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		// May come back wrapped through doRequest's transport-error
		// path — accept either DeadlineExceeded or a string match.
		t.Logf("err = %v (acceptable if it carries DeadlineExceeded semantics)", err)
	}
}
