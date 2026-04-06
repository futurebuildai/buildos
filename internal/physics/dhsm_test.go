package physics

import (
	"math"
	"testing"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/config"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/google/uuid"
)

// defaultGSF is the baseline GSF where SAF = 1.0
const defaultGSF = 2250.0

// defaultCfg is the standard config for baseline tests
var defaultCfg = config.DefaultPhysicsConfig()

// === SAF CALCULATION TESTS ===

func TestCalculateSAF_Baseline(t *testing.T) {
	// At baseline 2250 GSF, SAF should be 1.0
	saf := CalculateSAF(2250.0, defaultCfg)
	if saf != 1.0 {
		t.Errorf("expected SAF=1.0 at baseline, got %v", saf)
	}
}

func TestCalculateSAF_LargerHome(t *testing.T) {
	// At 4500 GSF (2x baseline), SAF should be ~1.68 (2^0.75)
	saf := CalculateSAF(4500.0, defaultCfg)
	expected := math.Pow(2.0, 0.75) // ~1.6818
	if math.Abs(saf-expected) > 0.01 {
		t.Errorf("expected SAF~%.2f for 4500 GSF, got %.4f", expected, saf)
	}
}

func TestCalculateSAF_SmallerHome(t *testing.T) {
	// At 1500 GSF (0.67x baseline), SAF should be < 1.0
	saf := CalculateSAF(1500.0, defaultCfg)
	expected := math.Pow(1500.0/2250.0, 0.75) // ~0.742
	if math.Abs(saf-expected) > 0.01 {
		t.Errorf("expected SAF~%.2f for 1500 GSF, got %.4f", expected, saf)
	}
	if saf >= 1.0 {
		t.Errorf("SAF should be < 1.0 for smaller homes, got %v", saf)
	}
}

func TestCalculateSAF_ZeroGSF(t *testing.T) {
	// Zero or negative GSF should return 1.0 (safe default)
	saf := CalculateSAF(0, defaultCfg)
	if saf != 1.0 {
		t.Errorf("expected SAF=1.0 for zero GSF, got %v", saf)
	}
}

// === FAANG THRESHOLD TESTS ===

// TestFAANG_HotSwapConfig proves config changes immediately affect schedule duration.
// FAANG Threshold #1: "Can you change the Standard House Size in a test setup
// and immediately see the schedule duration change without recompiling?"
func TestFAANG_HotSwapConfig(t *testing.T) {
	// Config 1: Standard baseline (2250 SF)
	cfg1 := config.PhysicsConfig{StandardHouseSizeSF: 2250.0, SizeAdjustmentExponent: 0.75}

	// Config 2: Larger baseline (3000 SF) - same house appears smaller
	cfg2 := config.PhysicsConfig{StandardHouseSizeSF: 3000.0, SizeAdjustmentExponent: 0.75}

	// Calculate SAF for 4500 SF with both configs
	saf1 := CalculateSAF(4500.0, cfg1) // (4500/2250)^0.75 ~ 1.68
	saf2 := CalculateSAF(4500.0, cfg2) // (4500/3000)^0.75 ~ 1.36

	// Verify different configs produce different results
	if saf1 == saf2 {
		t.Error("FAIL: Config change did not affect SAF calculation")
	}

	// Verify specific values
	expected1 := math.Pow(4500.0/2250.0, 0.75) // ~1.68
	expected2 := math.Pow(4500.0/3000.0, 0.75) // ~1.36

	if math.Abs(saf1-expected1) > 0.01 {
		t.Errorf("Config 1: expected SAF~%.4f, got %.4f", expected1, saf1)
	}
	if math.Abs(saf2-expected2) > 0.01 {
		t.Errorf("Config 2: expected SAF~%.4f, got %.4f", expected2, saf2)
	}

	t.Logf("Hot-Swap Test PASSED: cfg1 SAF=%.4f, cfg2 SAF=%.4f", saf1, saf2)
}

