//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// First real integration test for the FinancialsStore. Exercises three
// behaviors against a real Postgres: the empty-list case, an invoice
// round-trip, and the cross-tenant guard.

func TestFinancialsStore_ListProjectBudgets_Empty(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		got, err := s.ListProjectBudgets(ctx, tx, uuid.New())
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Errorf("empty DB should return zero budgets; got %d", len(got))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

func TestFinancialsStore_CreateInvoice_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Test Org")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	var inv models.Invoice
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		inv, qErr = s.CreateInvoice(ctx, tx, CreateInvoiceParams{
			ProjectID:    projectID,
			OrgID:        orgID,
			VendorName:   "Acme Lumber",
			AmountCents:  150_000,
			CurrencyCode: "USD",
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if inv.AmountCents != 150_000 {
		t.Errorf("AmountCents round-trip = %d, want 150000", inv.AmountCents)
	}
	if inv.CurrencyCode != "USD" {
		t.Errorf("CurrencyCode round-trip = %q, want USD", inv.CurrencyCode)
	}
	if inv.Status != "pending" {
		t.Errorf("Status default = %q, want pending", inv.Status)
	}

	// Verify it shows up in the list.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		invoices, err := s.ListInvoices(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(invoices) != 1 {
			t.Errorf("ListInvoices returned %d, want 1", len(invoices))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("list tx: %v", err)
	}
}

func TestFinancialsStore_VerifyProjectInOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projectInA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedProject(t, pool, projectInA, orgA, "Project in A")

	// Same org → ok.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return s.VerifyProjectInOrg(ctx, tx, projectInA, orgA)
	})
	if err != nil {
		t.Errorf("same-org verify failed: %v", err)
	}

	// Cross-org → ErrNotFound.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return s.VerifyProjectInOrg(ctx, tx, projectInA, orgB)
	})
	if err == nil {
		t.Error("cross-org verify should error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org verify returned %v, want ErrNotFound", err)
	}
}
