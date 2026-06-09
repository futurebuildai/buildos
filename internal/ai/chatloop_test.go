package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---- chatloop test scaffolding ----------------------------------------

// scriptedTransport returns a queued sequence of /v1/messages responses,
// one per call, and captures the decoded request body of each call so a
// test can assert the exact wire shape (incl. tool_use_id echo).
type scriptedTransport struct {
	responses []messagesResponse
	bodies    []messagesRequest
	calls     int
}

func (s *scriptedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	var req messagesRequest
	_ = json.Unmarshal(body, &req)
	s.bodies = append(s.bodies, req)

	idx := s.calls
	s.calls++
	if idx >= len(s.responses) {
		// Default terminal text so an over-eager loop doesn't 500; a
		// well-bounded test never reaches here.
		idx = len(s.responses) - 1
	}
	out, _ := json.Marshal(s.responses[idx])
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(out))),
		Header:     make(http.Header),
	}, nil
}

// newLoopTestClient wires a Client over a scripted transport with fast
// retries.
func newLoopTestClient(t *testing.T, resolver KeyResolver, st *scriptedTransport) *Client {
	t.Helper()
	c, err := NewClient(Config{
		KeyResolver: resolver,
		BaseURL:     "http://anthropic.invalid",
		Model:       "claude-opus-4-6",
		HTTPClient:  &http.Client{Transport: st},
		Retry:       RetryConfig{MaxAttempts: 2, BaseDelayMs: 1, Multiplier: 2.0},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func toolUseResp(blocks ...contentBlock) messagesResponse {
	return messagesResponse{
		ID: "msg_tu", Type: "message", Role: "assistant", Model: "m",
		StopReason: "tool_use", Content: blocks,
	}
}

func endTurnResp(text string) messagesResponse {
	return messagesResponse{
		ID: "msg_et", Type: "message", Role: "assistant", Model: "m",
		StopReason: "end_turn",
		Content:    []contentBlock{{Type: "text", Text: text}},
	}
}

func toolUseBlock(id, name string, input any) contentBlock {
	raw, _ := json.Marshal(input)
	return contentBlock{Type: "tool_use", ID: id, Name: name, Input: raw}
}

// recordingInvoker records each Invoke and returns scripted content/isError.
type recordingInvoker struct {
	fn    func(name string, input json.RawMessage) (string, bool, error)
	names []string
}

func (r *recordingInvoker) Invoke(_ context.Context, name string, input json.RawMessage) (string, bool, error) {
	r.names = append(r.names, name)
	return r.fn(name, input)
}

func baseReq(inv ToolInvoker) ToolLoopRequest {
	schema := json.RawMessage(`{"type":"object"}`)
	return ToolLoopRequest{
		Model:  "claude-opus-4-6",
		System: "you are a test assistant",
		Messages: []ToolLoopMessage{
			{Role: "user", Text: "what's the status?"},
		},
		Tools: []ToolSpec{
			{Name: "get_thing", Description: "gets a thing", InputSchema: schema},
		},
		Invoker: inv,
	}
}

// ---- tests ------------------------------------------------------------

// TestRunToolLoop_SingleTool: one tool_use turn then an end_turn synthesis.
// Asserts the answer, the trace, iteration count, and (crucially) the EXACT
// second-request body: an assistant turn echoing the verbatim tool_use block
// and a user turn carrying the matching tool_result with the same id.
func TestRunToolLoop_SingleTool(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{
		toolUseResp(toolUseBlock("toolu_a", "get_thing", map[string]any{"project_id": "p1"})),
		endTurnResp("Framing is on track."),
	}}
	c := newLoopTestClient(t, staticKey("k"), st)

	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return `{"status":"ok"}`, false, nil
	}}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if resp.FinalText != "Framing is on track." {
		t.Errorf("FinalText = %q", resp.FinalText)
	}
	if resp.Truncated {
		t.Error("Truncated = true, want false on clean end_turn")
	}
	if resp.Iterations != 2 {
		t.Errorf("Iterations = %d, want 2", resp.Iterations)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_thing" || resp.ToolCalls[0].IsError {
		t.Errorf("ToolCalls = %+v, want one clean get_thing", resp.ToolCalls)
	}
	if len(inv.names) != 1 || inv.names[0] != "get_thing" {
		t.Errorf("invoker names = %v", inv.names)
	}

	// First request: ToolChoice auto, tools present, the seed user turn only.
	if len(st.bodies) != 2 {
		t.Fatalf("got %d requests, want 2", len(st.bodies))
	}
	first := st.bodies[0]
	if first.ToolChoice == nil || first.ToolChoice.Type != "auto" {
		t.Errorf("first ToolChoice = %+v, want type=auto", first.ToolChoice)
	}
	if len(first.Tools) != 1 || first.Tools[0].Name != "get_thing" {
		t.Errorf("first Tools = %+v", first.Tools)
	}
	if first.MaxTokens != loopMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", first.MaxTokens, loopMaxTokens)
	}
	if len(first.Messages) != 1 || first.Messages[0].Role != "user" {
		t.Fatalf("first Messages = %+v, want one user turn", first.Messages)
	}

	// Second request: seed user + assistant(tool_use echo) + user(tool_result).
	second := st.bodies[1]
	if len(second.Messages) != 3 {
		t.Fatalf("second Messages len = %d, want 3", len(second.Messages))
	}
	asst := second.Messages[1]
	if asst.Role != "assistant" {
		t.Errorf("second msg[1] role = %q, want assistant", asst.Role)
	}
	if len(asst.Content) != 1 || asst.Content[0].Type != "tool_use" || asst.Content[0].ID != "toolu_a" || asst.Content[0].Name != "get_thing" {
		t.Errorf("assistant tool_use echo = %+v", asst.Content)
	}
	userTR := second.Messages[2]
	if userTR.Role != "user" {
		t.Errorf("second msg[2] role = %q, want user", userTR.Role)
	}
	if len(userTR.Content) != 1 {
		t.Fatalf("tool_result content len = %d, want 1", len(userTR.Content))
	}
	tr := userTR.Content[0]
	if tr.Type != "tool_result" {
		t.Errorf("tool_result block type = %q", tr.Type)
	}
	if tr.ToolUseID != "toolu_a" {
		t.Errorf("tool_use_id = %q, want toolu_a (must match the tool_use id)", tr.ToolUseID)
	}
	if tr.Content != `{"status":"ok"}` {
		t.Errorf("tool_result content = %q", tr.Content)
	}
	if tr.IsError {
		t.Error("tool_result is_error = true, want false")
	}
}

