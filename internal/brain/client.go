// Package brain is BuildOS's typed HTTP client for The Brain.
//
// Every BuildOS surface that needs auth/AI/3p/billing routes through
// this package — there is no other way to talk to Brain. Sub-clients
// (MaestroClient, BillingClient, …) live in their own files but share
// the same transport: a retrying http.Client that auto-unwraps Brain's
// {data, error, meta} envelope and reads the caller's Bearer token
// from context (see auth.go).
//
// Usage from a service-layer method:
//
//	func (s *DailyFocusService) Generate(ctx context.Context, projectID uuid.UUID) (Briefing, error) {
//	    resp, err := s.brain.Maestro.Chat(ctx, brain.ChatRequest{Message: "morning briefing for ..."})
//	    if err != nil {
//	        return Briefing{}, err
//	    }
//	    // resp.Reply, resp.SessionID, ...
//	}
//
// The token comes from the request context — auth middleware stashes
// the user's Bearer JWT via brain.ContextWithToken right after
// validating it.
package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// MetricsObserver is the consumer-side surface the brain client uses
// to report each completed attempt. Defined here so the brain
// package stays free of the prometheus dep — and so callers without
// a metrics setup can pass nil.
type MetricsObserver interface {
	ObserveBrain(method, path string, statusCode int, dur time.Duration)
}

// Client is the top-level Brain client. Sub-clients are exposed as
// fields so callers write `c.Maestro.Chat(...)`, `c.Billing.GetUsage(...)`.
type Client struct {
	Maestro *MaestroClient
	Billing *BillingClient

	baseURL    string
	httpClient *http.Client
	retry      RetryConfig
	timeouts   Timeouts
	breaker    *circuitBreaker
	logger     *slog.Logger
	metrics    MetricsObserver // optional; nil disables observation
}

// Config configures NewClient.
type Config struct {
	BaseURL    string          // e.g. "http://localhost:8081"
	HTTPClient *http.Client    // optional; default is a 60s-timeout client
	Retry      RetryConfig     // optional; default 3 attempts, 200ms base, 4x
	Timeouts   Timeouts        // optional; default Maestro=30s, Billing=5s
	Circuit    CircuitConfig   // optional; default 5 failures / 60s window, 30s open
	Logger     *slog.Logger    // optional; default slog.Default()
	Metrics    MetricsObserver // optional; when nil, ObserveBrain calls are skipped
}

// NewClient builds a Brain client. BaseURL is required.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("brain: BaseURL is required")
	}
	if cfg.HTTPClient == nil {
		// 60s ceiling: covers Maestro LLM round-trips (typically
		// 5-30s) without making /ready Pings linger when Brain is
		// down. Per-call ctx deadlines (e.g. 2s on /ready) still
		// override this floor in the more-restrictive direction.
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	// Wrap the transport with OTel HTTP instrumentation so every
	// Brain call gets a child span + propagates W3C `traceparent`.
	// otelhttp's NewTransport is a no-op when no global tracer is
	// configured, so dev rigs that haven't enabled OTel pay no cost.
	if cfg.HTTPClient.Transport == nil {
		cfg.HTTPClient.Transport = http.DefaultTransport
	}
	cfg.HTTPClient.Transport = otelhttp.NewTransport(cfg.HTTPClient.Transport,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			// Default name is the HTTP method which is too coarse;
			// "GET /api/billing/usage" is what an operator wants in
			// Jaeger / Tempo / Honeycomb.
			return r.Method + " " + r.URL.Path
		}),
	)
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	c := &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: cfg.HTTPClient,
		retry:      cfg.Retry.withDefaults(),
		timeouts:   cfg.Timeouts.withDefaults(),
		breaker:    newCircuitBreaker(cfg.Circuit),
		logger:     cfg.Logger,
		metrics:    cfg.Metrics,
	}
	c.Maestro = &MaestroClient{c: c}
	c.Billing = &BillingClient{c: c}
	return c, nil
}

// Ping issues an unauthenticated GET /health to Brain and returns nil
// on a 2xx response, an error otherwise. Used by BuildOS's /ready
// probe to confirm Brain reachability — no token is required (Brain's
// /health is always public). The single attempt has a short context
// deadline so a Brain hiccup doesn't stall the readiness response;
// callers should pass a ctx with WithTimeout.
//
// Body content is intentionally ignored: a 2xx response is sufficient
// signal regardless of what Brain reports inside the envelope.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("brain: build ping request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("brain: ping transport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return &HTTPError{StatusCode: resp.StatusCode}
	}
	return nil
}

