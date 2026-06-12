//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// photoLinkFixture wires an AssetService (the PhotoValidator) and a FieldService
// with that validator injected, over a fresh pool. Returns the asset store so
// tests can seed ready/pending assets directly, plus the org/user/project seed.
type photoLinkFixture struct {
	field   *FieldService
	asset   *AssetService
	store   *store.AssetStore
	pool    *pgxpool.Pool
	orgID   uuid.UUID
	subject string
	projID  uuid.UUID
}

func newPhotoLinkFixture(t *testing.T) photoLinkFixture {
	t.Helper()
	pool := testdb.NewPool(t)
	astStore := store.NewAssetStore()
	fieldStore := store.NewFieldStore()

	// A nil resolver means storage is "unconfigured" for presign/serve, but the
	// photo VALIDATION + linking paths are pure DB reads/writes that do not gate
	// on the object store — exactly what Chunk B's link path needs.
	asset := NewAssetService(pool, astStore, fieldStore, nil, &capturingAuditRecorder{}, nil, nil)
	field := NewFieldService(pool, fieldStore, store.NewFeedCardsStore(), &capturingAuditRecorder{}).
		WithPhotoValidator(asset)

	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "Photo Project")

	return photoLinkFixture{
		field: field, asset: asset, store: astStore, pool: pool,
		orgID: orgID, subject: userID.String(), projID: projID,
	}
}

