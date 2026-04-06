package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterProjectTools registers project and procurement lookup tools.
// Uses stub implementations until ProjectService and WeatherService are fully wired.
func RegisterProjectTools(r *Registry) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_project_details",
			Description: "Get project details including name, address, status, budget, and key dates.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			return fmt.Sprintf(`{"project_id":"%s","org_id":"%s","message":"Project details (stub)"}`,
				scope.ProjectID, scope.OrgID), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "list_tasks",
			Description: "List all tasks for the current project with their status, dates, and critical path information.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"status_filter":{"type":"string","description":"Optional filter: Pending, Ready, In_Progress, Completed, Blocked, Delayed"}}}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			var params struct {
				StatusFilter string `json:"status_filter"`
			}
			if input != nil {
				if err := json.Unmarshal(input, &params); err != nil {
					slog.Warn("failed to parse list_tasks input", "error", err)
				}
			}
			return fmt.Sprintf(`{"project_id":"%s","tasks":[],"total":0,"filter":"%s","message":"Task list (stub)"}`,
				scope.ProjectID, params.StatusFilter), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "list_procurement_items",
			Description: "List all procurement (long-lead) items for the project with their alert status, lead times, and calculated order dates. Useful for understanding supply chain risks.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			return fmt.Sprintf(`{"project_id":"%s","items":[],"total":0,"message":"Procurement items (stub)"}`,
				scope.ProjectID), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_weather_forecast",
			Description: "Get the weather forecast for the project location. Returns high/low temp, precipitation probability, and conditions. Critical for scheduling exterior work.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"latitude":{"type":"number","description":"Latitude of the project site"},"longitude":{"type":"number","description":"Longitude of the project site"}},"required":["latitude","longitude"]}`),
		},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var params struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			// TODO: wire to real WeatherService.GetForecast
			return fmt.Sprintf(`{"latitude":%.4f,"longitude":%.4f,"status":"unavailable","forecast":[],"message":"Weather forecast integration pending"}`,
				params.Latitude, params.Longitude), nil
		},
	})
}

// projectDetailRow holds project summary data from the database.
type projectDetailRow struct {
	ID      uuid.UUID  `json:"id"`
	Name    string     `json:"name"`
	Address *string    `json:"address,omitempty"`
	Status  string     `json:"status"`
	Created time.Time  `json:"created_at"`
	Updated *time.Time `json:"updated_at,omitempty"`
}

// taskRow holds task data for JSON serialization.
type taskRow struct {
	ID               uuid.UUID  `json:"id"`
	WBSCode          string     `json:"wbs_code"`
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	IsOnCriticalPath bool       `json:"is_on_critical_path"`
	EarlyStart       *time.Time `json:"early_start,omitempty"`
	EarlyFinish      *time.Time `json:"early_finish,omitempty"`
	TotalFloatDays   float64    `json:"total_float_days"`
	PlannedStart     *time.Time `json:"planned_start,omitempty"`
	PlannedEnd       *time.Time `json:"planned_end,omitempty"`
	ActualStart      *time.Time `json:"actual_start,omitempty"`
	ActualEnd        *time.Time `json:"actual_end,omitempty"`
}

// procurementRow holds procurement item data for JSON serialization.
type procurementRow struct {
	ID                        uuid.UUID  `json:"id"`
	Description               string     `json:"description"`
	EstimatedCostCents        int64      `json:"estimated_cost_cents"`
	EstimatedCostCurrencyCode string     `json:"estimated_cost_currency_code"`
	Status                    string     `json:"status"`
	MustOrderDate             *time.Time `json:"must_order_date,omitempty"`
	ExpectedDeliveryDate      *time.Time `json:"expected_delivery_date,omitempty"`
	SupplierName              string     `json:"supplier_name,omitempty"`
}

// projectForecastRow holds forecast computation results.
type projectForecastRow struct {
	ProjectID             uuid.UUID  `json:"project_id"`
	TotalTasks            int        `json:"total_tasks"`
	CompletedTasks        int        `json:"completed_tasks"`
	InProgressTasks       int        `json:"in_progress_tasks"`
	BlockedTasks          int        `json:"blocked_tasks"`
	CriticalPathTasks     int        `json:"critical_path_tasks"`
	EarliestStart         *time.Time `json:"earliest_start,omitempty"`
	LatestFinish          *time.Time `json:"latest_finish,omitempty"`
	CompletionPercentage  float64    `json:"completion_percentage"`
	OnTrack               bool       `json:"on_track"`
}

