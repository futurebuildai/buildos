package ai

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// ---- chatloop bounds defaults -----------------------------------------

// These mirror the agentic.LoopBounds §6 defaults. The service adapter
// normalizes agentic.LoopBounds before mapping them onto ToolLoopBounds,
// so by the time RunToolLoop sees them they are usually already positive.
// withDefaults() is the belt-and-suspenders floor for a request that
// reaches the loop with a zero/negative bound (e.g. a direct ai caller).
const (
	defaultLoopMaxIterations   = 6
	defaultLoopMaxToolCalls    = 12
	defaultLoopMaxToolsPerTurn = 4
	defaultLoopMaxResultBytes  = 256 * 1024 // 256 KiB
	// A multi-iteration tool loop (e.g. "which projects have critical-path
	// risk?" → list_projects then a schedule read per project then synthesize)
	// legitimately runs several model round-trips and exceeds 30s. 90s leaves
	// headroom under both the server WriteTimeout (120s) and Cloudflare's ~100s
	// edge timeout so the answer returns instead of 502-ing mid-stream.
	defaultLoopTimeout         = 90 * time.Second

	// loopMaxTokens is the per-turn output cap. Matches callTool/callText.
	loopMaxTokens = 4096
)

// ---- chatloop wire-neutral types --------------------------------------

// ToolSpec is the ai-side mirror of a model-facing tool declaration
// (name + description + JSON Schema for its input). Kept separate from
// agentic.ToolSpec so internal/ai imports nothing from internal/agentic;
// the service adapter maps one onto the other. Rendered onto the Messages
// API tools[] array.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ToolInvoker executes one model-requested tool call during the loop. The
// service adapter implements this by dispatching to the agentic registry's
// executor for the named tool. RunToolLoop calls it; internal/ai knows
// nothing about org/role — those are sealed inside the invoker closure.
//
// The returned content is fed back to the model verbatim as a tool_result
// block. isError marks a soft failure (bad args, not-found, forbidden) so
// the model sees it and recovers in prose; it never aborts the loop. A
// non-nil err is reserved for an invoker-internal failure; RunToolLoop
// treats it like a soft error result (the loop never propagates it as a
// fatal error) but stops honoring further blocks on this turn.
type ToolInvoker interface {
	Invoke(ctx context.Context, name string, input json.RawMessage) (content string, isError bool, err error)
}

// ToolLoopBounds caps cost + runtime for one RunToolLoop run. Zero/negative
// fields fall back to the package defaults via withDefaults().
type ToolLoopBounds struct {
	MaxIterations   int           // model<->server round-trips
	MaxToolCalls    int           // total tool executions across the run
	MaxToolsPerTurn int           // tool_use blocks honored per response
	MaxResultBytes  int           // cap on cumulative tool-result bytes fed back
	Timeout         time.Duration // wall-clock for the whole loop
}

// withDefaults returns a copy with zero/non-positive fields replaced by the
// package defaults.
func (b ToolLoopBounds) withDefaults() ToolLoopBounds {
	if b.MaxIterations <= 0 {
		b.MaxIterations = defaultLoopMaxIterations
	}
	if b.MaxToolCalls <= 0 {
		b.MaxToolCalls = defaultLoopMaxToolCalls
	}
	if b.MaxToolsPerTurn <= 0 {
		b.MaxToolsPerTurn = defaultLoopMaxToolsPerTurn
	}
	if b.MaxResultBytes <= 0 {
		b.MaxResultBytes = defaultLoopMaxResultBytes
	}
	if b.Timeout <= 0 {
		b.Timeout = defaultLoopTimeout
	}
	return b
}

// ToolLoopMessage is one prior conversational turn. Role is "user" or
// "assistant" only; the loop never accepts a client-supplied tool_result.
type ToolLoopMessage struct {
	Role string // "user" | "assistant"
	Text string
}

// ToolLoopRequest is one bounded multi-tool Messages loop.
type ToolLoopRequest struct {
	Model    string
	System   string
	Messages []ToolLoopMessage // mapped from agentic ChatInput
	Tools    []ToolSpec
	Invoker  ToolInvoker
	Bounds   ToolLoopBounds // mapped from agentic LoopBounds
}

// ToolCallRecord is the per-tool transparency surface: name + error flag.
type ToolCallRecord struct {
	Name    string
	IsError bool
}

// ToolLoopResponse is the loop's outcome.
type ToolLoopResponse struct {
	FinalText  string
	ToolCalls  []ToolCallRecord
	Iterations int
	Truncated  bool // hit a bound before end_turn
}

