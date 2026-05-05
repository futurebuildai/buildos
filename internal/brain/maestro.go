package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MaestroClient wraps Brain's POST /api/maestro/chat (and session
// endpoints). The chat endpoint is the entry point for every BuildOS
// AI feature — Brain's orchestrator handles intent classification,
// Anthropic dispatch, and (when applicable) cross-product flow
// coordination.
type MaestroClient struct {
	c *Client
}

// ChatRequest mirrors Brain's POST /api/maestro/chat body.
//
// SessionID is optional: omit to start a new session, supply to
// continue. Brain creates a session row keyed on the user's sub claim.
//
// The Reply on the response is what most callers will display; richer
// fields (Intent, Classification, ToolResults) are JSON-passthrough
// because their Brain-side shapes evolve faster than this client.
type ChatRequest struct {
	SessionID *uuid.UUID `json:"session_id,omitempty"`
	Message   string     `json:"message"`
}

// ChatResponse mirrors Brain's chatResponse envelope (already unwrapped
// from the outer {data,error,meta}).
type ChatResponse struct {
	SessionID      uuid.UUID       `json:"session_id"`
	Reply          string          `json:"reply"`
	Intent         json.RawMessage `json:"intent"`
	Classification json.RawMessage `json:"classification"`
	ToolResults    json.RawMessage `json:"tool_results,omitempty"`
}

// Chat sends a message to Brain's Maestro orchestrator and returns the
// reply. Validates the request locally so a malformed call doesn't
// burn an HTTP round-trip.
func (m *MaestroClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Message == "" {
		return nil, fmt.Errorf("brain.Maestro.Chat: message is required")
	}
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	raw, err := m.c.doRequest(ctx, "POST", "/api/maestro/chat", req)
	if err != nil {
		return nil, err
	}
	var resp ChatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("brain.Maestro.Chat: decode response: %w", err)
	}
	return &resp, nil
}

// withTimeout wraps the caller's ctx with the configured Maestro
// timeout, but only when the caller hasn't already set a tighter
// deadline. This lets per-call overrides (e.g., a 5s-deadline
// background job) take precedence while still bounding the LLM
// round-trip for callers that pass context.Background.
func (m *MaestroClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	t := m.c.timeouts.Maestro
	if t <= 0 {
		return ctx, func() {}
	}
	if d, ok := ctx.Deadline(); ok && time.Until(d) < t {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, t)
}

// Session is a chat session header. Real shape on Brain:
// pre_construction sessions table — id, user_sub, org_id, title,
// created_at, last_message_at.
type Session struct {
	ID            uuid.UUID `json:"id"`
	UserSub       string    `json:"user_sub"`
	OrgID         string    `json:"org_id"`
	Title         string    `json:"title"`
	CreatedAt     string    `json:"created_at"`
	LastMessageAt string    `json:"last_message_at,omitempty"`
}

// ListSessions fetches the current user's chat session headers.
func (m *MaestroClient) ListSessions(ctx context.Context) ([]Session, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	raw, err := m.c.doRequest(ctx, "GET", "/api/maestro/sessions", nil)
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(raw, &sessions); err != nil {
		return nil, fmt.Errorf("brain.Maestro.ListSessions: decode response: %w", err)
	}
	return sessions, nil
}

// Message is one chat message inside a session.
type Message struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	Role      string    `json:"role"` // "user" | "assistant" | "system"
	Content   string    `json:"content"`
	CreatedAt string    `json:"created_at"`
}

// GetSession returns the messages for a session in chronological order.
func (m *MaestroClient) GetSession(ctx context.Context, sessionID uuid.UUID) ([]Message, error) {
	if sessionID == uuid.Nil {
		return nil, fmt.Errorf("brain.Maestro.GetSession: session_id is required")
	}
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	raw, err := m.c.doRequest(ctx, "GET", "/api/maestro/sessions/"+sessionID.String(), nil)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, fmt.Errorf("brain.Maestro.GetSession: decode response: %w", err)
	}
	return msgs, nil
}