// RegisterProjectToolsWithPool registers project tools wired to actual database queries
// via a raw pgxpool.Pool connection. No ORM — raw SQL only.
func RegisterProjectToolsWithPool(r *Registry, pool *pgxpool.Pool) {
	// get_project_details — query projects table
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_project_details",
			Description: "Get project details including name, address, status, budget, and key dates.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			var p projectDetailRow
			err := pool.QueryRow(ctx, `
				SELECT id, name, address, status, created_at, updated_at
				FROM projects
				WHERE id = $1 AND org_id = $2`,
				scope.ProjectID, scope.OrgID,
			).Scan(&p.ID, &p.Name, &p.Address, &p.Status, &p.Created, &p.Updated)
			if err != nil {
				return "", fmt.Errorf("get project details: %w", err)
			}

			// Enrich with budget totals
			var totalEstimatedCents, totalActualCents int64
			var budgetCurrency string
			if err := pool.QueryRow(ctx, `
				SELECT COALESCE(SUM(estimated_cost_cents), 0),
					   COALESCE(SUM(actual_cost_cents), 0),
					   COALESCE(MAX(estimated_cost_currency_code), 'USD')
				FROM project_budgets WHERE project_id = $1`,
				scope.ProjectID,
			).Scan(&totalEstimatedCents, &totalActualCents, &budgetCurrency); err != nil {
				slog.Warn("failed to query budget totals for project details", "error", err, "project_id", scope.ProjectID)
				budgetCurrency = "USD"
			}

			result := map[string]interface{}{
				"project":                   p,
				"total_estimated_cents":     totalEstimatedCents,
				"total_actual_cents":        totalActualCents,
				"budget_variance_cents":     totalEstimatedCents - totalActualCents,
				"budget_currency_code":      budgetCurrency,
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
	})

	// list_tasks — query project_tasks table
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "list_tasks",
			Description: "List all tasks for the current project with their status, dates, and critical path information.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"status_filter":{"type":"string","description":"Optional filter: Pending, Ready, In_Progress, Completed, Blocked, Delayed"}}}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			var params struct {
				StatusFilter string `json:"status_filter"`
			}
			if input != nil {
				if err := json.Unmarshal(input, &params); err != nil {
					slog.Warn("failed to parse list_tasks input", "error", err)
				}
			}

			query := `
				SELECT id, wbs_code, name, status, is_on_critical_path,
					early_start, early_finish, total_float_days,
					planned_start, planned_end, actual_start, actual_end
				FROM project_tasks
				WHERE project_id = $1`
			args := []interface{}{scope.ProjectID}

			if params.StatusFilter != "" {
				query += ` AND status = $2`
				args = append(args, params.StatusFilter)
			}
			query += ` ORDER BY wbs_code`

			rows, err := pool.Query(ctx, query, args...)
			if err != nil {
				return "", fmt.Errorf("list tasks: %w", err)
			}
			defer rows.Close()

			var tasks []taskRow
			for rows.Next() {
				var t taskRow
				if err := rows.Scan(
					&t.ID, &t.WBSCode, &t.Name, &t.Status, &t.IsOnCriticalPath,
					&t.EarlyStart, &t.EarlyFinish, &t.TotalFloatDays,
					&t.PlannedStart, &t.PlannedEnd, &t.ActualStart, &t.ActualEnd,
				); err != nil {
					return "", fmt.Errorf("scan task: %w", err)
				}
				tasks = append(tasks, t)
			}
			if err := rows.Err(); err != nil {
				return "", fmt.Errorf("iterate tasks: %w", err)
			}

			result := map[string]interface{}{
				"project_id": scope.ProjectID,
				"tasks":      tasks,
				"total":      len(tasks),
				"filter":     params.StatusFilter,
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
	})

	// list_procurement_items — query procurement_items table
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "list_procurement_items",
			Description: "List all procurement (long-lead) items for the project with their alert status, lead times, and calculated order dates. Useful for understanding supply chain risks.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)

			rows, err := pool.Query(ctx, `
				SELECT id, description, estimated_cost_cents, estimated_cost_currency_code,
					status, must_order_date, expected_delivery_date, supplier_name
				FROM procurement_items
				WHERE project_id = $1
				ORDER BY CASE status
					WHEN 'CRITICAL' THEN 0 WHEN 'WARNING' THEN 1
					WHEN 'PENDING' THEN 2 WHEN 'DELIVERED' THEN 3 ELSE 4
				END, must_order_date ASC NULLS LAST`,
				scope.ProjectID,
			)
			if err != nil {
				return "", fmt.Errorf("list procurement items: %w", err)
			}
			defer rows.Close()

			var items []procurementRow
			for rows.Next() {
				var item procurementRow
				if err := rows.Scan(
					&item.ID, &item.Description, &item.EstimatedCostCents,
					&item.EstimatedCostCurrencyCode, &item.Status,
					&item.MustOrderDate, &item.ExpectedDeliveryDate, &item.SupplierName,
				); err != nil {
					return "", fmt.Errorf("scan procurement item: %w", err)
				}
				items = append(items, item)
			}
			if err := rows.Err(); err != nil {
				return "", fmt.Errorf("iterate procurement items: %w", err)
			}

			result := map[string]interface{}{
				"project_id": scope.ProjectID,
				"items":      items,
				"total":      len(items),
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
	})

	// get_project_forecast — compute forecast from tasks data
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_project_forecast",
			Description: "Get a computed forecast for the project: completion percentage, critical path summary, on-track status, and projected end date.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)

			var forecast projectForecastRow
			forecast.ProjectID = scope.ProjectID

			err := pool.QueryRow(ctx, `
				SELECT
					COUNT(*),
					COUNT(*) FILTER (WHERE status = 'Completed'),
					COUNT(*) FILTER (WHERE status = 'In_Progress'),
					COUNT(*) FILTER (WHERE status = 'Blocked'),
					COUNT(*) FILTER (WHERE is_on_critical_path = true),
					MIN(early_start),
					MAX(early_finish)
				FROM project_tasks
				WHERE project_id = $1`,
				scope.ProjectID,
			).Scan(
				&forecast.TotalTasks,
				&forecast.CompletedTasks,
				&forecast.InProgressTasks,
				&forecast.BlockedTasks,
				&forecast.CriticalPathTasks,
				&forecast.EarliestStart,
				&forecast.LatestFinish,
			)
			if err != nil {
				return "", fmt.Errorf("compute project forecast: %w", err)
			}

			if forecast.TotalTasks > 0 {
				forecast.CompletionPercentage = float64(forecast.CompletedTasks) / float64(forecast.TotalTasks) * 100
			}

			// On-track heuristic: no blocked tasks and no overdue incomplete tasks
			var overdueCount int
			today := time.Now().UTC().Truncate(24 * time.Hour)
			if err := pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM project_tasks
				WHERE project_id = $1
					AND planned_end < $2
					AND status NOT IN ('Completed')`,
				scope.ProjectID, today,
			).Scan(&overdueCount); err != nil {
				slog.Warn("failed to query overdue task count for forecast", "error", err, "project_id", scope.ProjectID)
			}
			forecast.OnTrack = (forecast.BlockedTasks == 0 && overdueCount == 0)

			b, _ := json.Marshal(forecast)
			return string(b), nil
		},
	})

	// get_weather_forecast — remains a stub until weather integration
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_weather_forecast",
			Description: "Get the weather forecast for the project location. Returns high/low temp, precipitation probability, and conditions. Critical for scheduling exterior work.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"latitude":{"type":"number","description":"Latitude of the project site"},"longitude":{"type":"number","description":"Longitude of the project site"}},"required":["latitude","longitude"]}`),
		},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var params struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			// TODO: wire to real WeatherService.GetForecast
			return fmt.Sprintf(`{"latitude":%.4f,"longitude":%.4f,"status":"unavailable","forecast":[],"message":"Weather forecast integration pending"}`,
				params.Latitude, params.Longitude), nil
		},
	})
}