// TestFAANG_ZeroValueSafety proves the system falls back to safe defaults.
// FAANG Threshold #2: "If the config is missing/zero, does the system panic
// or fall back to safe defaults?"
func TestFAANG_ZeroValueSafety(t *testing.T) {
	// Zero config - all values unset
	zeroCfg := config.PhysicsConfig{} // StandardHouseSizeSF=0, SizeAdjustmentExponent=0

	// Should NOT panic, should fall back to defaults
	saf := CalculateSAF(2250.0, zeroCfg)

	// With default StandardHouseSizeSF=2250 and exponent=0.75, SAF should be 1.0
	if saf != 1.0 {
		t.Errorf("FAIL: Expected SAF=1.0 with zero config (defaults applied), got %v", saf)
	}

	t.Log("Zero-Value Safety Test PASSED: System uses safe defaults")
}

// === EVENT DURATION LOCKING TESTS ===

func TestEventDurationLocking_InspectionBypassesSAF(t *testing.T) {
	// Inspection tasks should NOT be scaled by SAF
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "8.2",
		Name:             "Footings Inspection",
		BaseDurationDays: 1.0,
		IsInspection:     true, // KEY: This is an inspection
	}
	context := models.ProjectContext{}

	// Use very large GSF - should NOT affect inspection duration
	result := CalculateTaskDuration(task, 10000.0, context, nil, models.Forecast{}, defaultCfg)

	// Expected: 1.0 (no SAF applied) -> quantized to 1.0
	if result != 1.0 {
		t.Errorf("inspection should bypass SAF, expected 1.0, got %v", result)
	}
}

func TestEventDurationLocking_NonInspectionScalesBySAF(t *testing.T) {
	// Regular tasks SHOULD be scaled by SAF
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Floor Framing",
		BaseDurationDays: 5.0,
		IsInspection:     false,
	}
	context := models.ProjectContext{}

	// Use 4500 GSF (2x baseline) -> SAF ~ 1.68
	result := CalculateTaskDuration(task, 4500.0, context, nil, models.Forecast{}, defaultCfg)

	// Expected: 5.0 * 1.68 = 8.4 -> quantized to 8.5
	expected := 8.5
	if result != expected {
		t.Errorf("expected %.1f with SAF scaling, got %v", expected, result)
	}
}

// === QUANTIZATION TESTS ===

func TestQuantization_SnapsToHalfDay(t *testing.T) {
	testCases := []struct {
		base     float64
		expected float64
	}{
		{4.2, 4.5},  // 4.2 -> ceil(8.4) / 2 = 4.5
		{5.0, 5.0},  // 5.0 -> ceil(10) / 2 = 5.0
		{5.1, 5.5},  // 5.1 -> ceil(10.2) / 2 = 5.5
		{3.75, 4.0}, // 3.75 -> ceil(7.5) / 2 = 4.0
		{1.0, 1.0},  // 1.0 -> ceil(2) / 2 = 1.0
		{0.3, 0.5},  // 0.3 -> ceil(0.6) / 2 = 0.5
	}

	for _, tc := range testCases {
		task := models.WBSTask{
			ID:               uuid.New(),
			Code:             "test",
			Name:             "Test Task",
			BaseDurationDays: tc.base,
		}

		// Use baseline GSF so SAF = 1.0, no multipliers
		result := CalculateTaskDuration(task, defaultGSF, models.ProjectContext{}, nil, models.Forecast{}, defaultCfg)

		if result != tc.expected {
			t.Errorf("base=%.2f: expected %.1f, got %.1f", tc.base, tc.expected, result)
		}
	}
}

// === MULTIPLIER TESTS (UPDATED WITH NEW SIGNATURE) ===

func TestCalculateTaskDuration_NoMultipliers(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Floor Framing",
		BaseDurationDays: 5.0,
	}
	context := models.ProjectContext{}

	// Use baseline GSF so SAF = 1.0
	result := CalculateTaskDuration(task, defaultGSF, context, nil, models.Forecast{}, defaultCfg)

	// 5.0 -> quantized to 5.0
	if result != 5.0 {
		t.Errorf("expected 5.0, got %v", result)
	}
}

