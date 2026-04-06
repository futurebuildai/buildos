// Package physics -- equipment_validator.go
// Validates equipment availability for Site Prep tasks (WBS 7.x).
// DETERMINISTIC: No AI, pure DB lookups and date range comparisons.
// See BACKEND_SCOPE.md Section 20.3

package physics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EquipmentRequirement maps a WBS code prefix to the equipment type needed.
// Site Prep phase (7.x) requires heavy equipment for several sub-tasks.
var SitePrepEquipmentRequirements = map[string]string{
	"7.1": "excavator",     // Rough Grading
	"7.2": "excavator",     // Utility Trenching
	"7.3": "compactor",     // Soil Compaction
	"7.4": "grader",        // Fine Grading
	"7.5": "concrete_pump", // Foundation Prep (if applicable)
}

// EquipmentWarning represents a non-blocking equipment constraint issue.
// Warnings are informational -- the schedule still proceeds but flags the gap.
type EquipmentWarning struct {
	TaskWBSCode  string    `json:"task_wbs_code"`
	RequiredType string    `json:"required_type"`
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	Message      string    `json:"message"`
}

// ValidateEquipmentConstraints checks that required equipment is allocated to
// the project for the given task's date range. Only applies to Site Prep tasks
// (WBS codes starting with "7.").
//
// Returns nil if:
//   - The task is not a Site Prep task (non-7.x WBS code)
//   - Required equipment is allocated and available for the date range
//
// Returns an error if:
//   - Required equipment type is not allocated to the project for the date range
//   - Database query fails
func ValidateEquipmentConstraints(ctx context.Context, db *pgxpool.Pool, projectID uuid.UUID, taskWBSCode string, startDate, endDate time.Time) error {
	// Only validate Site Prep tasks (WBS 7.x)
	if !strings.HasPrefix(taskWBSCode, "7.") {
		return nil
	}

	// Look up required equipment type for this WBS code
	requiredType, ok := SitePrepEquipmentRequirements[taskWBSCode]
	if !ok {
		// No specific equipment requirement for this sub-task
		return nil
	}

	// Check if the required equipment type is allocated to this project
	// for the task's date range with an active/planned status.
	query := `
		SELECT COUNT(*)
		FROM equipment_allocations ea
		INNER JOIN fleet_assets fa ON ea.asset_id = fa.id
		WHERE ea.project_id = $1
			AND fa.asset_type = $2
			AND ea.status IN ('planned', 'active')
			AND daterange(ea.allocated_from, ea.allocated_to, '[]') @> daterange($3::date, $4::date, '[]')`

	var count int
	err := db.QueryRow(ctx, query, projectID, requiredType, startDate, endDate).Scan(&count)
	if err != nil {
		return fmt.Errorf("equipment constraint check failed for %s: %w", taskWBSCode, err)
	}

	if count == 0 {
		return fmt.Errorf("%s required for WBS %s but not allocated for %s to %s",
			requiredType, taskWBSCode,
			startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	}

	return nil
}

// ValidateProjectEquipment checks all Site Prep tasks in a project schedule
// for equipment availability. Returns a list of warnings (non-blocking).
// This is intended to be called after CPM scheduling to surface equipment gaps.
func ValidateProjectEquipment(ctx context.Context, db *pgxpool.Pool, projectID uuid.UUID, schedule []TaskSchedule) []EquipmentWarning {
	var warnings []EquipmentWarning

	for _, task := range schedule {
		if !strings.HasPrefix(task.WBSCode, "7.") {
			continue
		}

		requiredType, ok := SitePrepEquipmentRequirements[task.WBSCode]
		if !ok {
			continue
		}

		err := ValidateEquipmentConstraints(ctx, db, projectID, task.WBSCode, task.EarlyStart, task.EarlyFinish)
		if err != nil {
			warnings = append(warnings, EquipmentWarning{
				TaskWBSCode:  task.WBSCode,
				RequiredType: requiredType,
				StartDate:    task.EarlyStart,
				EndDate:      task.EarlyFinish,
				Message:      err.Error(),
			})
		}
	}

	return warnings
}
