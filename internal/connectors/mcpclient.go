package connectors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// requestCounter generates monotonically-increasing JSON-RPC request ids
// (process-wide; uniqueness within a single client exchange is all the protocol
// requires).
var requestCounter atomic.Int64

func nextRequestID() int { return int(requestCounter.Add(1)) }

// MCP Streamable-HTTP client (Phase 3b-ii). A minimal, hand-rolled (no SDK
// dependency) JSON-RPC 2.0 client over the MCP Streamable HTTP transport
// (spec 2025-06-18): a single endpoint that supports POST, replying with EITHER
// application/json (one message) OR text/event-stream (SSE). We implement the
// lifecycle (initialize → notifications/initialized) and tools/list + tools/call.
//
// EVERY failure here surfaces as a typed error the connector turns into a soft
// IsError tool result — the model self-corrects in prose; a connector call never
// aborts the chat loop or 500s the request.

const mcpProtocolVersion = "2025-06-18"

var (
	errMCPNonHTTPS     = errors.New("connectors: MCP endpoint must be https")
	errMCPSession      = errors.New("connectors: MCP session expired") // 404 with a session id
	errMCPUnauthorized = errors.New("connectors: MCP server rejected the credential")
	errMCPHTTPStatus   = errors.New("connectors: MCP server returned an error status")
	errMCPTooLarge     = errors.New("connectors: MCP response exceeded the byte cap")
	errMCPNoResponse   = errors.New("connectors: MCP server sent no matching response")
	errMCPBadMessage   = errors.New("connectors: MCP server sent a malformed message")
	errMCPRPC          = errors.New("connectors: MCP server returned a JSON-RPC error")
)

// MCPClient is a per-connection client to one MCP endpoint. It is cheap to build
// and used for a single short exchange (initialize → one call); sessions are not
// pooled in 3b-ii.
type MCPClient struct {
	http           *http.Client // the SSRF-guarded egress client
	endpoint       string
	bearer         string // optional Authorization: Bearer; "" => unauthenticated
	sessionID      string // captured from the initialize response, echoed afterward
	perCall        time.Duration
	maxResultBytes int
	clientName     string
	clientVersion  string
}

// MCPClientParams configures an MCPClient. HTTP must be an SSRF-guarded client
// (NewEgressClient). A zero perCall/maxResultBytes takes safe defaults.
type MCPClientParams struct {
	HTTP           *http.Client
	Endpoint       string
	Bearer         string
	PerCall        time.Duration
	MaxResultBytes int
	ClientVersion  string
}

// NewMCPClient builds a client. It does NOT dial; Initialize does.
func NewMCPClient(p MCPClientParams) *MCPClient {
	if p.PerCall <= 0 {
		p.PerCall = defaultEgressTimeout
	}
	if p.MaxResultBytes <= 0 {
		p.MaxResultBytes = defaultMaxResultBytes
	}
	return &MCPClient{
		http:           p.HTTP,
		endpoint:       p.Endpoint,
		bearer:         p.Bearer,
		perCall:        p.PerCall,
		maxResultBytes: p.MaxResultBytes,
		clientName:     "buildos",
		clientVersion:  p.ClientVersion,
	}
}

// defaultMaxResultBytes caps a single connector result well under the 256KiB
// cumulative chat-loop budget (which is checked-before but added-after with no
// per-result cap), so one oversized external result can't flood the context.
const defaultMaxResultBytes = 48 * 1024

// --- JSON-RPC framing ---

type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP result shapes ---

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type listToolsResult struct {
	Tools      []mcpTool `json:"tools"`
	NextCursor string    `json:"nextCursor"`
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type callToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Initialize runs the MCP handshake: POST initialize (capturing Mcp-Session-Id),
// then POST the notifications/initialized notification. Returns an error on any
// transport/protocol failure (the connector soft-fails it).
func (c *MCPClient) Initialize(ctx context.Context) error {
	resp, err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": c.clientName, "version": c.clientVersion},
	}, true)
	if err != nil {
		return err
	}
	var ir initializeResult
	if err := json.Unmarshal(resp.Result, &ir); err != nil {
		return fmt.Errorf("%w: initialize result", errMCPBadMessage)
	}
	// We accept whatever protocol version the server reports (forward/backward
	// tolerant); the header we send afterward stays our negotiated version.
	if err := c.notify(ctx, "notifications/initialized", nil); err != nil {
		return err
	}
	return nil
}

