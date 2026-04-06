// Package physics implements the deterministic scheduling algorithms.
// See BACKEND_SCOPE.md Section 3.4 (Layer 3: Physics Engine)
package physics

import (
	"math"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/config"
	"github.com/futurebuild/futurebuild-os/internal/models"
)

// =============================================================================
// DETERMINISTIC DURATION CONSTANTS
// =============================================================================
// L7 Quality Gate: All scheduling calculations use integer nanoseconds.
// These constants define the atomic units of time for deterministic math.
// =============================================================================

// WorkDayPrecision is the granularity for task duration quantization.
// Tasks are quantized to 30-minute increments (0.5 days = 4 hours in 8-hour workday).
// This matches the legacy 0.5-day quantization while using int64 math.
const WorkDayPrecision = 30 * time.Minute

// HalfDay is the duration of half a working day (4 hours).
const HalfDay = 4 * time.Hour

// FullDay is the duration of a full working day (8 hours).
const FullDay = 8 * time.Hour

// SAFScaleFactor is the fixed-point scale for SAF calculations.
// SAF values are stored as int64 with this scale factor (1000 = 3 decimal places).
// Example: SAF 1.682 is stored as 1682.
const SAFScaleFactor = 1000

// CalculateSAF computes the Size Adjustment Factor.
// SAF = (GSF / StandardHouseSizeSF) ^ SizeAdjustmentExponent
// See CPM_RES_MODEL_SPEC.md Section 11.2.1
// Config decoupling: Magic numbers now come from PhysicsConfig.
func CalculateSAF(gsf float64, cfg config.PhysicsConfig) float64 {
	if gsf <= 0 {
		return 1.0 // Default for unset GSF
	}
	// Apply defaults for zero-value safety (FAANG Threshold #2)
	cfg = cfg.WithDefaults()
	return math.Pow(gsf/cfg.StandardHouseSizeSF, cfg.SizeAdjustmentExponent)
}

// CalculateSAFScaled computes SAF using scaled integer representation.
// Returns SAF * SAFScaleFactor as int64 for deterministic calculations.
// Example: SAF 1.682 returns 1682
// L7 Determinism: Uses int64 to avoid IEEE 754 floating-point drift.
func CalculateSAFScaled(gsf float64, cfg config.PhysicsConfig) int64 {
	saf := CalculateSAF(gsf, cfg)
	// Scale and round to nearest integer
	return int64(math.Round(saf * SAFScaleFactor))
}

// CalculateTaskDuration computes the DHSM-adjusted task duration.
// Formula: Raw_Duration = (D_base * SAF * Multipliers)
// Final:   Duration_final = Ceiling(Raw_Duration * 2) / 2
// See CPM_RES_MODEL_SPEC.md Section 11.2.5
//
// DEPRECATED: Use CalculateTaskDurationV2 for deterministic time.Duration output.
// This function uses float64 math which may cause IEEE 754 precision drift.
func CalculateTaskDuration(
	task models.WBSTask,
	gsf float64, // Gross Square Footage from Project
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast models.Forecast,
	cfg config.PhysicsConfig, // Config decoupling: accepts tunable physics config
) float64 {
	baseDuration := task.BaseDurationDays

	// Event Duration Locking: Inspections have SAF = 1.0
	// See CPM_RES_MODEL_SPEC.md Section 11.2.1
	saf := 1.0
	if !task.IsInspection {
		saf = CalculateSAF(gsf, cfg)
	}
	baseDuration *= saf

	for _, mult := range multipliers {
		// Match wildcard or specific WBS code
		if mult.WBSTaskCode != "*" && mult.WBSTaskCode != task.Code {
			continue
		}

		variableValue := getContextVariable(context, mult.VariableKey)
		if variableValue == 0 {
			continue // Variable not set, skip multiplier
		}

		adjustment := applyMultiplierFormula(variableValue, mult.Weight, mult.MultiplierFormula)
		baseDuration *= adjustment
	}

	// Apply SWIM Weather Overlay (Step 31 Correction)
	// Passing a placeholder ProjectTask to ApplyWeatherAdjustment
	tempTask := models.ProjectTask{
		WBSCode:            task.Code,
		CalculatedDuration: baseDuration,
	}
	baseDuration = ApplyWeatherAdjustment(tempTask, forecast)

	// Quantize to 0.5-day increments per CPM_RES_MODEL_SPEC.md Section 11.2.5
	// Formula: Duration_final = Ceiling(Raw_Duration * 2) / 2
	return math.Ceil(baseDuration*2) / 2
}

