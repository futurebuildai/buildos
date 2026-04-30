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
)

// Client is the top-level Brain client. Sub-clients are exposed as
// fields so callers write `c.Maestro.Chat(...)`, `c.Billing.GetUsage(...)`.
type Client struct {
	Maestro *MaestroClient
	Billing *BillingClient

	baseURL    string
	httpClient *http.Client
	retry      RetryConfig
	logger     *slog.Logger
}

// Config configures NewClient.
type Config struct {
	BaseURL    string         // e.g. "http://localhost:8081"
	HTTPClient *http.Client   // optional; default is a 10s-timeout client
	Retry      RetryConfig    // optional; default 3 attempts, 100ms base
	Logger     *slog.Logger   // optional; default slog.Default()
}

// NewClient builds a Brain client. BaseURL is required.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("brain: BaseURL is required")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	c := &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: cfg.HTTPClient,
		retry:      cfg.Retry.withDefaults(),
		logger:     cfg.Logger,
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

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("brain: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("brain: %s %s: %w", method, path, err)
			c.logger.Warn("brain request transport error",
				"method", method, "path", path, "attempt", attempt+1, "error", err)
			continue // retry on transport error
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("brain: read response: %w", readErr)
			continue
		}

		// 5xx is retryable; 4xx is not.
		if resp.StatusCode >= 500 {
			lastErr = decodeHTTPError(resp.StatusCode, respBody)
			c.logger.Warn("brain 5xx response",
				"method", method, "path", path, "status", resp.StatusCode, "attempt", attempt+1)
			continue
		}

		// 4xx — return the typed error immediately, no retry.
		if resp.StatusCode >= 400 {
			return nil, decodeHTTPError(resp.StatusCode, respBody)
		}

		// 2xx — unwrap the envelope.
		var env envelope
		if len(respBody) > 0 {
			if err := json.Unmarshal(respBody, &env); err != nil {
				return nil, fmt.Errorf("brain: decode envelope: %w", err)
			}
		}
		if env.Error != nil {
			return nil, &HTTPError{
				StatusCode: resp.StatusCode,
				Code:       env.Error.Code,
				Message:    env.Error.Message,
				RequestID:  metaRequestID(env.Meta),
			}
		}
		return env.Data, nil
	}

	// Out of retries — wrap the last error as transient.
	if lastErr == nil {
		return nil, fmt.Errorf("%w: exhausted retries with no recorded error", ErrTransient)
	}
	return nil, fmt.Errorf("%w: %v", ErrTransient, lastErr)
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
