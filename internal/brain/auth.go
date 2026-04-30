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
