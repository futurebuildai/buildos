package brain

import "context"

// tokenContextKey is the unexported type used as the context key for the
// caller's Bearer token. Unexported so external packages can't accidentally
// collide.
type tokenContextKey struct{}

// ContextWithToken returns a new context carrying a Bearer token (without
// the "Bearer " prefix). The auth middleware stashes the token here after
// validating the JWT, so service-layer code that calls Brain doesn't need
// to plumb the token through every call signature.
func ContextWithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenContextKey{}, token)
}

// TokenFromContext returns the Bearer token previously installed by
// ContextWithToken. The bool is false when no token is present.
func TokenFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(tokenContextKey{}).(string)
	return v, ok && v != ""
}

// requestIDContextKey is the unexported key for the per-request ID
// the BuildOS side injects (matches chi's RequestID middleware).
type requestIDContextKey struct{}

// ContextWithRequestID returns a new context carrying the given
// request ID. BuildOS's auth middleware (or any other request-scoped
// hook) calls this so doRequest can propagate the value as an
// X-Request-ID header on outbound calls to Brain.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

// requestIDFromContext is the internal reader used by doRequest to
// stamp the X-Request-ID header. Empty result means "skip the header".
func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDContextKey{}).(string)
	return v
}

// RequestIDFromContext is the public getter the obs package uses to
// read the request_id for log correlation. Kept on the brain package
// (alongside the context key it owns) so there's a single source of
// truth — the value sent in X-Request-ID is the same value logs see.
func RequestIDFromContext(ctx context.Context) string {
	return requestIDFromContext(ctx)
}