// RunToolLoop runs the bounded multi-tool Messages loop. ToolChoice is
// "auto"; MaxTokens is 4096 per turn. Each iteration:
//
//  1. ctx deadline check (before the call), then POST /v1/messages with
//     the full message history + tools[].
//  2. stop_reason "end_turn" / "max_tokens" → collect text blocks, return.
//  3. stop_reason "tool_use" → for each tool_use block (capped at
//     MaxToolsPerTurn, globally at MaxToolCalls, cumulatively at
//     MaxResultBytes): call Invoker, build a tool_result block (is_error
//     from the invoker), append the assistant message echoing the EXACT
//     tool_use blocks + a user message of tool_result blocks (matching
//     tool_use_id), continue.
//  4. ctx deadline (Bounds.Timeout) or MaxIterations exhausted → return the
//     best text so far with Truncated=true and a nil error (loop-safety).
//
// Inherits messages() retry + circuit breaker + per-org key resolution (org
// id from ContextWithOrgID). Returns ai.ErrUnconfigured when no key, so the
// caller soft-fails.
func (c *Client) RunToolLoop(ctx context.Context, kind string, req ToolLoopRequest) (*ToolLoopResponse, error) {
	bounds := req.Bounds.withDefaults()

	// Wall-clock budget for the whole loop, layered over messages()'s
	// per-call retry/circuit budget.
	loopCtx, cancel := context.WithTimeout(ctx, bounds.Timeout)
	defer cancel()

	orgID := orgIDFromCtx(ctx)
	tools := mapToolSpecs(req.Tools)

	// Running conversation, seeded with the (already validated) history.
	msgs := make([]messageParam, 0, len(req.Messages)+8)
	for _, m := range req.Messages {
		msgs = append(msgs, messageParam{
			Role:    m.Role,
			Content: []contentBlock{textBlock(m.Text)},
		})
	}

	resp := &ToolLoopResponse{}
	var bestText string
	resultBytes := 0

	for iter := 0; iter < bounds.MaxIterations; iter++ {
		// (1) Deadline check before each model call. On a fired deadline
		// (or parent cancellation), bail gracefully with the best text so
		// far rather than surfacing a context error mid-loop.
		if loopCtx.Err() != nil {
			resp.Truncated = true
			resp.FinalText = bestText
			return finalize(c, ctx, kind, req, resp, bestText), nil
		}

		mreq := messagesRequest{
			Model:      modelOrDefault(c, req.Model),
			MaxTokens:  loopMaxTokens,
			System:     req.System,
			Messages:   msgs,
			Tools:      tools,
			ToolChoice: &toolChoice{Type: "auto"},
		}

		mresp, err := c.messages(loopCtx, kind, orgID, mreq)
		if err != nil {
			// A fired loop deadline manifests as a ctx error from
			// messages(); treat that as graceful truncation, everything
			// else (ErrUnconfigured / ErrRateLimited / ErrTransient /
			// *HTTPError) propagates so the caller soft-fails.
			if loopCtx.Err() != nil && (ctx.Err() == nil) {
				resp.Truncated = true
				resp.FinalText = bestText
				return finalize(c, ctx, kind, req, resp, bestText), nil
			}
			return nil, err
		}
		resp.Iterations = iter + 1

		// Capture any text the model emitted this turn as the running
		// best (used for graceful truncation).
		if txt := collectText(mresp.Content); txt != "" {
			bestText = txt
		}

		// (2) Terminal stop reasons → done.
		if mresp.StopReason != "tool_use" {
			resp.FinalText = collectText(mresp.Content)
			return resp, nil
		}

		// (3) tool_use → honor blocks under the bounds, build tool_results.
		toolUses := make([]contentBlock, 0, bounds.MaxToolsPerTurn)
		results := make([]contentBlock, 0, bounds.MaxToolsPerTurn)
		perTurn := 0
		for _, blk := range mresp.Content {
			if blk.Type != "tool_use" {
				continue
			}
			// Per-turn cap, global call cap, byte budget — any breach
			// stops honoring further blocks this turn (and trips
			// truncation below).
			if perTurn >= bounds.MaxToolsPerTurn {
				resp.Truncated = true
				break
			}
			if len(resp.ToolCalls) >= bounds.MaxToolCalls {
				resp.Truncated = true
				break
			}
			if resultBytes >= bounds.MaxResultBytes {
				resp.Truncated = true
				break
			}

			content, isErr, invErr := req.Invoker.Invoke(loopCtx, blk.Name, blk.Input)
			if invErr != nil {
				// An invoker-internal failure becomes a soft error
				// result; the loop never propagates it as fatal.
				content = "tool execution failed"
				isErr = true
			}

			resp.ToolCalls = append(resp.ToolCalls, ToolCallRecord{Name: blk.Name, IsError: isErr})

			// Echo the EXACT tool_use block on the assistant turn.
			toolUses = append(toolUses, blk)
			// Matching tool_result on the following user turn.
			results = append(results, contentBlock{
				Type:      "tool_result",
				ToolUseID: blk.ID,
				Content:   content,
				IsError:   isErr,
			})
			resultBytes += len(content)
			perTurn++
		}

		// Defensive: a "tool_use" stop_reason with no honored block (all
		// over budget, or malformed) — bail gracefully rather than send
		// an assistant turn with no tool_use / a user turn with no
		// tool_result (Anthropic 400s on either).
		if len(toolUses) == 0 {
			resp.Truncated = true
			resp.FinalText = bestText
			return finalize(c, ctx, kind, req, resp, bestText), nil
		}

		// Append the assistant turn (verbatim tool_use blocks) and the
		// user turn (matching tool_result blocks). Order is load-bearing.
		msgs = append(msgs,
			messageParam{Role: "assistant", Content: toolUses},
			messageParam{Role: "user", Content: results},
		)
	}

	// (4) MaxIterations exhausted without an end_turn → graceful truncation.
	resp.Truncated = true
	resp.FinalText = bestText
	return finalize(c, ctx, kind, req, resp, bestText), nil
}

