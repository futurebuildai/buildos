package skills

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ScheduleRecalcResult holds the output of a schedule recalculation.
// Defined here to avoid depending on service.ScheduleService directly.
// TODO: Wire to actual service types when integration is ready.
type ScheduleRecalcResult struct {
	TaskCount         int       `json:"task_count"`
	CriticalPathCount int       `json:"critical_path_count"`
	ProjectEnd        time.Time `json:"project_end"`
}

// ScheduleRecalcExecutor defines the interface for schedule recalculation.
// This avoids a direct dependency on the service package.
type ScheduleRecalcExecutor interface {
	RecalculateSchedule(ctx context.Context, projectID, orgID uuid.UUID) (*ScheduleRecalcResult, error)
}

// ScheduleRecalcSkill wraps ScheduleService.RecalculateSchedule for Tribunal-triggered execution.
type ScheduleRecalcSkill struct {
	executor ScheduleRecalcExecutor
}

// NewScheduleRecalcSkill creates a new ScheduleRecalcSkill.
func NewScheduleRecalcSkill(executor ScheduleRecalcExecutor) *ScheduleRecalcSkill {
	return &ScheduleRecalcSkill{executor: executor}
}

// ID returns the skill identifier.
func (s *ScheduleRecalcSkill) ID() string {
	return "schedule_recalc"
}

// Execute recalculates the schedule for a project.
// Required params: project_id (string UUID), org_id (string UUID)
func (s *ScheduleRecalcSkill) Execute(ctx context.Context, params map[string]any) (Result, error) {
	// Extract and validate required parameters
	projectIDStr, ok := params["project_id"].(string)
	if !ok || projectIDStr == "" {
		return Result{
			Success: false,
			Summary: "Missing required parameter: project_id",
		}, errors.New("missing required parameter: project_id")
	}

	orgIDStr, ok := params["org_id"].(string)
	if !ok || orgIDStr == "" {
		return Result{
			Success: false,
			Summary: "Missing required parameter: org_id",
		}, errors.New("missing required parameter: org_id")
	}

	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return Result{
			Success: false,
			Summary: fmt.Sprintf("Invalid project_id format: %v", err),
		}, fmt.Errorf("invalid project_id: %w", err)
	}

	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		return Result{
			Success: false,
			Summary: fmt.Sprintf("Invalid org_id format: %v", err),
		}, fmt.Errorf("invalid org_id: %w", err)
	}

	// Execute schedule recalculation
	result, err := s.executor.RecalculateSchedule(ctx, projectID, orgID)
	if err != nil {
		return Result{
			Success: false,
			Summary: fmt.Sprintf("Schedule recalculation failed: %v", err),
		}, err
	}

	// Build success result with CPM data
	return Result{
		Success: true,
		Summary: fmt.Sprintf("Schedule recalculated: %d tasks, %d on critical path, project ends %s",
			result.TaskCount, result.CriticalPathCount, result.ProjectEnd.Format("2006-01-02")),
		Data: map[string]any{
			"task_count":          result.TaskCount,
			"critical_path_count": result.CriticalPathCount,
			"project_end":         result.ProjectEnd,
		},
	}, nil
}
