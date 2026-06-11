//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// withTx runs fn inside a committed read-write tx against the pool.
func withTx(t *testing.T, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) {
	t.Helper()
	if err := pgx.BeginTxFunc(context.Background(), pool, pgx.TxOptions{}, fn); err != nil {
		t.Fatalf("tx: %v", err)
	}
}

// TestAssetStore_RoundTrip exercises the full lifecycle against a real Postgres:
// Create (pending) -> MarkReady (ready) -> GetByID -> ListByProject.
func TestAssetStore_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAssetStore()
	ctx := context.Background()

	orgID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Asset Org")
	testdb.SeedProject(t, pool, projID, orgID, "Asset Project")

	var created models.Asset
	withTx(t, pool, func(tx pgx.Tx) error {
		a, err := s.Create(ctx, tx, InsertAssetParams{
			OrgID:       orgID,
			ProjectID:   &projID,
			StorageKey:  "org/" + orgID.String() + "/project/" + projID.String() + "/a.jpg",
			ContentType: "image/jpeg",
			SizeBytes:   2048,
			UploadedBy:  "sub-1",
		})
		if err != nil {
			return err
		}
		created = a
		return nil
	})
	if created.Status != models.AssetStatusPending {
		t.Fatalf("status after create = %q, want pending", created.Status)
	}
	if created.ConfirmedAt != nil {
		t.Fatalf("confirmed_at should be nil on a pending asset")
	}

	// MarkReady with a checksum.
	checksum := "deadbeef"
	var ready models.Asset
	withTx(t, pool, func(tx pgx.Tx) error {
		a, err := s.MarkReady(ctx, tx, orgID, created.ID, &checksum)
		if err != nil {
			return err
		}
		ready = a
		return nil
	})
	if ready.Status != models.AssetStatusReady {
		t.Fatalf("status after mark-ready = %q, want ready", ready.Status)
	}
	if ready.ConfirmedAt == nil {
		t.Fatalf("confirmed_at should be set after mark-ready")
	}
	if ready.ChecksumSHA256 == nil || *ready.ChecksumSHA256 != checksum {
		t.Fatalf("checksum = %v, want %q", ready.ChecksumSHA256, checksum)
	}

	// MarkReady again is a no-op (only pending rows transition) -> ErrNotFound.
	withTxExpectErr(t, pool, ErrNotFound, func(tx pgx.Tx) error {
		_, err := s.MarkReady(ctx, tx, orgID, created.ID, nil)
		return err
	})

	// GetByID returns the ready asset.
	withTx(t, pool, func(tx pgx.Tx) error {
		got, err := s.GetByID(ctx, tx, orgID, created.ID)
		if err != nil {
			return err
		}
		if got.ID != created.ID || got.Status != models.AssetStatusReady {
			t.Fatalf("GetByID returned wrong row: %+v", got)
		}
		return nil
	})

	// ListByProject (ready-only) returns the one asset.
	withTx(t, pool, func(tx pgx.Tx) error {
		list, err := s.ListByProject(ctx, tx, orgID, projID, true)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != created.ID {
			t.Fatalf("ListByProject = %d rows, want 1 (%v)", len(list), list)
		}
		return nil
	})

	// ListByIDs (ready-only) resolves the id.
	withTx(t, pool, func(tx pgx.Tx) error {
		list, err := s.ListByIDs(ctx, tx, orgID, []uuid.UUID{created.ID, uuid.New()})
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != created.ID {
			t.Fatalf("ListByIDs = %d rows, want 1", len(list))
		}
		return nil
	})
}

// TestAssetStore_CrossOrgIsolation proves another org cannot read or mutate an
// asset: GetByID / MarkReady / ListByProject with a foreign org_id all behave as
// not-found (ErrNotFound or empty).
func TestAssetStore_CrossOrgIsolation(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAssetStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedProject(t, pool, projA, orgA, "Proj A")

	var asset models.Asset
	withTx(t, pool, func(tx pgx.Tx) error {
		a, err := s.Create(ctx, tx, InsertAssetParams{
			OrgID:       orgA,
			ProjectID:   &projA,
			StorageKey:  "org/" + orgA.String() + "/x.png",
			ContentType: "image/png",
			SizeBytes:   100,
			UploadedBy:  "sub-a",
		})
		if err != nil {
			return err
		}
		asset = a
		return nil
	})

	// Org B cannot GetByID org A's asset.
	withTxExpectErr(t, pool, ErrNotFound, func(tx pgx.Tx) error {
		_, err := s.GetByID(ctx, tx, orgB, asset.ID)
		return err
	})
	// Org B cannot MarkReady org A's asset.
	withTxExpectErr(t, pool, ErrNotFound, func(tx pgx.Tx) error {
		_, err := s.MarkReady(ctx, tx, orgB, asset.ID, nil)
		return err
	})
	// Org B cannot MarkFailed org A's asset.
	withTxExpectErr(t, pool, ErrNotFound, func(tx pgx.Tx) error {
		return s.MarkFailed(ctx, tx, orgB, asset.ID)
	})
	// Org B's ListByProject for org A's project returns nothing.
	withTx(t, pool, func(tx pgx.Tx) error {
		list, err := s.ListByProject(ctx, tx, orgB, projA, false)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("cross-org ListByProject returned %d rows, want 0", len(list))
		}
		return nil
	})
	// Org B's ListByIDs cannot resolve org A's asset id (mark it ready first).
	withTx(t, pool, func(tx pgx.Tx) error {
		_, err := s.MarkReady(ctx, tx, orgA, asset.ID, nil)
		return err
	})
	withTx(t, pool, func(tx pgx.Tx) error {
		list, err := s.ListByIDs(ctx, tx, orgB, []uuid.UUID{asset.ID})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("cross-org ListByIDs returned %d rows, want 0", len(list))
		}
		return nil
	})
}

// TestAssetStore_UniqueStorageKey proves the unique index on storage_key rejects
// a duplicate object key.
func TestAssetStore_UniqueStorageKey(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAssetStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Org Dup")

	key := "org/" + orgID.String() + "/dup.jpg"
	withTx(t, pool, func(tx pgx.Tx) error {
		_, err := s.Create(ctx, tx, InsertAssetParams{
			OrgID: orgID, StorageKey: key, ContentType: "image/jpeg", SizeBytes: 1, UploadedBy: "s",
		})
		return err
	})

	// A second insert with the same key must violate the unique index.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, e := s.Create(ctx, tx, InsertAssetParams{
			OrgID: orgID, StorageKey: key, ContentType: "image/jpeg", SizeBytes: 2, UploadedBy: "s",
		})
		return e
	})
	if err == nil {
		t.Fatal("duplicate storage_key insert succeeded, want unique-violation error")
	}
}

// withTxExpectErr runs fn in a tx and asserts the returned error matches want.
func withTxExpectErr(t *testing.T, pool *pgxpool.Pool, want error, fn func(tx pgx.Tx) error) {
	t.Helper()
	err := pgx.BeginTxFunc(context.Background(), pool, pgx.TxOptions{}, fn)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
