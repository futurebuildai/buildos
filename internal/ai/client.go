package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/futurebuildai/buildos/internal/connectors"
)

// Default model ids and limits. Opus is the heavy-reasoning default;
// Sonnet (the "fast" model) handles cheaper classification / briefing
// work.
const (
	defaultBaseURL   = "https://api.anthropic.com"
	defaultModel     = "claude-opus-4-6"
	defaultFastModel = "claude-sonnet-4-5"

	// anthropicVersion is the required API version header value.
	anthropicVersion = "2023-06-01"

	// defaultMaxImageBytes caps a fetched document image at ~5MB. The
	// Anthropic API rejects oversize images; we reject earlier to avoid
	// buffering attacker-controlled bytes.
	defaultMaxImageBytes int64 = 5 * 1024 * 1024

	// documentFetchTimeout bounds the SSRF-guarded fetch of an invoice
	// document_url (generous for a ~5MB image over a slow link).
	documentFetchTimeout = 20 * time.Second
)

// Client is the native Anthropic Messages API client. Construct via
// NewClient. Task methods (DailyBriefing, InvoiceExtract, …) live in
// tasks.go; they all funnel through messages().
type Client struct {
	keys          KeyResolver
	model         string
	fastModel     string
	baseURL       string
	httpClient    *http.Client
	docHTTPClient *http.Client
	retry         RetryConfig
	breaker       *circuitBreaker
	metrics       MetricsObserver // optional; nil disables observation
	maxImageBytes int64
	now           func() time.Time
}

// Config configures NewClient. Only KeyResolver is required.
type Config struct {
	// KeyResolver resolves the per-org Anthropic key at call time.
	// Required.
	KeyResolver KeyResolver
	// Model is the heavy-reasoning model id. Default "claude-opus-4-6".
	Model string
	// FastModel is the cheap/fast model id. Default "claude-sonnet-4-5".
	FastModel string
	// BaseURL is the Anthropic API base. Default
	// "https://api.anthropic.com".
	BaseURL string
	// HTTPClient is optional; default is a 60s-timeout client. Used for the
	// Anthropic API (a fixed, trusted host).
	HTTPClient *http.Client
	// DocumentFetchClient fetches the operator-supplied invoice document_url —
	// kept SEPARATE from HTTPClient because that URL is UNTRUSTED. The default
	// (when nil) is an SSRF-guarded egress client: a private-IP/metadata denylist
	// enforced at the resolved dial address (defeats DNS-rebind), redirects
	// refused, https only. Override only in tests.
	DocumentFetchClient *http.Client
	// Retry is optional; default 3 attempts, 200ms base, 4x.
	Retry RetryConfig
	// Circuit is optional; default 5 failures / 60s window, 30s open.
	Circuit CircuitConfig
	// Metrics is optional; when nil, ObserveAICall is skipped.
	Metrics MetricsObserver
	// MaxImageBytes caps fetched document images. Default ~5MB.
	MaxImageBytes int64
	// now overrides time.Now for deterministic tests. nil → time.Now.
	now func() time.Time
}

