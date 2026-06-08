package agentic

import "github.com/google/uuid"

// DelayCascadeInput is the request to run a delay-cascade reasoning pass. It
// names the org and project whose schedule just slipped; the orchestrator
// asks the Workspace to load the engine-computed context for this pair, runs
// the Reasoner over it, and asks the Workspace to apply the resulting plan.
type DelayCascadeInput struct {
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
}

// CascadeSlippedTask is one schedule task whose dates moved after the CPM
// engine recomputed. These are domain-neutral projections: the adapter in
// internal/service maps the engine/store rows into this shape, and the
// Reasoner adapter maps it onward to the AI wire form. EarlyFinish /
// LateFinish are wire-form date strings (typically YYYY-MM-DD). FloatDays is
// the remaining total float in whole days; a critical task has float 0.
type CascadeSlippedTask struct {
	WBS         string `json:"wbs"`
	Name        string `json:"name"`
	EarlyFinish string `json:"early_finish"`
	LateFinish  string `json:"late_finish"`
	FloatDays   int    `json:"float_days"`
	IsCritical  bool   `json:"is_critical"`
}

// CascadeProcurement is one procurement line in the slipped project's orbit.
// LeadTimeDays + MustOrderBy give the reasoner the ordering pressure;
// MustOrderBy is a wire-form date string (may be empty when unknown).
type CascadeProcurement struct {
	Description  string `json:"description"`
	Status       string `json:"status"`
	LeadTimeDays int    `json:"lead_time_days,omitempty"`
	MustOrderBy  string `json:"must_order_by,omitempty"`
}

// CascadeBudget is one cost-coded budget line. All monetary values are
// integer cents paired with CurrencyCode per the Composite Currency Pattern —
// never floats. agentic only ferries these numbers to the reasoner as
// context; it performs no currency arithmetic.
type CascadeBudget struct {
	WBS            string `json:"wbs"`
	EstimatedCents int64  `json:"estimated_cents"`
	CommittedCents int64  `json:"committed_cents"`
	ActualCents    int64  `json:"actual_cents"`
	CurrencyCode   string `json:"currency_code"`
}

// CascadeContext is the engine-computed snapshot the Workspace loads for a
// slipped project, and the sole input to the Reasoner. It carries only facts
// the deterministic engine and stores already produced — agentic computes
// none of it.
type CascadeContext struct {
	ProjectName  string               `json:"project_name"`
	SlippedTasks []CascadeSlippedTask `json:"slipped_tasks"`
	Procurement  []CascadeProcurement `json:"procurement,omitempty"`
	Budget       []CascadeBudget      `json:"budget,omitempty"`
}

// HasCriticalPath reports whether the context carries at least one critical
// slipped task. The orchestrator treats a context with no critical tasks as a
// no-op: a non-critical slip absorbs into float and is not worth surfacing a
// cross-module cascade for.
func (c CascadeContext) HasCriticalPath() bool {
	for _, t := range c.SlippedTasks {
		if t.IsCritical {
			return true
		}
	}
	return false
}

// CascadeImpact is one downstream impact the reasoner surfaces. Module is one
// of "schedule", "procurement", "crew", "budget". Severity is one of
// "critical", "high", "normal", "low". Title/Body render as a feed card;
// RecommendedAction is the suggested human next step.
type CascadeImpact struct {
	Module            string `json:"module"`
	Severity          string `json:"severity"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	RecommendedAction string `json:"recommended_action"`
}

// CascadePlan is the reasoner's advisory output: the ranked set of
// cross-module impacts to apply. It is judgment, not truth — the Workspace
// renders it into feed cards + an audit trail; it never mutates the schedule
// or the money.
type CascadePlan struct {
	Impacts []CascadeImpact `json:"impacts"`
}

// CascadeResult is the summary the orchestrator returns once a plan has been
// applied. CardsCreated counts the feed cards the Workspace persisted; Impacts
// counts the impacts the reasoner produced.
type CascadeResult struct {
	CardsCreated int `json:"cards_created"`
	Impacts      int `json:"impacts"`
}