// doRequest is the shared transport. It:
//   - Pulls the Bearer token from ctx via TokenFromContext.
//   - Marshals body to JSON if non-nil.
//   - Sends POST/GET with Authorization: Bearer <token>.
//   - Retries on 5xx and connection errors (exponential backoff).
//   - Decodes Brain's {data, error, meta} envelope.
//   - Returns the raw `data` payload for the caller to unmarshal, or a
//     typed *HTTPError for non-2xx responses.
//
// Retry budget is bounded by the context — once ctx is cancelled, the
// next attempt aborts even if delay hasn't elapsed.
func (c *Client) doRequest(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	token, ok := TokenFromContext(ctx)
	if !ok {
		return nil, ErrUnauthenticated
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("brain: marshal request: %w", err)
		}
	}

	url := c.baseURL + path
	var lastErr error

	for attempt := 0; attempt < c.retry.MaxAttempts; attempt++ {
		// Honor ctx cancellation before each attempt.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Backoff between attempts (skip on first try).
		if attempt > 0 {
			delay := time.Duration(float64(c.retry.BaseDelayMs)*math.Pow(c.retry.Multiplier, float64(attempt-1))) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Circuit-breaker gate. allow() returns false when the
		// breaker is open and the open-duration hasn't elapsed (or
		// when a half-open probe is already in flight). We treat a
		// gating refusal as a hard short-circuit — no retry loop
		// burn — because every other retry would also be refused.
		ok, gen := c.breaker.allow()
		if !ok {
			c.logger.Warn("brain circuit breaker open; short-circuiting",
				"method", method, "path", path, "attempt", attempt+1)
			return nil, ErrCircuitOpen
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(bodyBytes))
		if err != nil {
			// Breaker outcome unknown — don't credit success or
			// failure for a request we never sent.
			return nil, fmt.Errorf("brain: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		// Propagate the caller's request ID for end-to-end correlation
		// across BuildOS → Brain hops. Empty string is fine — the
		// header is omitted entirely (Brain logs will fall back to its
		// own request ID).
		if reqID := requestIDFromContext(ctx); reqID != "" {
			req.Header.Set("X-Request-ID", reqID)
		}

		attemptStart := time.Now()
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("brain: %s %s: %w", method, path, err)
			c.logger.Warn("brain request transport error",
				"method", method, "path", path, "attempt", attempt+1, "error", err)
			c.observe(method, path, 0, time.Since(attemptStart))
			c.breaker.recordFailure(gen)
			continue // retry on transport error
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		dur := time.Since(attemptStart)
		c.observe(method, path, resp.StatusCode, dur)

		if readErr != nil {
			lastErr = fmt.Errorf("brain: read response: %w", readErr)
			c.breaker.recordFailure(gen)
			continue
		}

		// 5xx is retryable; 4xx is not.
		if resp.StatusCode >= 500 {
			lastErr = decodeHTTPError(resp.StatusCode, respBody)
			c.logger.Warn("brain 5xx response",
				"method", method, "path", path, "status", resp.StatusCode, "attempt", attempt+1)
			c.breaker.recordFailure(gen)
			continue
		}

		// 4xx — return the typed error immediately, no retry. Client
		// errors don't indicate Brain-side instability so they don't
		// count against the breaker.
		if resp.StatusCode >= 400 {
			c.breaker.recordSuccess(gen) // brain reachable; just rejected our payload
			return nil, decodeHTTPError(resp.StatusCode, respBody)
		}

		// 2xx — unwrap the envelope.
		var env envelope
		if len(respBody) > 0 {
			if err := json.Unmarshal(respBody, &env); err != nil {
				c.breaker.recordSuccess(gen) // transport healthy; decode bug is ours
				return nil, fmt.Errorf("brain: decode envelope: %w", err)
			}
		}
		if env.Error != nil {
			c.breaker.recordSuccess(gen) // structured error from healthy brain
			return nil, &HTTPError{
				StatusCode: resp.StatusCode,
				Code:       env.Error.Code,
				Message:    env.Error.Message,
				RequestID:  metaRequestID(env.Meta),
			}
		}
		c.breaker.recordSuccess(gen)
		return env.Data, nil
	}

	// Out of retries — wrap the last error as transient.
	if lastErr == nil {
		return nil, fmt.Errorf("%w: exhausted retries with no recorded error", ErrTransient)
	}
	return nil, fmt.Errorf("%w: %v", ErrTransient, lastErr)
}

// observe forwards to the wrapped MetricsObserver if one was wired.
// Cheap (one nil check) so callers can sprinkle this without
// performance worry.
func (c *Client) observe(method, path string, statusCode int, dur time.Duration) {
	if c.metrics == nil {
		return
	}
	c.metrics.ObserveBrain(method, path, statusCode, dur)
}

// decodeHTTPError turns a raw response body into a typed HTTPError,
// preserving Brain's error.code / error.message when present.
func decodeHTTPError(status int, body []byte) error {
	e := &HTTPError{StatusCode: status}
	if len(body) > 0 {
		var env envelope
		if err := json.Unmarshal(body, &env); err == nil {
			if env.Error != nil {
				e.Code = env.Error.Code
				e.Message = env.Error.Message
			}
			e.RequestID = metaRequestID(env.Meta)
		}
	}
	return e
}

func metaRequestID(m *metaBlock) string {
	if m == nil {
		return ""
	}
	return m.RequestID
}