// NewClient builds an Anthropic client. KeyResolver is required.
func NewClient(cfg Config) (*Client, error) {
	if cfg.KeyResolver == nil {
		return nil, errors.New("ai: KeyResolver is required")
	}
	if cfg.HTTPClient == nil {
		// 60s ceiling: covers Anthropic LLM round-trips (typically
		// 5-30s) without letting a hung connection linger forever.
		// Per-call ctx deadlines still override this in the
		// more-restrictive direction.
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	// Wrap the transport with OTel HTTP instrumentation so every
	// Anthropic call gets a child span + propagates W3C `traceparent`.
	// otelhttp's NewTransport is a no-op when no global tracer is
	// configured, so dev rigs that haven't enabled OTel pay no cost.
	if cfg.HTTPClient.Transport == nil {
		cfg.HTTPClient.Transport = http.DefaultTransport
	}
	cfg.HTTPClient.Transport = otelhttp.NewTransport(cfg.HTTPClient.Transport,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.FastModel == "" {
		cfg.FastModel = defaultFastModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.MaxImageBytes <= 0 {
		cfg.MaxImageBytes = defaultMaxImageBytes
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	// The document fetch defaults to an SSRF-guarded client (fail-safe): the
	// invoice document_url is untrusted operator input and must NOT be fetched
	// through the plain client that can reach arbitrary internal hosts.
	docClient := cfg.DocumentFetchClient
	if docClient == nil {
		docClient = connectors.NewEgressClient(documentFetchTimeout)
	}
	return &Client{
		keys:          cfg.KeyResolver,
		model:         cfg.Model,
		fastModel:     cfg.FastModel,
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		httpClient:    cfg.HTTPClient,
		docHTTPClient: docClient,
		retry:         cfg.Retry.withDefaults(),
		breaker:       newCircuitBreaker(cfg.Circuit),
		metrics:       cfg.Metrics,
		maxImageBytes: cfg.MaxImageBytes,
		now:           cfg.now,
	}, nil
}

// ---- Anthropic wire types ---------------------------------------------

// messagesRequest is the POST /v1/messages body.
type messagesRequest struct {
	Model      string         `json:"model"`
	MaxTokens  int            `json:"max_tokens"`
	System     string         `json:"system,omitempty"`
	Messages   []messageParam `json:"messages"`
	Tools      []toolParam    `json:"tools,omitempty"`
	ToolChoice *toolChoice    `json:"tool_choice,omitempty"`
}

// messageParam is one entry in messages[]. Content is a slice of blocks
// (text and/or image).
type messageParam struct {
	Role    string         `json:"role"` // "user" | "assistant"
	Content []contentBlock `json:"content"`
}

// contentBlock is a single content block. Only the fields relevant to
// the active Type are populated. Supports:
//   - text:     {type:"text", text:"..."}
//   - image:    {type:"image", source:{type:"base64", media_type, data}}
//   - tool_use: {type:"tool_use", id, name, input} (responses only)
type contentBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// image block
	Source *imageSource `json:"source,omitempty"`

	// tool_use block (appears on responses)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block (appears on requests only) —
	// {type:"tool_result", tool_use_id, content, is_error}. Built by
	// RunToolLoop (chatloop.go) to feed an executed tool's deterministic
	// output back to the model on the next turn. ToolUseID MUST match the
	// id of the tool_use block being answered or Anthropic 400s.
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// imageSource is the base64 image payload inside an image content block.
type imageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "image/png"
	Data      string `json:"data"`       // base64-encoded bytes
}

// toolParam declares a tool the model may call.
type toolParam struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// toolChoice forces or hints tool selection.
type toolChoice struct {
	Type string `json:"type"`           // "auto" | "any" | "tool"
	Name string `json:"name,omitempty"` // required when Type == "tool"
}

// messagesResponse is the POST /v1/messages success body.
type messagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
}

// anthropicErrorEnvelope is the {type:"error", error:{type,message}}
// body Anthropic returns on non-2xx.
type anthropicErrorEnvelope struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// ---- transport --------------------------------------------------------

// messages issues a POST /v1/messages with the resolved org key and the
// retry/circuit-breaker policy. It maps transport/HTTP failures to the
// package sentinels:
//   - empty key / resolver error → ErrUnconfigured (no HTTP attempt)
//   - 429 persisting after retries → ErrRateLimited (Retry-After honored)
//   - 5xx / transport persisting after retries → ErrTransient
//   - open breaker → ErrCircuitOpen
//   - 4xx (non-429) → *HTTPError immediately, no retry
func (c *Client) messages(ctx context.Context, kind, orgID string, req messagesRequest) (*messagesResponse, error) {
	key, err := c.resolveKey(ctx, orgID)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai: marshal request: %w", err)
	}

	url := c.baseURL + "/v1/messages"
	var lastErr error
	rateLimited := false
	var lastRetryAfter time.Duration // server Retry-After hint from the prior attempt

	for attempt := 0; attempt < c.retry.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Backoff between attempts (skip on first). On a 429 we prefer
		// the server's Retry-After hint when it was larger.
		if attempt > 0 {
			delay := c.backoff(attempt)
			if rateLimited && lastRetryAfter > delay {
				delay = lastRetryAfter
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		// reset per-attempt hint
		lastRetryAfter = 0

		ok, gen, openFor := c.breaker.allow()
		if !ok {
			coErr := &CircuitOpenError{RetryAfter: openFor}
			c.observe(kind, req.Model, 0, coErr)
			return nil, coErr
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("ai: build request: %w", err)
		}
		httpReq.Header.Set("x-api-key", key)
		httpReq.Header.Set("anthropic-version", anthropicVersion)
		httpReq.Header.Set("content-type", "application/json")

		attemptStart := c.now()
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("ai: messages transport: %w", err)
			c.observe(kind, req.Model, c.now().Sub(attemptStart), err)
			c.breaker.recordFailure(gen)
			rateLimited = false
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		_ = resp.Body.Close()
		dur := c.now().Sub(attemptStart)

		if readErr != nil {
			lastErr = fmt.Errorf("ai: read response: %w", readErr)
			c.observe(kind, req.Model, dur, readErr)
			c.breaker.recordFailure(gen)
			rateLimited = false
			continue
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			lastErr = decodeAnthropicError(resp.StatusCode, respBody)
			c.observe(kind, req.Model, dur, lastErr)
			c.breaker.recordFailure(gen)
			rateLimited = true
			lastRetryAfter = retryAfter
			continue

		case resp.StatusCode >= 500:
			lastErr = decodeAnthropicError(resp.StatusCode, respBody)
			c.observe(kind, req.Model, dur, lastErr)
			c.breaker.recordFailure(gen)
			rateLimited = false
			continue

		case resp.StatusCode >= 400:
			// 4xx (non-429) — return immediately, no retry. Client
			// errors don't indicate upstream instability so they don't
			// count against the breaker.
			httpErr := decodeAnthropicError(resp.StatusCode, respBody)
			c.observe(kind, req.Model, dur, httpErr)
			c.breaker.recordSuccess(gen)
			return nil, httpErr

		default: // 2xx
			var out messagesResponse
			if err := json.Unmarshal(respBody, &out); err != nil {
				c.observe(kind, req.Model, dur, err)
				c.breaker.recordSuccess(gen) // transport healthy; decode bug is ours
				return nil, fmt.Errorf("ai: decode response: %w", err)
			}
			c.observe(kind, req.Model, dur, nil)
			c.breaker.recordSuccess(gen)
			return &out, nil
		}
	}

	// Out of retries — map to the right sentinel.
	if rateLimited {
		if lastErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrRateLimited, lastErr)
		}
		return nil, ErrRateLimited
	}
	if lastErr == nil {
		return nil, fmt.Errorf("%w: exhausted retries with no recorded error", ErrTransient)
	}
	return nil, fmt.Errorf("%w: %v", ErrTransient, lastErr)
}

