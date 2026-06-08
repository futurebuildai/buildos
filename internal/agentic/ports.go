package agentic

import (
	"context"

	"github.com/google/uuid"
)

// Reasoner is the judgment port: given an engine-computed CascadeContext, it
// returns the advisory CascadePlan. The concrete adapter (in internal/service)
// wraps the native AI client. Implementations that cannot reach a configured
// model SHOULD return a wrapped ErrReasonerUnavailable so the orchestrator can
// soft-fail (skip the cascade) rather than surface an error — AI is advisory,
// and a missing key must never block the deterministic flow.
type Reasoner interface {
	PlanCascade(ctx context.Context, c CascadeContext) (CascadePlan, error)
}

// CascadeWorkspace is the data/effect port. It owns the transaction boundary
// so agentic stays free of pgx and any store coupling: LoadCascadeContext
// reads the engine-computed snapshot for a slipped project, and ApplyCascade
// persists the reasoner's plan (feed cards + audit) atomically. The adapter in
// internal/service implements both against the real stores inside one tx.
type CascadeWorkspace interface {
	// LoadCascadeContext gathers the slipped-schedule / procurement / budget
	// snapshot for the org+project. A returned context with no critical
	// slipped tasks signals the orchestrator to no-op.
	LoadCascadeContext(ctx context.Context, orgID, projectID uuid.UUID) (CascadeContext, error)

	// ApplyCascade persists the advisory plan (feed cards + audit) for the
	// org+project in a single transaction and reports what it created.
	ApplyCascade(ctx context.Context, orgID, projectID uuid.UUID, plan CascadePlan) (CascadeResult, error)
}
