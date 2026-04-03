package physics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SitePrepEquipmentRequirements maps WBS code prefix to required equipment type.
var SitePrepEquipmentRequirements = map[string]string{
	"7.1": "excavator",
	"7.2": "excavator",
	"7.3": "compactor",
	"7.4": "grader",
	"7.5": "concrete_pump",
}

// EquipmentWarning represents a non-blocking equipment constraint issue.
type EquipmentWarning struct {
	TaskWBSCode  string    `json:"task_wbs_code"`
	RequiredType string    `json:"required_type"`
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	Message      string    `json:"message"`
}

// ValidateEquipmentConstraints checks that required equipment is allocated
// for the given task's date range. Only applies to Site Prep tasks (WBS 7.x).
func ValidateEquipmentConstraints(ctx context.Context, db *pgxpool.Pool, projectID uuid.UUID, taskWBSCode string, startDate, endDate time.Time) error {
	if !strings.HasPrefix(taskWBSCode, "7.") {
		return nil
	}

	requiredType, ok := SitePrepEquipmentRequirements[taskWBSCode]
	if !ok {
		return nil
	}

	query := `
		SELECT COUNT(*)
		FROM equipment_allocations ea
		INNER JOIN fleet_assets fa ON ea.asset_id = fa.id
		WHERE ea.project_id = $1
			AND fa.asset_type = $2
			AND ea.start_date <= $3
			AND ea.end_date >= $4`

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