func TestCalculateTaskDuration_SingleLinearMultiplier(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Floor Framing",
		BaseDurationDays: 5.0,
	}
	context := models.ProjectContext{
		SupplyChainVolatility: 2, // Value = 2
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "9.1",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.1, // 10% per unit
			MultiplierFormula: "linear",
		},
	}

	// Expected: 5.0 * 1.0(SAF) * (1 + (2 * 0.1)) = 5.0 * 1.2 = 6.0
	result := CalculateTaskDuration(task, defaultGSF, context, multipliers, models.Forecast{}, defaultCfg)

	if result != 6.0 {
		t.Errorf("expected 6.0, got %v", result)
	}
}

func TestCalculateTaskDuration_WildcardMultiplier(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "12.4",
		Name:             "Tile Work",
		BaseDurationDays: 3.0,
	}
	context := models.ProjectContext{
		SupplyChainVolatility: 3,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "*", // Wildcard - applies to all tasks
			VariableKey:       "supply_chain_volatility",
			Weight:            0.2,
			MultiplierFormula: "linear",
		},
	}

	// Expected: 3.0 * (1 + (3 * 0.2)) = 3.0 * 1.6 = 4.8 -> quantized to 5.0
	result := CalculateTaskDuration(task, defaultGSF, context, multipliers, models.Forecast{}, defaultCfg)

	if result != 5.0 {
		t.Errorf("expected 5.0, got %v", result)
	}
}

func TestCalculateTaskDuration_MultipleMultipliers(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Floor Framing",
		BaseDurationDays: 5.0,
	}
	context := models.ProjectContext{
		SupplyChainVolatility:  2,
		RoughInspectionLatency: 3,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "9.1",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.1,
			MultiplierFormula: "linear",
		},
		{
			ID:                uuid.New(),
			WBSTaskCode:       "*",
			VariableKey:       "rough_inspection_latency",
			Weight:            0.05,
			MultiplierFormula: "linear",
		},
	}

	// Expected: 5.0 * (1 + (2 * 0.1)) * (1 + (3 * 0.05))
	//         = 5.0 * 1.2 * 1.15 = 6.9 -> quantized to 7.0
	result := CalculateTaskDuration(task, defaultGSF, context, multipliers, models.Forecast{}, defaultCfg)

	if result != 7.0 {
		t.Errorf("expected 7.0, got %v", result)
	}
}

func TestCalculateTaskDuration_NonMatchingCode(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Floor Framing",
		BaseDurationDays: 5.0,
	}
	context := models.ProjectContext{
		SupplyChainVolatility: 2,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "10.0", // Does not match 9.1
			VariableKey:       "supply_chain_volatility",
			Weight:            0.5,
			MultiplierFormula: "linear",
		},
	}

	// Should return base duration since multiplier doesn't match
	result := CalculateTaskDuration(task, defaultGSF, context, multipliers, models.Forecast{}, defaultCfg)

	if result != 5.0 {
		t.Errorf("expected 5.0 (no multiplier applied), got %v", result)
	}
}

func TestCalculateTaskDuration_StepFormula(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.2",
		Name:             "Wall Framing",
		BaseDurationDays: 4.0,
	}
	context := models.ProjectContext{
		SupplyChainVolatility: 3, // 3 floors
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "9.2",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.3,
			MultiplierFormula: "step",
		},
	}

	// Step formula: 1 + (value - 1) * weight = 1 + (3 - 1) * 0.3 = 1.6
	// Expected: 4.0 * 1.6 = 6.4 -> quantized to 6.5
	result := CalculateTaskDuration(task, defaultGSF, context, multipliers, models.Forecast{}, defaultCfg)

	if result != 6.5 {
		t.Errorf("expected 6.5, got %v", result)
	}
}

func TestCalculateBatchDurations(t *testing.T) {
	tasks := []models.WBSTask{
		{ID: uuid.New(), Code: "9.1", Name: "Floor Framing", BaseDurationDays: 5.0},
		{ID: uuid.New(), Code: "9.2", Name: "Wall Framing", BaseDurationDays: 4.0},
	}
	context := models.ProjectContext{
		SupplyChainVolatility: 2,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "*",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.1,
			MultiplierFormula: "linear",
		},
	}

	results := CalculateBatchDurations(tasks, defaultGSF, context, multipliers, models.Forecast{}, defaultCfg)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Both should have multiplier: * (1 + 2 * 0.1) = * 1.2
	// 5.0 * 1.2 = 6.0, 4.0 * 1.2 = 4.8 -> quantized to 5.0
	if results[0].CalculatedDuration != 6.0 {
		t.Errorf("expected 6.0 for task 0, got %v", results[0].CalculatedDuration)
	}
	if results[1].CalculatedDuration != 5.0 {
		t.Errorf("expected 5.0 for task 1, got %v", results[1].CalculatedDuration)
	}
}

