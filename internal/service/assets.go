package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/storage"
	"github.com/futurebuildai/buildos/internal/store"
)

// Asset audit-log resource type + action constants. `/audit?action_prefix=
// asset.` reconstructs the upload lifecycle of any blob.
const (
	AuditResourceAsset = "asset"

	AuditActionAssetUploadRequested = "asset.upload_requested"
	AuditActionAssetUploaded        = "asset.uploaded"
	// AuditActionAssetLinkedToLog records an operator/field association of
	// confirmed assets to a daily log (Chunk B). ResourceID is the daily_logs id.
	AuditActionAssetLinkedToLog = "asset.linked_to_daily_log"
)

// Asset-storage sentinel errors. Handlers map these to HTTP status codes:
//
//	ErrStorageUnavailable → 503 STORAGE_UNAVAILABLE (object store unconfigured)
//	ErrInvalidInput       → 400 VALIDATION_ERROR (bad content-type / oversize)
//	ErrNotFound           → 404 NOT_FOUND (missing / cross-org / not ready)
var ErrStorageUnavailable = errors.New("assets: object store not configured")

// ErrInvalidPhotoAsset is returned when a daily-log / client-update photo id
// does not resolve to a confirmed ('ready'), org-owned blob whose project (if
// pinned) matches the target — i.e. an unknown, foreign, pending, or
// wrong-project id. Handlers map it to 400 INVALID_PHOTO_ASSET. The error is
// uniform across all failure reasons (no enumeration signal about which id or
// why), mirroring the store's count-based validation.
var ErrInvalidPhotoAsset = errors.New("assets: photo asset is not a confirmed, org-owned blob for this project")

// Upload limits (DAILY_REPORTS_CLIENT_UPDATES §9 defaults: D-1/D-2/D-3).
const (
	// MaxAssetSizeBytes caps a single uploaded object (D-1: 15 MiB). The size is
	// signed into the presigned PUT's Content-Length so R2 rejects an oversized
	// body; this is the server-side declared-size gate.
	MaxAssetSizeBytes int64 = 15 << 20 // 15 MiB
	// MaxAssetsPerDailyLog caps photos per daily log (D-3). Enforced at
	// daily-log persist (Chunk B); exported here as the shared default.
	MaxAssetsPerDailyLog = 20

	presignPutTTL = 5 * time.Minute  // D-5: PUT presign TTL
	presignGetTTL = 15 * time.Minute // D-5: operator-surface GET TTL
)

// allowedAssetContentTypes is the image-only allowlist (D-2). Declared in the
// presign request and SIGNED into the PUT so R2 rejects a mismatched upload.
var allowedAssetContentTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/heic": ".heic",
}

