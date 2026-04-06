package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/futurebuild/futurebuild-os/internal/service"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/jackc/pgx/v5/pgxpool"
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
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var params struct {
				Description   string   `json:"description"`
				SqftDelta     float64  `json:"square_footage_delta"`
				WBSCategories []string `json:"wbs_categories"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			// Rough estimate: ~$150/sqft average for residential construction
			estimatedCents := int64(params.SqftDelta * 15000) // $150.00 per sqft
			return fmt.Sprintf(`{"description":"%s","estimated_cost_cents":%d,"currency_code":"USD","message":"Cost estimate (stub)"}`,
				params.Description, estimatedCents), nil
		},
	})
}

// budgetPhaseRow holds per-WBS budget data for the budget summary.
type budgetPhaseRow struct {
	WBSCode               string `json:"wbs_code"`
	PhaseName             string `json:"phase_name"`
	EstimatedCostCents    int64  `json:"estimated_cost_cents"`
	CommittedCostCents    int64  `json:"committed_cost_cents"`
	ActualCostCents       int64  `json:"actual_cost_cents"`
	VarianceCents         int64  `json:"variance_cents"`
	CurrencyCode          string `json:"currency_code"`
}

// RegisterBudgetToolsWithService registers budget tools wired to BudgetService and pool.
func RegisterBudgetToolsWithService(r *Registry, budgetSvc *service.BudgetService, pool *pgxpool.Pool) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_budget_summary",
			Description: "Get a financial summary of the project including total budget, amount spent, remaining budget, and per-category breakdown. All amounts in cents (BIGINT).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"currency_code":{"type":"string","description":"Currency code filter (default USD). Only USD and CAD supported."}}}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)

			var params struct {
				CurrencyCode string `json:"currency_code"`
			}
			if input != nil {
				if err := json.Unmarshal(input, &params); err != nil {
					slog.Warn("failed to parse budget summary input", "error", err)
				}
			}
			if params.CurrencyCode == "" {
				params.CurrencyCode = "USD"
			}

			budgets, err := budgetSvc.ListBudgets(ctx, scope.ProjectID, params.CurrencyCode)
			if err != nil {
				return "", fmt.Errorf("list budgets: %w", err)
			}

			// Aggregate totals and build per-phase breakdown
			var totalEstimated, totalCommitted, totalActual int64
			phases := make([]budgetPhaseRow, 0, len(budgets))
			for _, b := range budgets {
				totalEstimated += b.EstimatedCostCents
				totalCommitted += b.CommittedCostCents
				totalActual += b.ActualCostCents
				phases = append(phases, budgetPhaseRow{
					WBSCode:            b.WBSCode,
					PhaseName:          b.PhaseName,
					EstimatedCostCents: b.EstimatedCostCents,
					CommittedCostCents: b.CommittedCostCents,
					ActualCostCents:    b.ActualCostCents,
					VarianceCents:      b.EstimatedCostCents - b.ActualCostCents,
					CurrencyCode:       b.EstimatedCostCurrencyCode,
				})
			}

			// Also count pending invoices
			var pendingInvoiceCount int
			var pendingInvoiceCents int64
			if err := pool.QueryRow(ctx, `
				SELECT COUNT(*), COALESCE(SUM(amount_cents), 0)
				FROM invoices
				WHERE project_id = $1 AND currency_code = $2 AND status = 'pending'`,
				scope.ProjectID, params.CurrencyCode,
			).Scan(&pendingInvoiceCount, &pendingInvoiceCents); err != nil {
				slog.Warn("failed to query pending invoices for budget summary", "error", err, "project_id", scope.ProjectID)
			}

			result := map[string]interface{}{
				"project_id":              scope.ProjectID,
				"currency_code":           params.CurrencyCode,
				"total_estimated_cents":   totalEstimated,
				"total_committed_cents":   totalCommitted,
				"total_actual_cents":      totalActual,
				"variance_cents":          totalEstimated - totalActual,
				"formatted_estimated":     formatCents(totalEstimated),
				"formatted_actual":        formatCents(totalActual),
				"formatted_variance":      formatCents(totalEstimated - totalActual),
				"phases":                  phases,
				"phase_count":             len(phases),
				"pending_invoice_count":   pendingInvoiceCount,
				"pending_invoice_cents":   pendingInvoiceCents,
			}

			b, _ := json.Marshal(result)
			return string(b), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "estimate_cost_impact",
			Description: "Estimate the cost impact of a project change (e.g., adding square footage, changing scope). Returns estimated cost delta and budget impact. All amounts in cents (BIGINT).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"description":{"type":"string","description":"Description of the change"},"square_footage_delta":{"type":"number","description":"Change in square footage (positive = addition, negative = reduction)"},"wbs_categories":{"type":"array","items":{"type":"string"},"description":"Affected WBS phase codes"}},"required":["description","square_footage_delta"]}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			var params struct {
				Description   string   `json:"description"`
				SqftDelta     float64  `json:"square_footage_delta"`
				WBSCategories []string `json:"wbs_categories"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			// Query historical average cost per sqft from existing project budgets
			// within the same org. Uses prospect GSF data linked via project_id.
			// Falls back to $150/sqft default if no historical data available.
			var avgCostPerSqftCents int64
			err := pool.QueryRow(ctx, `
				SELECT COALESCE(
					(SELECT ROUND(AVG(b.actual_cost_cents::float / NULLIF(pr.gsf, 0)))
					 FROM project_budgets b
					 JOIN projects p ON p.id = b.project_id
					 JOIN pre_construction_prospects pr ON pr.project_id = p.id
					 WHERE p.org_id = $1 AND pr.gsf > 0 AND b.actual_cost_cents > 0
					 AND b.actual_cost_currency_code = 'USD'),
					15000
				)`,
				scope.OrgID,
			).Scan(&avgCostPerSqftCents)
			if err != nil {
				// Fallback to default cost per sqft
				avgCostPerSqftCents = 15000 // $150.00
			}

			// Use math.Round to prevent truncation bias in float→int conversion.
		// SqftDelta is inherently float (square footage), so float intermediate is acceptable
		// for estimation context. Ledger transactions must use pure integer math.
		estimatedCents := int64(math.Round(params.SqftDelta * float64(avgCostPerSqftCents)))

			// If specific WBS categories provided, try to get category-specific averages
			var categoryBreakdown []map[string]interface{}
			if len(params.WBSCategories) > 0 {
				for _, wbs := range params.WBSCategories {
					var catAvgCents int64
					if err := pool.QueryRow(ctx, `
						SELECT COALESCE(AVG(actual_cost_cents), 0)
						FROM project_budgets
						WHERE project_id = $1 AND wbs_code = $2`,
						scope.ProjectID, wbs,
					).Scan(&catAvgCents); err != nil {
						slog.Warn("failed to query category average for cost estimate", "error", err, "wbs_code", wbs)
					}

					categoryBreakdown = append(categoryBreakdown, map[string]interface{}{
						"wbs_code":                 wbs,
						"historical_avg_cost_cents": catAvgCents,
						"currency_code":            "USD",
					})
				}
			}

			result := map[string]interface{}{
				"description":                params.Description,
				"square_footage_delta":        params.SqftDelta,
				"estimated_cost_cents":        estimatedCents,
				"currency_code":               "USD",
				"avg_cost_per_sqft_cents":     avgCostPerSqftCents,
				"formatted_estimated":         formatCents(estimatedCents),
				"formatted_avg_per_sqft":      formatCents(avgCostPerSqftCents),
				"methodology":                 "historical_average",
			}

			if len(categoryBreakdown) > 0 {
				result["category_breakdown"] = categoryBreakdown
			}

			b, _ := json.Marshal(result)
			return string(b), nil
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
