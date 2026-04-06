// Package interfaces defines service contracts for The Tribunal consensus system.
package interfaces

import (
	"context"

	"github.com/futurebuild/futurebuild-os/internal/futureshade/types"
)

// Juror represents a single LLM provider that can deliberate on a Case.
// Implementations must support context cancellation for timeout handling.
type Juror interface {
	// Consult submits a Case to the Juror and returns their Verdict.
	// The context should be used for timeout and cancellation.
	Consult(ctx context.Context, c types.Case) (types.Verdict, error)

	// ID returns the ModelID of this Juror.
	ID() types.ModelID
}

// TheGavel decides the final outcome based on collected Verdicts.
// It implements the consensus logic (Unanimous, Majority, Supervisor).
type TheGavel interface {
	// Deliberate synthesizes multiple Verdicts into a single TribunalDecision.
	Deliberate(strategy types.ConsensusStrategy, verdicts []types.Verdict) (types.TribunalDecision, error)
}

// TribunalService is the public API for submitting Cases to The Tribunal.
// It orchestrates Juror consultations and Gavel deliberation.
//
// FAIL-SAFE REQUIREMENT: Implementations MUST fail closed (return error)
// if persistence layer is unavailable. No un-audited decisions may be made.
type TribunalService interface {
	// Adjudicate submits a Case for judgment and returns the final Decision.
	Adjudicate(ctx context.Context, c types.Case) (types.TribunalDecision, error)
}
