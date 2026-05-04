// Package obs provides observability primitives shared across BuildOS:
// today, just a slog handler that injects context-stashed correlation
// ids (request_id, trace_id) into every log record. Future additions
// — Prometheus metrics, OTel traces — slot in alongside.
package obs

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/pii"
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

// Handle adds the request_id and (when an OTel span is active in
// ctx) the trace_id + span_id from ctx to the record, masks any
// Restricted-class PII attribute values per the pii.FieldClass
// catalog, and passes the result to the inner handler.
//
// The three correlation ids form the standard "find this request in
// the logs / find the trace / find the span" trio every observability
// stack expects.
//
// PII scrubbing applied here:
//   - Attribute keys matching the catalog at Restricted (email,
//     phone, gps_*, oidc_subject, ip_address, etc.) have their
//     values replaced with "[REDACTED]" (string) or sentinel
//     ("[REDACTED]" for non-string).
//   - Confidential and below are passed through unchanged. The
//     SIEM still sees vendor names, *_cents amounts, request_ids,
//     trace_ids — necessary for triage.
//   - Group attrs are recursively scrubbed.
//
// Author note: this catches per-call attrs (`slog.InfoContext(ctx,
// "msg", "email", "alice@x")`) AND logger-level attrs added via
// WithAttrs (which is also wrapped below). It does NOT catch raw
// strings interpolated into the message itself — authors who
// `fmt.Sprintf("logged in as %s", email)` bypass slog's structured
// path and bypass the scrubber. The convention is: keep PII out of
// `msg`, route it through attrs.
func (h *CorrelatingHandler) Handle(ctx context.Context, r slog.Record) error {
	// Build replacement record so we can mask existing attrs as we
	// copy them across. slog.Record's API doesn't allow in-place
	// attr replacement; rebuilding is the canonical pattern.
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)

	// Inject correlation ids first so they appear before user attrs.
	if reqID := requestIDFromContext(ctx); reqID != "" {
		out.AddAttrs(slog.String("request_id", reqID))
	}
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		out.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}

	// Walk the original record's attrs, scrub each, append to out.
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(scrubAttr(a))
		return true
	})

	return h.inner.Handle(ctx, out)
}

// WithAttrs delegates so caller-added attrs survive wrapping. Logger-
// level attrs (the ones a caller bakes in via `logger.With(...)`) are
// scrubbed at this stage so PII baked into a long-lived logger is
// masked before it ever reaches the inner handler. Per-call attrs
// flow through Handle and are scrubbed there.
func (h *CorrelatingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = scrubAttr(a)
	}
	return &CorrelatingHandler{inner: h.inner.WithAttrs(scrubbed)}
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

// scrubAttr returns a copy of a with its value masked when the attr
// key classifies as Restricted in pii.FieldClass. Group attrs recurse
// into their member attrs. Confidential and below are passed through
// unchanged — the slog/SIEM tier needs vendor names, cost figures,
// and correlation ids in clear for triage.
//
// Non-string Restricted attrs collapse to the "[REDACTED]" sentinel
// rather than zero-value. Length-preserving masks (per pii.MaskString
// semantics for Restricted) intentionally don't preserve length —
// length itself can leak (e.g. number of digits in a phone narrows
// the search space).
func scrubAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		members := a.Value.Group()
		scrubbed := make([]any, 0, len(members))
		for _, child := range members {
			c := scrubAttr(child)
			scrubbed = append(scrubbed, c)
		}
		return slog.Group(a.Key, scrubbed...)
	}

	cls := pii.ClassFor(a.Key)
	if !cls.IsAtLeast(pii.Restricted) {
		return a
	}

	// String-typed Restricted: reuse pii.MaskString so behavior
	// matches every other egress (Sentry, audit JSONB, future).
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, pii.MaskString(a.Value.String(), cls))
	}

	// Non-string Restricted (rare — e.g. an int-typed gps_lat
	// attribute, or a struct attr accidentally keyed "address"):
	// collapse to a sentinel. The exact value type is dropped on
	// purpose to keep the egress contract uniform.
	return slog.String(a.Key, "[REDACTED]")
}