// === COMBINED SAF + MULTIPLIER TEST ===

func TestCalculateTaskDuration_SAFWithMultiplier(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Floor Framing",
		BaseDurationDays: 5.0,
		IsInspection:     false,
	}
	context := models.ProjectContext{
		SupplyChainVolatility: 2,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "9.1",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.1,
			MultiplierFormula: "linear",
		},
	}

	// GSF = 4500 -> SAF = (4500/2250)^0.75 ~ 1.68
	// Expected: 5.0 * 1.68 * 1.2 = 10.08 -> quantized to 10.5
	result := CalculateTaskDuration(task, 4500.0, context, multipliers, models.Forecast{}, defaultCfg)

	if result != 10.5 {
		t.Errorf("expected 10.5 with SAF + multiplier, got %v", result)
	}
}

// === SWIM INTEGRATION TEST ===

func TestCalculateTaskDuration_WithWeather(t *testing.T) {
	// Task: Site Prep (Sensitive)
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "7.2",
		Name:             "Site Clearing",
		BaseDurationDays: 4.0,
		IsInspection:     false,
	}
	context := models.ProjectContext{} // No multipliers

	// Forecast: Heavy Rain (1.15 multiplier)
	forecast := models.Forecast{
		PrecipitationMM: 15.0,
	}

	// Calculation: 4.0 (Base) * 1.0 (SAF) * 1.15 (Weather) = 4.6
	// Quantization: Ceiling(4.6 * 2) / 2 = Ceiling(9.2) / 2 = 10 / 2 = 5.0
	result := CalculateTaskDuration(task, defaultGSF, context, nil, forecast, defaultCfg)

	expected := 5.0
	if result != expected {
		t.Errorf("expected %.1f with rain impact and quantization, got %v", expected, result)
	}

	// Forecast: Combined impact (Rain 1.15 * Freeze 1.25 = 1.4375)
	forecastCombined := models.Forecast{
		PrecipitationMM: 15.0,
		LowTempC:        -5.0,
	}

	// Calculation: 4.0 * 1.4375 = 5.75
	// Quantization: Ceiling(5.75 * 2) / 2 = Ceiling(11.5) / 2 = 6.0
	resultCombined := CalculateTaskDuration(task, defaultGSF, context, nil, forecastCombined, defaultCfg)

	expectedCombined := 6.0
	if resultCombined != expectedCombined {
		t.Errorf("expected %.1f with rain/freeze impact and quantization, got %v", expectedCombined, resultCombined)
	}
}

// =============================================================================
// RED TEST: DHSM Fixed-Point Migration - Prove Float64 Drift
// =============================================================================

