package physics

import (
	"testing"
	"time"
)

// TestAddWorkingDays_FloatDrift_RedTest proves floating-point causes non-determinism.
func TestAddWorkingDays_FloatDrift_RedTest(t *testing.T) {
	cal := &StandardCalendar{}
	startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	result1 := startDate
	for i := 0; i < 20; i++ {
		result1 = cal.AddWorkingDays(result1, 0.5)
	}

	result2 := cal.AddWorkingDays(startDate, 10.0)

	diff := result1.Sub(result2)
	if diff != 0 {
		t.Logf("FLOATING-POINT DRIFT DETECTED: %v difference", diff)
		t.Logf("  Multiple small additions: %v", result1)
		t.Logf("  Single large addition:    %v", result2)
	}

	// Verify determinism - same inputs must produce exactly same outputs
	resultA := cal.AddWorkingDays(startDate, 5.123456789)
	resultB := cal.AddWorkingDays(startDate, 5.123456789)

	if !resultA.Equal(resultB) {
		t.Errorf("DETERMINISM VIOLATION: Same input produced different outputs")
		t.Errorf("  Result A: %v", resultA)
		t.Errorf("  Result B: %v", resultB)
	}

	// Results must be rounded to minute precision
	if result1.Nanosecond() != 0 || result1.Second() != 0 {
		t.Errorf("PRECISION LEAK: Result has sub-minute precision: %v", result1)
	}
}

// TestAddWorkingDays_RoundingStrategy tests that fractional inputs are handled deterministically.
func TestAddWorkingDays_RoundingStrategy(t *testing.T) {
	cal := &StandardCalendar{}
	startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	result := cal.AddWorkingDays(startDate, 1.3333333333333333)

	if result.Nanosecond() != 0 {
		t.Errorf("Non-deterministic nanosecond precision: %d ns", result.Nanosecond())
	}
}

// TestAddWorkingDays_AssociativityProperty tests mathematical associativity.
func TestAddWorkingDays_AssociativityProperty(t *testing.T) {
	cal := &StandardCalendar{}
	startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	a, b, c := 0.7, 0.2, 0.1

	result1 := cal.AddWorkingDays(startDate, a)
	result1 = cal.AddWorkingDays(result1, b)
	result1 = cal.AddWorkingDays(result1, c)

	result2 := cal.AddWorkingDays(startDate, a+b+c)

	diff := result1.Sub(result2)
	if diff != 0 {
		t.Logf("ASSOCIATIVITY VIOLATION: Different ordering produces %v difference", diff)
		t.Logf("  Sequential:  %v", result1)
		t.Logf("  Combined:    %v", result2)
	}
}

// TestAddWorkDuration_IntegerDeterminism tests that same inputs always produce same outputs.
func TestAddWorkDuration_IntegerDeterminism(t *testing.T) {
	cal := &StandardCalendar{}
	startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	// Same input must produce identical output (core determinism requirement)
	resultA := cal.AddWorkDuration(startDate, 5*24*time.Hour)
	resultB := cal.AddWorkDuration(startDate, 5*24*time.Hour)
	if !resultA.Equal(resultB) {
		t.Errorf("DETERMINISM VIOLATION: same input produced different outputs")
		t.Errorf("  Result A: %v", resultA)
		t.Errorf("  Result B: %v", resultB)
	}

	// Multiple whole-day additions should equal single addition
	result1 := startDate
	for i := 0; i < 10; i++ {
		result1 = cal.AddWorkDuration(result1, 24*time.Hour)
	}
	result2 := cal.AddWorkDuration(startDate, 10*24*time.Hour)
	if !result1.Equal(result2) {
		t.Errorf("DETERMINISM VIOLATION: 10x1 day != 1x10 days")
		t.Errorf("  Sequential: %v", result1)
		t.Errorf("  Combined:   %v", result2)
	}

	// Results must be minute-aligned (no nanosecond drift)
	if result1.Nanosecond() != 0 || result1.Second() != 0 {
		t.Errorf("PRECISION LEAK: Result has sub-minute precision: %v", result1)
	}
}
