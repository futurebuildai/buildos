package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// RegisterScheduleTools registers schedule-related tools.
// Uses stub implementations until the ScheduleService is fully wired.
func RegisterScheduleTools(r *Registry) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "recalculate_schedule",
			Description: "Trigger a full CPM (Critical Path Method) recalculation of the project schedule. Use after task status changes or duration overrides to see the updated critical path and end date.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			// TODO: wire to real ScheduleService.RecalculateSchedule
			return fmt.Sprintf(`{"success":true,"project_id":"%s","message":"Schedule recalculation queued"}`, scope.ProjectID), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "delay_task",
			Description: "Delay a specific task by a number of days. This creates a duration override and triggers schedule recalculation.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string","description":"UUID of the task to delay"},"delay_days":{"type":"integer","description":"Number of days to delay"},"reason":{"type":"string","description":"Reason for the delay"}},"required":["task_id","delay_days","reason"]}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			var params struct {
				TaskID    string `json:"task_id"`
				DelayDays int    `json:"delay_days"`
				Reason    string `json:"reason"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			taskID, err := uuid.Parse(params.TaskID)
			if err != nil {
				return "", fmt.Errorf("invalid task_id: %w", err)
			}
			// TODO: wire to real ScheduleService.DelayTask
			return fmt.Sprintf(`{"success":true,"project_id":"%s","task_id":"%s","delay_days":%d,"reason":"%s","message":"Task delay applied"}`,
				scope.ProjectID, taskID, params.DelayDays, params.Reason), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "get_agent_focus_tasks",
			Description: "Get today's priority tasks for the project, including critical path tasks starting soon, tasks needing attention, and overdue items.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			// TODO: wire to real ScheduleService.GetAgentFocusTasks
			return fmt.Sprintf(`{"project_id":"%s","focus_tasks":[],"overdue_count":0,"critical_path_active":0,"message":"No focus tasks (stub)"}`,
				scope.ProjectID), nil
		},
	})
}
