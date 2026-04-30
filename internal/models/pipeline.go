package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PipelineStage names the stages a Prospect moves through from initial
// inquiry to permit issuance (which atomically transitions the record
// into a Project + CPM schedule in the construction execution plane).
type PipelineStage string

const (
	StageLead             PipelineStage = "LEAD"
	StageQualified        PipelineStage = "QUALIFIED"
	StageEstimateSent     PipelineStage = "ESTIMATE_SENT"
	StageVerbalCommitment PipelineStage = "VERBAL_COMMITMENT"
	StagePermitApplied    PipelineStage = "PERMIT_APPLIED"
	StagePermitIssued     PipelineStage = "PERMIT_ISSUED" // terminal — triggers Kanban→CPM
	StageLost             PipelineStage = "LOST"          // terminal
)

// Probability returns the percentage chance of conversion assigned to
// each stage. Used by the pipeline_analytics rollup to compute weighted
// forecast revenue (probability × total_estimated_cents, grouped by
// currency_code).
func (s PipelineStage) Probability() int {
	switch s {
	case StageLead:
		return 10
	case StageQualified:
		return 25
	case StageEstimateSent:
		return 50
	case StageVerbalCommitment:
		return 75
	case StagePermitApplied:
		return 85
	case StagePermitIssued:
		return 100
	case StageLost:
		return 0
	}
	return 0
}

// IsTerminal reports whether the stage cannot transition further.
// PERMIT_ISSUED is the success terminal; LOST is the failure terminal.
func (s PipelineStage) IsTerminal() bool {
	return s == StagePermitIssued || s == StageLost
}

// AllowedTransitions returns the stages a prospect at `from` may advance
// to. The pipeline is strictly forward; LOST is reachable from any
// non-terminal stage.
func AllowedTransitions(from PipelineStage) []PipelineStage {
	switch from {
	case StageLead:
		return []PipelineStage{StageQualified, StageLost}
	case StageQualified:
		return []PipelineStage{StageEstimateSent, StageLost}
	case StageEstimateSent:
		return []PipelineStage{StageVerbalCommitment, StageLost}
	case StageVerbalCommitment:
		return []PipelineStage{StagePermitApplied, StageLost}
	case StagePermitApplied:
		return []PipelineStage{StagePermitIssued, StageLost}
	case StagePermitIssued, StageLost:
		return nil
	}
	return nil
}

// CanTransition reports whether from → to is a permitted state change.
// Returns false for unknown stages, terminal-source transitions, and any
// "skip-the-line" attempts (e.g., LEAD → PERMIT_ISSUED).
func CanTransition(from, to PipelineStage) bool {
	for _, allowed := range AllowedTransitions(from) {
		if allowed == to {
			return true
		}
	}
	return false
}

