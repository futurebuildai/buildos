//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

func TestConnectorConfigStore_UpsertGetRoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewConnectorConfigStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Conn Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, err := s.Upsert(ctx, tx, UpsertConnectorConfigParams{
			OrgID: orgID, ConnectorName: "reference", Enabled: true, UpdatedBy: "admin",
		})
		if err != nil {
			return err
		}
		if !row.Enabled || row.ConnectorName != "reference" {
			t.Errorf("upserted row = %+v", row)
		}
		got, err := s.GetByName(ctx, tx, orgID, "reference")
		if err != nil {
			return err
		}
		if got.ID != row.ID || !got.Enabled {
			t.Errorf("get = %+v, want the upserted enabled row", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestConnectorConfigStore_UpsertOverwrites(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewConnectorConfigStore()
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Conn Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		first, err := s.Upsert(ctx, tx, UpsertConnectorConfigParams{OrgID: orgID, ConnectorName: "reference", Enabled: true, UpdatedBy: "a"})
		if err != nil {
			return err
		}
		second, err := s.Upsert(ctx, tx, UpsertConnectorConfigParams{OrgID: orgID, ConnectorName: "reference", Enabled: false, UpdatedBy: "b"})
		if err != nil {
			return err
		}
		if second.ID != first.ID {
			t.Errorf("upsert conflict should keep the same row id")
		}
		if second.Enabled || second.UpdatedBy != "b" {
			t.Errorf("second upsert did not overwrite: %+v", second)
		}
		all, err := s.ListByOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if len(all) != 1 {
			t.Errorf("ListByOrg len = %d, want 1 (no duplicate)", len(all))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestConnectorConfigStore_GetMiss_ErrNotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewConnectorConfigStore()
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Conn Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := s.GetByName(ctx, tx, orgID, "reference"); !errors.Is(err, ErrNotFound) {
			t.Errorf("get of an absent row = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestConnectorConfigStore_DeleteAndOrgIsolation(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewConnectorConfigStore()
	ctx := context.Background()
	orgA, orgB := uuid.New(), uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := s.Upsert(ctx, tx, UpsertConnectorConfigParams{OrgID: orgA, ConnectorName: "reference", Enabled: true, UpdatedBy: "a"}); err != nil {
			return err
		}
		// Org B cannot see org A's row.
		if _, err := s.GetByName(ctx, tx, orgB, "reference"); !errors.Is(err, ErrNotFound) {
			t.Errorf("org B get of org A's row = %v, want ErrNotFound", err)
		}
		// Delete returns rows-affected; idempotent.
		n, err := s.DeleteByName(ctx, tx, orgA, "reference")
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("delete affected = %d, want 1", n)
		}
		n2, err := s.DeleteByName(ctx, tx, orgA, "reference")
		if err != nil {
			return err
		}
		if n2 != 0 {
			t.Errorf("second delete affected = %d, want 0", n2)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