// ObjectStore is the consumer-side port AssetService needs from
// internal/storage. Declared here (not imported as the concrete type at the
// service boundary's call sites) so the storage adapter injects via interface
// and tests substitute an in-memory fake. The method set matches
// storage.ObjectStore exactly, so *storage.R2Store satisfies it directly.
type ObjectStore interface {
	PresignPut(ctx context.Context, key, contentType string, contentLength int64, ttl time.Duration) (url string, signedHeaders map[string]string, err error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (url string, err error)
	Get(ctx context.Context, key string) (body io.ReadCloser, contentType string, err error)
	Delete(ctx context.Context, key string) error
}

// ObjectStoreResolver resolves the per-org ObjectStore at call time. The R2
// access key + secret are sealed in the encrypted vault per (org, provider), so
// the adapter is built from credentials resolved on each call (mirroring how
// the AI client / mailer resolve their per-org key). It MUST soft-fail to
// (nil, nil) when storage is unconfigured for the org — AssetService maps a nil
// store to ErrStorageUnavailable (503), the same posture as AI/mailer. A
// non-nil error is a hard failure (e.g. malformed endpoint) and surfaces as 500.
//
// Wiring lives in internal/service (objectStoreResolverFromVault) so
// internal/storage stays leaf-isolated — it never learns about the vault.
type ObjectStoreResolver func(ctx context.Context, orgID uuid.UUID) (ObjectStore, error)

// AssetService is the business-logic surface for the object-storage substrate
// (Chunk A). It validates uploads (type/size allowlist), generates opaque
// object keys, persists pending→ready asset rows (one-tx + audit each
// mutation), and mints short-lived signed URLs. The blob bytes go DIRECT to R2
// via presigned URLs — they never transit this service on the happy path.
//
// store soft-fails: when the resolver is nil OR yields a nil store (R2
// unconfigured for the fork) every upload/serve path returns
// ErrStorageUnavailable → 503, the same posture as the AI client / mailer when
// their key is missing.
type AssetService struct {
	pool     *pgxpool.Pool
	store    *store.AssetStore
	fields   *store.FieldStore   // daily-log photo linking (Chunk B); may be nil
	resolver ObjectStoreResolver // nil => storage unconfigured (soft-fail)
	audit    AuditRecorder
	logger   *slog.Logger
	now      func() time.Time
}

// NewAssetService builds an AssetService. resolver may be nil (storage
// unconfigured → every storage path 503s). fields may be nil (daily-log linking
// disabled — LinkPhotosToDailyLog 500s a programmer error if invoked). audit nil
// → no-op recorder; logger nil → slog.Default; clock nil → time.Now.
func NewAssetService(pool *pgxpool.Pool, s *store.AssetStore, fields *store.FieldStore, resolver ObjectStoreResolver, audit AuditRecorder, logger *slog.Logger, clock func() time.Time) *AssetService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = time.Now
	}
	return &AssetService{pool: pool, store: s, fields: fields, resolver: resolver, audit: audit, logger: logger, now: clock}
}

// objStore resolves the per-org ObjectStore, returning ErrStorageUnavailable
// when storage is unconfigured for the org (nil resolver / nil store).
func (s *AssetService) objStore(ctx context.Context, orgID uuid.UUID) (ObjectStore, error) {
	if s.resolver == nil {
		return nil, ErrStorageUnavailable
	}
	objs, err := s.resolver(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if objs == nil {
		return nil, ErrStorageUnavailable
	}
	return objs, nil
}

// RequestUploadInput is the validated input for RequestUpload.
type RequestUploadInput struct {
	ProjectID   *uuid.UUID // optional: org-level asset when nil
	ContentType string
	SizeBytes   int64
}

// RequestUploadResult is what RequestUpload returns to the handler: the pending
// asset row, the presigned PUT URL, the headers the client must echo, and the
// expiry.
type RequestUploadResult struct {
	Asset         models.Asset
	UploadURL     string
	SignedHeaders map[string]string
	ExpiresAt     time.Time
}

// RequestUpload validates the declared content-type + size, verifies the
// project is in the caller's org, generates an opaque storage key, INSERTs a
// pending asset row + audit asset.upload_requested in ONE tx, then presigns a
// PUT. The client PUTs bytes directly to R2 with the returned signed headers.
func (s *AssetService) RequestUpload(ctx context.Context, orgID uuid.UUID, userSub string, in RequestUploadInput) (RequestUploadResult, error) {
	if orgID == uuid.Nil {
		return RequestUploadResult{}, fmt.Errorf("%w: org_id required", ErrInvalidInput)
	}
	// Resolve the per-org store first: an unconfigured fork 503s before we
	// create a pending row that could never be uploaded.
	objs, err := s.objStore(ctx, orgID)
	if err != nil {
		return RequestUploadResult{}, err
	}
	ext, ok := allowedAssetContentTypes[strings.ToLower(strings.TrimSpace(in.ContentType))]
	if !ok {
		return RequestUploadResult{}, fmt.Errorf("%w: content_type %q not allowed (images only: jpeg/png/webp/heic)", ErrInvalidInput, in.ContentType)
	}
	if in.SizeBytes <= 0 {
		return RequestUploadResult{}, fmt.Errorf("%w: byte_size must be positive", ErrInvalidInput)
	}
	if in.SizeBytes > MaxAssetSizeBytes {
		return RequestUploadResult{}, fmt.Errorf("%w: byte_size %d exceeds max %d (15 MiB)", ErrInvalidInput, in.SizeBytes, MaxAssetSizeBytes)
	}

	contentType := strings.ToLower(strings.TrimSpace(in.ContentType))
	storageKey := buildStorageKey(orgID, in.ProjectID, ext)

	var asset models.Asset
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if in.ProjectID != nil {
			if err := store.VerifyProjectInOrg(ctx, tx, *in.ProjectID, orgID); err != nil {
				return err // ErrNotFound on cross-org / missing
			}
		}
		a, qErr := s.store.Create(ctx, tx, store.InsertAssetParams{
			OrgID:       orgID,
			ProjectID:   in.ProjectID,
			StorageKey:  storageKey,
			ContentType: contentType,
			SizeBytes:   in.SizeBytes,
			UploadedBy:  userSub,
		})
		if qErr != nil {
			return qErr
		}
		asset = a

		meta, _ := json.Marshal(map[string]any{
			"content_type": contentType,
			"size_bytes":   in.SizeBytes,
			"project_id":   in.ProjectID,
		})
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       AuditActionAssetUploadRequested,
			ResourceType: AuditResourceAsset,
			ResourceID:   a.ID,
			After:        meta,
		})
		return nil
	})
	if err != nil {
		return RequestUploadResult{}, mapStoreError(err)
	}

	// Presign AFTER commit: the pending row must exist before the client can PUT.
	url, headers, err := objs.PresignPut(ctx, storageKey, contentType, in.SizeBytes, presignPutTTL)
	if err != nil {
		return RequestUploadResult{}, fmt.Errorf("presign put: %w", err)
	}
	return RequestUploadResult{
		Asset:         asset,
		UploadURL:     url,
		SignedHeaders: headers,
		ExpiresAt:     s.now().Add(presignPutTTL),
	}, nil
}

