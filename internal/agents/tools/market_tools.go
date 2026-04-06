package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
)

// RegisterMarketTools registers market conditions and seasonal cost tools.
// Provides static seasonal data until a real market data integration is built.
func RegisterMarketTools(r *Registry) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_market_conditions",
			Description: "Get seasonal construction cost forecast and labor availability for a region and start date. Shows optimal start windows and material cost trends.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"region":{"type":"string","description":"Regional cost region (e.g., 'TX-Austin', 'CA-Bay Area')"},"start_date":{"type":"string","description":"Project start date in YYYY-MM-DD format"}},"required":["start_date"]}`),
		},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var params struct {
				Region    string `json:"region"`
				StartDate string `json:"start_date"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			startDate, err := time.Parse("2006-01-02", params.StartDate)
			if err != nil {
				return "", fmt.Errorf("invalid start_date: %w", err)
			}

			// Simple seasonal cost factor model (peak in summer, low in winter)
			month := int(startDate.Month())
			seasonalFactor := monthlySeasonalFactor(month)

			result := map[string]interface{}{
				"start_month":          startDate.Month().String(),
				"seasonal_cost_factor": fmt.Sprintf("%.3f", seasonalFactor),
				"labor_availability": map[string]string{
					"general":     laborAvailability(month),
					"electrician": laborAvailability(month),
					"plumber":     laborAvailability(month),
					"framer":      laborAvailability(month),
				},
				"optimal_start_months": []string{"January", "February", "November", "December"},
				"message":              "Market conditions (stub — real data integration pending)",
			}

			if params.Region != "" {
				result["region"] = params.Region
			}

			b, _ := json.Marshal(result)
			return string(b), nil
		},
	})
}

// monthlySeasonalFactor returns a simple seasonal cost multiplier.
// 1.0 = baseline (winter), peaks in summer.
func monthlySeasonalFactor(month int) float64 {
	factors := map[int]float64{
		1: 0.95, 2: 0.95, 3: 1.00, 4: 1.05,
		5: 1.10, 6: 1.15, 7: 1.15, 8: 1.10,
		9: 1.05, 10: 1.00, 11: 0.95, 12: 0.95,
	}
	if f, ok := factors[month]; ok {
		return f
	}
	return 1.0
}

// laborAvailability returns a qualitative labor availability indicator.
func laborAvailability(month int) string {
	switch {
	case month >= 5 && month <= 8:
		return "tight"
	case month >= 11 || month <= 2:
		return "available"
	default:
		return "moderate"
	}
}
