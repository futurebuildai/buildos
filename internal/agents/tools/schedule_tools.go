package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/physics"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScheduleEngine provides CPM recalculation backed by the physics engine and database.
type ScheduleEngine struct {
	pool *pgxpool.Pool
}

// NewScheduleEngine creates a ScheduleEngine wired to a database pool.
func NewScheduleEngine(pool *pgxpool.Pool) *ScheduleEngine {
	return &ScheduleEngine{pool: pool}
}

// RecalculateSchedule loads the task graph from the database, runs ForwardPass + BackwardPass,
// and persists the computed dates back to the project_tasks table. Returns the CPM result
// for downstream consumption (tool responses, skill results, etc.).
func (e *ScheduleEngine) RecalculateSchedule(ctx context.Context, projectID, orgID uuid.UUID) (*CPMRecalcResult, error) {
	// 1. Load tasks for the project
	tasks, err := e.loadTasks(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load tasks: %w", err)
	}
	if len(tasks) == 0 {
		return &CPMRecalcResult{
			TaskCount: 0,
			Message:   "No tasks found for project",
		}, nil
	}

	// 2. Load dependencies
	deps, err := e.loadDependencies(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load dependencies: %w", err)
	}

	// 3. Determine project start date (earliest planned_start or today)
	projectStart := time.Now().UTC().Truncate(24 * time.Hour)
	for _, t := range tasks {
		if t.PlannedStart != nil && t.PlannedStart.Before(projectStart) {
			projectStart = *t.PlannedStart
		}
	}

	// 4. Build dependency graph and run CPM
	graph := physics.BuildDependencyGraph(tasks, deps)

	if err := physics.DetectCycle(graph); err != nil {
		return nil, fmt.Errorf("cycle detected in task graph: %w", err)
	}

	cal := &physics.StandardCalendar{} // Mon-Fri default

	// Forward pass
	schedule, err := physics.ForwardPass(graph, projectStart, cal, nil)
	if err != nil {
		return nil, fmt.Errorf("forward pass: %w", err)
	}

	// Backward pass
	criticalPath, err := physics.BackwardPass(graph, schedule, cal, nil)
	if err != nil {
		return nil, fmt.Errorf("backward pass: %w", err)
	}

	// 5. Persist results back to project_tasks
	if err := e.persistSchedule(ctx, schedule); err != nil {
		return nil, fmt.Errorf("persist schedule: %w", err)
	}

	// 6. Compute project end date
	var projectEnd time.Time
	for _, sched := range schedule {
		if sched.EarlyFinish.After(projectEnd) {
			projectEnd = sched.EarlyFinish
		}
	}

	criticalCount := 0
	for _, sched := range schedule {
		if sched.IsCritical {
			criticalCount++
		}
	}

	slog.Info("CPM recalculation completed",
		"project_id", projectID,
		"task_count", len(schedule),
		"critical_path_count", criticalCount,
		"project_end", projectEnd.Format("2006-01-02"),
	)

	return &CPMRecalcResult{
		TaskCount:         len(schedule),
		CriticalPathCount: criticalCount,
		CriticalPath:      criticalPath,
		ProjectEnd:        projectEnd,
		Message:           "Schedule recalculated successfully",
	}, nil
}

// CPMRecalcResult holds the output of a CPM recalculation.
type CPMRecalcResult struct {
	TaskCount         int       `json:"task_count"`
	CriticalPathCount int       `json:"critical_path_count"`
	CriticalPath      []string  `json:"critical_path"`
	ProjectEnd        time.Time `json:"project_end"`
	Message           string    `json:"message"`
}