// RegisterProjectToolsWithDB registers project tools wired to actual database queries.
// Call this instead of RegisterProjectTools when services are available.
func RegisterProjectToolsWithDB(r *Registry, projectLister ProjectLister, procLister ProcurementLister) {
	if projectLister != nil {
		r.Register(Tool{
			Definition: ai.ToolDefinition{
				Name:        "get_project_details",
				Description: "Get project details including name, address, status, budget, and key dates.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
			},
			Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
				scope := MustGetScope(ctx)
				data, err := projectLister.GetProjectSummary(ctx, scope.ProjectID, scope.OrgID)
				if err != nil {
					return "", fmt.Errorf("get project: %w", err)
				}
				b, _ := json.Marshal(data)
				return string(b), nil
			},
		})
	}

	if procLister != nil {
		r.Register(Tool{
			Definition: ai.ToolDefinition{
				Name:        "list_procurement_items",
				Description: "List all procurement (long-lead) items for the project with their alert status, lead times, and calculated order dates.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
			},
			Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
				scope := MustGetScope(ctx)
				data, err := procLister.ListProcurementItems(ctx, scope.ProjectID)
				if err != nil {
					return "", fmt.Errorf("list procurement items: %w", err)
				}
				b, _ := json.Marshal(data)
				return string(b), nil
			},
		})
	}
}

// ProjectLister is the interface for project queries needed by tools.
type ProjectLister interface {
	GetProjectSummary(ctx context.Context, projectID, orgID uuid.UUID) (interface{}, error)
}

// ProcurementLister is the interface for procurement queries needed by tools.
type ProcurementLister interface {
	ListProcurementItems(ctx context.Context, projectID uuid.UUID) (interface{}, error)
}