// ConfirmUpload transitions a pending asset to ready after the client's PUT
// succeeded. One tx + audit asset.uploaded. checksum is optional. Daily-log
// linking (Chunk B) requires status='ready'.
func (s *AssetService) ConfirmUpload(ctx context.Context, orgID uuid.UUID, userSub string, assetID uuid.UUID, checksum *string) (models.Asset, error) {
	// Confirm is a pure DB status transition (pending -> ready); it does not
	// touch R2, so it does not gate on storage being configured.
	if orgID == uuid.Nil || assetID == uuid.Nil {
		return models.Asset{}, fmt.Errorf("%w: org_id and asset id required", ErrInvalidInput)
	}
	var asset models.Asset
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a, qErr := s.store.MarkReady(ctx, tx, orgID, assetID, checksum)
		if qErr != nil {
			return qErr
		}
		asset = a
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       AuditActionAssetUploaded,
			ResourceType: AuditResourceAsset,
			ResourceID:   a.ID,
		})
		return nil
	})
	if err != nil {
		return models.Asset{}, mapStoreError(err)
	}
	return asset, nil
}

// ValidatePhotoAssets asserts every id in ids resolves to a confirmed
// ('ready'), org-owned blob whose project (if pinned) matches projectID — the
// daily-log photo-link guard (Chunk B). It closes the dangling-id gap: a daily
// log may only reference confirmed, org-owned blobs. Runs inside the caller's tx
// so the validation and the mutation it guards are atomic.
//
// nil/empty ids is a no-op (a daily log with no photos is valid). Returns
// ErrInvalidPhotoAsset (uniform — no enumeration signal) if ANY id fails. Does
// NOT gate on storage being configured: validation is a pure DB read (the rows
// exist regardless of whether R2 is reachable).
func (s *AssetService) ValidatePhotoAssets(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, ids []uuid.UUID) error {
	deduped := dedupeUUIDs(ids)
	if len(deduped) == 0 {
		return nil
	}
	n, err := s.store.CountReadyForProject(ctx, tx, orgID, projectID, deduped)
	if err != nil {
		return err
	}
	if n != len(deduped) {
		return ErrInvalidPhotoAsset
	}
	return nil
}

