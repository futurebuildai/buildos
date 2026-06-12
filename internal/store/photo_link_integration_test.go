//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestCountReadyForProject covers the photo-link validation primitive: only
// 'ready', org-owned, project-matched (or org-level) assets are counted.
func TestCountReadyForProject(t *testing.T) {
	pool := testdb.NewPool(t)
	as := NewAssetStore()
	ctx := context.Background()

	orgID := uuid.New()
	projID := uuid.New()
	otherProj := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Org")
	testdb.SeedProject(t, pool, projID, orgID, "Proj")
	testdb.SeedProject(t, pool, otherProj, orgID, "OtherProj")

	mk := func(project *uuid.UUID, ready bool) uuid.UUID {
		var id uuid.UUID
		withTx(t, pool, func(tx pgx.Tx) error {
			a, err := as.Create(ctx, tx, InsertAssetParams{
				OrgID:       orgID,
				ProjectID:   project,
				StorageKey:  "k/" + uuid.NewString(),
				ContentType: "image/jpeg",
				SizeBytes:   10,
				UploadedBy:  "sub",
			})
			if err != nil {
				return err
			}
			id = a.ID
			if ready {
				_, err = as.MarkReady(ctx, tx, orgID, a.ID, nil)
			}
			return err
		})
		return id
	}

	readyProj := mk(&projID, true)
	pendingProj := mk(&projID, false)
	readyOrgLevel := mk(nil, true)
	readyWrongProj := mk(&otherProj, true)

	withTx(t, pool, func(tx pgx.Tx) error {
		// ready project + org-level → both count.
		n, err := as.CountReadyForProject(ctx, tx, orgID, projID, []uuid.UUID{readyProj, readyOrgLevel})
		if err != nil {
			return err
		}
		if n != 2 {
			t.Errorf("ready+orglevel count = %d, want 2", n)
		}
		// pending not counted.
		n, _ = as.CountReadyForProject(ctx, tx, orgID, projID, []uuid.UUID{pendingProj})
		if n != 0 {
			t.Errorf("pending count = %d, want 0", n)
		}
		// wrong-project not counted.
		n, _ = as.CountReadyForProject(ctx, tx, orgID, projID, []uuid.UUID{readyWrongProj})
		if n != 0 {
			t.Errorf("wrong-project count = %d, want 0", n)
		}
		// empty ids → 0, no error.
		n, err = as.CountReadyForProject(ctx, tx, orgID, projID, nil)
		if err != nil || n != 0 {
			t.Errorf("empty ids count = %d err = %v, want 0/nil", n, err)
		}
		return nil
	})
}

// TestAppendDailyLogPhotos covers the union-append: existing-first ordering,
// de-dup, the per-log cap, and the absent-log ErrNotFound.
func TestAppendDailyLogPhotos(t *testing.T) {
	pool := testdb.NewPool(t)
	fs := NewFieldStore()
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Org")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "Proj")

	day := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	existing := uuid.New()
	// Seed a daily log with one existing photo id.
	if _, err := pool.Exec(ctx, `
		INSERT INTO daily_logs (org_id, project_id, reported_by, log_date, work_summary, photo_asset_ids, idempotency_key)
		VALUES ($1, $2, $3, $4, 'work', ARRAY[$5]::uuid[], $6)`,
		orgID, projID, userID, day, existing, uuid.New()); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	newA, newB := uuid.New(), uuid.New()
	withTx(t, pool, func(tx pgx.Tx) error {
		dl, err := fs.AppendDailyLogPhotos(ctx, tx, AppendDailyLogPhotosParams{
			OrgID: orgID, ProjectID: projID, LogDate: day,
			AssetIDs: []uuid.UUID{newA, existing, newB}, MaxPhotos: 20,
		})
		if err != nil {
			return err
		}
		// existing first, then the new ones, de-duped (existing not doubled).
		if len(dl.PhotoAssetIDs) != 3 {
			t.Errorf("len = %d, want 3 (%v)", len(dl.PhotoAssetIDs), dl.PhotoAssetIDs)
		}
		if dl.PhotoAssetIDs[0] != existing {
			t.Errorf("first id = %v, want existing %v", dl.PhotoAssetIDs[0], existing)
		}
		return nil
	})

	// Cap guard: a max of 3 with one more id (4 distinct) → ErrPhotoLimit.
	withTx(t, pool, func(tx pgx.Tx) error {
		_, err := fs.AppendDailyLogPhotos(ctx, tx, AppendDailyLogPhotosParams{
			OrgID: orgID, ProjectID: projID, LogDate: day,
			AssetIDs: []uuid.UUID{uuid.New()}, MaxPhotos: 3,
		})
		if !errors.Is(err, ErrPhotoLimit) {
			t.Errorf("cap exceeded: err = %v, want ErrPhotoLimit", err)
		}
		return nil
	})

	// Absent log (different day) → ErrNotFound.
	withTx(t, pool, func(tx pgx.Tx) error {
		_, err := fs.AppendDailyLogPhotos(ctx, tx, AppendDailyLogPhotosParams{
			OrgID: orgID, ProjectID: projID,
			LogDate:  time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
			AssetIDs: []uuid.UUID{uuid.New()}, MaxPhotos: 20,
		})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("absent log: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
