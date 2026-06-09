package agentic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Experience is the conversational-assistant capability. Registered in
// NewRegistry() so the orchestrator's capability gate (the Phase-3 disable
// seam) governs it like delay_cascade / foresight.
const Experience Capability = "experience"

// ErrAssistantUnavailable signals no AI planner is available (no Anthropic key,
// or a worker/no-AI binary). Converse propagates it so the handler soft-fails
// to 503. Mirrors ErrReasonerUnavailable.
var ErrAssistantUnavailable = errors.New("agentic: assistant unavailable")

// Default loop bounds. A zero LoopBounds field falls back to these via
// LoopBounds.withDefaults so callers can leave fields unset.
const (
	defaultMaxIterations   = 6
	defaultMaxToolCalls    = 12
	defaultMaxToolsPerTurn = 4
	defaultMaxResultBytes  = 256 * 1024 // 256 KiB
	defaultTimeout         = 30 * time.Second
)

// LoopBounds caps cost + runtime. Zero fields take safe defaults (see §7).
type LoopBounds struct {
	MaxIterations   int           // model<->server round-trips. Default 6.
	MaxToolCalls    int           // total tool executions across the run. Default 12.
	MaxToolsPerTurn int           // tool_use blocks honored per response. Default 4.
	MaxResultBytes  int           // cap on cumulative tool-result bytes fed back. Default 256 KiB.
	Timeout         time.Duration // wall-clock for the whole loop. Default 30s.
}

// withDefaults returns a copy of b with every non-positive field replaced by
// its safe default, so an unset LoopBounds is still fully bounded.
func (b LoopBounds) withDefaults() LoopBounds {
	if b.MaxIterations <= 0 {
		b.MaxIterations = defaultMaxIterations
	}
	if b.MaxToolCalls <= 0 {
		b.MaxToolCalls = defaultMaxToolCalls
	}
	if b.MaxToolsPerTurn <= 0 {
		b.MaxToolsPerTurn = defaultMaxToolsPerTurn
	}
	if b.MaxResultBytes <= 0 {
		b.MaxResultBytes = defaultMaxResultBytes
	}
	if b.Timeout <= 0 {
		b.Timeout = defaultTimeout
	}
	return b
}

// ChatTurn is one prior conversational turn. Roles are "user"/"assistant"
// only; the server never accepts a client-supplied tool_result.
type ChatTurn struct {
	Role string // "user" | "assistant"
	Text string
}

// ChatInput is one stateless conversational request. History is
// CLIENT-SUPPLIED and UNTRUSTED (no server persistence in 2c — see §10).
type ChatInput struct {
	Messages []ChatTurn // prior turns + the new user turn (last)
}

// ChatResult is the grounded answer plus a transparency trail.
type ChatResult struct {
	Reply         string          // model's final natural-language synthesis
	ToolCallsMade []ToolCallTrace // name + isError per executed tool (NOT args/results)
	Iterations    int
	Truncated     bool // hit a bound before end_turn
}

// ToolCallTrace is the wire-safe transparency surface: name + error flag only.
// Args/results are NOT returned to the client (they may carry Confidential data
// scrubbed for the model but not the wire). Full args/results go to audit_log.
type ToolCallTrace struct {
	Name    string
	IsError bool
}

// ChatPlanner is the model-loop PORT. Its sole adapter lives in
// internal/service over *ai.Client.RunToolLoop. agentic declares it and never
// imports internal/ai. A no-key org surfaces as ErrAssistantUnavailable
// (translated by the adapter from ai.ErrUnconfigured) so Converse soft-fails.
type ChatPlanner interface {
	Plan(ctx context.Context, sys string, in ChatInput, reg *AssistantRegistry, b LoopBounds) (ChatResult, error)
}

// Assistant is the experience orchestrator. Like Orchestrator /
// ForesightOrchestrator it holds only ports, the capability registry, the loop
// bounds, and a logger — no AI client, no store, no pgx.
type Assistant struct {
	planner  ChatPlanner
	registry *Registry // shared capability registry (Experience gate)
	bounds   LoopBounds
	logger   *slog.Logger
}

// NewAssistant wires the planner port + bounds + logger. A nil logger becomes
// slog.Default(). The capability registry is seeded in-code (NewRegistry).
// Loop bounds are normalized so an unset field still gets its safe default.
func NewAssistant(p ChatPlanner, b LoopBounds, logger *slog.Logger) *Assistant {
	if logger == nil {
		logger = slog.Default()
	}
	return &Assistant{
		planner:  p,
		registry: NewRegistry(),
		bounds:   b.withDefaults(),
		logger:   logger,
	}
}

// Converse gates on the Experience capability (Phase-3 disable seam), then
// delegates the bounded loop to the planner over the caller-scoped registry.
// ErrAssistantUnavailable is propagated for the service/handler to soft-fail to
// 503. The registry is built by the SERVICE per-request, already bound to
// caller org+role. agentic itself holds no caller identity.
func (a *Assistant) Converse(ctx context.Context, sys string, in ChatInput, reg *AssistantRegistry) (ChatResult, error) {
	if _, ok := a.registry.Lookup(Experience); !ok {
		// The capability isn't registered/enabled. In Phase 2c the in-code
		// registry always seeds experience so this never trips; the gate is the
		// seam Phase 3's configurable registry uses to disable the assistant per
		// deployment without a code change.
		return ChatResult{}, fmt.Errorf("agentic: capability %q not registered", Experience)
	}

	res, err := a.planner.Plan(ctx, sys, in, reg, a.bounds)
	if err != nil {
		if errors.Is(err, ErrAssistantUnavailable) {
			// AI is advisory: a no-key org surfaces here. Propagate the sentinel
			// so the service/handler soft-fails to 503 rather than 500.
			a.logger.WarnContext(ctx, "assistant unavailable",
				slog.Any("error", err))
			return ChatResult{}, err
		}
		return ChatResult{}, fmt.Errorf("agentic: assistant plan: %w", err)
	}
	return res, nil
}