// Prospect is one CRM Kanban row.
type Prospect struct {
	ID             uuid.UUID     `json:"id"`
	OrgID          uuid.UUID     `json:"org_id"`
	Name           string        `json:"name"`
	ClientName     string        `json:"client_name"`
	ClientEmail    *string       `json:"client_email,omitempty"`
	ClientPhone    *string       `json:"client_phone,omitempty"`
	Address        *string       `json:"address,omitempty"`
	GSF            *int          `json:"gsf,omitempty"`
	PipelineStage  PipelineStage `json:"pipeline_stage"`
	ProbabilityPct int           `json:"probability_pct"`
	Source         *string       `json:"source,omitempty"`
	Notes          *string       `json:"notes,omitempty"`
	LostReason     *string       `json:"lost_reason,omitempty"`
	ProjectID      *uuid.UUID    `json:"project_id,omitempty"` // set on PERMIT_ISSUED
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// PipelineEstimate is a versioned pre-permit estimate with structured
// line items. Currency lives once on the row; line items inherit it
// (no per-line currency, no cross-currency arithmetic risk inside an
// estimate).
type PipelineEstimate struct {
	ID                  uuid.UUID                 `json:"id"`
	ProspectID          uuid.UUID                 `json:"prospect_id"`
	Version             int                       `json:"version"`
	TotalEstimatedCents int64                     `json:"total_estimated_cents"`
	CurrencyCode        string                    `json:"currency_code"`
	LineItems           PipelineEstimateLineItems `json:"line_items"`
	MarginPct           int                       `json:"margin_pct"`
	Status              string                    `json:"status"` // draft, sent, revised, accepted
	SentAt              *time.Time                `json:"sent_at,omitempty"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

// Estimate statuses.
const (
	EstimateStatusDraft    = "draft"
	EstimateStatusSent     = "sent"
	EstimateStatusRevised  = "revised"
	EstimateStatusAccepted = "accepted"
)

// IsValidEstimateStatus reports whether s is one of the allowed values.
func IsValidEstimateStatus(s string) bool {
	switch s {
	case EstimateStatusDraft, EstimateStatusSent, EstimateStatusRevised, EstimateStatusAccepted:
		return true
	default:
		return false
	}
}

// CanTransitionEstimateStatus reports whether the from→to status change
// is permitted. Accepted is terminal; no-op self-transitions are
// rejected; any forward move within the allowed set is otherwise OK
// (we don't model "draft must precede revised" because real workflows
// re-edit drafts in revision-requested cycles).
func CanTransitionEstimateStatus(from, to string) bool {
	if from == to {
		return false
	}
	if from == EstimateStatusAccepted {
		return false
	}
	if !IsValidEstimateStatus(from) || !IsValidEstimateStatus(to) {
		return false
	}
	return true
}

// PipelineEstimateLineItem is one row of an estimate. estimated_cents
// uses the parent estimate's currency_code.
type PipelineEstimateLineItem struct {
	WBSCode        string `json:"wbs_code"`
	Description    string `json:"description"`
	EstimatedCents int64  `json:"estimated_cents"`
	Unit           string `json:"unit,omitempty"`
	Quantity       int    `json:"quantity,omitempty"`
}

// PipelineEstimateLineItems is a slice that scans from a JSONB column
// and serializes back as JSON for inserts.
type PipelineEstimateLineItems []PipelineEstimateLineItem

// Scan implements sql.Scanner for JSONB columns. Accepts []byte, string,
// or nil. Empty bytes scan to a nil slice.
func (l *PipelineEstimateLineItems) Scan(src any) error {
	if src == nil {
		*l = nil
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("PipelineEstimateLineItems.Scan: unsupported type %T", src)
	}
	if len(data) == 0 {
		*l = nil
		return nil
	}
	return json.Unmarshal(data, l)
}

// Permit is a municipal permit tracked while a prospect waits for the
// jurisdiction to approve construction.
type Permit struct {
	ID                uuid.UUID  `json:"id"`
	ProspectID        uuid.UUID  `json:"prospect_id"`
	PermitType        string     `json:"permit_type"`  // building, electrical, plumbing, mechanical
	Jurisdiction      string     `json:"jurisdiction"` // city/county that issues
	ApplicationNumber *string    `json:"application_number,omitempty"`
	SubmittedDate     *time.Time `json:"submitted_date,omitempty"`
	ExpectedIssueDate *time.Time `json:"expected_issue_date,omitempty"`
	ActualIssueDate   *time.Time `json:"actual_issue_date,omitempty"`
	FeeCents          int64      `json:"fee_cents"`
	FeeCurrencyCode   string     `json:"fee_currency_code"`
	Status            string     `json:"status"` // not_submitted, submitted, under_review, revisions_requested, approved, denied
	Notes             *string    `json:"notes,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Permit statuses.
const (
	PermitStatusNotSubmitted       = "not_submitted"
	PermitStatusSubmitted          = "submitted"
	PermitStatusUnderReview        = "under_review"
	PermitStatusRevisionsRequested = "revisions_requested"
	PermitStatusApproved           = "approved"
	PermitStatusDenied             = "denied"
)

// IsValidPermitStatus reports whether s is one of the allowed values.
func IsValidPermitStatus(s string) bool {
	switch s {
	case PermitStatusNotSubmitted, PermitStatusSubmitted, PermitStatusUnderReview,
		PermitStatusRevisionsRequested, PermitStatusApproved, PermitStatusDenied:
		return true
	default:
		return false
	}
}

// CanTransitionPermitStatus reports whether the from→to status change
// is permitted. Approved and Denied are terminal. Self-transitions
// rejected. Other transitions are allowed because municipal permit
// workflows are messy in practice (back-dating, parallel applications,
// etc.) — the service layer trusts the bookkeeper updating status.
func CanTransitionPermitStatus(from, to string) bool {
	if from == to {
		return false
	}
	if from == PermitStatusApproved || from == PermitStatusDenied {
		return false
	}
	if !IsValidPermitStatus(from) || !IsValidPermitStatus(to) {
		return false
	}
	return true
}

// ProspectWithDetails bundles a prospect with its full set of estimates
// and permits. Returned by GET /pipeline/prospects/{id} so the client
// can render the detail view in a single round-trip.
type ProspectWithDetails struct {
	Prospect  Prospect           `json:"prospect"`
	Estimates []PipelineEstimate `json:"estimates"`
	Permits   []Permit           `json:"permits"`
}