// seedAsset inserts an asset row (pending) and optionally marks it ready.
func (f photoLinkFixture) seedAsset(t *testing.T, orgID uuid.UUID, projectID *uuid.UUID, ready bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pgx.BeginTxFunc(ctx, f.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a, err := f.store.Create(ctx, tx, store.InsertAssetParams{
			OrgID:       orgID,
			ProjectID:   projectID,
			StorageKey:  "org/" + orgID.String() + "/" + uuid.NewString() + ".jpg",
			ContentType: "image/jpeg",
			SizeBytes:   1024,
			UploadedBy:  f.subject,
		})
		if err != nil {
			return err
		}
		id = a.ID
		if ready {
			if _, err := f.store.MarkReady(ctx, tx, orgID, a.ID, nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	return id
}

// TestDailyLog_PhotoValidation proves the field daily-log write rejects photo
// ids that are not confirmed, org-owned blobs for the project, and accepts
// ready org-owned ones.
func TestDailyLog_PhotoValidation(t *testing.T) {
	f := newPhotoLinkFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()

	ready := f.seedAsset(t, f.orgID, &f.projID, true)    // good
	pending := f.seedAsset(t, f.orgID, &f.projID, false) // not confirmed
	orgLevel := f.seedAsset(t, f.orgID, nil, true)       // org-level, ready (allowed)

	// Cross-org asset (different org + its own project).
	otherOrg := uuid.New()
	otherProj := uuid.New()
	testdb.SeedOrg(t, f.pool, otherOrg, "Other")
	testdb.SeedProject(t, f.pool, otherProj, otherOrg, "Other Project")
	foreign := f.seedAsset(t, otherOrg, &otherProj, true)

	// Asset pinned to a DIFFERENT project in the SAME org → must be rejected.
	otherSameOrgProj := uuid.New()
	testdb.SeedProject(t, f.pool, otherSameOrgProj, f.orgID, "Sibling Project")
	wrongProject := f.seedAsset(t, f.orgID, &otherSameOrgProj, true)

	mk := func(ids ...uuid.UUID) DailyLogInput {
		return DailyLogInput{
			ProjectID:      f.projID,
			LogDate:        now,
			WorkSummary:    "framing complete",
			PhotoAssetIDs:  ids,
			IdempotencyKey: uuid.New(),
		}
	}

	// Happy path: ready org-owned (project + org-level) accepted.
	if _, err := f.field.DailyLog(ctx, f.orgID, f.subject, mk(ready, orgLevel)); err != nil {
		t.Fatalf("DailyLog with ready ids: unexpected err %v", err)
	}

	bad := []struct {
		name string
		ids  []uuid.UUID
	}{
		{"unknown id", []uuid.UUID{uuid.New()}},
		{"pending id", []uuid.UUID{pending}},
		{"foreign (cross-org) id", []uuid.UUID{foreign}},
		{"wrong-project id", []uuid.UUID{wrongProject}},
		{"mixed good+foreign", []uuid.UUID{ready, foreign}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.field.DailyLog(ctx, f.orgID, f.subject, mk(c.ids...))
			if !errors.Is(err, ErrInvalidPhotoAsset) {
				t.Errorf("err = %v, want ErrInvalidPhotoAsset", err)
			}
		})
	}
}

// TestLinkPhotosToDailyLog drives the operator/web "Add photos" path against a
// real DB: it appends confirmed assets to an EXISTING daily log, is idempotent,
// rejects cross-org and pending ids, 404s when no log exists, and 404s on a
// cross-org project.
func TestLinkPhotosToDailyLog(t *testing.T) {
	f := newPhotoLinkFixture(t)
	ctx := context.Background()
	day := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)

	// A daily log to attach photos to (no photos initially).
	if _, err := f.field.DailyLog(ctx, f.orgID, f.subject, DailyLogInput{
		ProjectID:      f.projID,
		LogDate:        day,
		WorkSummary:    "drywall",
		IdempotencyKey: uuid.New(),
	}); err != nil {
		t.Fatalf("seed daily log: %v", err)
	}

	a1 := f.seedAsset(t, f.orgID, &f.projID, true)
	a2 := f.seedAsset(t, f.orgID, &f.projID, true)

	// Link two confirmed assets.
	dl, err := f.asset.LinkPhotosToDailyLog(ctx, f.orgID, f.subject, f.projID, day, []uuid.UUID{a1, a2})
	if err != nil {
		t.Fatalf("link photos: %v", err)
	}
	if len(dl.PhotoAssetIDs) != 2 {
		t.Fatalf("after link, photo_asset_ids = %v, want 2", dl.PhotoAssetIDs)
	}

	// Idempotent: re-linking the same ids is a no-op union (still 2).
	dl, err = f.asset.LinkPhotosToDailyLog(ctx, f.orgID, f.subject, f.projID, day, []uuid.UUID{a1})
	if err != nil {
		t.Fatalf("re-link: %v", err)
	}
	if len(dl.PhotoAssetIDs) != 2 {
		t.Fatalf("after re-link, photo_asset_ids = %v, want 2 (idempotent)", dl.PhotoAssetIDs)
	}

	// Pending id → INVALID_PHOTO_ASSET.
	pending := f.seedAsset(t, f.orgID, &f.projID, false)
	if _, err := f.asset.LinkPhotosToDailyLog(ctx, f.orgID, f.subject, f.projID, day, []uuid.UUID{pending}); !errors.Is(err, ErrInvalidPhotoAsset) {
		t.Errorf("pending link: err = %v, want ErrInvalidPhotoAsset", err)
	}

	// Cross-org asset → INVALID_PHOTO_ASSET.
	otherOrg := uuid.New()
	otherProj := uuid.New()
	testdb.SeedOrg(t, f.pool, otherOrg, "Other")
	testdb.SeedProject(t, f.pool, otherProj, otherOrg, "Other Project")
	foreign := f.seedAsset(t, otherOrg, &otherProj, true)
	if _, err := f.asset.LinkPhotosToDailyLog(ctx, f.orgID, f.subject, f.projID, day, []uuid.UUID{foreign}); !errors.Is(err, ErrInvalidPhotoAsset) {
		t.Errorf("foreign link: err = %v, want ErrInvalidPhotoAsset", err)
	}

	// No daily log on a different day → ErrNotFound.
	a3 := f.seedAsset(t, f.orgID, &f.projID, true)
	noLogDay := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	if _, err := f.asset.LinkPhotosToDailyLog(ctx, f.orgID, f.subject, f.projID, noLogDay, []uuid.UUID{a3}); !errors.Is(err, ErrNotFound) {
		t.Errorf("link to absent log: err = %v, want ErrNotFound", err)
	}

	// Cross-org project (project not in caller's org) → ErrNotFound.
	if _, err := f.asset.LinkPhotosToDailyLog(ctx, f.orgID, f.subject, otherProj, day, []uuid.UUID{a3}); !errors.Is(err, ErrNotFound) {
		t.Errorf("link to cross-org project: err = %v, want ErrNotFound", err)
	}
}

// seedAssetCT inserts a ready asset with a specific content type, so ServeAsset
// can be exercised against an un-strippable (webp/heic) blob.
func (f photoLinkFixture) seedAssetCT(t *testing.T, orgID uuid.UUID, projectID *uuid.UUID, contentType, ext string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pgx.BeginTxFunc(ctx, f.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a, err := f.store.Create(ctx, tx, store.InsertAssetParams{
			OrgID:       orgID,
			ProjectID:   projectID,
			StorageKey:  "org/" + orgID.String() + "/" + uuid.NewString() + ext,
			ContentType: contentType,
			SizeBytes:   1024,
			UploadedBy:  f.subject,
		})
		if err != nil {
			return err
		}
		id = a.ID
		_, err = f.store.MarkReady(ctx, tx, orgID, a.ID, nil)
		return err
	})
	if err != nil {
		t.Fatalf("seed asset (%s): %v", contentType, err)
	}
	return id
}

