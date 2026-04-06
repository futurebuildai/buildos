package physics

import (
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

func TestApplyWeatherAdjustment(t *testing.T) {
	tests := []struct {
		name             string
		taskWBS          string
		baseDuration     float64
		forecast         models.Forecast
		expectedDuration float64
	}{
		{
			name:         "Pre-dry-in: No weather impact",
			taskWBS:      "8.0",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 0.0,
				LowTempC:        20.0,
				HighTempC:       25.0,
			},
			expectedDuration: 10.0,
		},
		{
			name:         "Pre-dry-in: Heavy rainimpact (*1.15)",
			taskWBS:      "8.0",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 15.0,
				LowTempC:        10.0,
				HighTempC:       20.0,
			},
			expectedDuration: 11.5,
		},
		{
			name:         "Pre-dry-in: Freezing impact (*1.25)",
			taskWBS:      "8.1",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 0.0,
				LowTempC:        -5.0,
				HighTempC:       5.0,
			},
			expectedDuration: 12.5,
		},
		{
			name:         "Pre-dry-in: Extreme heat impact (*1.10)",
			taskWBS:      "7.4",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 0.0,
				LowTempC:        25.0,
				HighTempC:       38.0,
			},
			expectedDuration: 11.0,
		},
		{
			name:         "Pre-dry-in: Combined impact (Rain + Freeze: 1.15 * 1.25 = 1.4375 -> *10 = 14.38)",
			taskWBS:      "8.2",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 15.0,
				LowTempC:        -5.0,
				HighTempC:       5.0,
			},
			expectedDuration: 14.375,
		},
		{
			name:         "Interior work: No weather impact (WBS 10.1)",
			taskWBS:      "10.1",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 15.0,
				LowTempC:        -5.0,
				HighTempC:       38.0,
			},
			expectedDuration: 10.0,
		},
		{
			name:         "Exterior finishes: Weather impact (WBS 13.2)",
			taskWBS:      "13.2",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 15.0,
				LowTempC:        10.0,
				HighTempC:       20.0,
			},
			expectedDuration: 11.5,
		},
		{
			name:         "Edge case: WBS 10.0 (Bypass)",
			taskWBS:      "10.0",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 15.0,
			},
			expectedDuration: 10.0,
		},
		{
			name:         "Edge case: WBS 9.9 (Sensitive)",
			taskWBS:      "9.9",
			baseDuration: 10.0,
			forecast: models.Forecast{
				PrecipitationMM: 15.0,
			},
			expectedDuration: 11.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := models.ProjectTask{
				WBSCode:            tt.taskWBS,
				CalculatedDuration: tt.baseDuration,
			}
			got := ApplyWeatherAdjustment(task, tt.forecast)
			if got != tt.expectedDuration {
				t.Errorf("ApplyWeatherAdjustment() = %v, want %v", got, tt.expectedDuration)
			}
		})
	}
}
