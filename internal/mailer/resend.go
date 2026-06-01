package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// defaultResendBaseURL is Resend's public API root. Overridable via
// WithBaseURL for tests against an httptest.Server.
const defaultResendBaseURL = "https://api.resend.com"

// defaultTimeout caps a single send. Transactional email is best-effort
// relative to the originating request, so we keep this tight.
const defaultTimeout = 15 * time.Second

// ResendMailer is the production Mailer: it resolves the per-org Resend
// API key, then POSTs the message to the Resend /emails endpoint.
//
// The sender identity (from / fromName) is configured here on the
// concrete mailer, NOT in the per-org vault — the vault holds only the
// secret API key.
type ResendMailer struct {
	keys       KeyResolver
	from       string
	fromName   string
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// Option configures a ResendMailer at construction.
type Option func(*ResendMailer)

// WithBaseURL overrides the Resend API root. Primarily for tests
// pointing at an httptest.Server.
func WithBaseURL(u string) Option {
	return func(m *ResendMailer) { m.baseURL = u }
}

// WithHTTPClient injects a custom http.Client (e.g. an httptest client
// or one with a different timeout). The transport is left as-is — the
// caller owns its instrumentation when injecting.
func WithHTTPClient(c *http.Client) Option {
	return func(m *ResendMailer) { m.httpClient = c }
}

// WithLogger sets the slog logger. Defaults to slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(m *ResendMailer) { m.logger = l }
}

// NewResendMailer builds a ResendMailer. keys resolves the per-org API
// key; from / fromName are the static sender identity. The default
// http.Client wraps its transport with otelhttp so each send is a child
// span (a no-op when no global tracer is configured), with a tight
// timeout.
func NewResendMailer(keys KeyResolver, from, fromName string, opts ...Option) *ResendMailer {
	m := &ResendMailer{
		keys:     keys,
		from:     from,
		fromName: fromName,
		baseURL:  defaultResendBaseURL,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.httpClient == nil {
		m.httpClient = &http.Client{
			Timeout: defaultTimeout,
			// otelhttp's NewTransport is a no-op without a global
			// tracer, so dev rigs pay no cost.
			Transport: otelhttp.NewTransport(http.DefaultTransport,
				otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
					return r.Method + " " + r.URL.Path
				}),
			),
		}
	}
	return m
}

// resendRequest is the wire shape for POST /emails.
type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
}

// Send resolves the org's Resend key and POSTs the message. A missing
// key returns ErrMailerUnconfigured (soft — logged, not fatal). A
// non-2xx response maps to a typed error carrying the status only; the
// API key and recipient address are PII/secret and never appear in any
// error or log.
func (m *ResendMailer) Send(ctx context.Context, orgID string, msg Message) error {
	key, err := m.keys.ResendKey(ctx, orgID)
	if err != nil {
		return fmt.Errorf("mailer: resolve key: %w", err)
	}
	if key == "" {
		m.logger.WarnContext(ctx, "mailer unconfigured; dropping email",
			"org_id", orgID)
		return ErrMailerUnconfigured
	}

	payload := resendRequest{
		From:    m.fromAddress(),
		To:      []string{msg.To},
		Subject: msg.Subject,
		HTML:    msg.HTMLBody,
		Text:    msg.TextBody,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("mailer: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mailer: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		// Transport error — never includes the key; safe to wrap.
		return fmt.Errorf("mailer: send transport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain a bounded amount so the connection can be reused; the body
	// content is not surfaced (it could echo the recipient address).
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.logger.WarnContext(ctx, "resend rejected email",
			"org_id", orgID,
			"status", resp.StatusCode,
		)
		return &SendError{StatusCode: resp.StatusCode}
	}
	return nil
}

// fromAddress renders the sender identity. With a fromName it produces
// "Name <addr>"; otherwise the bare address.
func (m *ResendMailer) fromAddress() string {
	if m.fromName != "" {
		return fmt.Sprintf("%s <%s>", m.fromName, m.from)
	}
	return m.from
}

// SendError is returned when Resend responds with a non-2xx status. It
// carries the HTTP status only — never the API key, recipient, or
// response body — so it is safe to log and bubble up.
type SendError struct {
	StatusCode int
}

func (e *SendError) Error() string {
	return fmt.Sprintf("mailer: resend returned status %d", e.StatusCode)
}
