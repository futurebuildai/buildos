package ai

import (
	"sync"
	"time"
)

// circuitState models the standard CB state machine.
//
//	closed     — calls flow through normally; failures are counted in
//	             the rolling window; threshold trip opens the breaker.
//	open       — calls short-circuit immediately with ErrCircuitOpen
//	             until the open-duration elapses; then we move to
//	             half-open and admit a single probe.
//	halfOpen   — exactly one in-flight probe is permitted. If it
//	             succeeds we transition back to closed and reset the
//	             failure window. If it fails we go back to open and
//	             restart the open-duration timer.
//
// All transitions are guarded by cb.mu.
type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// CircuitConfig configures the AI client's circuit breaker.
// Zero values fall back to the defaults applied by withDefaults.
type CircuitConfig struct {
	// FailureThreshold is the number of failures within FailureWindow
	// that flips the breaker to open. Default 5.
	FailureThreshold int
	// FailureWindow is the rolling window in which failures are counted
	// toward FailureThreshold. Default 60s.
	FailureWindow time.Duration
	// OpenDuration is how long the breaker stays in open state before
	// admitting a half-open probe. Default 30s.
	OpenDuration time.Duration
	// Now overrides time.Now for deterministic tests. nil → time.Now.
	Now func() time.Time
}

func (cc CircuitConfig) withDefaults() CircuitConfig {
	if cc.FailureThreshold <= 0 {
		cc.FailureThreshold = 5
	}
	if cc.FailureWindow <= 0 {
		cc.FailureWindow = 60 * time.Second
	}
	if cc.OpenDuration <= 0 {
		cc.OpenDuration = 30 * time.Second
	}
	if cc.Now == nil {
		cc.Now = time.Now
	}
	return cc
}

// circuitBreaker is the in-house breaker shared by every AI HTTP
// attempt. We avoid pulling in an external dep (gobreaker / hystrix)
// because the failure-window / open-duration / half-open-probe state
// machine fits in ~80 LOC and the AI client is the only consumer.
type circuitBreaker struct {
	cfg CircuitConfig

	mu             sync.Mutex
	state          circuitState
	failures       []time.Time // sliding window of recent failures
	openedAt       time.Time   // when the breaker last flipped to open
	probeOutstand  bool        // true when half-open probe is in flight
	generationSeed uint64      // monotonic; increments on every state transition so stale outcomes can't corrupt newer state
}

func newCircuitBreaker(cfg CircuitConfig) *circuitBreaker {
	return &circuitBreaker{cfg: cfg.withDefaults()}
}

// allow returns (canProceed, generation). generation is later passed
// back to recordSuccess / recordFailure so an outcome from a stale
// state transition (e.g., half-open probe completes after the breaker
// was reset by a manual intervention) can't flip the wrong state.
// allow reports whether a call may proceed, the breaker generation, and — when
// it returns false because the breaker is OPEN — the remaining time until the
// breaker promotes to half-open (for an accurate client-facing Retry-After). The
// duration is 0 in every admit case and in half-open (a probe is in flight, so
// the wait is indeterminate).
func (cb *circuitBreaker) allow() (bool, uint64, time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := cb.cfg.Now()
	switch cb.state {
	case circuitClosed:
		return true, cb.generationSeed, 0
	case circuitOpen:
		if now.Sub(cb.openedAt) >= cb.cfg.OpenDuration {
			// Open duration elapsed; promote to half-open and admit
			// a single probe.
			cb.state = circuitHalfOpen
			cb.probeOutstand = true
			cb.generationSeed++
			return true, cb.generationSeed, 0
		}
		return false, cb.generationSeed, cb.openedAt.Add(cb.cfg.OpenDuration).Sub(now)
	case circuitHalfOpen:
		// At most one probe in flight; everyone else short-circuits
		// until the probe resolves.
		if cb.probeOutstand {
			return false, cb.generationSeed, 0
		}
		cb.probeOutstand = true
		return true, cb.generationSeed, 0
	}
	return true, cb.generationSeed, 0
}

// recordSuccess collapses the in-window failure list (success ⇒ no
// reason to keep stale failures) and, if we were probing in half-open,
// closes the breaker.
func (cb *circuitBreaker) recordSuccess(gen uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if gen != cb.generationSeed {
		return // outcome is from a stale generation; discard
	}
	if cb.state == circuitHalfOpen {
		cb.state = circuitClosed
		cb.probeOutstand = false
		cb.failures = nil
		cb.generationSeed++
		return
	}
	// Closed-state success — clear in-window failures so a one-off
	// blip doesn't accumulate alongside new failures.
	cb.failures = nil
}

// recordFailure appends a failure to the rolling window and trips the
// breaker if we cross the threshold inside FailureWindow.
func (cb *circuitBreaker) recordFailure(gen uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if gen != cb.generationSeed {
		return // outcome from a stale generation; discard
	}
	now := cb.cfg.Now()
	if cb.state == circuitHalfOpen {
		// Probe failed — reopen and restart the open-duration timer.
		cb.state = circuitOpen
		cb.openedAt = now
		cb.probeOutstand = false
		cb.generationSeed++
		return
	}
	// Append, then prune anything outside the window.
	cb.failures = append(cb.failures, now)
	cutoff := now.Add(-cb.cfg.FailureWindow)
	first := 0
	for ; first < len(cb.failures); first++ {
		if !cb.failures[first].Before(cutoff) {
			break
		}
	}
	cb.failures = cb.failures[first:]

	if len(cb.failures) >= cb.cfg.FailureThreshold {
		cb.state = circuitOpen
		cb.openedAt = now
		cb.failures = nil
		cb.generationSeed++
	}
}

// snapshot is for tests: returns the current state without mutating.
func (cb *circuitBreaker) snapshot() (circuitState, int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state, len(cb.failures)
}