// LinkPhotosToDailyLog associates already-confirmed assets with the daily log
// for a (project, date) — the operator/web "Add photos" path (Chunk B §3) and
// the field reconciliation path. It (1) verifies the project is in the org, (2)
// validates every asset id is ready + org-owned + project-matched, (3) unions
// the ids into the day's daily_logs row (de-duped, capped at
// MaxAssetsPerDailyLog), (4) audits asset.linked_to_daily_log — all in ONE tx.
//
// Errors: ErrNotFound (project / daily-log for that day missing or cross-org),
// ErrInvalidPhotoAsset (any unknown/foreign/pending/wrong-project id),
// ErrInvalidInput (oversize set / bad args). The append is idempotent: re-linking
// already-attached ids is a no-op union.
func (s *AssetService) LinkPhotosToDailyLog(ctx context.Context, orgID uuid.UUID, userSub string, projectID uuid.UUID, day time.Time, assetIDs []uuid.UUID) (models.DailyLog, error) {
	if orgID == uuid.Nil || projectID == uuid.Nil {
		return models.DailyLog{}, fmt.Errorf("%w: org_id and project_id are required", ErrInvalidInput)
	}
	if day.IsZero() {
		return models.DailyLog{}, fmt.Errorf("%w: log_date is required", ErrInvalidInput)
	}
	deduped := dedupeUUIDs(assetIDs)
	if len(deduped) == 0 {
		return models.DailyLog{}, fmt.Errorf("%w: at least one asset id is required", ErrInvalidInput)
	}
	if len(deduped) > MaxAssetsPerDailyLog {
		return models.DailyLog{}, fmt.Errorf("%w: cannot link more than %d photos at once", ErrInvalidInput, MaxAssetsPerDailyLog)
	}
	if s.fields == nil {
		// Programmer error: the service was wired without the FieldStore.
		return models.DailyLog{}, fmt.Errorf("assets: daily-log linking not configured")
	}

	var dl models.DailyLog
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, orgID); err != nil {
			return err
		}
		if err := s.ValidatePhotoAssets(ctx, tx, orgID, projectID, deduped); err != nil {
			return err
		}
		row, err := s.fields.AppendDailyLogPhotos(ctx, tx, store.AppendDailyLogPhotosParams{
			OrgID:     orgID,
			ProjectID: projectID,
			LogDate:   day,
			AssetIDs:  deduped,
			MaxPhotos: MaxAssetsPerDailyLog,
		})
		if err != nil {
			return err
		}
		dl = row
		meta, _ := json.Marshal(map[string]any{
			"project_id":  projectID,
			"log_date":    day.UTC().Format("2006-01-02"),
			"asset_count": len(deduped),
		})
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       AuditActionAssetLinkedToLog,
			ResourceType: AuditResourceAsset,
			ResourceID:   row.ID,
			Metadata:     meta,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrPhotoLimit) {
			return models.DailyLog{}, fmt.Errorf("%w: daily log would exceed %d photos", ErrInvalidInput, MaxAssetsPerDailyLog)
		}
		if errors.Is(err, ErrInvalidPhotoAsset) {
			return models.DailyLog{}, ErrInvalidPhotoAsset
		}
		return models.DailyLog{}, mapStoreError(err)
	}
	return dl, nil
}

// dedupeUUIDs returns the distinct non-nil ids in input order.
func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// GetAsset returns a single asset's metadata, org-scoped. ErrNotFound on
// missing / cross-org. (Reads are not audited — house style.)
func (s *AssetService) GetAsset(ctx context.Context, orgID, assetID uuid.UUID) (models.Asset, error) {
	if orgID == uuid.Nil || assetID == uuid.Nil {
		return models.Asset{}, fmt.Errorf("%w: org_id and asset id required", ErrInvalidInput)
	}
	var asset models.Asset
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		a, qErr := s.store.GetByID(ctx, tx, orgID, assetID)
		if qErr != nil {
			return qErr
		}
		asset = a
		return nil
	})
	if err != nil {
		return models.Asset{}, mapStoreError(err)
	}
	return asset, nil
}

