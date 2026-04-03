package physics

import (
	"math"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/config"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/models/types"
)

// WorkDayPrecision is the granularity for task duration quantization (30 minutes).
const WorkDayPrecision = 30 * time.Minute

// HalfDay is the duration of half a working day (4 hours).
const HalfDay = 4 * time.Hour

// FullDay is the duration of a full working day (8 hours).
const FullDay = 8 * time.Hour

// SAFScaleFactor is the fixed-point scale for SAF calculations (3 decimal places).
const SAFScaleFactor = 1000

// CalculateSAF computes the Size Adjustment Factor.
// SAF = (GSF / StandardHouseSizeSF) ^ SizeAdjustmentExponent
func CalculateSAF(gsf float64, cfg config.PhysicsConfig) float64 {
	if gsf <= 0 {
		return 1.0
	}
	cfg = cfg.WithDefaults()
	return math.Pow(gsf/cfg.StandardHouseSizeSF, cfg.SizeAdjustmentExponent)
}

// CalculateSAFScaled computes SAF using scaled integer representation.
// Returns SAF * SAFScaleFactor as int64 for deterministic calculations.
func CalculateSAFScaled(gsf float64, cfg config.PhysicsConfig) int64 {
	saf := CalculateSAF(gsf, cfg)
	return int64(math.Round(saf * SAFScaleFactor))
}

// CalculateTaskDuration computes the DHSM-adjusted task duration.
// DEPRECATED: Use CalculateTaskDurationV2 for deterministic time.Duration output.
func CalculateTaskDuration(
	task models.WBSTask,
	gsf float64,
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast types.Forecast,
	cfg config.PhysicsConfig,
) float64 {
	baseDuration := task.BaseDurationDays

	saf := 1.0
	if !task.IsInspection {
		saf = CalculateSAF(gsf, cfg)
	}
	baseDuration *= saf

	for _, mult := range multipliers {
		if mult.WBSTaskCode != "*" && mult.WBSTaskCode != task.Code {
			continue
		}

		variableValue := getContextVariable(context, mult.VariableKey)
		if variableValue == 0 {
			continue
		}

		adjustment := applyMultiplierFormula(variableValue, mult.Weight, mult.MultiplierFormula)
		baseDuration *= adjustment
	}

	tempTask := models.ProjectTask{
		WBSCode:            task.Code,
		CalculatedDuration: baseDuration,
	}
	baseDuration = ApplyWeatherAdjustment(tempTask, forecast)

	return math.Ceil(baseDuration*2) / 2
}

// CalculateTaskDurationV2 computes the DHSM-adjusted task duration as time.Duration.
// Uses int64 nanoseconds internally to eliminate IEEE 754 drift.
func CalculateTaskDurationV2(
	task models.WBSTask,
	gsf float64,
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast types.Forecast,
	cfg config.PhysicsConfig,
) time.Duration {
	baseNanos := int64(task.BaseDurationDays * float64(24*time.Hour))

	safScaled := int64(SAFScaleFactor)
	if !task.IsInspection {
		safScaled = CalculateSAFScaled(gsf, cfg)
	}

	baseNanos = (baseNanos * safScaled) / SAFScaleFactor

	for _, mult := range multipliers {
		if mult.WBSTaskCode != "*" && mult.WBSTaskCode != task.Code {
			continue
		}

		variableValue := getContextVariable(context, mult.VariableKey)
		if variableValue == 0 {
			continue
		}

		adjustment := applyMultiplierFormula(variableValue, mult.Weight, mult.MultiplierFormula)
		adjustmentScaled := int64(math.Round(adjustment * SAFScaleFactor))

		baseNanos = (baseNanos * adjustmentScaled) / SAFScaleFactor
	}

	tempTask := models.ProjectTask{
		WBSCode:            task.Code,
		CalculatedDuration: float64(baseNanos) / float64(24*time.Hour),
	}
	adjustedDays := ApplyWeatherAdjustment(tempTask, forecast)
	baseNanos = int64(adjustedDays * float64(24*time.Hour))

	halfDayNanos := int64(HalfDay)
	remainder := baseNanos % halfDayNanos
	if remainder > 0 {
		baseNanos = baseNanos + (halfDayNanos - remainder)
	}

	return time.Duration(baseNanos)
}

// DurationToDays converts a time.Duration to float64 days.
func DurationToDays(d time.Duration) float64 {
	return float64(d) / float64(24*time.Hour)
}

// DaysToDuration converts float64 days to time.Duration.
func DaysToDuration(days float64) time.Duration {
	nanos := int64(days * float64(24*time.Hour))
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

// applyMultiplierFormula applies the multiplier formula to compute adjustment.
func applyMultiplierFormula(value, weight float64, formula string) float64 {
	switch formula {
	case "linear":
		return 1 + (value * weight)
	case "scaled":
		baseline := 2000.0
		scale := 10000.0
		return 1 + (value-baseline)/scale*weight
	case "step":
		return 1 + (value-1)*weight
	default:
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
type DHSMResultV2 struct {
	WBSCode            string        `json:"wbs_code"`
	BaseDuration       time.Duration `json:"base_duration"`
	CalculatedDuration time.Duration `json:"calculated_duration"`
}

// CalculateBatchDurations computes DHSM durations for multiple tasks.
// DEPRECATED: Use CalculateBatchDurationsV2 for deterministic output.
func CalculateBatchDurations(
	tasks []models.WBSTask,
	gsf float64,
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast types.Forecast,
	cfg config.PhysicsConfig,
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
func CalculateBatchDurationsV2(
	tasks []models.WBSTask,
	gsf float64,
	context models.ProjectContext,
	multipliers []models.DurationMultiplier,
	forecast types.Forecast,
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
