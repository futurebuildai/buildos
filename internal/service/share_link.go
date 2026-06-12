package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Share-link audit resource + action constants (Chunk E). `/audit?action_prefix=
// client_update.share_link.` reconstructs a link's create→revoke history.
const (
	AuditResourceShareLink = "client_update_share_link"

	AuditActionShareLinkCreated = "client_update.share_link.created"
	AuditActionShareLinkRevoked = "client_update.share_link.revoked"
)

// Share-link sentinel errors. Handlers map these to HTTP status codes:
//
//	ErrShareLinkNotSent → 422 UPDATE_NOT_SENT (a link is mintable only post-send)
//	ErrInvalidShareToken → 404 NOT_FOUND (uniform: missing/expired/revoked/mismatch)
//	ErrNotFound          → 404 NOT_FOUND (operator-side cross-org/missing link)
var (
	// ErrShareLinkNotSent is returned by CreateShareLink when the client update is
	// not yet 'sent'. Per §9-10 (default) a public link is mintable ONLY after
	// the update was sent — the homeowner sees what was emailed, and a draft's
	// unreviewed prose never reaches a public URL.
	ErrShareLinkNotSent = errors.New("share_link: client update is not sent")
	// ErrInvalidShareToken is the UNIFORM error returned by ResolvePublicUpdate /
	// ResolvePublicPhoto on ANY failure (missing, expired, revoked, malformed,
	// hash mismatch). Distinguishing reasons would leak probe information — this
	// mirrors the bootstrap token's ErrInvalidBootstrapToken exactly. The handler
	// maps it to a uniform 404.
	ErrInvalidShareToken = errors.New("share_link: invalid or expired token")
)

// shareTokenByteLen is the cleartext length for a newly-minted share token. 32
// bytes (256 bits) of CSPRNG entropy — identical to the bootstrap token, well
// beyond any feasible brute force against a deterministic sha256.
const shareTokenByteLen = 32

// DefaultShareLinkTTL is the default lifetime applied when CreateShareLink is
// called without an explicit TTL (§9-6 / D-6: 30 days — a homeowner link wants a
// longer but bounded life than the 7-day bootstrap token, and the operator can
// revoke at any time).
const DefaultShareLinkTTL = 30 * 24 * time.Hour

// MaxShareLinkTTL caps an operator-supplied TTL. A bounded life is a hard part
// of the security model — an unbounded link is a standing credential.
const MaxShareLinkTTL = 365 * 24 * time.Hour

// publicPhotoTTL is a defensive cap; the public photo path proxies bytes
// same-origin (it never hands a presigned URL to the homeowner), so this is not
// used for presigning — it documents the intended freshness (D-5: 5 min) for
// any future signed-URL path. The proxy re-validates the token + curated set on
// every request.
const publicPhotoTTL = 5 * time.Minute

// shareLinkClientUpdateReader is the narrow slice of ClientUpdateStore the
// share-link service needs: load a client update by id, org-scoped.
type shareLinkClientUpdateReader interface {
	GetByID(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) (models.ClientUpdate, error)
}

// shareLinkProjectReader is the narrow slice of ProjectStore the service needs:
// the project's display name for the public page (Public PII — no address, no
// client contact).
type shareLinkProjectReader interface {
	GetByID(ctx context.Context, tx pgx.Tx, projectID, orgID uuid.UUID) (models.Project, error)
}

// shareLinkUserResolver resolves a JWT sub → users.id (created_by FK).
type shareLinkUserResolver interface {
	LookupUserIDBySubject(ctx context.Context, tx pgx.Tx, subject string, orgID uuid.UUID) (uuid.UUID, error)
}

// ShareLinkService owns the public-share token lifecycle (Chunk E) and the
// redaction-safe public projection. It is the security boundary for the FIRST
// surface outside everything-behind-auth:
//
//   - CreateShareLink / RevokeShareLink / ListShareLinks are the AUTHENTICATED
//     operator surface (owner/admin); each mutation is one-tx + audit.
//   - ResolvePublicUpdate / ResolvePublicPhoto are the UNAUTHENTICATED public
//     resolution paths. They take a cleartext token (NOT an org/id), resolve it
//     to a link (uniform not-found on any failure), then build ONLY the
//     allowlisted models.PublicUpdate — raw ERP physically cannot escape.
type ShareLinkService struct {
	pool          *pgxpool.Pool
	store         *store.ShareLinkStore
	clientUpdates shareLinkClientUpdateReader
	projects      shareLinkProjectReader
	users         shareLinkUserResolver
	audit         AuditRecorder
	now           func() time.Time
}

