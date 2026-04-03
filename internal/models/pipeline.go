package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PipelineStage represents the stages of the pre-construction pipeline.
type PipelineStage string

const (
	StageLead             PipelineStage = "LEAD"
	StageQualified        PipelineStage = "QUALIFIED"
	StageEstimateSent     PipelineStage = "ESTIMATE_SENT"
	StageVerbalCommitment PipelineStage = "VERBAL_COMMITMENT"
	StagePermitApplied    PipelineStage = "PERMIT_APPLIED"
	StagePermitIssued     PipelineStage = "PERMIT_ISSUED"
	StageLost             PipelineStage = "LOST"
)

// StageOrder defines the valid forward progression of pipeline stages.
var StageOrder = []PipelineStage{
	StageLead,
	StageQualified,
	StageEstimateSent,
	StageVerbalCommitment,
	StagePermitApplied,
	StagePermitIssued,
}

// StageProbability maps each stage to its weighted pipeline probability.
var StageProbability = map[PipelineStage]int{
	StageLead:             10,
	StageQualified:        25,
	StageEstimateSent:     50,
	StageVerbalCommitment: 75,
	StagePermitApplied:    85,
	StagePermitIssued:     100,
	StageLost:             0,
}

// Prospect represents a pre-construction CRM pipeline entry.
type Prospect struct {
	ID            uuid.UUID     `json:"id"`
	OrgID         uuid.UUID     `json:"org_id"`
	Name          string        `json:"name"`
	ClientName    string        `json:"client_name"`
	ClientEmail   *string       `json:"client_email,omitempty"`
	ClientPhone   *string       `json:"client_phone,omitempty"`
	Address       *string       `json:"address,omitempty"`
	GSF           *int          `json:"gsf,omitempty"`
	PipelineStage PipelineStage `json:"pipeline_stage"`
	ProbabilityPct int          `json:"probability_pct"`
	Source        *string       `json:"source,omitempty"`
	Notes         *string       `json:"notes,omitempty"`
	LostReason    *string       `json:"lost_reason,omitempty"`
	ProjectID     *uuid.UUID    `json:"project_id,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// ProspectDetail is a Prospect with its associated estimates and permits.
type ProspectDetail struct {
	Prospect
	Estimates []PipelineEstimate `json:"estimates"`
	Permits   []Permit           `json:"permits"`
}

// PipelineEstimate represents a preliminary cost estimate for a prospect.
type PipelineEstimate struct {
	ID                  uuid.UUID       `json:"id"`
	ProspectID          uuid.UUID       `json:"prospect_id"`
	Version             int             `json:"version"`
	TotalEstimatedCents int64           `json:"total_estimated_cents"`
	CurrencyCode        string          `json:"currency_code"`
	LineItems           json.RawMessage `json:"line_items"`
	MarginPct           int             `json:"margin_pct"`
	Status              string          `json:"status"`
	SentAt              *time.Time      `json:"sent_at,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// EstimateStatus constants.
const (
	EstimateStatusDraft    = "draft"
	EstimateStatusSent     = "sent"
	EstimateStatusRevised  = "revised"
	EstimateStatusAccepted = "accepted"
)

// Permit represents a municipal permit tracked for a prospect.
type Permit struct {
	ID                uuid.UUID  `json:"id"`
	ProspectID        uuid.UUID  `json:"prospect_id"`
	PermitType        string     `json:"permit_type"`
	Jurisdiction      string     `json:"jurisdiction"`
	ApplicationNumber *string    `json:"application_number,omitempty"`
	SubmittedDate     *time.Time `json:"submitted_date,omitempty"`
	ExpectedIssueDate *time.Time `json:"expected_issue_date,omitempty"`
	ActualIssueDate   *time.Time `json:"actual_issue_date,omitempty"`
	FeeCents          int64      `json:"fee_cents"`
	FeeCurrencyCode   string     `json:"fee_currency_code"`
	Status            string     `json:"status"`
	Notes             *string    `json:"notes,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// PermitStatus constants.
const (
	PermitStatusNotSubmitted      = "not_submitted"
	PermitStatusSubmitted         = "submitted"
	PermitStatusUnderReview       = "under_review"
	PermitStatusRevisionsRequested = "revisions_requested"
	PermitStatusApproved          = "approved"
	PermitStatusDenied            = "denied"
)

// PipelineAnalytics is the weighted revenue forecast grouped by currency.
type PipelineAnalytics struct {
	CurrencyCode       string `json:"currency_code"`
	TotalProspects     int    `json:"total_prospects"`
	WeightedRevenueCents int64 `json:"weighted_revenue_cents"`
	ByStage            []StageAnalytics `json:"by_stage"`
}

// StageAnalytics is per-stage breakdown within pipeline analytics.
type StageAnalytics struct {
	Stage              PipelineStage `json:"stage"`
	Count              int           `json:"count"`
	TotalEstimatedCents int64       `json:"total_estimated_cents"`
	WeightedCents      int64        `json:"weighted_cents"`
}