// TestRunToolLoop_MultiToolPerTurn: a single response with two tool_use
// blocks → both honored, both echoed on one assistant turn, both answered on
// one user turn with matching ids.
func TestRunToolLoop_MultiToolPerTurn(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{
		toolUseResp(
			toolUseBlock("toolu_1", "get_thing", map[string]any{"a": 1}),
			toolUseBlock("toolu_2", "get_thing", map[string]any{"b": 2}),
		),
		endTurnResp("done"),
	}}
	c := newLoopTestClient(t, staticKey("k"), st)

	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return `{"ok":true}`, false, nil
	}}
	resp, err := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("ToolCalls = %d, want 2", len(resp.ToolCalls))
	}
	if len(inv.names) != 2 {
		t.Errorf("invoker called %d times, want 2", len(inv.names))
	}

	second := st.bodies[1]
	asst := second.Messages[1]
	if len(asst.Content) != 2 {
		t.Fatalf("assistant echoed %d tool_use blocks, want 2", len(asst.Content))
	}
	userTR := second.Messages[2]
	if len(userTR.Content) != 2 {
		t.Fatalf("user carried %d tool_result blocks, want 2", len(userTR.Content))
	}
	if userTR.Content[0].ToolUseID != "toolu_1" || userTR.Content[1].ToolUseID != "toolu_2" {
		t.Errorf("tool_use_id ordering = %q,%q want toolu_1,toolu_2",
			userTR.Content[0].ToolUseID, userTR.Content[1].ToolUseID)
	}
}