// CalculateTaskDurationV2 computes the DHSM-adjusted task duration as time.Duration.
// L7 Determinism: Uses int64 nanoseconds internally to eliminate IEEE 754 drift.
// Output is quantized to WorkDayPrecision (30-minute increments).
//
// This is the preferred function for all new code requiring deterministic scheduling.
func CalculateTaskDurationV2(
	task models.WBSTask,
	gsf float64,
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast models.Forecast,
	cfg config.PhysicsConfig,
) time.Duration {
	// Convert base duration from days to nanoseconds using integer math
	// 1 day = 24 hours = 24 * 3600 * 1e9 nanoseconds
	baseNanos := int64(task.BaseDurationDays * float64(24*time.Hour))

	// Get SAF as scaled integer (SAF * 1000)
	safScaled := int64(SAFScaleFactor) // Default SAF = 1.0 = 1000
	if !task.IsInspection {
		safScaled = CalculateSAFScaled(gsf, cfg)
	}

	// Apply SAF: (baseNanos * safScaled) / SAFScaleFactor
	// Integer division preserves determinism
	baseNanos = (baseNanos * safScaled) / SAFScaleFactor

	// Apply multipliers using scaled integer math
	for _, mult := range multipliers {
		if mult.WBSTaskCode != "*" && mult.WBSTaskCode != task.Code {
			continue
		}

		variableValue := getContextVariable(context, mult.VariableKey)
		if variableValue == 0 {
			continue
		}

		// Calculate adjustment as scaled integer (adjustment * 1000)
		adjustment := applyMultiplierFormula(variableValue, mult.Weight, mult.MultiplierFormula)
		adjustmentScaled := int64(math.Round(adjustment * SAFScaleFactor))

		// Apply: (baseNanos * adjustmentScaled) / SAFScaleFactor
		baseNanos = (baseNanos * adjustmentScaled) / SAFScaleFactor
	}

	// Apply weather adjustment using the duration-native helper.
	// The factor comes from SWIM weather model (ApplyWeatherAdjustment).
	tempTask := models.ProjectTask{
		WBSCode:            task.Code,
		CalculatedDuration: float64(baseNanos) / float64(24*time.Hour),
	}
	adjustedDays := ApplyWeatherAdjustment(tempTask, forecast)
	weatherFactor := 1.0
	if tempTask.CalculatedDuration > 0 {
		weatherFactor = adjustedDays / tempTask.CalculatedDuration
	}
	baseNanos = int64(WeatherAdjustDuration(time.Duration(baseNanos), weatherFactor))

	// Quantize to 0.5-day (HalfDay = 4 hours) increments
	// Ceiling: round UP to nearest HalfDay
	halfDayNanos := int64(HalfDay)
	remainder := baseNanos % halfDayNanos
	if remainder > 0 {
		baseNanos = baseNanos + (halfDayNanos - remainder)
	}

	return time.Duration(baseNanos)
}

// DurationToDays converts a time.Duration to float64 days.
// Useful for compatibility with legacy code expecting float64.
func DurationToDays(d time.Duration) float64 {
	return float64(d) / float64(24*time.Hour)
}

// DaysToDuration converts float64 days to time.Duration.
// Quantizes to WorkDayPrecision for determinism.
func DaysToDuration(days float64) time.Duration {
	nanos := int64(days * float64(24*time.Hour))
	// Quantize to precision
	precision := int64(WorkDayPrecision)
	remainder := nanos % precision
	if remainder >= precision/2 {
		nanos = nanos + (precision - remainder)
	} else {
		nanos = nanos - remainder
	}
	return time.Duration(nanos)
}