// TestDHSM_FloatDrift_RedTest proves floating-point causes non-determinism in DHSM.
func TestDHSM_FloatDrift_RedTest(t *testing.T) {
	// Test 1: IEEE 754 non-associativity affects SAF calculations
	a := 5.123456789
	b := 1.6818 // SAF for 4500 GSF
	c := 1.2    // 20% multiplier

	result1 := (a * b) * c
	result2 := a * (b * c)

	if result1 != result2 {
		t.Logf("IEEE 754 NON-ASSOCIATIVITY DEMONSTRATED:")
		t.Logf("  ((a * b) * c) = %.20f", result1)
		t.Logf("  (a * (b * c)) = %.20f", result2)
		t.Logf("  Difference: %.20e", result1-result2)
	}

	// Test 2: Precision boundary at quantization edge
	edgeCases := []float64{
		4.9999999999999, // Should quantize to 5.0
		5.0000000000001, // Should quantize to 5.5
		4.75,            // Right on 0.25 boundary
	}

	for _, base := range edgeCases {
		saf := math.Pow(4500.0/2250.0, 0.75)
		computed := base * saf
		quantized := math.Ceil(computed*2) / 2

		t.Logf("Base=%.15f, SAF=%.15f, Computed=%.15f, Quantized=%.1f",
			base, saf, computed, quantized)
	}

	// Test 3: Accumulated error through multiple multiplications
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Test Task",
		BaseDurationDays: 5.123456789012345,
	}
	context := models.ProjectContext{
		SupplyChainVolatility:  3,
		RoughInspectionLatency: 2,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "9.1",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.123456789,
			MultiplierFormula: "linear",
		},
		{
			ID:                uuid.New(),
			WBSTaskCode:       "*",
			VariableKey:       "rough_inspection_latency",
			Weight:            0.098765432,
			MultiplierFormula: "linear",
		},
	}

	// Calculate 1000 times - should all be identical
	results := make(map[float64]int)
	for i := 0; i < 1000; i++ {
		result := CalculateTaskDuration(task, 4567.89, context, multipliers, models.Forecast{}, defaultCfg)
		results[result]++
	}

	if len(results) > 1 {
		t.Errorf("NON-DETERMINISM: Got %d different results from identical inputs!", len(results))
		for result, count := range results {
			t.Logf("  Result %.15f occurred %d times", result, count)
		}
	} else {
		t.Logf("Determinism check passed: single unique result")
	}

	t.Log("=== CURRENT IMPLEMENTATION STATUS ===")
	t.Log("CalculateTaskDuration returns float64 - violates L7 determinism requirement")
	t.Log("After migration, should return time.Duration (int64 nanoseconds)")
}

// TestDHSM_Determinism_Requirement documents the L7 determinism requirement.
func TestDHSM_Determinism_Requirement(t *testing.T) {
	t.Log("L7 QUALITY GATE: Determinism Requirement")
	t.Log("----------------------------------------")
	t.Log("1. Same input MUST produce identical output on x86 and ARM64")
	t.Log("2. SAF calculations must use fixed-point or lookup tables")
	t.Log("3. Duration must use time.Duration (int64 nanoseconds)")
	t.Log("4. No float64 in the critical path from input to output")
	t.Log("")
	t.Log("Current implementation uses float64 - this violates L7 standards")
}

// =============================================================================
// V2 DETERMINISTIC FUNCTION TESTS
// =============================================================================

func TestCalculateTaskDurationV2_Basic(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Floor Framing",
		BaseDurationDays: 5.0,
	}
	context := models.ProjectContext{}

	result := CalculateTaskDurationV2(task, defaultGSF, context, nil, models.Forecast{}, defaultCfg)

	// 5.0 days = 120 hours, quantized to half-day = 5 days = 120 hours
	expected := 120 * time.Hour
	if result != expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestCalculateTaskDurationV2_Determinism(t *testing.T) {
	task := models.WBSTask{
		ID:               uuid.New(),
		Code:             "9.1",
		Name:             "Test Task",
		BaseDurationDays: 5.123456789012345,
	}
	context := models.ProjectContext{
		SupplyChainVolatility:  3,
		RoughInspectionLatency: 2,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "9.1",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.123456789,
			MultiplierFormula: "linear",
		},
		{
			ID:                uuid.New(),
			WBSTaskCode:       "*",
			VariableKey:       "rough_inspection_latency",
			Weight:            0.098765432,
			MultiplierFormula: "linear",
		},
	}

	// Calculate 1000 times - MUST all be identical
	results := make(map[time.Duration]int)
	for i := 0; i < 1000; i++ {
		result := CalculateTaskDurationV2(task, 4567.89, context, multipliers, models.Forecast{}, defaultCfg)
		results[result]++
	}

	if len(results) != 1 {
		t.Errorf("DETERMINISM VIOLATION: Got %d different results from identical inputs!", len(results))
		for result, count := range results {
			t.Logf("  Result %v occurred %d times", result, count)
		}
	} else {
		t.Log("V2 Determinism verified: single unique result")
	}
}

