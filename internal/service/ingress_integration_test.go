//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestBudgetService_CreateProjectBudgets_BatchPersistsAndLists proves the
// budget baseline batch lands, one audit row per line, the single currency
// fans into all three columns, and the rows round-trip through
// ListProjectBudgets / GetProjectBudgets in WBS order.
func TestBudgetService_CreateProjectBudgets_BatchPersistsAndLists(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()

	created, err := svc.CreateProjectBudgets(ctx, fx.orgID, "owner-sub", fx.projectID, []CreateProjectBudgetLine{
		{WBSCode: "03-30", PhaseName: "Foundation", CurrencyCode: "USD", EstimatedCostCents: 12000000},
		{WBSCode: "01-00", PhaseName: "Site Prep", CurrencyCode: "USD", EstimatedCostCents: 4500000, CommittedCostCents: 1000000},
	})
	if err != nil {
		t.Fatalf("CreateProjectBudgets: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %d, want 2", len(created))
	}
	for _, b := range created {
		// Single currency fanned into all three columns (chk_budget_currency_match).
		if b.EstimatedCostCurrencyCode != "USD" || b.CommittedCostCurrencyCode != "USD" || b.ActualCostCurrencyCode != "USD" {
			t.Errorf("budget %s currency columns not all USD: %+v", b.WBSCode, b)
		}
	}
	if got := auditCount(t, svc, fx.orgID, "budget.created"); got != 2 {
		t.Errorf("budget.created audit rows = %d, want 2 (one per line)", got)
	}

	// Round-trip read (WBS order).
	got, err := svc.GetProjectBudgets(ctx, fx.projectID, fx.orgID)
	if err != nil {
		t.Fatalf("GetProjectBudgets: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("listed = %d, want 2", len(got))
	}
	if got[0].WBSCode != "01-00" || got[1].WBSCode != "03-30" {
		t.Errorf("budget order = %q, %q; want 01-00, 03-30", got[0].WBSCode, got[1].WBSCode)
	}
}

// TestBudgetService_CreateProjectBudgets_MixedCurrencyAcrossLines proves mixed
// currency ACROSS lines is allowed (rollup groups by currency_code); each line
// is independently valid.
func TestBudgetService_CreateProjectBudgets_MixedCurrencyAcrossLines(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()

	created, err := svc.CreateProjectBudgets(ctx, fx.orgID, "owner-sub", fx.projectID, []CreateProjectBudgetLine{
		{WBSCode: "01-00", PhaseName: "Site Prep", CurrencyCode: "USD", EstimatedCostCents: 100},
		{WBSCode: "03-30", PhaseName: "Foundation", CurrencyCode: "CAD", EstimatedCostCents: 200},
	})
	if err != nil {
		t.Fatalf("mixed-currency batch: %v", err)
	}
	if created[0].EstimatedCostCurrencyCode != "USD" || created[1].EstimatedCostCurrencyCode != "CAD" {
		t.Errorf("currencies = %s, %s; want USD, CAD", created[0].EstimatedCostCurrencyCode, created[1].EstimatedCostCurrencyCode)
	}
}

// TestBudgetService_CreateProjectBudgets_CrossTenantHidden proves a foreign org
// gets ErrNotFound and nothing lands.
func TestBudgetService_CreateProjectBudgets_CrossTenantHidden(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()
	foreign := uuid.New()

	_, err := svc.CreateProjectBudgets(ctx, foreign, "intruder", fx.projectID, []CreateProjectBudgetLine{
		{WBSCode: "01-00", PhaseName: "p", CurrencyCode: "USD", EstimatedCostCents: 1},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	got, _ := svc.GetProjectBudgets(ctx, fx.projectID, fx.orgID)
	if len(got) != 0 {
		t.Errorf("budgets after rejected cross-tenant create = %d, want 0", len(got))
	}
}

// newHRWriteService wires an HRService with a REAL audit recorder + seeded org.
func newHRWriteService(t *testing.T) (*HRService, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Builders")
	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	return NewHRService(pool, store.NewHRStore(), audit), orgID
}

// TestHRService_CreateEmployeeAndCertification_RoundTrip proves the employee +
// cert create paths land, audit, and round-trip through the List reads. The
// cert is scoped indirectly (no org_id) via VerifyEmployeeInOrg.
func TestHRService_CreateEmployeeAndCertification_RoundTrip(t *testing.T) {
	svc, orgID := newHRWriteService(t)
	ctx := context.Background()

	hire := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	emp, err := svc.CreateEmployee(ctx, CreateEmployeeInput{
		OrgID:         orgID,
		CallerUserSub: "owner-sub",
		FirstName:     "Dana",
		LastName:      "Cole",
		Role:          "Foreman",
		HireDate:      &hire,
	})
	if err != nil {
		t.Fatalf("CreateEmployee: %v", err)
	}
	if emp.ID == uuid.Nil || emp.OrgID != orgID {
		t.Fatalf("employee = %+v, want a persisted org-scoped row", emp)
	}

	expiry := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)
	cert, err := svc.CreateCertification(ctx, CreateCertificationInput{
		OrgID:         orgID,
		CallerUserSub: "owner-sub",
		EmployeeID:    emp.ID,
		CertType:      "osha_10",
		ExpiryDate:    expiry,
	})
	if err != nil {
		t.Fatalf("CreateCertification: %v", err)
	}
	if cert.Status != "active" {
		t.Errorf("cert status = %q, want active (default)", cert.Status)
	}

	// Round-trips.
	emps, err := svc.ListEmployees(ctx, orgID)
	if err != nil || len(emps) != 1 {
		t.Fatalf("ListEmployees: got %d (err=%v), want 1", len(emps), err)
	}
	certs, err := svc.ListCertifications(ctx, orgID, emp.ID)
	if err != nil || len(certs) != 1 {
		t.Fatalf("ListCertifications: got %d (err=%v), want 1", len(certs), err)
	}
}

// TestHRService_CreateCertification_CrossOrgEmployeeHidden proves a cert create
// against another org's employee returns ErrEmployeeNotFound (never leaks
// existence) and writes nothing.
func TestHRService_CreateCertification_CrossOrgEmployeeHidden(t *testing.T) {
	svc, orgA := newHRWriteService(t)
	ctx := context.Background()

	// Seed an employee in a DIFFERENT org via the same pool.
	orgB := uuid.New()
	testdb.SeedOrg(t, svc.pool, orgB, "Other Builders")
	empB, err := svc.CreateEmployee(ctx, CreateEmployeeInput{
		OrgID: orgB, FirstName: "X", LastName: "Y", Role: "Laborer",
	})
	if err != nil {
		t.Fatalf("seed orgB employee: %v", err)
	}

	_, err = svc.CreateCertification(ctx, CreateCertificationInput{
		OrgID:      orgA, // caller is orgA, employee belongs to orgB
		EmployeeID: empB.ID,
		CertType:   "osha_10",
		ExpiryDate: time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, ErrEmployeeNotFound) {
		t.Fatalf("err = %v, want ErrEmployeeNotFound (cross-org)", err)
	}
	// orgB's employee has no cert.
	certs, _ := svc.ListCertifications(ctx, orgB, empB.ID)
	if len(certs) != 0 {
		t.Errorf("certs on orgB employee = %d, want 0", len(certs))
	}
}