// TestRunToolLoop_MaxToolsPerTurnCap: three tool_use blocks but
// MaxToolsPerTurn=2 → only two honored, truncation flagged, the assistant
// echo + tool_result count both capped at 2 (so no orphaned tool_use/id).
func TestRunToolLoop_MaxToolsPerTurnCap(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{
		toolUseResp(
			toolUseBlock("toolu_1", "get_thing", map[string]any{}),
			toolUseBlock("toolu_2", "get_thing", map[string]any{}),
			toolUseBlock("toolu_3", "get_thing", map[string]any{}),
		),
		endTurnResp("ok"),
	}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return `{}`, false, nil
	}}
	req := baseReq(inv)
	req.Bounds = ToolLoopBounds{MaxToolsPerTurn: 2}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", req)
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if len(inv.names) != 2 {
		t.Errorf("invoker called %d times, want 2 (per-turn cap)", len(inv.names))
	}
	if !resp.Truncated {
		t.Error("Truncated = false, want true after per-turn cap")
	}
	// Assistant echo and tool_result must be balanced at 2 (never echo a
	// tool_use without answering it — Anthropic 400s).
	second := st.bodies[1]
	if len(second.Messages[1].Content) != 2 || len(second.Messages[2].Content) != 2 {
		t.Errorf("echo/result counts = %d/%d, want 2/2",
			len(second.Messages[1].Content), len(second.Messages[2].Content))
	}
}

// TestRunToolLoop_ErrorResultRecovery: the invoker returns an error result;
// it is fed back with is_error=true and the loop continues to synthesis.
func TestRunToolLoop_ErrorResultRecovery(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{
		toolUseResp(toolUseBlock("toolu_e", "get_thing", map[string]any{})),
		endTurnResp("I couldn't read that, but here's what I know."),
	}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return "forbidden", true, nil
	}}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if len(resp.ToolCalls) != 1 || !resp.ToolCalls[0].IsError {
		t.Errorf("ToolCalls = %+v, want one is_error", resp.ToolCalls)
	}
	if resp.FinalText == "" {
		t.Error("FinalText empty, want synthesis after error recovery")
	}
	tr := st.bodies[1].Messages[2].Content[0]
	if !tr.IsError || tr.Content != "forbidden" {
		t.Errorf("tool_result = %+v, want is_error=true content=forbidden", tr)
	}
}

// TestRunToolLoop_InvokerInternalError: invoker returns a non-nil err → fed
// back as a soft error result, loop continues (never propagated as fatal).
func TestRunToolLoop_InvokerInternalError(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{
		toolUseResp(toolUseBlock("toolu_x", "get_thing", map[string]any{})),
		endTurnResp("recovered"),
	}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return "", false, errors.New("boom")
	}}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	if err != nil {
		t.Fatalf("RunToolLoop returned fatal err: %v, want nil (soft)", err)
	}
	if len(resp.ToolCalls) != 1 || !resp.ToolCalls[0].IsError {
		t.Errorf("ToolCalls = %+v, want one is_error from invoker err", resp.ToolCalls)
	}
	tr := st.bodies[1].Messages[2].Content[0]
	if !tr.IsError || tr.Content != "tool execution failed" {
		t.Errorf("tool_result = %+v, want is_error + 'tool execution failed'", tr)
	}
}

// TestRunToolLoop_MaxIterationTruncation: a model that always emits a
// tool_use (never end_turn) terminates at MaxIterations with Truncated=true,
// ToolCalls <= MaxToolCalls, the best text so far, and a nil error.
func TestRunToolLoop_MaxIterationTruncation(t *testing.T) {
	// Every response is a tool_use that also carries some text.
	tu := messagesResponse{
		ID: "msg_loop", Type: "message", Role: "assistant", Model: "m",
		StopReason: "tool_use",
		Content: []contentBlock{
			{Type: "text", Text: "thinking..."},
			toolUseBlock("toolu_loop", "get_thing", map[string]any{}),
		},
	}
	st := &scriptedTransport{responses: []messagesResponse{tu, tu, tu, tu, tu, tu, tu, tu}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return `{}`, false, nil
	}}
	req := baseReq(inv)
	req.Bounds = ToolLoopBounds{MaxIterations: 3, MaxToolCalls: 12}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", req)
	if err != nil {
		t.Fatalf("RunToolLoop: %v, want nil on graceful truncation", err)
	}
	if !resp.Truncated {
		t.Error("Truncated = false, want true at MaxIterations")
	}
	if resp.Iterations != 3 {
		t.Errorf("Iterations = %d, want 3", resp.Iterations)
	}
	if len(resp.ToolCalls) != 3 {
		t.Errorf("ToolCalls = %d, want 3 (one per iteration)", len(resp.ToolCalls))
	}
	if resp.FinalText != "thinking..." {
		t.Errorf("FinalText = %q, want best text 'thinking...'", resp.FinalText)
	}
}

