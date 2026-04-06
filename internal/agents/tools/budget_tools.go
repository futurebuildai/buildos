package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
)

// RegisterBudgetTools registers budget and cost-related tools.
// Uses stub implementations until BudgetService is fully wired.
func RegisterBudgetTools(r *Registry) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_budget_summary",
			Description: "Get a financial summary of the project including total budget, amount spent, remaining budget, and per-category breakdown. All amounts in cents (BIGINT).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			// TODO: wire to real BudgetService.GetFinancialSummary
			return fmt.Sprintf(`{"project_id":"%s","currency_code":"USD","total_estimated_cents":0,"total_actual_cents":0,"variance_cents":0,"message":"Budget summary (stub)"}`,
				scope.ProjectID), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "estimate_cost_impact",
			Description: "Estimate the cost impact of a project change (e.g., adding square footage, changing scope). Returns estimated cost delta and budget impact. All amounts in cents (BIGINT).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"description":{"type":"string","description":"Description of the change"},"square_footage_delta":{"type":"number","description":"Change in square footage (positive = addition, negative = reduction)"},"wbs_categories":{"type":"array","items":{"type":"string"},"description":"Affected WBS phase codes"}},"required":["description","square_footage_delta"]}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var params struct {
				Description   string   `json:"description"`
				SqftDelta     float64  `json:"square_footage_delta"`
				WBSCategories []string `json:"wbs_categories"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			// TODO: wire to real cost estimation logic
			// Rough estimate: ~$150/sqft average for residential construction
			estimatedCents := int64(params.SqftDelta * 15000) // $150.00 per sqft
			return fmt.Sprintf(`{"description":"%s","estimated_cost_cents":%d,"currency_code":"USD","message":"Cost estimate (stub)"}`,
				params.Description, estimatedCents), nil
		},
	})
}

// formatCents formats cents as a dollar string (e.g., 123456 -> "1,234.56").
func formatCents(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	dollars := cents / 100
	remainder := cents % 100

	// Format with comma separators
	s := fmt.Sprintf("%d", dollars)
	if len(s) > 3 {
		var result []byte
		for i, c := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				result = append(result, ',')
			}
			result = append(result, byte(c))
		}
		s = string(result)
	}

	if negative {
		return fmt.Sprintf("-%s.%02d", s, remainder)
	}
	return fmt.Sprintf("%s.%02d", s, remainder)
}
