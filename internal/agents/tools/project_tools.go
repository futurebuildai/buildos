package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
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
			// TODO: wire to real ProjectService.GetProject
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
				_ = json.Unmarshal(input, &params)
			}
			// TODO: wire to real ProjectService.ListTasks
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
			// TODO: wire to real ProcurementService.ListByProject
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
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var params struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			// TODO: wire to real WeatherService.GetForecast
			return fmt.Sprintf(`{"latitude":%.4f,"longitude":%.4f,"forecast":[],"message":"Weather forecast (stub)"}`,
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
