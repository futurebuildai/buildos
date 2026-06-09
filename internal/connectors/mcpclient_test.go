package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubRequest is what the stub server parses: Method + optional ID (nil =>
// notification). Uses the package-internal framing (this test is in-package).
type stubRequest struct {
	Method string `json:"method"`
	ID     *int   `json:"id"`
}

// mcpStub returns a handler emulating an MCP Streamable-HTTP server in either
// "json" or "sse" reply mode. behavior lets a test override one method's reply.
func mcpStub(mode string, tools []mcpTool, callResult callToolResult, overrides map[string]func(http.ResponseWriter)) http.HandlerFunc {
	writeResult := func(w http.ResponseWriter, id int, result any) {
		raw, _ := json.Marshal(result)
		resp := jsonrpcResponse{JSONRPC: "2.0", ID: &id, Result: raw}
		body, _ := json.Marshal(resp)
		if mode == "sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			// Emit an unrelated server notification first to prove the client
			// skips non-matching messages, then the real response.
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n"))
			_, _ = w.Write([]byte("data: " + string(body) + "\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var req stubRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if ov, ok := overrides[req.Method]; ok {
			ov(w)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "test-session-123")
			writeResult(w, derefID(req.ID), initializeResult{ProtocolVersion: mcpProtocolVersion})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeResult(w, derefID(req.ID), listToolsResult{Tools: tools})
		case "tools/call":
			writeResult(w, derefID(req.ID), callResult)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func derefID(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func newStubClient(t *testing.T, h http.HandlerFunc) *MCPClient {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	// srv.Client() trusts the test cert and dials loopback directly — the
	// SSRF-guarded egress client (which REFUSES loopback) is proven separately
	// in egress_test.go; here we exercise the PROTOCOL.
	return NewMCPClient(MCPClientParams{HTTP: srv.Client(), Endpoint: srv.URL, ClientVersion: "test"})
}

func eachMode(t *testing.T, fn func(t *testing.T, mode string)) {
	for _, mode := range []string{"json", "sse"} {
		t.Run(mode, func(t *testing.T) { fn(t, mode) })
	}
}

func TestMCPClient_InitializeAndSession(t *testing.T) {
	eachMode(t, func(t *testing.T, mode string) {
		c := newStubClient(t, mcpStub(mode, nil, callToolResult{}, nil))
		if err := c.Initialize(context.Background()); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		if c.sessionID != "test-session-123" {
			t.Errorf("sessionID = %q, want the captured session", c.sessionID)
		}
	})
}

func TestMCPClient_ListTools(t *testing.T) {
	eachMode(t, func(t *testing.T, mode string) {
		tools := []mcpTool{
			{Name: "search", Description: "search docs", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "fetch", Description: "fetch a doc", InputSchema: json.RawMessage(`{"type":"object"}`)},
		}
		c := newStubClient(t, mcpStub(mode, tools, callToolResult{}, nil))
		if err := c.Initialize(context.Background()); err != nil {
			t.Fatalf("init: %v", err)
		}
		got, err := c.ListTools(context.Background())
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}
		if len(got) != 2 || got[0].Name != "search" || got[1].Name != "fetch" {
			t.Fatalf("tools = %+v", got)
		}
	})
}

func TestMCPClient_CallTool(t *testing.T) {
	eachMode(t, func(t *testing.T, mode string) {
		res := callToolResult{Content: []mcpContent{{Type: "text", Text: "hello from mcp"}}}
		c := newStubClient(t, mcpStub(mode, nil, res, nil))
		if err := c.Initialize(context.Background()); err != nil {
			t.Fatalf("init: %v", err)
		}
		content, isErr, err := c.CallTool(context.Background(), "search", json.RawMessage(`{"q":"x"}`))
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if isErr {
			t.Error("expected isError=false")
		}
		if !strings.Contains(content, "hello from mcp") {
			t.Errorf("content = %q", content)
		}
	})
}

func TestMCPClient_CallTool_MCPIsError(t *testing.T) {
	res := callToolResult{Content: []mcpContent{{Type: "text", Text: "tool failed upstream"}}, IsError: true}
	c := newStubClient(t, mcpStub("json", nil, res, nil))
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	content, isErr, err := c.CallTool(context.Background(), "search", nil)
	if err != nil {
		t.Fatalf("an MCP isError result must be (text,true,nil), not a Go error: %v", err)
	}
	if !isErr || !strings.Contains(content, "tool failed upstream") {
		t.Errorf("got (%q, %v), want the upstream error surfaced as isError", content, isErr)
	}
}

func TestMCPClient_JSONRPCError(t *testing.T) {
	c := newStubClient(t, mcpStub("json", nil, callToolResult{}, map[string]func(http.ResponseWriter){
		"tools/call": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"error":{"code":-32601,"message":"method not found"}}`))
		},
	}))
	_ = c.Initialize(context.Background())
	_, _, err := c.CallTool(context.Background(), "search", nil)
	if !errors.Is(err, errMCPRPC) {
		t.Fatalf("err = %v, want errMCPRPC", err)
	}
}

func TestMCPClient_HTTPErrorAndUnauthorized(t *testing.T) {
	cases := []struct {
		status  int
		wantErr error
	}{
		{http.StatusInternalServerError, errMCPHTTPStatus},
		{http.StatusBadGateway, errMCPHTTPStatus},
		{http.StatusUnauthorized, errMCPUnauthorized},
		{http.StatusForbidden, errMCPUnauthorized},
	}
	for _, tc := range cases {
		c := newStubClient(t, mcpStub("json", nil, callToolResult{}, map[string]func(http.ResponseWriter){
			"tools/call": func(w http.ResponseWriter) { w.WriteHeader(tc.status) },
		}))
		_ = c.Initialize(context.Background())
		_, _, err := c.CallTool(context.Background(), "search", nil)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("status %d: err = %v, want %v", tc.status, err, tc.wantErr)
		}
	}
}

func TestMCPClient_NonHTTPS(t *testing.T) {
	c := NewMCPClient(MCPClientParams{HTTP: http.DefaultClient, Endpoint: "http://example.com/mcp"})
	if err := c.Initialize(context.Background()); !errors.Is(err, errMCPNonHTTPS) {
		t.Fatalf("err = %v, want errMCPNonHTTPS (http endpoint refused)", err)
	}
}

func TestMCPClient_OversizedResult(t *testing.T) {
	huge := strings.Repeat("A", 1024)
	res := callToolResult{Content: []mcpContent{{Type: "text", Text: huge}}}
	c := newStubClient(t, mcpStub("json", nil, res, nil))
	c.maxResultBytes = 256 // tighten the cap below the result size
	_ = c.Initialize(context.Background())
	_, _, err := c.CallTool(context.Background(), "search", nil)
	if !errors.Is(err, errMCPTooLarge) {
		t.Fatalf("err = %v, want errMCPTooLarge", err)
	}
}

func TestRenderContent(t *testing.T) {
	got := renderContent([]mcpContent{
		{Type: "text", Text: "line one"},
		{Type: "image", Text: ""},
		{Type: "text", Text: "line two"},
	})
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") || !strings.Contains(got, "[image content omitted]") {
		t.Errorf("renderContent = %q", got)
	}
}
