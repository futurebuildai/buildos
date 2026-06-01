// Package mailer is BuildOS's transactional email surface. It abstracts
// the upstream email provider (Resend) behind a small Mailer interface
// so the service layer can send notifications without knowing the
// provider, and so dev/CI rigs can swap in a no-op implementation.
//
// Per-org sending is gated on a Resend API key resolved at send time
// via a KeyResolver — the key is per-org secret material held in an
// encrypted vault (see internal/cryptobox), so it is never embedded in
// config and never logged.
//
// PII / secret handling: the recipient address (Message.To) is
// PII-Restricted (per internal/pii) and the Resend API key is secret
// material. NEITHER may EVER be written to a log, error message, or
// span attribute. Implementations log the org_id (Internal) only.
package mailer

import (
	"context"
	"errors"
	"log/slog"
)

// ErrMailerUnconfigured signals that no Resend API key is available for
// the org. It is a soft condition: the caller should treat email as
// best-effort and not fail the originating request. Send returns this
// (rather than panicking or blocking) when the KeyResolver yields "".
var ErrMailerUnconfigured = errors.New("mailer: no Resend API key configured for org")

// Message is a single outbound transactional email. HTMLBody and
// TextBody are both sent when present (Resend accepts a multipart
// payload); at least one should be non-empty.
//
// To is PII-Restricted — never log it.
type Message struct {
	To       string
	Subject  string
	HTMLBody string
	TextBody string
}

// Mailer sends a single transactional email on behalf of an org. The
// orgID selects the per-org Resend key; the concrete implementation
// owns the sender identity (from address / from name).
type Mailer interface {
	Send(ctx context.Context, orgID string, msg Message) error
}

// KeyResolver resolves the per-org Resend API key. Implementations
// typically read an encrypted row from the credential vault and decrypt
// it (see internal/cryptobox). Returning ("", nil) means "no key
// configured for this org" — Send treats that as ErrMailerUnconfigured.
//
// The returned key is secret material — never log it.
type KeyResolver interface {
	ResendKey(ctx context.Context, orgID string) (string, error)
}

// noopMailer satisfies Mailer without sending anything. It logs the
// org_id (Internal) — never the recipient — and returns nil so it never
// blocks the originating request. Used in dev/CI and as the default
// when no provider is configured.
type noopMailer struct {
	logger *slog.Logger
}

// NewNoopMailer returns a Mailer that drops every message. If logger is
// nil, slog.Default() is used.
func NewNoopMailer(logger *slog.Logger) Mailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &noopMailer{logger: logger}
}

// Send logs the drop (org_id only — the recipient is PII-Restricted)
// and returns nil.
func (m *noopMailer) Send(ctx context.Context, orgID string, msg Message) error {
	m.logger.InfoContext(ctx, "noop mailer dropping email",
		"org_id", orgID,
		"subject_len", len(msg.Subject),
	)
	return nil
}