// ListTools returns every tool the server advertises, following nextCursor up to
// a bounded number of pages/tools.
func (c *MCPClient) ListTools(ctx context.Context) ([]ToolDef, error) {
	const maxPages = 20
	const maxTools = 256
	var out []ToolDef
	cursor := ""
	for page := 0; page < maxPages; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		resp, err := c.request(ctx, "tools/list", params, true)
		if err != nil {
			return nil, err
		}
		var lr listToolsResult
		if err := json.Unmarshal(resp.Result, &lr); err != nil {
			return nil, fmt.Errorf("%w: tools/list result", errMCPBadMessage)
		}
		for _, t := range lr.Tools {
			out = append(out, ToolDef{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
			if len(out) >= maxTools {
				return out, nil
			}
		}
		if lr.NextCursor == "" {
			break
		}
		cursor = lr.NextCursor
	}
	return out, nil
}

// CallTool invokes a remote tool and maps its content blocks to a single string.
// It returns (content, isError, err): a JSON-RPC/transport failure is a Go error
// (errMCPRPC etc., which the connector renders as a soft IsError); an MCP
// tool-level isError result surfaces as (text, true, nil).
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	resp, err := c.request(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": json.RawMessage(args),
	}, true)
	if err != nil {
		return "", false, err
	}
	var cr callToolResult
	if err := json.Unmarshal(resp.Result, &cr); err != nil {
		return "", false, fmt.Errorf("%w: tools/call result", errMCPBadMessage)
	}
	return renderContent(cr.Content), cr.IsError, nil
}

// renderContent flattens MCP content blocks to text. Text blocks are joined;
// non-text blocks become a short placeholder (the model gets a faithful, bounded
// summary, never raw binary).
func renderContent(blocks []mcpContent) string {
	var b strings.Builder
	for _, blk := range blocks {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		switch blk.Type {
		case "text":
			b.WriteString(blk.Text)
		case "":
			b.WriteString(blk.Text) // tolerate servers that omit type on text
		default:
			b.WriteString("[" + blk.Type + " content omitted]")
		}
	}
	return b.String()
}

// --- transport ---

// request sends a JSON-RPC request and returns the matching response. expectResp
// is true for methods that return a result.
func (c *MCPClient) request(ctx context.Context, method string, params any, expectResp bool) (jsonrpcResponse, error) {
	id := nextRequestID()
	body, err := json.Marshal(jsonrpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return jsonrpcResponse{}, fmt.Errorf("marshal %s: %w", method, err)
	}
	httpResp, err := c.post(ctx, body)
	if err != nil {
		return jsonrpcResponse{}, err
	}
	defer httpResp.Body.Close()

	// Capture a session id offered at initialize time.
	if sid := httpResp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}
	if err := c.checkStatus(httpResp); err != nil {
		return jsonrpcResponse{}, err
	}
	if !expectResp {
		return jsonrpcResponse{}, nil
	}

	msg, err := c.readResponse(httpResp, id)
	if err != nil {
		return jsonrpcResponse{}, err
	}
	if msg.Error != nil {
		return jsonrpcResponse{}, fmt.Errorf("%w: [%d] %s", errMCPRPC, msg.Error.Code, msg.Error.Message)
	}
	return msg, nil
}

// notify sends a JSON-RPC notification (no id, no response expected; server
// returns 202).
func (c *MCPClient) notify(ctx context.Context, method string, params any) error {
	body, err := json.Marshal(jsonrpcNotification{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("marshal %s: %w", method, err)
	}
	httpResp, err := c.post(ctx, body)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(httpResp.Body, 1<<14))
	return c.checkStatus(httpResp)
}