// NewShareLinkService wires the share-link service. A nil AuditRecorder is
// replaced with the no-op; a nil clock uses time.Now.
func NewShareLinkService(
	pool *pgxpool.Pool,
	s *store.ShareLinkStore,
	clientUpdates shareLinkClientUpdateReader,
	projects shareLinkProjectReader,
	users shareLinkUserResolver,
	audit AuditRecorder,
	clock func() time.Time,
) *ShareLinkService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	if clock == nil {
		clock = time.Now
	}
	return &ShareLinkService{
		pool:          pool,
		store:         s,
		clientUpdates: clientUpdates,
		projects:      projects,
		users:         users,
		audit:         audit,
		now:           clock,
	}
}

// IssuedShareLink is the result of CreateShareLink — the caller MUST surface
// Cleartext to the operator ONCE (it becomes the /p/<cleartext> URL they send)
// and never again. Only the hash landed in the DB.
type IssuedShareLink struct {
	Link      models.ShareLink
	Cleartext string
}

// CreateShareLink mints a public-share token for a SENT client update. It (1)
// verifies the update is in the caller's org AND status='sent' (§9-10: links
// are post-send only), (2) generates a 32-byte CSPRNG cleartext, (3) INSERTs the
// hashed token + audit client_update.share_link.created in ONE tx, and returns
// the cleartext ONCE. ttl<=0 → DefaultShareLinkTTL; ttl is capped at
// MaxShareLinkTTL (a bounded life is part of the security model).
func (s *ShareLinkService) CreateShareLink(ctx context.Context, orgID uuid.UUID, userSub string, clientUpdateID uuid.UUID, ttl time.Duration) (IssuedShareLink, error) {
	if orgID == uuid.Nil || clientUpdateID == uuid.Nil {
		return IssuedShareLink{}, fmt.Errorf("%w: org_id and client_update_id are required", ErrInvalidInput)
	}
	if ttl <= 0 {
		ttl = DefaultShareLinkTTL
	}
	if ttl > MaxShareLinkTTL {
		ttl = MaxShareLinkTTL
	}

	cleartext, err := generateShareTokenCleartext()
	if err != nil {
		return IssuedShareLink{}, fmt.Errorf("generate share token: %w", err)
	}
	hash := hashShareToken(cleartext)
	expires := s.now().Add(ttl)

	var out models.ShareLink
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		cu, err := s.clientUpdates.GetByID(ctx, tx, orgID, clientUpdateID)
		if err != nil {
			return err // ErrNotFound on cross-org / missing
		}
		if cu.Status != models.ClientUpdateStatusSent {
			return ErrShareLinkNotSent
		}
		createdBy, err := s.users.LookupUserIDBySubject(ctx, tx, userSub, orgID)
		if err != nil {
			return err
		}
		link, err := s.store.Create(ctx, tx, store.CreateShareLinkParams{
			OrgID:          orgID,
			ClientUpdateID: clientUpdateID,
			TokenHash:      hash,
			ExpiresAt:      expires,
			CreatedBy:      createdBy,
		})
		if err != nil {
			return err
		}
		out = link
		// Audit references the link id + update id only — never the cleartext or
		// the hash (the hash is a credential).
		s.recordAudit(ctx, tx, orgID, userSub, AuditActionShareLinkCreated, link.ID, map[string]any{
			"client_update_id": clientUpdateID,
			"expires_at":       expires.UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrShareLinkNotSent) {
			return IssuedShareLink{}, ErrShareLinkNotSent
		}
		return IssuedShareLink{}, mapStoreError(err)
	}
	return IssuedShareLink{Link: out, Cleartext: cleartext}, nil
}

// RevokeShareLink sets revoked_at on a link, org-scoped, in ONE tx + audit
// client_update.share_link.revoked. After revoke, GET /p/{token} resolves to a
// uniform 404. Idempotent: revoking an already-revoked/expired link returns the
// current row without error.
func (s *ShareLinkService) RevokeShareLink(ctx context.Context, orgID uuid.UUID, userSub string, linkID uuid.UUID) (models.ShareLink, error) {
	if orgID == uuid.Nil || linkID == uuid.Nil {
		return models.ShareLink{}, fmt.Errorf("%w: org_id and link id are required", ErrInvalidInput)
	}
	var out models.ShareLink
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		link, err := s.store.Revoke(ctx, tx, orgID, linkID, s.now())
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Either the link is cross-org/missing OR already revoked. Re-check
				// to keep revoke idempotent: an already-revoked own link is a no-op
				// success, a truly-missing/cross-org link is a 404.
				existing, gerr := s.store.GetByID(ctx, tx, orgID, linkID)
				if gerr != nil {
					return gerr // ErrNotFound (cross-org / missing)
				}
				out = existing
				return nil
			}
			return err
		}
		out = link
		s.recordAudit(ctx, tx, orgID, userSub, AuditActionShareLinkRevoked, link.ID, map[string]any{
			"client_update_id": link.ClientUpdateID,
		})
		return nil
	})
	if err != nil {
		return models.ShareLink{}, mapStoreError(err)
	}
	return out, nil
}