func TestCalculateTaskDurationV2_Quantization(t *testing.T) {
	testCases := []struct {
		baseDays      float64
		expectedHours int
	}{
		{4.2, 104}, // 4.2 days = 100.8 hrs -> ceil to half-day -> 104 hrs (4.33 days)
		{5.0, 120}, // 5.0 days = 120 hrs -> exactly half-day aligned
		{5.1, 124}, // 5.1 days = 122.4 hrs -> ceil to 124 hrs
		{3.75, 92}, // 3.75 days = 90 hrs -> ceil to 92 hrs
		{1.0, 24},  // 1.0 day = 24 hrs -> aligned
		{0.3, 8},   // 0.3 days = 7.2 hrs -> ceil to 8 hrs (HalfDay)
	}

	for _, tc := range testCases {
		task := models.WBSTask{
			ID:               uuid.New(),
			Code:             "test",
			Name:             "Test Task",
			BaseDurationDays: tc.baseDays,
		}

		result := CalculateTaskDurationV2(task, defaultGSF, models.ProjectContext{}, nil, models.Forecast{}, defaultCfg)
		expected := time.Duration(tc.expectedHours) * time.Hour

		if result != expected {
			t.Errorf("baseDays=%.2f: expected %v, got %v", tc.baseDays, expected, result)
		}
	}
}

func TestCalculateSAFScaled(t *testing.T) {
	testCases := []struct {
		gsf      float64
		expected int64
	}{
		{2250.0, 1000}, // Baseline: SAF = 1.0 = 1000
		{4500.0, 1682}, // 2x baseline: SAF = 2^0.75 ~ 1.682 = 1682
		{1500.0, 738},  // Smaller: SAF = (1500/2250)^0.75 ~ 0.738 = 738
		{0, 1000},      // Zero GSF: default SAF = 1.0 = 1000
	}

	for _, tc := range testCases {
		result := CalculateSAFScaled(tc.gsf, defaultCfg)
		// Allow +/-1 for rounding differences
		if result < tc.expected-1 || result > tc.expected+1 {
			t.Errorf("gsf=%.0f: expected ~%d, got %d", tc.gsf, tc.expected, result)
		}
	}
}

func TestDurationToDays(t *testing.T) {
	testCases := []struct {
		duration time.Duration
		expected float64
	}{
		{24 * time.Hour, 1.0},
		{48 * time.Hour, 2.0},
		{12 * time.Hour, 0.5},
		{120 * time.Hour, 5.0},
	}

	for _, tc := range testCases {
		result := DurationToDays(tc.duration)
		if result != tc.expected {
			t.Errorf("duration=%v: expected %.2f days, got %.2f", tc.duration, tc.expected, result)
		}
	}
}

func TestDaysToDuration(t *testing.T) {
	testCases := []struct {
		days     float64
		expected time.Duration
	}{
		{1.0, 24 * time.Hour},
		{2.0, 48 * time.Hour},
		{5.0, 120 * time.Hour},
	}

	for _, tc := range testCases {
		result := DaysToDuration(tc.days)
		if result != tc.expected {
			t.Errorf("days=%.2f: expected %v, got %v", tc.days, tc.expected, result)
		}
	}
}

func TestCalculateBatchDurationsV2(t *testing.T) {
	tasks := []models.WBSTask{
		{ID: uuid.New(), Code: "9.1", Name: "Floor Framing", BaseDurationDays: 5.0},
		{ID: uuid.New(), Code: "9.2", Name: "Wall Framing", BaseDurationDays: 4.0},
	}
	context := models.ProjectContext{
		SupplyChainVolatility: 2,
	}
	multipliers := []models.DurationMultiplier{
		{
			ID:                uuid.New(),
			WBSTaskCode:       "*",
			VariableKey:       "supply_chain_volatility",
			Weight:            0.1,
			MultiplierFormula: "linear",
		},
	}

	results := CalculateBatchDurationsV2(tasks, defaultGSF, context, multipliers, models.Forecast{}, defaultCfg)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, r := range results {
		if r.CalculatedDuration <= 0 {
			t.Errorf("result[%d]: expected positive duration, got %v", i, r.CalculatedDuration)
		}
		t.Logf("Task %s: BaseDuration=%v, CalculatedDuration=%v",
			r.WBSCode, r.BaseDuration, r.CalculatedDuration)
	}
}