// loadTasks retrieves all tasks for a project from the database.
func (e *ScheduleEngine) loadTasks(ctx context.Context, projectID uuid.UUID) ([]models.ProjectTask, error) {
	rows, err := e.pool.Query(ctx, `
		SELECT id, project_id, wbs_code, name, is_inspection,
			early_start, early_finish, late_start, late_finish,
			total_float_days, is_on_critical_path,
			calculated_duration, weather_adjusted_duration, manual_override_days, override_reason,
			status, planned_start, planned_end, actual_start, actual_end,
			verified_by_vision, verification_confidence, is_human_review_required
		FROM project_tasks
		WHERE project_id = $1
		ORDER BY wbs_code`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []models.ProjectTask
	for rows.Next() {
		var t models.ProjectTask
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.WBSCode, &t.Name, &t.IsInspection,
			&t.EarlyStart, &t.EarlyFinish, &t.LateStart, &t.LateFinish,
			&t.TotalFloatDays, &t.IsOnCriticalPath,
			&t.CalculatedDuration, &t.WeatherAdjustedDuration, &t.ManualOverrideDays, &t.OverrideReason,
			&t.Status, &t.PlannedStart, &t.PlannedEnd, &t.ActualStart, &t.ActualEnd,
			&t.VerifiedByVision, &t.VerificationConfidence, &t.IsHumanReviewRequired,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// loadDependencies retrieves all task dependencies for a project.
func (e *ScheduleEngine) loadDependencies(ctx context.Context, projectID uuid.UUID) ([]models.TaskDependency, error) {
	rows, err := e.pool.Query(ctx, `
		SELECT id, project_id, predecessor_id, successor_id,
			dependency_type, lag_days, is_inspection_gate
		FROM task_dependencies
		WHERE project_id = $1`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query dependencies: %w", err)
	}
	defer rows.Close()

	var deps []models.TaskDependency
	for rows.Next() {
		var d models.TaskDependency
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.PredecessorID, &d.SuccessorID,
			&d.DependencyType, &d.LagDays, &d.IsInspectionGate,
		); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// persistSchedule writes CPM results back to the project_tasks table.
func (e *ScheduleEngine) persistSchedule(ctx context.Context, schedule map[uuid.UUID]physics.TaskSchedule) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for taskID, sched := range schedule {
		_, err := tx.Exec(ctx, `
			UPDATE project_tasks SET
				early_start = $2,
				early_finish = $3,
				late_start = $4,
				late_finish = $5,
				total_float_days = $6,
				is_on_critical_path = $7
			WHERE id = $1`,
			taskID,
			sched.EarlyStart, sched.EarlyFinish,
			sched.LateStart, sched.LateFinish,
			sched.TotalFloat, sched.IsCritical,
		)
		if err != nil {
			return fmt.Errorf("update task %s: %w", taskID, err)
		}
	}

	return tx.Commit(ctx)
}

// DelayTask adds days to a task's duration override and triggers recalculation.
func (e *ScheduleEngine) DelayTask(ctx context.Context, projectID, taskID uuid.UUID, delayDays int, reason string) (*CPMRecalcResult, error) {
	// Read current duration
	var currentOverride *float64
	var calculatedDuration float64
	err := e.pool.QueryRow(ctx, `
		SELECT manual_override_days, calculated_duration
		FROM project_tasks
		WHERE id = $1 AND project_id = $2`,
		taskID, projectID,
	).Scan(&currentOverride, &calculatedDuration)
	if err != nil {
		return nil, fmt.Errorf("read task duration: %w", err)
	}

	// Compute new duration: add delay to existing override or base duration
	baseDuration := calculatedDuration
	if currentOverride != nil {
		baseDuration = *currentOverride
	}
	newDuration := baseDuration + float64(delayDays)

	// Update task with new override
	_, err = e.pool.Exec(ctx, `
		UPDATE project_tasks SET
			manual_override_days = $3,
			override_reason = $4,
			status = CASE WHEN status != 'Completed' THEN 'Delayed' ELSE status END
		WHERE id = $1 AND project_id = $2`,
		taskID, projectID, newDuration, reason,
	)
	if err != nil {
		return nil, fmt.Errorf("update task delay: %w", err)
	}

	slog.Info("task delayed",
		"task_id", taskID,
		"project_id", projectID,
		"delay_days", delayDays,
		"new_duration", newDuration,
		"reason", reason,
	)

	// Trigger full recalculation so downstream tasks reflect the delay
	// orgID is uuid.Nil here because recalc only needs projectID for task queries
	return e.RecalculateSchedule(ctx, projectID, uuid.Nil)
}

// focusTaskRow holds tasks requiring attention within a time window.
type focusTaskRow struct {
	ID               uuid.UUID  `json:"id"`
	WBSCode          string     `json:"wbs_code"`
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	IsOnCriticalPath bool       `json:"is_on_critical_path"`
	EarlyStart       *time.Time `json:"early_start,omitempty"`
	EarlyFinish      *time.Time `json:"early_finish,omitempty"`
	PlannedStart     *time.Time `json:"planned_start,omitempty"`
	PlannedEnd       *time.Time `json:"planned_end,omitempty"`
	TotalFloatDays   float64    `json:"total_float_days"`
	Urgency          string     `json:"urgency"`
}

// GetAgentFocusTasks returns tasks that need attention: starting soon, overdue, or on critical path.
func (e *ScheduleEngine) GetAgentFocusTasks(ctx context.Context, projectID uuid.UUID) ([]focusTaskRow, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	windowEnd := today.Add(3 * 24 * time.Hour) // 3-day lookahead

	rows, err := e.pool.Query(ctx, `
		SELECT id, wbs_code, name, status, is_on_critical_path,
			early_start, early_finish, planned_start, planned_end, total_float_days
		FROM project_tasks
		WHERE project_id = $1
			AND status NOT IN ('Completed')
			AND (
				-- Overdue: planned_end is past
				(planned_end < $2 AND status != 'Completed')
				-- Starting within 3-day window
				OR (early_start >= $2 AND early_start <= $3)
				-- On critical path and in progress
				OR (is_on_critical_path = true AND status = 'In_Progress')
				-- Blocked tasks
				OR status = 'Blocked'
			)
		ORDER BY
			CASE status WHEN 'Blocked' THEN 0 WHEN 'Delayed' THEN 1 ELSE 2 END,
			is_on_critical_path DESC,
			early_start ASC NULLS LAST`,
		projectID, today, windowEnd,
	)
	if err != nil {
		return nil, fmt.Errorf("query focus tasks: %w", err)
	}
	defer rows.Close()

	var tasks []focusTaskRow
	for rows.Next() {
		var t focusTaskRow
		if err := rows.Scan(
			&t.ID, &t.WBSCode, &t.Name, &t.Status, &t.IsOnCriticalPath,
			&t.EarlyStart, &t.EarlyFinish, &t.PlannedStart, &t.PlannedEnd, &t.TotalFloatDays,
		); err != nil {
			return nil, fmt.Errorf("scan focus task: %w", err)
		}
		// Classify urgency
		switch {
		case t.Status == "Blocked":
			t.Urgency = "blocked"
		case t.PlannedEnd != nil && t.PlannedEnd.Before(today):
			t.Urgency = "overdue"
		case t.IsOnCriticalPath:
			t.Urgency = "critical_path"
		default:
			t.Urgency = "upcoming"
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

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
			return fmt.Sprintf(`{"project_id":"%s","focus_tasks":[],"overdue_count":0,"critical_path_active":0,"message":"No focus tasks (stub)"}`,
				scope.ProjectID), nil
		},
	})
}

// RegisterScheduleToolsWithEngine registers schedule tools wired to the physics engine.
func RegisterScheduleToolsWithEngine(r *Registry, engine *ScheduleEngine) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "recalculate_schedule",
			Description: "Trigger a full CPM (Critical Path Method) recalculation of the project schedule. Use after task status changes or duration overrides to see the updated critical path and end date.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)

			result, err := engine.RecalculateSchedule(ctx, scope.ProjectID, scope.OrgID)
			if err != nil {
				return "", fmt.Errorf("recalculate schedule: %w", err)
			}

			b, _ := json.Marshal(map[string]interface{}{
				"success":             true,
				"project_id":          scope.ProjectID,
				"task_count":          result.TaskCount,
				"critical_path_count": result.CriticalPathCount,
				"critical_path":       result.CriticalPath,
				"project_end":         result.ProjectEnd.Format("2006-01-02"),
				"message":             result.Message,
			})
			return string(b), nil
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

			result, err := engine.DelayTask(ctx, scope.ProjectID, taskID, params.DelayDays, params.Reason)
			if err != nil {
				return "", fmt.Errorf("delay task: %w", err)
			}

			b, _ := json.Marshal(map[string]interface{}{
				"success":             true,
				"project_id":          scope.ProjectID,
				"task_id":             taskID,
				"delay_days":          params.DelayDays,
				"reason":              params.Reason,
				"task_count":          result.TaskCount,
				"critical_path_count": result.CriticalPathCount,
				"project_end":         result.ProjectEnd.Format("2006-01-02"),
				"message":             "Task delayed and schedule recalculated",
			})
			return string(b), nil
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

			tasks, err := engine.GetAgentFocusTasks(ctx, scope.ProjectID)
			if err != nil {
				return "", fmt.Errorf("get focus tasks: %w", err)
			}

			// Count by urgency
			overdueCount := 0
			criticalPathActive := 0
			blockedCount := 0
			for _, t := range tasks {
				switch t.Urgency {
				case "overdue":
					overdueCount++
				case "critical_path":
					criticalPathActive++
				case "blocked":
					blockedCount++
				}
			}

			b, _ := json.Marshal(map[string]interface{}{
				"project_id":          scope.ProjectID,
				"focus_tasks":         tasks,
				"total":               len(tasks),
				"overdue_count":       overdueCount,
				"critical_path_active": criticalPathActive,
				"blocked_count":       blockedCount,
			})
			return string(b), nil
		},
	})
}