// finalize handles the budget-exhaustion path. If the loop already has best
// text, return as-is. Otherwise attempt ONE no-tool synthesis turn under a
// FRESH short sub-context (the loop Timeout has already fired, so loopCtx is
// dead) — this is the "MaxIterations + 1" true round-trip ceiling. Any error
// on that final turn is swallowed: the caller still gets a graceful 200.
func finalize(c *Client, parent context.Context, kind string, req ToolLoopRequest, resp *ToolLoopResponse, bestText string) *ToolLoopResponse {
	if bestText != "" {
		resp.FinalText = bestText
		return resp
	}
	// No text accumulated — try a final synthesis turn under a fresh,
	// short sub-context derived from the (possibly still-live) PARENT ctx,
	// not the dead loopCtx.
	if parent.Err() != nil {
		return resp
	}
	finalCtx, cancel := context.WithTimeout(parent, defaultLoopTimeout)
	defer cancel()

	mreq := messagesRequest{
		Model:     modelOrDefault(c, req.Model),
		MaxTokens: loopMaxTokens,
		System:    req.System,
		Messages: append(mapHistory(req.Messages), messageParam{
			Role:    "user",
			Content: []contentBlock{textBlock("Summarize what you found so far. Do not call any tools.")},
		}),
		// No tools[] / tool_choice — force a plain synthesis turn.
	}
	mresp, err := c.messages(finalCtx, kind, orgIDFromCtx(parent), mreq)
	if err == nil && mresp != nil {
		resp.FinalText = collectText(mresp.Content)
	}
	return resp
}

// ---- helpers ----------------------------------------------------------

// modelOrDefault falls back to the client's heavy-reasoning model when the
// request omits one.
func modelOrDefault(c *Client, model string) string {
	if model != "" {
		return model
	}
	return c.model
}

// mapToolSpecs renders the neutral ToolSpec list onto the Messages API
// toolParam[] array. Returns nil for an empty list (omitempty drops it).
func mapToolSpecs(specs []ToolSpec) []toolParam {
	if len(specs) == 0 {
		return nil
	}
	out := make([]toolParam, 0, len(specs))
	for _, s := range specs {
		out = append(out, toolParam{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: s.InputSchema,
		})
	}
	return out
}

// mapHistory rebuilds the seed messages (text-only) for the fresh-context
// synthesis turn in finalize.
func mapHistory(in []ToolLoopMessage) []messageParam {
	out := make([]messageParam, 0, len(in)+1)
	for _, m := range in {
		out = append(out, messageParam{
			Role:    m.Role,
			Content: []contentBlock{textBlock(m.Text)},
		})
	}
	return out
}

// collectText concatenates the text blocks of a response.
func collectText(blocks []contentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type == "text" {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}
