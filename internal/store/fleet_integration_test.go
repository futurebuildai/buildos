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

func TestFleetStore_CreateAsset_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFleetStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")

	serial := "SN-12345"
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.CreateAsset(ctx, tx, CreateAssetParams{
			OrgID:        orgID,
			Name:         "Cat 320 excavator",
			AssetType:    "excavator",
			SerialNumber: &serial,
		})
		if err != nil {
			return err
		}
		if got.OrgID != orgID || got.Name != "Cat 320 excavator" || got.AssetType != "excavator" {
			t.Errorf("round-trip mismatch: %+v", got)
		}
		if got.Status != "available" {
			t.Errorf("default status = %q, want available", got.Status)
		}
		if got.SerialNumber == nil || *got.SerialNumber != serial {
			t.Errorf("serial = %v, want %s", got.SerialNumber, serial)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
}

func TestFleetStore_ListAssets_OrderingAndCrossOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFleetStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	// Seed across SEPARATE transactions so each INSERT gets a distinct
	// transaction timestamp from now(). Inside one tx every row shares
	// the same transaction-start time, leaving the tie-breaking order
	// of created_at DESC undefined.
	for _, spec := range []struct {
		org  uuid.UUID
		name string
	}{
		{orgA, "first-A"},
		{orgB, "first-B"},
		{orgA, "second-A"},
	} {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			_, err := s.CreateAsset(ctx, tx, CreateAssetParams{
				OrgID:     spec.org,
				Name:      spec.name,
				AssetType: "excavator",
			})
			return err
		})
		if err != nil {
			t.Fatalf("seed %s: %v", spec.name, err)
		}
		// Tiny delay so created_at advances visibly between rows.
		time.Sleep(2 * time.Millisecond)
	}

	t.Run("returns only org's assets, newest first", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListAssets(ctx, tx, ListAssetsParams{OrgID: orgA})
			if err != nil {
				return err
			}
			if len(rows) != 2 {
				t.Fatalf("got %d rows, want 2", len(rows))
			}
			if rows[0].Name != "second-A" || rows[1].Name != "first-A" {
				t.Errorf("ordering mismatch: %s, %s", rows[0].Name, rows[1].Name)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("status filter narrows", func(t *testing.T) {
		// Mark one asset 'maintenance' to filter by it.
		_, err := pool.Exec(ctx, `UPDATE fleet_assets SET status = 'maintenance' WHERE org_id = $1 AND name = 'first-A'`, orgA)
		if err != nil {
			t.Fatalf("set status: %v", err)
		}
		err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListAssets(ctx, tx, ListAssetsParams{OrgID: orgA, StatusFilter: []string{"maintenance"}})
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Errorf("got %d rows, want 1", len(rows))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestFleetStore_AllocateAsset_GiSTConflict(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFleetStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	var assetID uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a, err := s.CreateAsset(ctx, tx, CreateAssetParams{
			OrgID:     orgID,
			Name:      "loader-1",
			AssetType: "loader",
		})
		if err != nil {
			return err
		}
		assetID = a.ID
		return nil
	})
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	may1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	may10 := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	may15 := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	may20 := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	t.Run("first allocation succeeds", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			_, err := s.AllocateAsset(ctx, tx, AllocateAssetParams{
				AssetID: assetID, ProjectID: projectID, StartDate: may1, EndDate: may10,
			})
			return err
		})
		if err != nil {
			t.Errorf("first allocation: %v", err)
		}
	})

	t.Run("overlapping range returns ErrAllocationConflict", func(t *testing.T) {
		// Manual Begin/Rollback because once the INSERT trips the
		// exclusion violation the transaction is in error state — any
		// subsequent statement (including COMMIT) errors. We must roll
		// back explicitly rather than relying on BeginTxFunc.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = s.AllocateAsset(ctx, tx, AllocateAssetParams{
			AssetID: assetID, ProjectID: projectID, StartDate: may1, EndDate: may15,
		})
		if !errors.Is(err, ErrAllocationConflict) {
			t.Errorf("err = %v, want ErrAllocationConflict", err)
		}
	})

	t.Run("non-overlapping range succeeds", func(t *testing.T) {
		// [may15, may20) doesn't intersect [may1, may10).
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			_, err := s.AllocateAsset(ctx, tx, AllocateAssetParams{
				AssetID: assetID, ProjectID: projectID, StartDate: may15, EndDate: may20,
			})
			return err
		})
		if err != nil {
			t.Errorf("second allocation: %v", err)
		}
	})
}

func TestFleetStore_VerifyAssetInOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFleetStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	var assetA uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a, err := s.CreateAsset(ctx, tx, CreateAssetParams{OrgID: orgA, Name: "x", AssetType: "y"})
		if err != nil {
			return err
		}
		assetA = a.ID
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("belongs returns nil", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			return s.VerifyAssetInOrg(ctx, tx, assetA, orgA)
		})
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("cross-org returns ErrFleetAssetNotFound", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			err := s.VerifyAssetInOrg(ctx, tx, assetA, orgB)
			if !errors.Is(err, ErrFleetAssetNotFound) {
				t.Errorf("err = %v, want ErrFleetAssetNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
