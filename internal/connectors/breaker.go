package connectors

import (
	"sync"
	"time"
)

// Circuit breaker for connector egress (Phase 3b-ii). A connectors-local copy of
// the proven internal/ai breaker (those symbols are unexported + process-global,
// so they cannot be shared). Keyed PER (org, endpoint) by BreakerRegistry: a
// flaky tenant server must trip the breaker only for that tenant's endpoint, not
// for all connectors.

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

// BreakerConfig tunes a breaker. Zero values take defaults.
type BreakerConfig struct {
	FailureThreshold int           // failures within FailureWindow that open the breaker. Default 5.
	FailureWindow    time.Duration // rolling window for counting failures. Default 60s.
	OpenDuration     time.Duration // how long open before admitting a half-open probe. Default 30s.
	Now              func() time.Time
}

func (c BreakerConfig) withDefaults() BreakerConfig {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.FailureWindow <= 0 {
		c.FailureWindow = 60 * time.Second
	}
	if c.OpenDuration <= 0 {
		c.OpenDuration = 30 * time.Second
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Breaker is a closed/open/half-open circuit breaker. generationSeed makes stale
// outcomes (from a pre-transition attempt) unable to corrupt newer state.
type Breaker struct {
	cfg BreakerConfig

	mu       sync.Mutex
	state    breakerState
	failures []time.Time
	openedAt time.Time
	probing  bool
	gen      uint64
}

func newBreaker(cfg BreakerConfig) *Breaker { return &Breaker{cfg: cfg.withDefaults()} }

// Allow reports whether a call may proceed, and a generation token to pass back
// to RecordSuccess/RecordFailure.
func (b *Breaker) Allow() (bool, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.cfg.Now()
	switch b.state {
	case breakerClosed:
		return true, b.gen
	case breakerOpen:
		if now.Sub(b.openedAt) >= b.cfg.OpenDuration {
			b.state = breakerHalfOpen
			b.probing = true
			b.gen++
			return true, b.gen
		}
		return false, b.gen
	case breakerHalfOpen:
		if b.probing {
			return false, b.gen
		}
		b.probing = true
		return true, b.gen
	}
	return true, b.gen
}

// RecordSuccess closes a half-open breaker / clears the failure window.
func (b *Breaker) RecordSuccess(gen uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if gen != b.gen {
		return
	}
	if b.state == breakerHalfOpen {
		b.state = breakerClosed
		b.probing = false
		b.failures = nil
		b.gen++
		return
	}
	b.failures = nil
}

// RecordFailure appends a failure and opens the breaker on threshold.
func (b *Breaker) RecordFailure(gen uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if gen != b.gen {
		return
	}
	now := b.cfg.Now()
	if b.state == breakerHalfOpen {
		b.state = breakerOpen
		b.openedAt = now
		b.probing = false
		b.gen++
		return
	}
	b.failures = append(b.failures, now)
	cutoff := now.Add(-b.cfg.FailureWindow)
	first := 0
	for ; first < len(b.failures); first++ {
		if !b.failures[first].Before(cutoff) {
			break
		}
	}
	b.failures = b.failures[first:]
	if len(b.failures) >= b.cfg.FailureThreshold {
		b.state = breakerOpen
		b.openedAt = now
		b.failures = nil
		b.gen++
	}
}

// BreakerRegistry hands out a per-key Breaker, lazily created and held for the
// process lifetime. The service holds one registry and keys it by org+endpoint.
type BreakerRegistry struct {
	cfg BreakerConfig
	mu  sync.Mutex
	m   map[string]*Breaker
}

// NewBreakerRegistry constructs a registry with the given (defaulted) config.
func NewBreakerRegistry(cfg BreakerConfig) *BreakerRegistry {
	return &BreakerRegistry{cfg: cfg.withDefaults(), m: make(map[string]*Breaker)}
}

// Get returns the breaker for a key, creating it on first use.
func (r *BreakerRegistry) Get(key string) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.m[key]
	if !ok {
		b = newBreaker(r.cfg)
		r.m[key] = b
	}
	return b
}