func (c *MCPClient) post(ctx context.Context, body []byte) (*http.Response, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil || u.Scheme != "https" {
		return nil, errMCPNonHTTPS
	}
	callCtx, cancel := context.WithTimeout(ctx, c.perCall)
	// The caller owns the cancel via the returned body close path; we cancel on
	// the body being drained. Simpler + safe: cancel after the call returns and
	// the body is read. We attach cancel to the request context and call it in
	// the readers via a wrapper. To keep it simple, cancel is deferred by the
	// caller through a context that lives for the whole exchange.
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocolVersion)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("MCP POST: %w", err)
	}
	// Cancel the per-call context once the body is closed.
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

func (c *MCPClient) checkStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusNotFound && c.sessionID != "":
		c.sessionID = ""
		return errMCPSession
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errMCPUnauthorized
	case resp.StatusCode == http.StatusAccepted, resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode >= 400:
		return fmt.Errorf("%w: %d", errMCPHTTPStatus, resp.StatusCode)
	default:
		return nil
	}
}

// readResponse demuxes by Content-Type: a single application/json message, or an
// SSE stream from which we pull the response matching wantID. Bounded by the
// per-result byte cap.
func (c *MCPClient) readResponse(resp *http.Response, wantID int) (jsonrpcResponse, error) {
	ct := resp.Header.Get("Content-Type")
	limited := io.LimitReader(resp.Body, int64(c.maxResultBytes)+1)
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSSEResponse(limited, wantID, c.maxResultBytes)
	}
	// Default: a single JSON object (application/json or unspecified).
	raw, err := io.ReadAll(limited)
	if err != nil {
		return jsonrpcResponse{}, fmt.Errorf("read body: %w", err)
	}
	if len(raw) > c.maxResultBytes {
		return jsonrpcResponse{}, errMCPTooLarge
	}
	var msg jsonrpcResponse
	if err := json.Unmarshal(raw, &msg); err != nil {
		return jsonrpcResponse{}, fmt.Errorf("%w: %v", errMCPBadMessage, err)
	}
	return msg, nil
}

// readSSEResponse reads an SSE stream, accumulating each event's data: field(s)
// and parsing them as JSON-RPC, returning the first message whose id matches.
// Server-initiated requests/notifications (and non-JSON data) are ignored.
// Bounded to max bytes total.
func readSSEResponse(r io.Reader, wantID, max int) (jsonrpcResponse, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 8*1024), max+1)
	var data strings.Builder
	total := 0

	tryFlush := func() (jsonrpcResponse, bool) {
		if data.Len() == 0 {
			return jsonrpcResponse{}, false
		}
		raw := data.String()
		data.Reset()
		var msg jsonrpcResponse
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			return jsonrpcResponse{}, false // not a JSON-RPC message; ignore
		}
		if msg.ID != nil && *msg.ID == wantID {
			return msg, true
		}
		return jsonrpcResponse{}, false
	}

	for sc.Scan() {
		line := sc.Text()
		total += len(line) + 1
		if total > max {
			return jsonrpcResponse{}, errMCPTooLarge
		}
		if line == "" { // event boundary
			if msg, ok := tryFlush(); ok {
				return msg, nil
			}
			continue
		}
		if d, ok := strings.CutPrefix(line, "data:"); ok {
			d = strings.TrimPrefix(d, " ")
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(d)
		}
		// event:, id:, retry:, and comments (:) are ignored.
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return jsonrpcResponse{}, errMCPTooLarge
		}
		return jsonrpcResponse{}, fmt.Errorf("read SSE: %w", err)
	}
	if msg, ok := tryFlush(); ok { // stream ended without a trailing blank line
		return msg, nil
	}
	return jsonrpcResponse{}, errMCPNoResponse
}

// cancelOnClose cancels the per-call context when the body is closed, so a
// timeout context does not leak.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
