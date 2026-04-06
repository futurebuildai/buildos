// Package tribunal contains types and logic for The Tribunal consensus system.
package tribunal

import (
	"errors"
	"time"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a decision is not found.
var ErrNotFound = errors.New("decision not found")

// DecisionStatus represents the outcome of a Tribunal decision.
// Maps to tribunal_decision_status enum in database.
type DecisionStatus string

const (
	DecisionPending  DecisionStatus = "pending"
	DecisionApproved DecisionStatus = "APPROVED"
	DecisionRejected DecisionStatus = "REJECTED"
	DecisionConflict DecisionStatus = "CONFLICT"
)

// VoteType represents an individual model's vote.
// Maps to tribunal_vote_type enum in database.
type VoteType string

const (
	VoteYea     VoteType = "YEA"
	VoteNay     VoteType = "NAY"
	VoteAbstain VoteType = "ABSTAIN"
)

// ModelVote represents a single model's vote in a Tribunal decision.
type ModelVote struct {
	ID         uuid.UUID `json:"id,omitempty"`
	DecisionID uuid.UUID `json:"decision_id,omitempty"`
	ExpertRole string    `json:"expert_role"`
	Vote       VoteType  `json:"vote"`
	Reasoning  string    `json:"reasoning"`
	Confidence float32   `json:"confidence,omitempty"`
	ModelUsed  string    `json:"model_used,omitempty"`
	LatencyMs  int       `json:"latency_ms"`
	TokenCount int       `json:"token_count,omitempty"`
}

// DecisionSummary is the list view response for tribunal decisions.
type DecisionSummary struct {
	ID              uuid.UUID      `json:"id"`
	CaseID          string         `json:"case_id"`
	Status          DecisionStatus `json:"status"`
	Category        string         `json:"category"`
	Description     string         `json:"description"`
	Timestamp       time.Time      `json:"timestamp"`
	ModelsConsulted []string       `json:"models_consulted"`
}

// DecisionDetail is the full detail view response including individual model votes.
type DecisionDetail struct {
	ID             uuid.UUID      `json:"id"`
	CaseID         string         `json:"case_id"`
	Status         DecisionStatus `json:"status"`
	Category       string         `json:"category"`
	Description    string         `json:"description"`
	ConsensusScore float64        `json:"consensus_score"`
	Votes          []ModelVote    `json:"votes"`
	PolicyLinks    []string       `json:"policy_links,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
}

// ListDecisionsFilter holds query parameters for filtering decisions.
type ListDecisionsFilter struct {
	Limit     int            `json:"limit"`
	Offset    int            `json:"offset"`
	Status    DecisionStatus `json:"status,omitempty"`
	Category  string         `json:"category,omitempty"`
	StartDate *time.Time     `json:"start_date,omitempty"`
	EndDate   *time.Time     `json:"end_date,omitempty"`
	Search    string         `json:"search,omitempty"`
}

// ListDecisionsResponse is the paginated response for the list endpoint.
type ListDecisionsResponse struct {
	Decisions []DecisionSummary `json:"decisions"`
	Total     int               `json:"total"`
	HasMore   bool              `json:"has_more"`
}

// TribunalRequest represents a request for a Tribunal decision.
type TribunalRequest struct {
	CaseID   string `json:"case_id"`
	Category string `json:"category"` // Category of the case (e.g., "code_review", "scheduling")
	Intent   string `json:"intent"`   // The standardized intent or problem description
	Context  string `json:"context"`  // Additional context (file snapshots, diffs)
}

// TribunalResponse represents the final consensus decision.
type TribunalResponse struct {
	DecisionID     uuid.UUID      `json:"decision_id"`
	Status         DecisionStatus `json:"status"`
	ConsensusScore float64        `json:"consensus_score"`
	Summary        string         `json:"summary"` // Synthesized reasoning
	Plan           string         `json:"plan"`    // The recommended action/plan
}

// Jury represents the panel of Claude models evaluating the case.
// All jury members use Anthropic Claude exclusively.
type Jury struct {
	Architect   ai.Client // Claude Opus 4.6 (antagonist)
	Reviewer    ai.Client // Claude Sonnet 4.5 (institutional memory)
	Coordinator ai.Client // Claude Sonnet 4.5 (synthesizer)
}

// DiagnosisRequest contains input for self-healing diagnosis.
type DiagnosisRequest struct {
	// ErrorTrace is the error message and context from the failure.
	ErrorTrace string `json:"error_trace"`

	// MethodContext identifies which operation failed.
	MethodContext string `json:"method_context"`

	// SystemState contains current configuration values for context.
	SystemState map[string]string `json:"system_state"`
}

// DiagnosisResponse contains the result of a self-healing diagnosis.
type DiagnosisResponse struct {
	// Diagnosis is the parsed fault diagnosis.
	Diagnosis string `json:"diagnosis"`

	// Confidence is how confident the model is in the diagnosis.
	Confidence float64 `json:"confidence"`

	// Reasoning explains the diagnosis.
	Reasoning string `json:"reasoning"`

	// ProposedAction describes what to do.
	ProposedAction ProposedAction `json:"proposed_action"`

	// SessionID tracks this diagnosis session.
	SessionID uuid.UUID `json:"session_id"`

	// LatencyMs is the time taken to complete the diagnosis.
	LatencyMs int `json:"latency_ms"`

	// ModelUsed identifies which model performed the diagnosis.
	ModelUsed string `json:"model_used"`
}

// ProposedAction describes a safe remediation action.
type ProposedAction struct {
	Type  string `json:"type"`  // UPDATE_CONFIG, CLEAR_CACHE, RETRY, NO_OP
	Key   string `json:"key"`   // Configuration key name
	Value string `json:"value"` // New value or null
}
