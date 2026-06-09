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

func TestAgentConfigStore_UpsertGetRoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAgentConfigStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Config Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, err := s.Upsert(ctx, tx, UpsertAgentConfigParams{
			OrgID:      orgID,
			Capability: "foresight",
			Enabled:    false,
			Config:     []byte(`{"budget_burn_percent":50}`),
			UpdatedBy:  "admin-sub",
		})
		if err != nil {
			return err
		}
		if row.Enabled || row.Capability != "foresight" || row.UpdatedBy != "admin-sub" {
			t.Errorf("upserted row = %+v", row)
		}
		if string(row.Config) != `{"budget_burn_percent": 50}` && string(row.Config) != `{"budget_burn_percent":50}` {
			t.Errorf("config round-trip = %s", row.Config)
		}

		got, err := s.GetByCapability(ctx, tx, orgID, "foresight")
		if err != nil {
			return err
		}
		if got.ID != row.ID || got.Enabled {
			t.Errorf("get = %+v, want the upserted row (disabled)", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestAgentConfigStore_UpsertOverwritesNotDuplicates(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAgentConfigStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Config Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		first, err := s.Upsert(ctx, tx, UpsertAgentConfigParams{OrgID: orgID, Capability: "experience", Enabled: true, UpdatedBy: "a"})
		if err != nil {
			return err
		}
		second, err := s.Upsert(ctx, tx, UpsertAgentConfigParams{OrgID: orgID, Capability: "experience", Enabled: false, UpdatedBy: "b"})
		if err != nil {
			return err
		}
		// ON CONFLICT must UPDATE in place (same row id), not insert a duplicate.
		if second.ID != first.ID {
			t.Errorf("upsert conflict should keep the same row id: first=%s second=%s", first.ID, second.ID)
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

func TestAgentConfigStore_GetMiss_ErrNotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAgentConfigStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Config Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		_, err := s.GetByCapability(ctx, tx, orgID, "foresight")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("get of an absent row = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestAgentConfigStore_DeleteReturnsRowsAffected(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAgentConfigStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Config Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := s.Upsert(ctx, tx, UpsertAgentConfigParams{OrgID: orgID, Capability: "delay_cascade", Enabled: false, UpdatedBy: "a"}); err != nil {
			return err
		}
		n, err := s.DeleteByCapability(ctx, tx, orgID, "delay_cascade")
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("delete affected = %d, want 1", n)
		}
		// Idempotent: a second delete affects 0 rows.
		n2, err := s.DeleteByCapability(ctx, tx, orgID, "delay_cascade")
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

func TestAgentConfigStore_OrgIsolation(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAgentConfigStore()
	ctx := context.Background()

	orgA, orgB := uuid.New(), uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := s.Upsert(ctx, tx, UpsertAgentConfigParams{OrgID: orgA, Capability: "foresight", Enabled: false, UpdatedBy: "a"}); err != nil {
			return err
		}
		// Org B must not see org A's row.
		_, err := s.GetByCapability(ctx, tx, orgB, "foresight")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("org B get of org A's row = %v, want ErrNotFound", err)
		}
		bRows, err := s.ListByOrg(ctx, tx, orgB)
		if err != nil {
			return err
		}
		if len(bRows) != 0 {
			t.Errorf("org B ListByOrg len = %d, want 0", len(bRows))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