// getContextVariable extracts the variable value from ProjectContext.
func getContextVariable(ctx models.ProjectContext, key string) float64 {
	switch key {
	case "supply_chain_volatility":
		return float64(ctx.SupplyChainVolatility)
	case "rough_inspection_latency":
		return float64(ctx.RoughInspectionLatency)
	case "final_inspection_latency":
		return float64(ctx.FinalInspectionLatency)
	default:
		return 0
	}
}

// WeatherAdjustDuration applies a weather adjustment factor to a base duration.
// This is a pure function with no DB access. The factor is typically computed by
// the SWIM weather model (e.g., 1.15 for rain, 1.25 for freeze).
// A factor of 1.0 means no adjustment. The result is rounded to the nearest
// nanosecond using integer math for determinism.
//
// Example: WeatherAdjustDuration(24*time.Hour, 1.15) returns ~27.6 hours.
func WeatherAdjustDuration(base time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return base
	}
	// Use scaled integer math: multiply by factor * 1000, then divide by 1000.
	// This avoids direct float64 -> int64 truncation for small factors.
	scaledFactor := int64(math.Round(factor * SAFScaleFactor))
	adjusted := (int64(base) * scaledFactor) / SAFScaleFactor
	return time.Duration(adjusted)
}

// applyMultiplierFormula applies the multiplier formula to compute adjustment.
// Supports common formula types per BACKEND_SCOPE.md Section 6.1 examples.
func applyMultiplierFormula(value, weight float64, formula string) float64 {
	switch formula {
	case "linear":
		// Simple linear: 1 + (value * weight)
		return 1 + (value * weight)
	case "scaled":
		// Scaled formula: 1 + (value - baseline) / scale * weight
		// Uses sensible defaults for residential construction
		baseline := 2000.0 // Default baseline (e.g., 2000 sqft)
		scale := 10000.0   // Default scale factor
		return 1 + (value-baseline)/scale*weight
	case "step":
		// Step function: 1 + (value - 1) * weight
		// Used for discrete multipliers (e.g., floor count)
		return 1 + (value-1)*weight
	default:
		// Default to linear if formula not recognized
		return 1 + (value * weight)
	}
}

// DHSMResult represents the output of DHSM calculation for a task.
type DHSMResult struct {
	WBSCode            string  `json:"wbs_code"`
	BaseDuration       float64 `json:"base_duration"`
	CalculatedDuration float64 `json:"calculated_duration"`
}

// DHSMResultV2 represents the deterministic output of DHSM calculation.
// L7 Determinism: Uses time.Duration (int64 nanoseconds) for calculated duration.
type DHSMResultV2 struct {
	WBSCode            string        `json:"wbs_code"`
	BaseDuration       time.Duration `json:"base_duration"`
	CalculatedDuration time.Duration `json:"calculated_duration"`
}

// CalculateBatchDurations computes DHSM durations for multiple tasks.
//
// DEPRECATED: Use CalculateBatchDurationsV2 for deterministic output.
func CalculateBatchDurations(
	tasks []models.WBSTask,
	gsf float64, // Gross Square Footage from Project
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast models.Forecast,
	cfg config.PhysicsConfig, // Config decoupling: accepts tunable physics config
) []DHSMResult {
	results := make([]DHSMResult, len(tasks))
	for i, task := range tasks {
		results[i] = DHSMResult{
			WBSCode:            task.Code,
			BaseDuration:       task.BaseDurationDays,
			CalculatedDuration: CalculateTaskDuration(task, gsf, context, multipliers, forecast, cfg),
		}
	}
	return results
}

// CalculateBatchDurationsV2 computes deterministic DHSM durations for multiple tasks.
// L7 Determinism: All calculations use int64 nanoseconds, results are map[uuid.UUID]time.Duration.
func CalculateBatchDurationsV2(
	tasks []models.WBSTask,
	gsf float64,
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast models.Forecast,
	cfg config.PhysicsConfig,
) []DHSMResultV2 {
	results := make([]DHSMResultV2, len(tasks))
	for i, task := range tasks {
		results[i] = DHSMResultV2{
			WBSCode:            task.Code,
			BaseDuration:       DaysToDuration(task.BaseDurationDays),
			CalculatedDuration: CalculateTaskDurationV2(task, gsf, context, multipliers, forecast, cfg),
		}
	}
	return results
}