// ListShareLinks returns a client update's share links newest-first, org-scoped
// (active/expired/revoked). Read-only, not audited. TokenHash is json:"-" so the
// cleartext is never reconstructable from a list response.
func (s *ShareLinkService) ListShareLinks(ctx context.Context, orgID, clientUpdateID uuid.UUID) ([]models.ShareLink, error) {
	if orgID == uuid.Nil || clientUpdateID == uuid.Nil {
		return nil, fmt.Errorf("%w: org_id and client_update_id are required", ErrInvalidInput)
	}
	var out []models.ShareLink
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		// Verify the update is in the org before listing (uniform 404 on cross-org).
		if _, err := s.clientUpdates.GetByID(ctx, tx, orgID, clientUpdateID); err != nil {
			return err
		}
		rows, err := s.store.ListByClientUpdate(ctx, tx, orgID, clientUpdateID)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return out, nil
}

// ResolvePublicUpdate is the UNAUTHENTICATED public-page resolution path. Given
// a cleartext token it (1) resolves the active link (uniform not-found on any
// failure), (2) loads the linked client update + project ORG-SCOPED to the
// link's org, and (3) builds ONLY the allowlisted models.PublicUpdate — raw ERP
// (safety, crew, GPS, cents, recipient_email, sibling projects, internal notes)
// physically cannot reach the projection because the struct has no fields for
// it. It also bumps best-effort view telemetry (its own tx; failures swallowed).
//
// Returns ErrInvalidShareToken on ANY token failure — never distinguishing
// missing / expired / revoked / malformed (enumeration defense). A non-sent
// update (should not happen, links are post-send) also resolves to invalid.
func (s *ShareLinkService) ResolvePublicUpdate(ctx context.Context, cleartext string) (models.PublicUpdate, error) {
	link, err := s.resolveActiveLink(ctx, cleartext)
	if err != nil {
		return models.PublicUpdate{}, err
	}

	var pub models.PublicUpdate
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		// Org-scoped to the LINK's org (read from the resolved row, not from any
		// caller input). A token for org A can never load org B's update.
		cu, err := s.clientUpdates.GetByID(ctx, tx, link.OrgID, link.ClientUpdateID)
		if err != nil {
			return err
		}
		// Defense in depth: only a sent update is publishable. A link only mints
		// post-send, but re-check here so a status regression can't leak a draft.
		if cu.Status != models.ClientUpdateStatusSent {
			return ErrInvalidShareToken
		}
		proj, err := s.projects.GetByID(ctx, tx, cu.ProjectID, link.OrgID)
		if err != nil {
			return err
		}
		// THE REDACTION GATE: copy ONLY the allowlisted, client-safe fields into
		// the projection. edited_body is the operator-reviewed prose; the photo set
		// is the operator-curated subset. Nothing else crosses.
		pub = models.PublicUpdate{
			ProjectName:   proj.Name,
			Body:          cu.EditedBody,
			UpdateDate:    cu.PeriodEnd,
			PhotoAssetIDs: dedupeUUIDs(cu.PhotoAssetIDs),
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidShareToken) {
			return models.PublicUpdate{}, ErrInvalidShareToken
		}
		// A missing update/project behind a valid token collapses to the same
		// uniform invalid-token response (no enumeration signal).
		if errors.Is(err, store.ErrNotFound) {
			return models.PublicUpdate{}, ErrInvalidShareToken
		}
		return models.PublicUpdate{}, mapStoreError(err)
	}

	s.touchView(ctx, link.ID)
	return pub, nil
}

// ResolvePublicPhotoTarget is what ResolvePublicPhoto returns: the org_id and
// asset_id the public photo proxy must use to fetch the bytes. The caller
// (public handler) hands these to AssetService.ServeAsset (which EXIF-strips).
type ResolvePublicPhotoTarget struct {
	OrgID   uuid.UUID
	AssetID uuid.UUID
}

