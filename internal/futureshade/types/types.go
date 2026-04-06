// Package types defines domain types for The Tribunal consensus system.
package types

// ModelID identifies an LLM provider used as a Juror.
type ModelID string

const (
	// ModelClaudeOpus is the Claude Opus 4.6 model (primary antagonist).
	ModelClaudeOpus ModelID = "claude-opus"
	// ModelClaudeSonnet is the Claude Sonnet 4.5 model (reviewer/coordinator).
	ModelClaudeSonnet ModelID = "claude-sonnet"
)

// ConsensusStrategy defines how The Gavel determines a final ruling.
type ConsensusStrategy string

const (
	StrategyUnanimous  ConsensusStrategy = "UNANIMOUS"
	StrategyMajority   ConsensusStrategy = "MAJORITY_RULE"
	StrategySupervisor ConsensusStrategy = "SUPERVISOR"
)

// MaxContextSize is the maximum allowed size for Case.Context in bytes.
// This limit prevents token overflow attacks and ensures predictable costs.
// Validation must be performed at the service layer before processing.
const MaxContextSize = 100 * 1024 // 100KB

// Case represents a request for judgment submitted to The Tribunal.
type Case struct {
	ID                string            // Unique identifier
	Context           string            // File content, diffs, etc. (Max 100KB - see MaxContextSize)
	Instructions      string            // What to do with the context
	RequiredConsensus ConsensusStrategy // Strategy for reaching a decision
	Jurors            []ModelID         // Which models should deliberate
}

// Verdict is a single Juror's response to a Case.
type Verdict struct {
	ModelID    ModelID // Which Juror produced this verdict
	Content    string  // The response content
	Confidence float64 // Confidence score (0.0 - 1.0)
	LatencyMs  int64   // Time taken to produce verdict
	CostMicros int64   // Estimated cost in microdollars (BIGINT, not float)
}

// TribunalDecision is the final synthesized result from The Gavel.
type TribunalDecision struct {
	ID                 string                 // Unique identifier
	CaseID             string                 // Reference to the originating Case
	FinalRuling        string                 // The synthesized decision
	ConsensusReached   bool                   // Whether consensus was achieved
	DissentingOpinions []Verdict              // Verdicts that disagreed with the ruling
	Metadata           map[string]interface{} // Additional context (timing, cost totals, etc.)
}
