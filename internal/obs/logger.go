// Package obs provides observability primitives shared across BuildOS:
// today, just a slog handler that injects context-stashed correlation
// ids (request_id, trace_id) into every log record. Future additions
// — Prometheus metrics, OTel traces — slot in alongside.
package obs

import (
	"context"
	"log/slog"

	"github.com/futurebuildai/buildos/internal/brain"
)

// CorrelatingHandler wraps a slog.Handler and stamps every record with
// the request ID found in context (the same one BuildOS propagates to
// Brain via X-Request-ID). Records emitted with a context-less call —
// e.g. slog.Info(...) without InfoContext — see no extra attrs.
//
// Why a handler wrapper rather than per-call fields: every server- and
// service-layer log line that takes a ctx should attach the request_id
// automatically. Per-call attrs require every author to remember; the
// wrapper makes it impossible to forget.
type CorrelatingHandler struct {
	inner slog.Handler
}

// NewCorrelatingHandler wraps inner so context-bound fields land on
// every record routed through this handler.
func NewCorrelatingHandler(inner slog.Handler) *CorrelatingHandler {
	return &CorrelatingHandler{inner: inner}
}

// Enabled forwards to the inner handler — wrapping doesn't change the
// level filter.
func (h *CorrelatingHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.inner.Enabled(ctx, lvl)
}

// Handle adds the request_id (and any future correlation ids) from ctx
// to the record, then passes it to the inner handler.
func (h *CorrelatingHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID := requestIDFromContext(ctx); reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs delegates so caller-added attrs survive wrapping.
func (h *CorrelatingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &CorrelatingHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup delegates likewise.
func (h *CorrelatingHandler) WithGroup(name string) slog.Handler {
	return &CorrelatingHandler{inner: h.inner.WithGroup(name)}
}

// requestIDFromContext mirrors the brain package's helper. We reach
// across the package boundary intentionally: the request_id is the
// SAME value BuildOS sends to Brain, so reading it from the brain
// context key keeps a single source of truth — no risk of "logs say
// X, Brain header says Y."
func requestIDFromContext(ctx context.Context) string {
	// Reuse brain.ContextWithRequestID/get via a small re-export
	// pattern: we can't import an unexported symbol, so we expose a
	// thin getter on the brain package. To avoid a dependency-cycle
	// risk, the brain package owns the context key.
	return brain.RequestIDFromContext(ctx)
}
