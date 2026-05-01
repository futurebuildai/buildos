package middleware

import (
	"errors"
	"net/http"
)

// Body size ceilings. The default applies to every authenticated route
// unless explicitly overridden via MaxBodySize on a route group. The
// numbers are intentionally generous — JSON-only endpoints rarely
// exceed a few KB, but room above that protects against an attacker
// trickle-streaming megabytes to a slow handler.
const (
	// DefaultMaxBodyBytes caps every JSON endpoint at 10 MiB. None of
	// today's payloads come close (the largest is daily_log photos +
	// summary, which is photo_asset_id references, not raw photos);
	// 10 MiB leaves headroom for a future "richer agent payload"
	// without forcing an audit pass to bump the cap.
	DefaultMaxBodyBytes = 10 << 20 // 10 MiB

	// A2AInboundMaxBodyBytes is the cap on inbound A2A webhooks from
	// Brain. Already enforced inline in the handler; kept here so the
	// router can remind operators of the value via the typed
	// constant.
	A2AInboundMaxBodyBytes = 1 << 20 // 1 MiB

	// FileUploadMaxBodyBytes is the cap reserved for endpoints that
	// accept binary uploads (none today, but the next sprint's
	// proof-of-progress photo upload will land here). 25 MiB is a
	// single high-resolution photo's typical worst case.
	FileUploadMaxBodyBytes = 25 << 20 // 25 MiB
)

// MaxBodySize wraps the request so reading more than `limit` bytes
// fails with http.ErrBodyReadAfterClose, surfaced as a 413
// Payload Too Large.
//
// The middleware uses http.MaxBytesReader, which:
//   - Replaces r.Body with a LimitedReader.
//   - Calls Close() on the underlying body when the cap trips, so
//     the connection can be reused without dangling unread bytes.
//   - Returns a *http.MaxBytesError on overflow — distinguishable
//     from a generic transport error so the handler knows to send
//     413 instead of 500.
//
// Mount per-route group rather than globally when an endpoint needs
// a tighter or looser cap (e.g. A2A inbound is 1 MiB, file upload
// will be 25 MiB).
func MaxBodySize(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// IsBodyTooLarge reports whether err is a body-size-cap overflow.
// Handlers that want to surface 413 (instead of the generic 400 they
// emit on JSON decode errors) check this on json.NewDecoder errors.
func IsBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}
