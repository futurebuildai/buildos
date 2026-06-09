package agentic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
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
// ForesightOrchestrator it holds only ports, the capability registry, the
// per-org config resolver, the loop bounds, and a logger — no AI client, no
// store, no pgx.
type Assistant struct {
	planner  ChatPlanner
	registry *Registry      // static capability catalog (Experience known?)
	resolver ConfigResolver // per-org enabled state; nil => enabled-with-default
	bounds   LoopBounds
	logger   *slog.Logger
}

// NewAssistant wires the planner port + config resolver + bounds + logger. A nil
// logger becomes slog.Default(). The capability catalog is seeded in-code
// (NewRegistry). A nil resolver means "enabled with the catalog default"
// (pre-3a behavior). Loop bounds are normalized so an unset field still gets its
// safe default.
func NewAssistant(p ChatPlanner, resolver ConfigResolver, b LoopBounds, logger *slog.Logger) *Assistant {
	if logger == nil {
		logger = slog.Default()
	}
	return &Assistant{
		planner:  p,
		registry: NewRegistry(),
		resolver: resolver,
		bounds:   b.withDefaults(),
		logger:   logger,
	}
}

// resolveEnabled returns the per-org CapabilityConfig for Experience. With no
// resolver wired it falls back to the catalog default (enabled-with-default).
func (a *Assistant) resolveEnabled(ctx context.Context, orgID uuid.UUID, c Capability) (CapabilityConfig, error) {
	if a.resolver == nil {
		d, _ := a.registry.Lookup(c)
		return CapabilityConfig{Enabled: d.DefaultEnabled, Config: d.DefaultConfig}, nil
	}
	return a.resolver.Resolve(ctx, orgID, c)
}

// Converse gates on the Experience capability (catalog known? + per-org enabled?)
// then delegates the bounded loop to the planner over the caller-scoped registry.
// ErrAssistantUnavailable (no AI key) is propagated for the handler to soft-fail
// to 503; ErrCapabilityDisabled (admin turned it off) is propagated for the
// handler to map to a clean 403. The registry is built by the SERVICE per-request,
// already bound to caller org+role. orgID is used ONLY to key resolver.Resolve —
// it must NEVER influence tool scoping (that stays structural in the per-request
// registry the service seals from claims).
func (a *Assistant) Converse(ctx context.Context, orgID uuid.UUID, sys string, in ChatInput, reg *AssistantRegistry) (ChatResult, error) {
	if _, ok := a.registry.Lookup(Experience); !ok {
		// Not in the static catalog — a wiring bug. Unreachable in real wiring
		// (NewRegistry always seeds experience).
		return ChatResult{}, fmt.Errorf("agentic: capability %q not registered", Experience)
	}

	cfg, err := a.resolveEnabled(ctx, orgID, Experience)
	if err != nil {
		// Config read failure is infrastructure, not advisory: return hard so
		// the handler surfaces 5xx rather than silently running a possibly-
		// disabled assistant.
		return ChatResult{}, fmt.Errorf("agentic: resolve config: %w", err)
	}
	if !cfg.Enabled {
		a.logger.InfoContext(ctx, "experience disabled by config",
			slog.String("reason", "capability_disabled"))
		return ChatResult{}, ErrCapabilityDisabled
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
