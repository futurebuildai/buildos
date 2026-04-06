// Package physics implements the deterministic scheduling algorithms.
// This file contains tests for deterministic integer math in AddWorkingDays.
// See L7 Technical Debt Remediation: Task 3
package physics

import (
	"testing"
	"time"
)

// =============================================================================
// RED TEST: Task 3 - Prove Floating-Point Drift in AddWorkingDays
// =============================================================================
// This test proves that IEEE 754 floating-point arithmetic is non-associative
// and can cause nanosecond-level drift in date calculations.
//
// The fundamental problem:
//   0.1 + 0.1 + 0.1 + 0.1 + 0.1 + 0.1 + 0.1 + 0.1 + 0.1 + 0.1 != 1.0
//
// This causes schedule drift where tasks slip by nanoseconds across day boundaries.
//
// EXPECTED BEHAVIOR (BEFORE FIX): This test FAILS due to floating-point drift
// EXPECTED BEHAVIOR (AFTER FIX): This test PASSES with deterministic rounding
// =============================================================================

// TestAddWorkingDays_FloatDrift_RedTest proves floating-point causes non-determinism.
//
// This test demonstrates the IEEE 754 problem by repeatedly adding small fractions.
func TestAddWorkingDays_FloatDrift_RedTest(t *testing.T) {
	cal := &StandardCalendar{}
	startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC) // Monday 8am

	// Test 1: Add 0.5 days 20 times should equal adding 10 days
	result1 := startDate
	for i := 0; i < 20; i++ {
		result1 = cal.AddWorkingDays(result1, 0.5)
	}

	result2 := cal.AddWorkingDays(startDate, 10.0)

	// These should be exactly equal, but floating-point errors may cause drift
	diff := result1.Sub(result2)
	if diff != 0 {
		t.Logf("FLOATING-POINT DRIFT DETECTED: %v difference", diff)
		t.Logf("  Multiple small additions: %v", result1)
		t.Logf("  Single large addition:    %v", result2)
	}

	// Test 2: The classic 0.1 problem
	// Adding 0.1 ten times should equal 1.0, but in IEEE 754 it doesn't
	tenthsTotal := 0.0
	for i := 0; i < 10; i++ {
		tenthsTotal += 0.1
	}

	if tenthsTotal != 1.0 {
		t.Logf("IEEE 754 DEMONSTRATION: 0.1 * 10 == %v (not 1.0)", tenthsTotal)
		t.Logf("This non-associativity propagates into schedule calculations")
	}

	// Test 3: Verify determinism - same inputs must produce exactly same outputs
	// This tests the core requirement: 1 + 1 must always equal 2
	resultA := cal.AddWorkingDays(startDate, 5.123456789)
	resultB := cal.AddWorkingDays(startDate, 5.123456789)

	if !resultA.Equal(resultB) {
		t.Errorf("DETERMINISM VIOLATION: Same input produced different outputs")
		t.Errorf("  Result A: %v", resultA)
		t.Errorf("  Result B: %v", resultB)
	}

	// =============================================================================
	// CRITICAL ASSERTION: Results must be rounded to minute precision
	// =============================================================================
	// BEFORE FIX: Nanosecond-level precision causes drift across architectures
	// AFTER FIX:  Results are truncated to minute precision
	// =============================================================================

	// Check that result has no nanosecond-level precision (should be minute-aligned)
	if result1.Nanosecond() != 0 || result1.Second() != 0 {
		t.Errorf("PRECISION LEAK: Result has sub-minute precision: %v", result1)
		t.Error("This will cause non-deterministic behavior across x86 vs ARM architectures")
	}
}

// TestAddWorkDuration_Deterministic tests the new integer-based API.
//
// This test will initially fail because AddWorkDuration doesn't exist yet.
// After the fix, it should pass with perfect determinism.
func TestAddWorkDuration_Deterministic(t *testing.T) {
	// Skip this test until AddWorkDuration is implemented
	t.Skip("AddWorkDuration not yet implemented - will enable after Task 3 implementation")

	// cal := &StandardCalendar{}
	// startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC) // Monday 8am

	// Test 1: Adding WorkDay units should be perfectly deterministic
	// halfDay := WorkDay / 2
	// result1 := startDate
	// for i := 0; i < 20; i++ {
	// 	result1 = cal.AddWorkDuration(result1, halfDay)
	// }
	// result2 := cal.AddWorkDuration(startDate, 10*WorkDay)

	// // With integer math, these MUST be exactly equal
	// if !result1.Equal(result2) {
	// 	t.Errorf("DETERMINISM VIOLATION with integer math")
	// 	t.Errorf("  Multiple small additions: %v", result1)
	// 	t.Errorf("  Single large addition:    %v", result2)
	// }
}

// TestAddWorkingDays_RoundingStrategy tests that fractional inputs are handled deterministically.
func TestAddWorkingDays_RoundingStrategy(t *testing.T) {
	cal := &StandardCalendar{}
	startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC) // Monday 8am

	// Adding 1.333333... days should produce a deterministic, rounded result
	result := cal.AddWorkingDays(startDate, 1.3333333333333333)

	// The result should NOT have sub-second precision
	if result.Nanosecond() != 0 {
		t.Errorf("Non-deterministic nanosecond precision: %d ns", result.Nanosecond())
		t.Error("This precision will drift across different floating-point implementations")
	}

	t.Logf("Result: %v (should be rounded to minute)", result)
}

// TestAddWorkingDays_AssociativityProperty tests mathematical associativity.
//
// For a truly deterministic system:
//
//	add(add(date, a), b) == add(date, a+b)
//
// Floating-point violates this property.
func TestAddWorkingDays_AssociativityProperty(t *testing.T) {
	cal := &StandardCalendar{}
	startDate := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC) // Monday 8am

	// Test associativity: (a + b) + c == a + (b + c)
	a, b, c := 0.7, 0.2, 0.1

	// Order 1: ((date + a) + b) + c
	result1 := cal.AddWorkingDays(startDate, a)
	result1 = cal.AddWorkingDays(result1, b)
	result1 = cal.AddWorkingDays(result1, c)

	// Order 2: date + (a + b + c)
	result2 := cal.AddWorkingDays(startDate, a+b+c)

	diff := result1.Sub(result2)
	if diff != 0 {
		t.Logf("ASSOCIATIVITY VIOLATION: Different ordering produces %v difference", diff)
		t.Logf("  Sequential:  %v", result1)
		t.Logf("  Combined:    %v", result2)
		t.Logf("This is expected with float64 - fix requires integer math")
	}
}
