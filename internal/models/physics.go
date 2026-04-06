// Package models provides domain types for the FutureBuild OS system.
// This file defines types consumed by the physics engine (CPM, DHSM, SWIM, Scoping).
package models

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Dependency Type (formerly in pkg/types)
// =============================================================================

// DependencyType defines the relationship between two tasks.
// See DATA_SPINE_SPEC.md Section 3.4
type DependencyType string

const (
	DependencyTypeFS DependencyType = "FS"
	DependencyTypeSS DependencyType = "SS"
	DependencyTypeFF DependencyType = "FF"
	DependencyTypeSF DependencyType = "SF"
)

// =============================================================================
// Task Status (formerly in pkg/types)
// =============================================================================

// TaskStatus defines the lifecycle of a ProjectTask.
// See API_AND_TYPES_SPEC.md Section 1.1
type TaskStatus string

const (
	TaskStatusPending           TaskStatus = "Pending"
	TaskStatusReady             TaskStatus = "Ready"
	TaskStatusInProgress        TaskStatus = "In_Progress"
	TaskStatusInspectionPending TaskStatus = "Inspection_Pending"
	TaskStatusCompleted         TaskStatus = "Completed"
	TaskStatusBlocked           TaskStatus = "Blocked"
	TaskStatusDelayed           TaskStatus = "Delayed"
)

// =============================================================================
// Forecast (formerly in pkg/types)
// =============================================================================

// Forecast holds weather forecast data used by the SWIM weather model.
type Forecast struct {
	Date                     string  `json:"date"`
	HighTempC                float64 `json:"high_temp_c"`
	LowTempC                 float64 `json:"low_temp_c"`
	PrecipitationMM          float64 `json:"precipitation_mm"`
	PrecipitationProbability float64 `json:"precipitation_probability"`
	Conditions               string  `json:"conditions"`
}

// =============================================================================
// ProjectTask (formerly in internal/models)
// =============================================================================

// ProjectTask represents a specific instance of a task for a live project.
// See DATA_SPINE_SPEC.md Section 3.3
type ProjectTask struct {
	ID                      uuid.UUID  `json:"id" db:"id"`
	ProjectID               uuid.UUID  `json:"project_id" db:"project_id" validate:"required"`
	WBSCode                 string     `json:"wbs_code" db:"wbs_code" validate:"required"`
	Name                    string     `json:"name" db:"name" validate:"required"`
	IsInspection            bool       `json:"is_inspection" db:"is_inspection"`
	EarlyStart              *time.Time `json:"early_start,omitempty" db:"early_start"`
	EarlyFinish             *time.Time `json:"early_finish,omitempty" db:"early_finish"`
	LateStart               *time.Time `json:"late_start,omitempty" db:"late_start"`
	LateFinish              *time.Time `json:"late_finish,omitempty" db:"late_finish"`
	TotalFloatDays          float64    `json:"total_float_days" db:"total_float_days"`
	IsOnCriticalPath        bool       `json:"is_on_critical_path" db:"is_on_critical_path"`
	CalculatedDuration      float64    `json:"calculated_duration" db:"calculated_duration"`
	WeatherAdjustedDuration float64    `json:"weather_adjusted_duration" db:"weather_adjusted_duration"`
	ManualOverrideDays      *float64   `json:"manual_override_days,omitempty" db:"manual_override_days"`
	OverrideReason          string     `json:"override_reason,omitempty" db:"override_reason"`
	Status                  TaskStatus `json:"status" db:"status"`
	PlannedStart            *time.Time `json:"planned_start" db:"planned_start"`
	PlannedEnd              *time.Time `json:"planned_end" db:"planned_end"`
	ActualStart             *time.Time `json:"actual_start" db:"actual_start"`
	ActualEnd               *time.Time `json:"actual_end" db:"actual_end"`
	VerifiedByVision        bool       `json:"verified_by_vision" db:"verified_by_vision"`
	VerificationConfidence  float64    `json:"verification_confidence" db:"verification_confidence"`
	IsHumanReviewRequired   bool       `json:"is_human_review_required" db:"is_human_review_required"`
}