// TestServeAsset_RefusesUnstrippableType is the regression guard for review
// finding M1: the public photo proxy (ServeAsset is its only caller) must NEVER
// stream a type it cannot EXIF-strip (webp/heic — HEIC is the iPhone default),
// because the raw bytes would leak GPS EXIF to an unauthenticated homeowner.
// A ready webp asset → ErrNotFound (the proxy maps it to a 404 image).
func TestServeAsset_RefusesUnstrippableType(t *testing.T) {
	f := newPhotoLinkFixture(t)
	ctx := context.Background()

	// An AssetService with a CONFIGURED object store (the fixture's default is
	// nil → 503). The fake returns webp-ish bytes; the strip decision falls back
	// to the asset row's content_type when the store reports an empty CT.
	fake := &fakeObjectStore{getBody: io.NopCloser(strings.NewReader("RIFF....WEBPdata")), getCT: ""}
	svc := NewAssetService(f.pool, f.store, store.NewFieldStore(), configuredResolver(fake), &capturingAuditRecorder{}, nil, nil)

	webp := f.seedAssetCT(t, f.orgID, &f.projID, "image/webp", ".webp")
	if _, _, err := svc.ServeAsset(ctx, f.orgID, webp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ServeAsset(webp) = %v, want ErrNotFound (un-strippable type must not stream raw on the public proxy)", err)
	}

	heic := f.seedAssetCT(t, f.orgID, &f.projID, "image/heic", ".heic")
	if _, _, err := svc.ServeAsset(ctx, f.orgID, heic); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ServeAsset(heic) = %v, want ErrNotFound", err)
	}
}

// TestReportProgress_PhotoValidation is the regression guard for review finding
// #6: FieldService.ReportProgress previously stored an unvalidated photo_asset_id.
// It now enforces the same confirmed+org+project invariant the daily-log path
// does — a pinned photo must be a ready, org-owned blob for the task's project.
func TestReportProgress_PhotoValidation(t *testing.T) {
	f := newPhotoLinkFixture(t)
	ctx := context.Background()
	uid := uuid.MustParse(f.subject)
	taskID := seedFieldServiceTask(t, f.pool, f.projID, uid, "progress-photo-task")

	// A pending (unconfirmed) asset for the right project → rejected.
	pending := f.seedAsset(t, f.orgID, &f.projID, false)
	if _, err := f.field.ReportProgress(ctx, f.orgID, f.subject, ReportProgressInput{
		TaskID: taskID, PercentComplete: 30, IdempotencyKey: uuid.New(), PhotoAssetID: &pending,
	}); !errors.Is(err, ErrInvalidPhotoAsset) {
		t.Errorf("pending photo: err = %v, want ErrInvalidPhotoAsset", err)
	}

	// A ready asset from a DIFFERENT org → rejected.
	otherOrg := uuid.New()
	otherProj := uuid.New()
	testdb.SeedOrg(t, f.pool, otherOrg, "Other Org")
	testdb.SeedProject(t, f.pool, otherProj, otherOrg, "Other Project")
	foreign := f.seedAsset(t, otherOrg, &otherProj, true)
	if _, err := f.field.ReportProgress(ctx, f.orgID, f.subject, ReportProgressInput{
		TaskID: taskID, PercentComplete: 30, IdempotencyKey: uuid.New(), PhotoAssetID: &foreign,
	}); !errors.Is(err, ErrInvalidPhotoAsset) {
		t.Errorf("foreign-org photo: err = %v, want ErrInvalidPhotoAsset", err)
	}

	// A ready, org-owned asset for the task's project → accepted.
	good := f.seedAsset(t, f.orgID, &f.projID, true)
	if _, err := f.field.ReportProgress(ctx, f.orgID, f.subject, ReportProgressInput{
		TaskID: taskID, PercentComplete: 40, IdempotencyKey: uuid.New(), PhotoAssetID: &good,
	}); err != nil {
		t.Errorf("valid photo: err = %v, want success", err)
	}
}
