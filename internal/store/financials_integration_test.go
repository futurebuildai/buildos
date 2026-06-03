//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedBudgetPhase inserts a single project_budgets row. All three monetary
// pairs share `currency` to satisfy the chk_budget_currency_match CHECK.
func seedBudgetPhase(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, currency string, est, com, act int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO project_budgets (
			project_id, wbs_code, phase_name,
			estimated_cost_cents, estimated_cost_currency_code,
			committed_cost_cents, committed_cost_currency_code,
			actual_cost_cents, actual_cost_currency_code
		) VALUES ($1, $2, $3, $4, $5, $6, $5, $7, $5)`,
		projectID, wbs, "phase "+wbs, est, currency, com, act)
	if err != nil {
		t.Fatalf("seed project_budget: %v", err)
	}
}

// seedARSnapshot inserts a single ar_aging_snapshots row.
func seedARSnapshot(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID, date time.Time, currency string, total int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO ar_aging_snapshots (
			org_id, snapshot_date, currency_code,
			current_cents, days_30_cents, days_60_cents, days_90_plus_cents,
			total_receivable_cents
		) VALUES ($1, $2, $3, $4, 0, 0, 0, $4)`,
		orgID, date, currency, total)
	if err != nil {
		t.Fatalf("seed ar_aging_snapshot: %v", err)
	}
}