// =============================================================================
// TaskDependency (formerly in internal/models)
// =============================================================================

// TaskDependency represents an edge in the Project Task DAG.
// See DATA_SPINE_SPEC.md Section 3.4
type TaskDependency struct {
	ID               uuid.UUID      `json:"id" db:"id"`
	ProjectID        uuid.UUID      `json:"project_id" db:"project_id" validate:"required"`
	PredecessorID    uuid.UUID      `json:"predecessor_id" db:"predecessor_id" validate:"required"`
	SuccessorID      uuid.UUID      `json:"successor_id" db:"successor_id" validate:"required"`
	DependencyType   DependencyType `json:"dependency_type" db:"dependency_type"`
	LagDays          int            `json:"lag_days" db:"lag_days"`
	IsInspectionGate bool           `json:"is_inspection_gate" db:"is_inspection_gate"`
}

// =============================================================================
// DurationMultiplier (formerly in internal/models)
// =============================================================================

// MultiplierSource defines the origin of a duration multiplier.
// See BACKEND_SCOPE.md Section 4.2
type MultiplierSource string

const (
	MultiplierSourceDefault       MultiplierSource = "default"
	MultiplierSourceOrgTrained    MultiplierSource = "org_trained"
	MultiplierSourceGlobalTrained MultiplierSource = "global_trained"
)

// DurationMultiplier represents a weighted variable for the DHSM calculator.
// See BACKEND_SCOPE.md Section 4.2
type DurationMultiplier struct {
	ID                uuid.UUID        `json:"id" db:"id"`
	OrgID             *uuid.UUID       `json:"org_id,omitempty" db:"org_id"`
	WBSTaskCode       string           `json:"wbs_task_code" db:"wbs_task_code"`
	VariableKey       string           `json:"variable_key" db:"variable_key"`
	Weight            float64          `json:"weight" db:"weight"`
	MultiplierFormula string           `json:"multiplier_formula" db:"multiplier_formula"`
	MinValue          float64          `json:"min_value" db:"min_value"`
	MaxValue          float64          `json:"max_value" db:"max_value"`
	Source            MultiplierSource `json:"source" db:"source"`
	Confidence        float64          `json:"confidence" db:"confidence"`
}

// =============================================================================
// ProjectContext (formerly in internal/models)
// =============================================================================

// ProjectContext holds contextual variables for duration calculations.
type ProjectContext struct {
	ID                     uuid.UUID `json:"id" db:"id"`
	ProjectID              uuid.UUID `json:"project_id" db:"project_id"`
	SupplyChainVolatility  int       `json:"supply_chain_volatility" db:"supply_chain_volatility"`
	RoughInspectionLatency int       `json:"rough_inspection_latency" db:"rough_inspection_latency"`
	FinalInspectionLatency int       `json:"final_inspection_latency" db:"final_inspection_latency"`
	ZipCode                string    `json:"zip_code" db:"zip_code"`
	ClimateZone            string    `json:"climate_zone" db:"climate_zone"`
}

// =============================================================================
// WBSTask and WBSDependency (formerly in internal/data)
// =============================================================================

// WBSTask is a single task from the WBS master template.
type WBSTask struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	BaseDurationDays float64  `json:"base_duration_days"`
	ResponsibleParty string   `json:"responsible_party"`
	Deliverable      string   `json:"deliverable"`
	Notes            string   `json:"notes"`
	IsInspection     bool     `json:"is_inspection"`
	IsMilestone      bool     `json:"is_milestone"`
	IsLongLead       bool     `json:"is_long_lead"`
	LeadTimeWeeksMin int      `json:"lead_time_weeks_min"`
	LeadTimeWeeksMax int      `json:"lead_time_weeks_max"`
	PredecessorCodes []string `json:"predecessor_codes"`
	// ID is used by WBSTask in DHSM calculations but not required for WBS template loading.
	ID uuid.UUID `json:"id,omitempty"`
}

// WBSDependency represents a dependency edge parsed from the WBS template.
type WBSDependency struct {
	PredecessorCode string
	SuccessorCode   string
	Type            DependencyType
	LagDays         int
}
