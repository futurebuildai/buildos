package models

import (
	"time"

	"github.com/futurebuild/futurebuild-os/internal/models/types"
	"github.com/google/uuid"
)

// Project represents a construction project.
type Project struct {
	ID               uuid.UUID  `json:"id"`
	OrgID            uuid.UUID  `json:"org_id"`
	Name             string     `json:"name"`
	Address          string     `json:"address,omitempty"`
	PermitIssuedDate *time.Time `json:"permit_issued_date,omitempty"`
	ProjectStartDate *time.Time `json:"project_start_date,omitempty"`
	Status           string     `json:"status"`
	GSF              *int       `json:"gsf,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ProjectTask represents a WBS-based task in a project schedule.
// Includes both database-persisted fields and physics-computed transient fields.
type ProjectTask struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	WBSCode   string    `json:"wbs_code"`
	Name      string    `json:"name"`

	// Duration fields — persisted
	DurationDays int `json:"duration_days"`

	// CPM-computed schedule fields — persisted after recalculation
	EarlyStart  *time.Time `json:"early_start,omitempty"`
	EarlyFinish *time.Time `json:"early_finish,omitempty"`
	LateStart   *time.Time `json:"late_start,omitempty"`
	LateFinish  *time.Time `json:"late_finish,omitempty"`
	TotalFloat  *int       `json:"total_float,omitempty"`
	IsCritical  bool       `json:"is_critical"`

	// Status fields
	Status          string    `json:"status"`
	PercentComplete int       `json:"percent_complete"`
	AssignedCrew    []uuid.UUID `json:"assigned_crew,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Physics-computed transient fields (not directly persisted, used by CPM engine)
	CalculatedDuration      float64  `json:"-"`
	WeatherAdjustedDuration float64  `json:"-"`
	ManualOverrideDays      *float64 `json:"-"`
}

// TaskDependency represents a dependency relationship between two tasks.
type TaskDependency struct {
	ID             uuid.UUID            `json:"id"`
	ProjectID      uuid.UUID            `json:"project_id"`
	PredecessorID  uuid.UUID            `json:"predecessor_id"`
	SuccessorID    uuid.UUID            `json:"successor_id"`
	DependencyType types.DependencyType `json:"dependency_type"`
	LagDays        int                  `json:"lag_days"`
}

// WBSTask represents a WBS template task for DHSM duration calculations.
// Used by the scoping engine and DHSM calculator.
type WBSTask struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	BaseDurationDays float64  `json:"base_duration_days"`
	ResponsibleParty string   `json:"responsible_party,omitempty"`
	Deliverable      string   `json:"deliverable,omitempty"`
	PredecessorCodes []string `json:"predecessor_codes,omitempty"`
	IsInspection     bool     `json:"is_inspection"`
}

// WBSTemplateDep represents a dependency in the WBS template (string-based codes).
type WBSTemplateDep struct {
	PredecessorCode string               `json:"predecessor_code"`
	SuccessorCode   string               `json:"successor_code"`
	Type            types.DependencyType `json:"type"`
	LagDays         int                  `json:"lag_days"`
}

// ProjectContext holds project-level variables that influence DHSM calculations.
type ProjectContext struct {
	SupplyChainVolatility   int `json:"supply_chain_volatility"`
	RoughInspectionLatency  int `json:"rough_inspection_latency"`
	FinalInspectionLatency  int `json:"final_inspection_latency"`
}

// DurationMultiplier defines a configurable multiplier rule for DHSM.
type DurationMultiplier struct {
	WBSTaskCode       string  `json:"wbs_task_code"`
	VariableKey       string  `json:"variable_key"`
	Weight            float64 `json:"weight"`
	MultiplierFormula string  `json:"multiplier_formula"`
}
