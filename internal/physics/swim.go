package physics

import (
	"strconv"
	"strings"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/models/types"
)

// ApplyWeatherAdjustment applies the SWIM weather model to a task's duration.
// Weather sensitivity scope: WBS < 10.0 (pre-dry-in) OR WBS 13.x (exterior finishes).
// Multipliers:
// - Precipitation > 10mm: * 1.15
// - Low Temp < 0 C: * 1.25 (frozen ground delays)
// - High Temp > 35 C: * 1.10 (heat restrictions)
func ApplyWeatherAdjustment(task models.ProjectTask, forecast types.Forecast) float64 {
	if !isWeatherSensitive(task.WBSCode) {
		return task.CalculatedDuration
	}

	multiplier := 1.0

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
func isWeatherSensitive(wbs string) bool {
	parts := strings.Split(wbs, ".")
	if len(parts) == 0 {
		return false
	}

	major, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return false
	}

	if major < 10.0 {
		return true
	}

	if major == 13.0 {
		return true
	}

	return false
}