// resolveKey fetches the org's Anthropic key, mapping empty/err to
// ErrUnconfigured.
func (c *Client) resolveKey(ctx context.Context, orgID string) (string, error) {
	key, err := c.keys.AnthropicKey(ctx, orgID)
	if err != nil || key == "" {
		return "", ErrUnconfigured
	}
	return key, nil
}

// backoff computes the inter-attempt delay for the given attempt index
// (attempt >= 1).
func (c *Client) backoff(attempt int) time.Duration {
	return time.Duration(float64(c.retry.BaseDelayMs)*math.Pow(c.retry.Multiplier, float64(attempt-1))) * time.Millisecond
}

// observe forwards to the wrapped MetricsObserver if one was wired.
func (c *Client) observe(kind, model string, dur time.Duration, err error) {
	if c.metrics == nil {
		return
	}
	c.metrics.ObserveAICall(kind, model, dur, err)
}

// parseRetryAfter parses a Retry-After header. Anthropic sends
// delta-seconds; we support that form. Returns 0 when absent/unparseable.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// decodeAnthropicError turns a raw error response body into a typed
// *HTTPError, preserving Anthropic's error.type / error.message.
func decodeAnthropicError(status int, body []byte) *HTTPError {
	e := &HTTPError{StatusCode: status}
	if len(body) > 0 {
		var env anthropicErrorEnvelope
		if err := json.Unmarshal(body, &env); err == nil {
			e.Type = env.Error.Type
			e.Message = env.Error.Message
		}
	}
	return e
}

// ---- request helpers --------------------------------------------------

// callTool issues a messages request that forces the model to call the
// named tool, then returns the raw tool_use.Input JSON for the caller to
// unmarshal into a typed output struct.
func (c *Client) callTool(ctx context.Context, kind, model, system string, userContent []contentBlock, toolName string, toolSchema json.RawMessage) (json.RawMessage, error) {
	req := messagesRequest{
		Model:     model,
		MaxTokens: 4096,
		System:    system,
		Messages: []messageParam{
			{Role: "user", Content: userContent},
		},
		Tools: []toolParam{
			{Name: toolName, InputSchema: toolSchema},
		},
		ToolChoice: &toolChoice{Type: "tool", Name: toolName},
	}
	resp, err := c.messages(ctx, kind, orgIDFromCtx(ctx), req)
	if err != nil {
		return nil, err
	}
	for _, blk := range resp.Content {
		if blk.Type == "tool_use" && blk.Name == toolName {
			return blk.Input, nil
		}
	}
	return nil, fmt.Errorf("ai: %s: model returned no tool_use for %q (stop_reason=%q)", kind, toolName, resp.StopReason)
}

// callText issues a plain messages request and returns the concatenated
// text blocks of the response.
func (c *Client) callText(ctx context.Context, kind, model, system string, userContent []contentBlock) (string, error) {
	req := messagesRequest{
		Model:     model,
		MaxTokens: 4096,
		System:    system,
		Messages: []messageParam{
			{Role: "user", Content: userContent},
		},
	}
	resp, err := c.messages(ctx, kind, orgIDFromCtx(ctx), req)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, blk := range resp.Content {
		if blk.Type == "text" {
			b.WriteString(blk.Text)
		}
	}
	return b.String(), nil
}

// textBlock is a convenience for building a single text content block.
func textBlock(s string) contentBlock {
	return contentBlock{Type: "text", Text: s}
}

// ---- org-id context plumbing ------------------------------------------

type orgIDKey struct{}

// ContextWithOrgID stashes the org id on the context so task methods can
// resolve the per-org Anthropic key without an explicit parameter on
// every call. Callers (service layer) set this right after auth, the
// same way the Brain client read the bearer token from context.
func ContextWithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDKey{}, orgID)
}

// orgIDFromCtx returns the org id stashed by ContextWithOrgID, or "" if
// absent (an empty org id flows to the KeyResolver, which decides
// whether a default/global key applies).
func orgIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(orgIDKey{}).(string); ok {
		return v
	}
	return ""
}
