package models

import (
	"time"

	"github.com/google/uuid"
)

// ShareLink is one public-share token for a client update (Chunk E —
// DAILY_REPORTS_CLIENT_UPDATES). It backs the FIRST surface outside the
// everything-behind-auth invariant: an unauthenticated, token-gated, read-only
// homeowner progress page at GET /p/{token}.
//
// Security model mirrors models.SetupBootstrapToken EXACTLY: the 32-byte CSPRNG
// cleartext is shown ONCE at create and NEVER persisted — only its sha256 hash
// lands in TokenHash. The hash is treated like a credential (json:"-", never
// logged). Resolution returns a uniform not-found on any failure (missing /
// expired / revoked / mismatch) so a probe of /p/{token} leaks nothing.
//
// Unlike the bootstrap token, a share link is EXPIRABLE (default 30 days) and
// operator-REVOCABLE (RevokedAt). It is NOT one-shot — a homeowner may reload
// the page repeatedly; LastViewedAt / ViewCount are best-effort, PII-free view
// telemetry.
type ShareLink struct {
	ID             uuid.UUID `json:"id"`
	OrgID          uuid.UUID `json:"org_id"`
	ClientUpdateID uuid.UUID `json:"client_update_id"`
	// TokenHash is the sha256 of the cleartext — internal only, NEVER emitted or
	// logged. The cleartext is returned exactly once by CreateShareLink.
	TokenHash    string     `json:"-"`
	ExpiresAt    time.Time  `json:"expires_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	CreatedBy    uuid.UUID  `json:"created_by"`
	LastViewedAt *time.Time `json:"last_viewed_at,omitempty"`
	ViewCount    int64      `json:"view_count"`
	CreatedAt    time.Time  `json:"created_at"`
}

// IsActive reports whether the link is currently resolvable: not revoked and
// not expired. The service is the source of truth for clock resolution; this is
// a convenience used in tests + operator-side status derivation.
func (l ShareLink) IsActive(now time.Time) bool {
	return l.RevokedAt == nil && now.Before(l.ExpiresAt)
}

// PublicUpdate is the dedicated, redaction-safe projection rendered on the
// public homeowner page (GET /p/{token}). It PHYSICALLY CANNOT carry raw ERP:
// it has ONLY the operator-approved, client-safe fields. There is no path from
// a client_update row or a daily report into this struct except through the
// public handler's allowlist builder, so safety incidents, crew identities,
// GPS/EXIF, *_cents/budget, recipient_email, internal notes, schedule
// internals, and sibling-project data can never reach the page — the same
// discipline as the Chunk C AI client-task allowlist.
type PublicUpdate struct {
	// ProjectName is the project display name (Public PII).
	ProjectName string
	// Body is the operator-edited, client-safe prose (the curated edited_body
	// the operator already reviewed before send).
	Body string
	// UpdateDate is the period date the update covers.
	UpdateDate time.Time
	// PhotoAssetIDs is the operator-curated photo set. They render ONLY via the
	// same-origin proxy GET /p/{token}/photos/{assetID}; the raw R2 host/key
	// never appears in client HTML.
	PhotoAssetIDs []uuid.UUID
}
