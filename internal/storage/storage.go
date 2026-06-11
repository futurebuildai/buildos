// Package storage is the object-storage substrate for BuildOS: a small
// ObjectStore port plus an S3-compatible (Cloudflare R2) adapter built on a
// hand-rolled AWS Signature V4 presigner over net/http.
//
// LEAF PACKAGE (isolation gate). storage imports ONLY the standard library
// (incl. crypto/*) — no internal/service, internal/store, internal/ai,
// internal/worker, internal/physics, internal/currency. It declares the
// ObjectStore port the rest of BuildOS consumes; adapters that wire per-fork
// credentials live in internal/service. The dependency arrow points inward
// (callers → storage), never outward. Enforced by scripts/check-isolation.sh.
//
// Dependency decision (DAILY_REPORTS_CLIENT_UPDATES.md §A.1 / D-9): the
// presigner is hand-rolled (~stdlib crypto) rather than pulling aws-sdk-go-v2,
// mirroring the owner-approved hand-rolled MCP-client precedent. R2 is
// S3-compatible and presigned PUT/GET need only SigV4 query-param presigning,
// which is well-specified and stable. ZERO new module dependency.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrUnconfigured is returned by storage operations when the adapter has no
// usable configuration (endpoint/bucket/credentials). The service layer treats
// this as a SOFT-FAIL and surfaces a 503 STORAGE_UNAVAILABLE — the same posture
// as the AI client / mailer when their per-org key is missing. The server still
// boots; storage-dependent endpoints just 503 until an admin configures R2.
var ErrUnconfigured = errors.New("storage: object store unconfigured")

// ObjectStore is the port the rest of BuildOS consumes for blob storage.
// Implementations are constructed from per-fork config (ADR-002: storage is
// per-fork, never hardcoded). The happy-path upload/download bytes go DIRECT to
// R2 via presigned URLs — they never transit the Go server — so this port is
// mostly URL construction (pure, network-free) plus an optional server-side
// Get used by the same-origin photo proxy.
type ObjectStore interface {
	// PresignPut returns a time-limited URL the client PUTs bytes to, with
	// Content-Type and Content-Length signed in (so R2 rejects a mismatched
	// upload). key is the opaque object key the caller chose. signedHeaders
	// are the request headers the client MUST echo verbatim on the PUT for the
	// signature to verify.
	PresignPut(ctx context.Context, key, contentType string, contentLength int64, ttl time.Duration) (url string, signedHeaders map[string]string, err error)

	// PresignGet returns a time-limited read URL for key.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (url string, err error)

	// Get streams object bytes from the store (used by the same-origin photo
	// proxy so the R2 host never appears in client HTML/CSP). EXIF stripping
	// happens in the service layer on top of this, not here. The caller MUST
	// Close the returned reader.
	Get(ctx context.Context, key string) (body io.ReadCloser, contentType string, err error)

	// Delete removes an object (asset hard-delete / GC).
	Delete(ctx context.Context, key string) error
}
