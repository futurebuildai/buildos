package obs

import "context"

// requestIDContextKey is the unexported key for the per-request ID the
// BuildOS side injects (matches chi's RequestID middleware). Unexported so
// external packages can't accidentally collide.
type requestIDContextKey struct{}

// ContextWithRequestID returns a new context carrying the given request ID.
// The auth middleware (or any request-scoped hook) calls this so log records
// and downstream egress wrappers can stamp the same request_id.
//
// This used to live on the brain package alongside the outbound X-Request-ID
// header it fed; with The Brain removed, obs owns the request_id correlation
// trio outright, so the context key lives here.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

// RequestIDFromContext reads the request_id previously installed by
// ContextWithRequestID. Empty result means "no request_id in scope" and
// callers should skip the field rather than emit an empty value.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDContextKey{}).(string)
	return v
}
