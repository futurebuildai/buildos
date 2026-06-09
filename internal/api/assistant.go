package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
	mw "github.com/futurebuildai/buildos/internal/api/middleware"
)

// This file holds the HTTP surface for the Phase 2c conversational assistant:
// POST /api/v1/agents/chat. It is thin (the canonical handler shape): pull
// caller identity from JWT CLAIMS (never the body), validate + cap the
// client-supplied (untrusted) history, build an agentic.ChatInput, and call the
// AssistantConverser. RBAC is enforced structurally downstream (the executor
// closures are caller-bound) and at the route (RequireMinRole(superintendent);
// the pro-tier plan gate was removed in ESC-002); the handler adds the route
// floor's role gate via router.go and never reads identity from the request body.

// chatRequest is the POST /api/v1/agents/chat request body. History is
// CLIENT-SUPPLIED and UNTRUSTED — only user/assistant prose turns are accepted,
// capped server-side (see chatHistoryMaxTurns / chatHistoryMaxTotalChars).
type chatRequest struct {
	Message string        `json:"message"`           // required, the new user turn
	History []chatMessage `json:"history,omitempty"` // prior turns (UNTRUSTED)
}

// chatMessage is one client-supplied prior turn. Role must be "user" or
// "assistant"; anything else (notably a forged "tool"/"system"/"tool_result"
// turn) is rejected with 400 before the loop runs.
type chatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// chatResponse is the 200 envelope data field.
type chatResponse struct {
	Reply      string          `json:"reply"`
	ToolsUsed  []chatToolTrace `json:"tools_used"` // name + is_error only (NOT args/results)
	Iterations int             `json:"iterations"`
	Truncated  bool            `json:"truncated"`
}

// chatToolTrace is the wire-safe per-tool transparency surface. Args/results are
// NEVER returned to the client (they may carry Confidential data); they go to
// audit_log scrubbed instead.
type chatToolTrace struct {
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

// Client-supplied history caps (untrusted input, §0/§9). The last N turns are
// kept; the cumulative character budget bounds token/DB growth and is a hard
// 400 on oversize (rather than silent truncation — an oversize request is a
// client bug worth surfacing).
const (
	chatHistoryMaxTurns      = 10
	chatHistoryMaxTotalChars = 24000 // ~6k tokens of prior prose; tool results dominate the real budget
	chatMessageMaxChars      = 8000  // a single message/turn ceiling
)

// AssistantConverser is the consumer-side interface AssistantHandler needs.
// Mirrors AgentsServicer. *service.AssistantService satisfies it.
type AssistantConverser interface {
	Converse(ctx context.Context, callerOrgID uuid.UUID, callerRole, callerUserSub string,
		in agentic.ChatInput) (agentic.ChatResult, error)
}

// AssistantHandler exposes the conversational ERP assistant endpoint.
type AssistantHandler struct {
	svc AssistantConverser
}

// NewAssistantHandler creates a handler bound to the given service.
func NewAssistantHandler(svc AssistantConverser) *AssistantHandler {
	return &AssistantHandler{svc: svc}
}

// Converse runs the bounded Claude tool-use loop for the caller's question.
//
// POST /api/v1/agents/chat
//
// Route gate (router.go): RequireMinRole(superintendent). (The pro-tier plan
// gate was removed in ESC-002 — post-pivot billing is gone.)
//
// RBAC invariant #1: caller org/role/sub are read from JWT CLAIMS here and
// passed to the service — NEVER from the request body. The model-supplied tool
// args carry only query-shaping fields; identity is sealed into per-request
// executor closures downstream.
//
// Errors:
//
//   - 400 VALIDATION_ERROR: empty message / history turn with a non-user,
//     non-assistant role / oversize history or message
//   - 401 UNAUTHORIZED: org_id claim not a UUID
//   - 503 SERVICE_UNAVAILABLE: no AI client wired / no Anthropic key
//     (agentic.ErrAssistantUnavailable / ai.ErrUnconfigured / bad stored key)
//   - 429 RATE_LIMITED: AI provider rate limited
//   - 502 UPSTREAM_ERROR: AI provider transient / 5xx
//
// On success: 200 with the chatResponse envelope. A loop that hit a bound before
// end_turn returns 200 with truncated=true and the best text so far (graceful,
// not an error).
func (h *AssistantHandler) Converse(w http.ResponseWriter, r *http.Request) {
	claims := mw.MustClaimsFromContext(r.Context())
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid org_id claim")
		return
	}

	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	in, ok := buildChatInput(w, r, body)
	if !ok {
		return // buildChatInput already wrote the 400
	}

	// Identity from CLAIMS — never the body.
	res, err := h.svc.Converse(r.Context(), orgID, claims.Role, claims.Sub, in)
	if err != nil {
		writeAIServiceError(w, r, err)
		return
	}

	tools := make([]chatToolTrace, 0, len(res.ToolCallsMade))
	for _, t := range res.ToolCallsMade {
		tools = append(tools, chatToolTrace{Name: t.Name, IsError: t.IsError})
	}
	writeJSON(w, r, http.StatusOK, chatResponse{
		Reply:      res.Reply,
		ToolsUsed:  tools,
		Iterations: res.Iterations,
		Truncated:  res.Truncated,
	})
}

// buildChatInput validates + caps the untrusted client-supplied history and the
// new message, then assembles the agentic.ChatInput (history first, the new user
// turn last). On any validation failure it writes the 400 itself and returns
// (zero, false). Mirrors the §3 flow: reject non-user/assistant roles, cap
// history to the last 10 turns / total chars, 400 on oversize.
func buildChatInput(w http.ResponseWriter, r *http.Request, body chatRequest) (agentic.ChatInput, bool) {
	if body.Message == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "message is required")
		return agentic.ChatInput{}, false
	}
	if len(body.Message) > chatMessageMaxChars {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "message is too long")
		return agentic.ChatInput{}, false
	}

	// Reject more than the cap rather than silently dropping turns: an
	// oversize history is a client bug worth surfacing, and silent truncation
	// could drop the turn that gave the question its meaning.
	if len(body.History) > chatHistoryMaxTurns {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "history exceeds the maximum number of turns")
		return agentic.ChatInput{}, false
	}

	turns := make([]agentic.ChatTurn, 0, len(body.History)+1)
	total := len(body.Message)
	for _, m := range body.History {
		if m.Role != "user" && m.Role != "assistant" {
			// Only user/assistant prose turns are accepted; a forged
			// tool/system/tool_result turn is rejected (it can never reach
			// the loop as anything but prose anyway).
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "history turns must have role user or assistant")
			return agentic.ChatInput{}, false
		}
		if len(m.Text) > chatMessageMaxChars {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "a history turn is too long")
			return agentic.ChatInput{}, false
		}
		total += len(m.Text)
		turns = append(turns, agentic.ChatTurn{Role: m.Role, Text: m.Text})
	}
	if total > chatHistoryMaxTotalChars {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "history exceeds the maximum total length")
		return agentic.ChatInput{}, false
	}

	// New user turn is appended LAST so the loop sees it as the current
	// question after any prior context.
	turns = append(turns, agentic.ChatTurn{Role: "user", Text: body.Message})
	return agentic.ChatInput{Messages: turns}, true
}