// TestRunToolLoop_MaxToolCallsCap: a low MaxToolCalls stops honoring blocks
// once the global ceiling is hit, even mid-turn.
func TestRunToolLoop_MaxToolCallsCap(t *testing.T) {
	tu := toolUseResp(
		toolUseBlock("toolu_1", "get_thing", map[string]any{}),
		toolUseBlock("toolu_2", "get_thing", map[string]any{}),
	)
	st := &scriptedTransport{responses: []messagesResponse{tu, tu, tu, tu}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return `{}`, false, nil
	}}
	req := baseReq(inv)
	req.Bounds = ToolLoopBounds{MaxIterations: 6, MaxToolCalls: 3, MaxToolsPerTurn: 4}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", req)
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if len(resp.ToolCalls) != 3 {
		t.Errorf("ToolCalls = %d, want exactly 3 (global cap)", len(resp.ToolCalls))
	}
	if !resp.Truncated {
		t.Error("Truncated = false, want true after MaxToolCalls")
	}
}

// TestRunToolLoop_MaxResultBytesCap: once cumulative tool-result bytes
// exceed MaxResultBytes, no further tool_use blocks are honored and the run
// truncates (the size ceiling, distinct from the count ceiling).
func TestRunToolLoop_MaxResultBytesCap(t *testing.T) {
	tu := toolUseResp(toolUseBlock("toolu_big", "get_thing", map[string]any{}))
	st := &scriptedTransport{responses: []messagesResponse{tu, tu, tu, tu, tu, tu}}
	c := newLoopTestClient(t, staticKey("k"), st)
	big := strings.Repeat("x", 200) // each result is 200 bytes
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return big, false, nil
	}}
	req := baseReq(inv)
	// Budget of 300 bytes admits the first result (resultBytes 0 -> 200),
	// then the second iteration sees resultBytes=200 < 300 and admits a
	// second (-> 400), the third sees 400 >= 300 and stops.
	req.Bounds = ToolLoopBounds{MaxIterations: 6, MaxToolCalls: 12, MaxResultBytes: 300}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", req)
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if !resp.Truncated {
		t.Error("Truncated = false, want true after MaxResultBytes")
	}
	if len(resp.ToolCalls) != 2 {
		t.Errorf("ToolCalls = %d, want 2 (byte budget admits 2 x 200B under 300)", len(resp.ToolCalls))
	}
}

// TestRunToolLoop_EndTurnExit: an immediate end_turn (no tools) returns the
// text with no tool calls and Truncated=false.
func TestRunToolLoop_EndTurnExit(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{
		endTurnResp("Direct answer, no tools needed."),
	}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		t.Error("invoker must not be called on immediate end_turn")
		return "", false, nil
	}}

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if resp.FinalText != "Direct answer, no tools needed." {
		t.Errorf("FinalText = %q", resp.FinalText)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %d, want 0", len(resp.ToolCalls))
	}
	if resp.Truncated {
		t.Error("Truncated = true, want false")
	}
	if resp.Iterations != 1 {
		t.Errorf("Iterations = %d, want 1", resp.Iterations)
	}
}

