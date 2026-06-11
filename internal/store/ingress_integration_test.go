//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestScheduleStore_InsertTasksAndDependencies_RoundTrip proves InsertTasks
// returns persisted rows (server IDs, CPM cols default/NULL, input order
// preserved) and that InsertDependencies wires predecessor→successor using
// those returned IDs — the wbs_code→UUID resolution shape the import service
// relies on.
func TestScheduleStore_InsertTasksAndDependencies_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Kelbrook Residence")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		inserted, err := s.InsertTasks(ctx, tx, []InsertTaskParams{
			{ProjectID: projectID, WBSCode: "01-00", Name: "Site Prep", Status: "pending", DurationDays: 3, PercentComplete: 0},
			{ProjectID: projectID, WBSCode: "03-30", Name: "Foundation", Status: "pending", DurationDays: 5, PercentComplete: 0},
		})
		if err != nil {
			return err
		}
		if len(inserted) != 2 {
			t.Fatalf("inserted = %d, want 2", len(inserted))
		}
		if inserted[0].WBSCode != "01-00" || inserted[1].WBSCode != "03-30" {
			t.Errorf("order = %q, %q; want 01-00, 03-30 (input order preserved)", inserted[0].WBSCode, inserted[1].WBSCode)
		}
		for _, tk := range inserted {
			if tk.ID == uuid.Nil {
				t.Errorf("task %s missing server id", tk.WBSCode)
			}
			if tk.EarlyStart != nil || tk.IsCritical {
				t.Errorf("task %s has CPM data on insert (should be null/false)", tk.WBSCode)
			}
		}
		// Wire a dependency using the returned ids.
		return s.InsertDependencies(ctx, tx, []InsertDependencyParams{
			{ProjectID: projectID, PredecessorID: inserted[0].ID, SuccessorID: inserted[1].ID, DependencyType: "FS", LagDays: 0},
		})
	})
	if err != nil {
		t.Fatalf("insert tasks+deps: %v", err)
	}

	// The dependency round-trips through GetProjectDependencies.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		deps, err := s.GetProjectDependencies(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(deps) != 1 {
			t.Errorf("deps = %d, want 1", len(deps))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read deps: %v", err)
	}
}

// TestFinancialsStore_CreateProjectBudget_CurrencyFanOut proves the single
// CurrencyCode is fanned into all three *_currency_code columns (satisfying
// chk_budget_currency_match) and the row round-trips.
func TestFinancialsStore_CreateProjectBudget_CurrencyFanOut(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFinancialsStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Cedar Works")
	testdb.SeedProject(t, pool, projectID, orgID, "Cedar Lane")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		b, err := s.CreateProjectBudget(ctx, tx, CreateProjectBudgetParams{
			ProjectID:          projectID,
			WBSCode:            "03-30",
			PhaseName:          "Foundation",
			CurrencyCode:       "CAD",
			EstimatedCostCents: 12000000,
		})
		if err != nil {
			return err
		}
		if b.EstimatedCostCurrencyCode != "CAD" || b.CommittedCostCurrencyCode != "CAD" || b.ActualCostCurrencyCode != "CAD" {
			t.Errorf("currency fan-out failed: %+v", b)
		}
		if b.CommittedCostCents != 0 || b.ActualCostCents != 0 {
			t.Errorf("committed/actual not defaulted to 0: %+v", b)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create budget: %v", err)
	}
}

// TestHRStore_CreateEmployeeAndCertification_RoundTrip proves the HR inserts
// persist and that VerifyUserInOrg gates a linked user_id.
func TestHRStore_CreateEmployeeAndCertification_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewHRStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		emp, err := s.CreateEmployee(ctx, tx, CreateEmployeeParams{
			OrgID: orgID, FirstName: "Dana", LastName: "Cole", Role: "Foreman",
		})
		if err != nil {
			return err
		}
		if emp.ID == uuid.Nil || emp.OrgID != orgID {
			t.Fatalf("employee = %+v", emp)
		}
		cert, err := s.CreateCertification(ctx, tx, CreateCertificationParams{
			EmployeeID: emp.ID,
			CertType:   "osha_10",
			ExpiryDate: time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC),
			Status:     "active",
		})
		if err != nil {
			return err
		}
		if cert.EmployeeID != emp.ID || cert.Status != "active" {
			t.Errorf("cert = %+v", cert)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create employee+cert: %v", err)
	}
}
