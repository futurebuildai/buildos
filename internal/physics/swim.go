package physics

import (
	"strconv"
	"strings"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ApplyWeatherAdjustment applies the SWIM weather model to a task's duration.
// Weather sensitivity scope: WBS < 10.0 (pre-dry-in) OR WBS 13.x (exterior finishes).
// Internal work (WBS >= 10.0 AND WBS != 13.x) bypasses weather adjustments.
// Multipliers (BACKEND_SCOPE.md Section 6.2):
// - Precipitation > 10mm: * 1.15
// - Low Temp < 0 C: * 1.25 (frozen ground delays)
// - High Temp > 35 C: * 1.10 (heat restrictions)
// See BACKEND_SCOPE.md Section 6.2 and CPM_RES_MODEL_SPEC.md Section 19.2.
func ApplyWeatherAdjustment(task models.ProjectTask, forecast models.Forecast) float64 {
	// See CPM_RES_MODEL_SPEC.md Section 19.2: Weather Sensitivity Rule (SWIM)
	if !isWeatherSensitive(task.WBSCode) {
		return task.CalculatedDuration
	}

	multiplier := 1.0

	// See BACKEND_SCOPE.md Section 6.2 multipliers
	if forecast.PrecipitationMM > 10.0 {
		multiplier *= 1.15
	}

	if forecast.LowTempC < 0.0 {
		multiplier *= 1.25
	}

	if forecast.HighTempC > 35.0 {
		multiplier *= 1.10
	}

	return task.CalculatedDuration * multiplier
}

// isWeatherSensitive determines if a task is subject to SWIM weather adjustments.
// Scope: WBS < 10.0 OR WBS 13.x
func isWeatherSensitive(wbs string) bool {
	parts := strings.Split(wbs, ".")
	if len(parts) == 0 {
		return false
	}

	major, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return false
	}

	// WBS < 10.0 is pre-dry-in
	if major < 10.0 {
		return true
	}

	// WBS 13.x is exterior finishes
	if major == 13.0 {
		return true
	}

	return false
}