// TestRunToolLoop_MaxTokensExit: stop_reason "max_tokens" is terminal too.
func TestRunToolLoop_MaxTokensExit(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{{
		ID: "m", Type: "message", Role: "assistant", Model: "m",
		StopReason: "max_tokens",
		Content:    []contentBlock{{Type: "text", Text: "partial..."}},
	}}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return "", false, nil
	}}
	resp, err := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if resp.FinalText != "partial..." || resp.Truncated {
		t.Errorf("resp = %+v, want partial text and Truncated=false", resp)
	}
}

// TestRunToolLoop_ErrUnconfiguredPassthrough: an empty key surfaces
// ai.ErrUnconfigured verbatim (so the service translates it to
// ErrAssistantUnavailable and the handler 503s).
func TestRunToolLoop_ErrUnconfiguredPassthrough(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{endTurnResp("x")}}
	c := newLoopTestClient(t, staticKey(""), st) // empty key
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return "", false, nil
	}}

	_, err := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	if !errors.Is(err, ErrUnconfigured) {
		t.Fatalf("err = %v, want ErrUnconfigured", err)
	}
	if st.calls != 0 {
		t.Errorf("transport hit %d times, want 0 (key unconfigured short-circuits)", st.calls)
	}
}

// TestRunToolLoop_HTTPErrorPropagates: a non-unconfigured upstream failure
// (e.g. 400) propagates as an error rather than being swallowed as
// truncation.
func TestRunToolLoop_HTTPErrorPropagates(t *testing.T) {
	rt := stubTransport{fn: func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`)),
			Header:     make(http.Header),
		}, nil
	}}
	c, err := NewClient(Config{
		KeyResolver: staticKey("k"),
		BaseURL:     "http://anthropic.invalid",
		HTTPClient:  &http.Client{Transport: rt},
		Retry:       RetryConfig{MaxAttempts: 2, BaseDelayMs: 1, Multiplier: 2.0},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return "", false, nil
	}}
	_, runErr := c.RunToolLoop(context.Background(), "experience_chat", baseReq(inv))
	var httpErr *HTTPError
	if !errors.As(runErr, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("err = %v, want *HTTPError{400}", runErr)
	}
}

// TestRunToolLoop_DefaultsApplied: a zero-bounds request gets the package
// defaults (verified indirectly: a single tool turn + end_turn succeeds and
// MaxTokens is the default loop cap).
func TestRunToolLoop_DefaultsApplied(t *testing.T) {
	st := &scriptedTransport{responses: []messagesResponse{
		toolUseResp(toolUseBlock("t", "get_thing", map[string]any{})),
		endTurnResp("ok"),
	}}
	c := newLoopTestClient(t, staticKey("k"), st)
	inv := &recordingInvoker{fn: func(_ string, _ json.RawMessage) (string, bool, error) {
		return `{}`, false, nil
	}}
	req := baseReq(inv)
	req.Bounds = ToolLoopBounds{} // all zero → defaults

	resp, err := c.RunToolLoop(context.Background(), "experience_chat", req)
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if resp.FinalText != "ok" {
		t.Errorf("FinalText = %q", resp.FinalText)
	}
	if st.bodies[0].MaxTokens != loopMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", st.bodies[0].MaxTokens, loopMaxTokens)
	}
}

// TestToolLoopBounds_withDefaults exercises the bound floor directly.
func TestToolLoopBounds_withDefaults(t *testing.T) {
	got := ToolLoopBounds{}.withDefaults()
	want := ToolLoopBounds{
		MaxIterations:   defaultLoopMaxIterations,
		MaxToolCalls:    defaultLoopMaxToolCalls,
		MaxToolsPerTurn: defaultLoopMaxToolsPerTurn,
		MaxResultBytes:  defaultLoopMaxResultBytes,
		Timeout:         defaultLoopTimeout,
	}
	if got != want {
		t.Errorf("withDefaults() = %+v, want %+v", got, want)
	}
	// Positive values are preserved.
	custom := ToolLoopBounds{MaxIterations: 2, MaxToolCalls: 3, MaxToolsPerTurn: 1, MaxResultBytes: 99, Timeout: time.Second}
	if custom.withDefaults() != custom {
		t.Errorf("withDefaults() mutated positive bounds: %+v", custom.withDefaults())
	}
}
