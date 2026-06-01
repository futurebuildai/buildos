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

// Integration coverage for ProjectStore CRUD against a real Postgres:
// create round-trip (with DB defaults + nullable address), get with the
// cross-org guard, list status-filter + pagination ordering, and a
// COALESCE partial update.

// strPtr lives in setup_integration_test.go (same package + build tag);
// ptrInt is local to the project tests.
func ptrInt(i int) *int { return &i }

func TestProjectStore_Create_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProjectStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Test Org")

	permit := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	var created uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		p, qErr := s.Create(ctx, tx, CreateProjectParams{
			OrgID:            orgID,
			Name:             "Maple St",
			Address:          strPtr("1 Maple"),
			PermitIssuedDate: &permit,
			ProjectStartDate: &start,
			GSF:              ptrInt(3000),
		})
		if qErr != nil {
			return qErr
		}
		created = p.ID
		// DB applies the 'active' status default.
		if p.Status != "active" {
			t.Errorf("status default = %q, want active", p.Status)
		}
		if p.Address != "1 Maple" {
			t.Errorf("address round-trip = %q, want 1 Maple", p.Address)
		}
		if p.GSF == nil || *p.GSF != 3000 {
			t.Errorf("gsf round-trip = %v, want 3000", p.GSF)
		}
		if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
			t.Errorf("timestamps not populated: created=%v updated=%v", p.CreatedAt, p.UpdatedAt)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	// GetByID should fetch it back scoped to the org.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		got, qErr := s.GetByID(ctx, tx, created, orgID)
		if qErr != nil {
			return qErr
		}
		if got.Name != "Maple St" {
			t.Errorf("GetByID name = %q, want Maple St", got.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("get tx: %v", err)
	}
}

func TestProjectStore_Create_NullAddress(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProjectStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Test Org")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		p, qErr := s.Create(ctx, tx, CreateProjectParams{
			OrgID:   orgID,
			Name:    "No Address",
			Address: nil,
		})
		if qErr != nil {
			return qErr
		}
		// A SQL NULL address must scan as "" not error.
		if p.Address != "" {
			t.Errorf("nil address should scan as empty string, got %q", p.Address)
		}
		if p.GSF != nil {
			t.Errorf("nil gsf should remain nil, got %v", p.GSF)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}
}

func TestProjectStore_GetByID_CrossOrgIsNotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProjectStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedProject(t, pool, projID, orgA, "A's project")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		// Org B asking for org A's project must look identical to "missing".
		_, qErr := s.GetByID(ctx, tx, projID, orgB)
		if !errors.Is(qErr, ErrNotFound) {
			t.Errorf("cross-org get err = %v, want ErrNotFound", qErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

func TestProjectStore_ListByOrg_StatusFilterAndIsolation(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProjectStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	// Org A: two active, one archived. Org B: one active (isolation).
	testdb.SeedProject(t, pool, uuid.New(), orgA, "A active 1")
	testdb.SeedProject(t, pool, uuid.New(), orgA, "A active 2")
	archivedID := uuid.New()
	testdb.SeedProject(t, pool, archivedID, orgA, "A archived")
	testdb.SeedProject(t, pool, uuid.New(), orgB, "B active")

	// Flip one of A's to archived via the store Update path.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, qErr := s.Update(ctx, tx, UpdateProjectParams{
			ID:     archivedID,
			OrgID:  orgA,
			Status: strPtr("archived"),
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("update tx: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		// No filter: A sees its 3, not B's.
		all, qErr := s.ListByOrg(ctx, tx, orgA, "", 50, 0)
		if qErr != nil {
			return qErr
		}
		if len(all) != 3 {
			t.Errorf("ListByOrg(A) = %d, want 3 (isolation leak?)", len(all))
		}
		// Status filter: only the 2 active.
		active, qErr := s.ListByOrg(ctx, tx, orgA, "active", 50, 0)
		if qErr != nil {
			return qErr
		}
		if len(active) != 2 {
			t.Errorf("ListByOrg(A, active) = %d, want 2", len(active))
		}
		// Archived filter: only the 1.
		archived, qErr := s.ListByOrg(ctx, tx, orgA, "archived", 50, 0)
		if qErr != nil {
			return qErr
		}
		if len(archived) != 1 {
			t.Errorf("ListByOrg(A, archived) = %d, want 1", len(archived))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

func TestProjectStore_ListByOrg_Pagination(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProjectStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Test Org")
	for i := 0; i < 5; i++ {
		testdb.SeedProject(t, pool, uuid.New(), orgID, "P")
	}

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		page1, qErr := s.ListByOrg(ctx, tx, orgID, "", 2, 0)
		if qErr != nil {
			return qErr
		}
		if len(page1) != 2 {
			t.Errorf("page1 = %d, want 2", len(page1))
		}
		page3, qErr := s.ListByOrg(ctx, tx, orgID, "", 2, 4)
		if qErr != nil {
			return qErr
		}
		if len(page3) != 1 {
			t.Errorf("page3 = %d, want 1 (5 total, offset 4)", len(page3))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

func TestProjectStore_Update_PartialPatch(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProjectStore()
	ctx := context.Background()

	orgID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Test Org")
	testdb.SeedProject(t, pool, projID, orgID, "Original")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Patch only the name; status/address/gsf left nil → unchanged.
		p, qErr := s.Update(ctx, tx, UpdateProjectParams{
			ID:    projID,
			OrgID: orgID,
			Name:  strPtr("Renamed"),
		})
		if qErr != nil {
			return qErr
		}
		if p.Name != "Renamed" {
			t.Errorf("name = %q, want Renamed", p.Name)
		}
		if p.Status != "active" {
			t.Errorf("status should be unchanged 'active', got %q", p.Status)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update tx: %v", err)
	}
}

func TestProjectStore_Update_CrossOrgIsNotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProjectStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedProject(t, pool, projID, orgA, "A's project")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, qErr := s.Update(ctx, tx, UpdateProjectParams{
			ID:    projID,
			OrgID: orgB,
			Name:  strPtr("hijack"),
		})
		if !errors.Is(qErr, ErrNotFound) {
			t.Errorf("cross-org update err = %v, want ErrNotFound", qErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update tx: %v", err)
	}
}