// ResolvePublicPhoto is the UNAUTHENTICATED photo-proxy resolution path. Given a
// cleartext token + an asset id, it (1) resolves the active link (uniform
// not-found), (2) loads the linked update ORG-SCOPED to the link's org, and (3)
// verifies the asset id is in the update's operator-CURATED photo set. ONLY then
// does it return the (org, asset) the proxy may stream. A photo NOT in the
// curated set — including a real, ready asset from the SAME project or org —
// resolves to ErrInvalidShareToken (uniform 404): a leaked token grants ONLY
// the curated photos of that one update, nothing else.
func (s *ShareLinkService) ResolvePublicPhoto(ctx context.Context, cleartext string, assetID uuid.UUID) (ResolvePublicPhotoTarget, error) {
	if assetID == uuid.Nil {
		return ResolvePublicPhotoTarget{}, ErrInvalidShareToken
	}
	link, err := s.resolveActiveLink(ctx, cleartext)
	if err != nil {
		return ResolvePublicPhotoTarget{}, err
	}

	var target ResolvePublicPhotoTarget
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		cu, err := s.clientUpdates.GetByID(ctx, tx, link.OrgID, link.ClientUpdateID)
		if err != nil {
			return err
		}
		if cu.Status != models.ClientUpdateStatusSent {
			return ErrInvalidShareToken
		}
		// The asset MUST be in THIS update's curated set. Membership is the only
		// authorization: not "same project", not "same org" — the exact curated id.
		found := false
		for _, id := range cu.PhotoAssetIDs {
			if id == assetID {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalidShareToken
		}
		target = ResolvePublicPhotoTarget{OrgID: link.OrgID, AssetID: assetID}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidShareToken) {
			return ResolvePublicPhotoTarget{}, ErrInvalidShareToken
		}
		if errors.Is(err, store.ErrNotFound) {
			return ResolvePublicPhotoTarget{}, ErrInvalidShareToken
		}
		return ResolvePublicPhotoTarget{}, mapStoreError(err)
	}
	return target, nil
}

// resolveActiveLink hashes the cleartext and looks up the active (unrevoked,
// unexpired) link. Returns ErrInvalidShareToken on empty/malformed cleartext OR
// any not-found — the UNIFORM error that defends against enumeration.
func (s *ShareLinkService) resolveActiveLink(ctx context.Context, cleartext string) (models.ShareLink, error) {
	if !looksLikeShareToken(cleartext) {
		return models.ShareLink{}, ErrInvalidShareToken
	}
	hash := hashShareToken(cleartext)
	now := s.now()

	var out models.ShareLink
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		link, err := s.store.GetActiveByHash(ctx, tx, hash, now)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrInvalidShareToken
			}
			return err
		}
		out = link
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidShareToken) {
			return models.ShareLink{}, ErrInvalidShareToken
		}
		return models.ShareLink{}, mapStoreError(err)
	}
	return out, nil
}

// touchView records a best-effort page view in its own tx. The public read has
// already succeeded; a telemetry failure must NOT fail the page, so any error is
// swallowed (logged-free — there is no PII and nothing actionable per-request).
func (s *ShareLinkService) touchView(ctx context.Context, linkID uuid.UUID) {
	_ = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return s.store.TouchView(ctx, tx, linkID, s.now())
	})
}

// recordAudit writes one audit row inside the supplied tx (rides the mutation).
func (s *ShareLinkService) recordAudit(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, userSub, action string, id uuid.UUID, meta map[string]any) {
	metadata, err := json.Marshal(meta)
	if err != nil {
		return
	}
	s.audit.Record(ctx, tx, AuditEntry{
		OrgID:        orgID,
		UserSub:      userSub,
		Action:       action,
		ResourceType: AuditResourceShareLink,
		ResourceID:   id,
		Metadata:     metadata,
	})
}

// looksLikeShareToken cheaply rejects an obviously-malformed token before a DB
// round-trip: base64url-no-pad of 32 bytes is exactly 43 chars and decodes
// cleanly. Catches probes/typos at the door (same shape check the bootstrap
// SeedBootstrapTokenIfNeeded applies to its cleartext).
func looksLikeShareToken(cleartext string) bool {
	if len(cleartext) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(cleartext)
	return err == nil
}

// generateShareTokenCleartext returns 32 random bytes encoded as base64url-no-pad
// (43 chars). URL-safe so it drops straight into a /p/<token> path.
func generateShareTokenCleartext() (string, error) {
	buf := make([]byte, shareTokenByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashShareToken returns the hex-encoded sha256 of the cleartext. Deterministic
// so the unique index supports a direct lookup; cryptographically sufficient
// because the cleartext carries 256 bits of CSPRNG entropy (identical reasoning
// to hashBootstrapToken).
func hashShareToken(cleartext string) string {
	sum := sha256.Sum256([]byte(cleartext))
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(sum)*2)
	for i, b := range sum {
		out[i*2] = hexdigits[b>>4]
		out[i*2+1] = hexdigits[b&0x0f]
	}
	return string(out)
}