// SignedGetURL verifies the asset is in the caller's org and 'ready', then
// returns a short-lived presigned GET URL. ErrStorageUnavailable when the
// store is unconfigured; ErrNotFound on missing / cross-org / not-ready.
func (s *AssetService) SignedGetURL(ctx context.Context, orgID, assetID uuid.UUID, ttl time.Duration) (string, error) {
	objs, err := s.objStore(ctx, orgID)
	if err != nil {
		return "", err
	}
	asset, err := s.GetAsset(ctx, orgID, assetID)
	if err != nil {
		return "", err
	}
	if asset.Status != models.AssetStatusReady {
		// A pending/failed asset has no servable bytes; uniform NotFound.
		return "", ErrNotFound
	}
	if ttl <= 0 {
		ttl = presignGetTTL
	}
	return objs.PresignGet(ctx, asset.StorageKey, ttl)
}

// ListProjectAssets returns an org's project assets newest-first. readyOnly
// defaults the gallery to confirmed blobs. (Read; not audited.)
func (s *AssetService) ListProjectAssets(ctx context.Context, orgID, projectID uuid.UUID, readyOnly bool) ([]models.Asset, error) {
	if orgID == uuid.Nil || projectID == uuid.Nil {
		return nil, fmt.Errorf("%w: org_id and project id required", ErrInvalidInput)
	}
	var out []models.Asset
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, orgID); err != nil {
			return err
		}
		rows, qErr := s.store.ListByProject(ctx, tx, orgID, projectID, readyOnly)
		if qErr != nil {
			return qErr
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return out, nil
}

// ServeAsset is the same-origin proxy path used by the PUBLIC homeowner page
// (its only caller — the operator surface redirects to a signed R2 URL instead).
// It verifies the asset is in the caller's org + ready, streams the bytes from
// the object store, and — for image types stdlib can decode (jpeg/png) —
// re-encodes to STRIP EXIF metadata (D-4: GPS EXIF is Restricted PII and must
// never reach a client/public page).
//
// SECURITY (review finding M1): a type we cannot decode-and-re-encode (webp,
// heic — HEIC is the iPhone default capture format) would otherwise be streamed
// RAW, EXIF intact, to the unauthenticated homeowner. We therefore REFUSE to
// serve any type the stripper can't process: ErrNotFound (the public proxy maps
// it to a 404 image). A field-captured HEIC/WebP photo curated into a client
// update shows as a broken image rather than leaking the homeowner's GPS — until
// a webp/heic decoder (or an upload-time JPEG derivative) lands. The upload
// allowlist still accepts webp/heic (operators view them internally via the
// authenticated signed-URL path, which never goes through this proxy).
// The caller MUST Close the returned reader.
func (s *AssetService) ServeAsset(ctx context.Context, orgID, assetID uuid.UUID) (io.ReadCloser, string, error) {
	objs, err := s.objStore(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	asset, err := s.GetAsset(ctx, orgID, assetID)
	if err != nil {
		return nil, "", err
	}
	if asset.Status != models.AssetStatusReady {
		return nil, "", ErrNotFound
	}
	body, ct, err := objs.Get(ctx, asset.StorageKey)
	if err != nil {
		return nil, "", fmt.Errorf("serve asset: %w", err)
	}
	if ct == "" {
		ct = asset.ContentType
	}
	stripped, newCT, ok := stripImageMetadata(body, ct)
	_ = body.Close()
	if !ok {
		// Cannot strip EXIF from this type → do NOT serve it on the public proxy.
		// Streaming raw bytes here would leak GPS EXIF to an unauthenticated viewer.
		slog.WarnContext(ctx, "asset.serve.unstrippable_type_refused",
			"asset_id", assetID, "content_type", ct)
		return nil, "", ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(stripped)), newCT, nil
}

// stripImageMetadata decodes a jpeg/png and re-encodes it, dropping ALL
// metadata (incl. EXIF GPS) as a side effect of the stdlib decode→encode round
// trip. Returns (bytes, contentType, true) on success, or (nil, "", false) when
// the type isn't stdlib-decodable (caller streams raw). It is bounded by
// MaxAssetSizeBytes to cap memory on a hostile/huge object.
func stripImageMetadata(r io.Reader, contentType string) ([]byte, string, bool) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	limited := io.LimitReader(r, MaxAssetSizeBytes+1)
	switch ct {
	case "image/jpeg":
		img, err := jpeg.Decode(limited)
		if err != nil {
			return nil, "", false
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", false
		}
		return buf.Bytes(), "image/jpeg", true
	case "image/png":
		img, err := png.Decode(limited)
		if err != nil {
			return nil, "", false
		}
		var buf bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(&buf, img); err != nil {
			return nil, "", false
		}
		return buf.Bytes(), "image/png", true
	default:
		return nil, "", false
	}
}

