package connectors

import (
	"testing"
	"time"
)

func TestBreaker_OpensAfterThreshold(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	b := newBreaker(BreakerConfig{FailureThreshold: 3, FailureWindow: time.Minute, OpenDuration: 30 * time.Second, Now: clock})

	for i := 0; i < 3; i++ {
		ok, gen := b.Allow()
		if !ok {
			t.Fatalf("call %d should be allowed while closed", i)
		}
		b.RecordFailure(gen)
	}
	if ok, _ := b.Allow(); ok {
		t.Fatal("breaker should be OPEN after crossing the failure threshold")
	}

	// After the open duration, a single half-open probe is admitted.
	now = now.Add(31 * time.Second)
	ok, gen := b.Allow()
	if !ok {
		t.Fatal("a half-open probe should be admitted after OpenDuration")
	}
	// A second concurrent caller is denied while the probe is outstanding.
	if ok2, _ := b.Allow(); ok2 {
		t.Error("only one half-open probe may be in flight")
	}
	// The probe succeeds → breaker closes.
	b.RecordSuccess(gen)
	if ok, _ := b.Allow(); !ok {
		t.Error("breaker should be CLOSED after a successful half-open probe")
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	b := newBreaker(BreakerConfig{FailureThreshold: 1, OpenDuration: 10 * time.Second, Now: clock})

	_, gen := b.Allow()
	b.RecordFailure(gen) // threshold 1 → open
	now = now.Add(11 * time.Second)
	_, gen = b.Allow() // half-open probe
	b.RecordFailure(gen)
	if ok, _ := b.Allow(); ok {
		t.Error("a failed half-open probe must reopen the breaker")
	}
}

func TestBreakerRegistry_PerKeyIsolation(t *testing.T) {
	reg := NewBreakerRegistry(BreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute})
	a := reg.Get("orgA|https://a")
	bk := reg.Get("orgB|https://b")
	if a == bk {
		t.Fatal("distinct keys must get distinct breakers")
	}
	if a2 := reg.Get("orgA|https://a"); a2 != a {
		t.Fatal("the same key must return the same breaker")
	}
	// Tripping A must not affect B.
	_, gen := a.Allow()
	a.RecordFailure(gen)
	if ok, _ := a.Allow(); ok {
		t.Error("breaker A should be open")
	}
	if ok, _ := bk.Allow(); !ok {
		t.Error("breaker B must be unaffected by A's failures")
	}
}