// setProjectStatus flips a seeded project's status (SeedProject is always
// 'active'); the rollup must exclude 'archived' projects.
func setProjectStatus(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE projects SET status = $2 WHERE id = $1`, projectID, status); err != nil {
		t.Fatalf("set project status: %v", err)
	}
}

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

func TestVerifyProjectInOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projectInA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedProject(t, pool, projectInA, orgA, "Project in A")

	// Same org → ok.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return VerifyProjectInOrg(ctx, tx, projectInA, orgA)
	})
	if err != nil {
		t.Errorf("same-org verify failed: %v", err)
	}

	// Cross-org → ErrNotFound.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return VerifyProjectInOrg(ctx, tx, projectInA, orgB)
	})
	if err == nil {
		t.Error("cross-org verify should error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org verify returned %v, want ErrNotFound", err)
	}
}

// TestFinancialsStore_ListProjectFinancials exercises the per-project
// rollup: phases sum within (project, currency), projects with zero budget
// rows are omitted, and the currency filter narrows to a single bucket.
func TestFinancialsStore_ListProjectFinancials(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	orgID := uuid.New()
	usdProject := uuid.New()
	cadProject := uuid.New()
	emptyProject := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Prairie Builders")
	testdb.SeedProject(t, pool, usdProject, orgID, "Aspen Ridge")
	testdb.SeedProject(t, pool, cadProject, orgID, "Maple Court")
	testdb.SeedProject(t, pool, emptyProject, orgID, "No Budget Yet")

	// Aspen Ridge: two USD phases that must sum.
	seedBudgetPhase(t, pool, usdProject, "01-100", "USD", 100_000, 50_000, 25_000)
	seedBudgetPhase(t, pool, usdProject, "02-200", "USD", 200_000, 80_000, 40_000)
	// Maple Court: one CAD phase.
	seedBudgetPhase(t, pool, cadProject, "01-100", "CAD", 300_000, 90_000, 10_000)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		all, err := s.ListProjectFinancials(ctx, tx, orgID, "")
		if err != nil {
			return err
		}
		// emptyProject has no budget rows → omitted by the INNER JOIN.
		if len(all) != 2 {
			t.Fatalf("ListProjectFinancials(all) = %d rows, want 2 (empty project omitted)", len(all))
		}
		var usd *models.ProjectFinancial
		for i := range all {
			if all[i].ProjectID == usdProject {
				usd = &all[i]
			}
		}
		if usd == nil {
			t.Fatalf("USD project missing from financials")
		}
		if usd.PhaseCount != 2 {
			t.Errorf("USD PhaseCount = %d, want 2", usd.PhaseCount)
		}
		if usd.TotalEstimatedCents != 300_000 {
			t.Errorf("USD TotalEstimatedCents = %d, want 300000", usd.TotalEstimatedCents)
		}
		if usd.TotalCommittedCents != 130_000 {
			t.Errorf("USD TotalCommittedCents = %d, want 130000", usd.TotalCommittedCents)
		}

		usdOnly, err := s.ListProjectFinancials(ctx, tx, orgID, "USD")
		if err != nil {
			return err
		}
		if len(usdOnly) != 1 || usdOnly[0].CurrencyCode != "USD" {
			t.Errorf("ListProjectFinancials(USD) = %+v, want 1 USD row", usdOnly)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

// TestFinancialsStore_CorporateRollupAndList drives the write path
// (UpsertCorporateRollupForQuarter) and the read path (ListCorporateBudgets)
// together: active+completed projects contribute per-currency buckets,
// archived projects are excluded, and a re-run upserts in place.
func TestFinancialsStore_CorporateRollupAndList(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	orgID := uuid.New()
	activeProj := uuid.New()
	completedProj := uuid.New()
	archivedProj := uuid.New()
	cadProj := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Summit Homes")
	testdb.SeedProject(t, pool, activeProj, orgID, "Active Build")
	testdb.SeedProject(t, pool, completedProj, orgID, "Done Build")
	testdb.SeedProject(t, pool, archivedProj, orgID, "Old Build")
	testdb.SeedProject(t, pool, cadProj, orgID, "North Build")
	setProjectStatus(t, pool, completedProj, "completed")
	setProjectStatus(t, pool, archivedProj, "archived")

	seedBudgetPhase(t, pool, activeProj, "01-100", "USD", 100_000, 0, 0)
	seedBudgetPhase(t, pool, completedProj, "01-100", "USD", 50_000, 0, 0)
	seedBudgetPhase(t, pool, archivedProj, "01-100", "USD", 999_000, 0, 0) // must NOT count
	seedBudgetPhase(t, pool, cadProj, "01-100", "CAD", 70_000, 0, 0)

	const fy, q = 2026, 2

	var affected int64
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		affected, qErr = s.UpsertCorporateRollupForQuarter(ctx, tx, fy, q)
		return qErr
	})
	if err != nil {
		t.Fatalf("rollup: %v", err)
	}
	if affected != 2 {
		t.Fatalf("rollup affected = %d, want 2 (USD + CAD buckets)", affected)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		all, err := s.ListCorporateBudgets(ctx, tx, orgID, "")
		if err != nil {
			return err
		}
		if len(all) != 2 {
			t.Fatalf("ListCorporateBudgets(all) = %d, want 2", len(all))
		}
		usd, err := s.ListCorporateBudgets(ctx, tx, orgID, "USD")
		if err != nil {
			return err
		}
		if len(usd) != 1 {
			t.Fatalf("ListCorporateBudgets(USD) = %d, want 1", len(usd))
		}
		// 100k active + 50k completed; archived's 999k excluded.
		if usd[0].TotalEstimatedCents != 150_000 {
			t.Errorf("USD bucket total = %d, want 150000 (archived excluded)", usd[0].TotalEstimatedCents)
		}
		if usd[0].ProjectCount != 2 {
			t.Errorf("USD bucket project_count = %d, want 2", usd[0].ProjectCount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}

	// Re-run upserts in place — still 2 rows, not 4.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, qErr := s.UpsertCorporateRollupForQuarter(ctx, tx, fy, q)
		return qErr
	})
	if err != nil {
		t.Fatalf("rollup re-run: %v", err)
	}
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		all, err := s.ListCorporateBudgets(ctx, tx, orgID, "")
		if err != nil {
			return err
		}
		if len(all) != 2 {
			t.Errorf("after re-run ListCorporateBudgets = %d, want 2 (upsert in place)", len(all))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

// TestFinancialsStore_ListLatestARAgingSnapshots proves the DISTINCT ON
// returns the newest snapshot per currency, and the currency filter
// narrows to one bucket.
func TestFinancialsStore_ListLatestARAgingSnapshots(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Lakeside Builders")

	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	seedARSnapshot(t, pool, orgID, older, "USD", 10_000)
	seedARSnapshot(t, pool, orgID, newer, "USD", 99_000) // latest USD
	seedARSnapshot(t, pool, orgID, older, "CAD", 5_000)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		all, err := s.ListLatestARAgingSnapshots(ctx, tx, orgID, "")
		if err != nil {
			return err
		}
		if len(all) != 2 {
			t.Fatalf("ListLatest(all) = %d, want 2 (one per currency)", len(all))
		}
		for _, snap := range all {
			if snap.CurrencyCode == "USD" && snap.TotalReceivableCents != 99_000 {
				t.Errorf("latest USD total = %d, want 99000 (newest snapshot)", snap.TotalReceivableCents)
			}
		}
		cad, err := s.ListLatestARAgingSnapshots(ctx, tx, orgID, "CAD")
		if err != nil {
			return err
		}
		if len(cad) != 1 || cad[0].CurrencyCode != "CAD" {
			t.Errorf("ListLatest(CAD) = %+v, want 1 CAD row", cad)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

// TestFinancialsStore_UpdateInvoice covers the status/paid_date COALESCE
// update plus the ErrNotFound guard on a missing id.
func TestFinancialsStore_UpdateInvoice(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Riverbend Co")
	testdb.SeedProject(t, pool, projectID, orgID, "Willow Walk")

	var inv models.Invoice
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		inv, qErr = s.CreateInvoice(ctx, tx, CreateInvoiceParams{
			ProjectID:    projectID,
			OrgID:        orgID,
			VendorName:   "Cascade Concrete",
			AmountCents:  250_000,
			CurrencyCode: "USD",
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}

	paid := models.InvoiceStatusPaid
	paidDate := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		updated, qErr := s.UpdateInvoice(ctx, tx, UpdateInvoiceParams{
			ID:       inv.ID,
			Status:   &paid,
			PaidDate: &paidDate,
		})
		if qErr != nil {
			return qErr
		}
		if updated.Status != models.InvoiceStatusPaid {
			t.Errorf("updated status = %q, want paid", updated.Status)
		}
		if updated.PaidDate == nil || !updated.PaidDate.Equal(paidDate) {
			t.Errorf("updated paid_date = %v, want %v", updated.PaidDate, paidDate)
		}
		// AmountCents preserved (not part of the patch).
		if updated.AmountCents != 250_000 {
			t.Errorf("AmountCents drifted to %d, want 250000", updated.AmountCents)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update invoice: %v", err)
	}

	// Missing id → ErrNotFound.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, qErr := s.UpdateInvoice(ctx, tx, UpdateInvoiceParams{ID: uuid.New(), Status: &paid})
		return qErr
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateInvoice(missing) = %v, want ErrNotFound", err)
	}
}