// buildStorageKey generates the opaque object key for a new asset. Convention:
// org/<org>/project/<proj>/<uuid><ext> (or org/<org>/<uuid><ext> for org-level
// assets). The key contains only Internal UUIDs — no PII.
func buildStorageKey(orgID uuid.UUID, projectID *uuid.UUID, ext string) string {
	id := uuid.NewString()
	if projectID != nil {
		return path.Join("org", orgID.String(), "project", projectID.String(), id+ext)
	}
	return path.Join("org", orgID.String(), id+ext)
}

// ObjectStoreCredsResolver resolves the per-org R2 access key + secret from the
// encrypted vault. *VaultService satisfies this (ObjectStoreCreds). Declared as
// an interface so cmd/server wires the resolver without AssetService importing
// the concrete vault type, and so tests can inject a fake.
type ObjectStoreCredsResolver interface {
	ObjectStoreCreds(ctx context.Context, orgID uuid.UUID) (accessKeyID, secretAccessKey string, err error)
}

// ObjectStoreConfig is the non-secret per-fork object-store configuration
// (endpoint + bucket + region) supplied from internal/config. Mirrors the
// storage.Config shape minus the secrets, which come from the vault.
type ObjectStoreConfig struct {
	Endpoint string
	Bucket   string
	Region   string
}

// NewVaultObjectStoreResolver builds the ObjectStoreResolver AssetService uses.
// It SOFT-FAILS to (nil, nil) — storage unconfigured — when the endpoint/bucket
// are empty OR the vault has no object_store credential for the org. Otherwise
// it constructs an *storage.R2Store from the config + the per-org vault creds.
// This is the only place the vault and internal/storage meet; internal/storage
// stays leaf-isolated (it never imports the vault).
//
// creds may be nil (vault disabled) → always unconfigured.
func NewVaultObjectStoreResolver(cfg ObjectStoreConfig, creds ObjectStoreCredsResolver) ObjectStoreResolver {
	return func(ctx context.Context, orgID uuid.UUID) (ObjectStore, error) {
		if cfg.Endpoint == "" || cfg.Bucket == "" || creds == nil {
			return nil, nil // soft-fail: storage not configured for this fork
		}
		accessKey, secret, err := creds.ObjectStoreCreds(ctx, orgID)
		if err != nil {
			return nil, err
		}
		if accessKey == "" || secret == "" {
			return nil, nil // soft-fail: no credential sealed for this org
		}
		st, err := storage.NewR2Store(storage.Config{
			Endpoint:        cfg.Endpoint,
			Bucket:          cfg.Bucket,
			Region:          cfg.Region,
			AccessKeyID:     accessKey,
			SecretAccessKey: secret,
		})
		if err != nil {
			if errors.Is(err, storage.ErrUnconfigured) {
				return nil, nil // soft-fail
			}
			return nil, err
		}
		return st, nil
	}
}
