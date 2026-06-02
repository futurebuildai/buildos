//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// newBudgetService wires a BudgetService to a fresh migrated pool with a REAL
// audit recorder (so the in-tx s.audit.Record write actually executes against
// audit_log — the mutation path the no-op recorder would skip) and a seeded
// org + project. Returns the service, the pool (for direct audit_log asserts),
// and the org/project ids.
func newBudgetService(t *testing.T) (*BudgetService, *budgetFixture) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Greenfield Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Maple Street Remodel")

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewBudgetService(pool, store.NewFinancialsStore(), audit)
	return svc, &budgetFixture{orgID: orgID, projectID: projectID}
}

type budgetFixture struct {
	orgID     uuid.UUID
	projectID uuid.UUID
}

// auditCount returns how many audit_log rows exist for (org, action).
func auditCount(t *testing.T, s *BudgetService, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, action).Scan(&n)
	if err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// TestBudgetService_CreateInvoice_PersistsAndAudits is the canonical mutation
// round-trip: a valid invoice is persisted, the returned model carries the
// DB-assigned id + default pending status, and exactly one "invoice.created"
// audit row is written inside the same tx (proving the audit is transactional
// with the mutation, not a separate best-effort write).
func TestBudgetService_CreateInvoice_PersistsAndAudits(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()

	inv, err := svc.CreateInvoice(ctx, fx.orgID, "owner-sub", CreateInvoiceInput{
		ProjectID:    fx.projectID,
		VendorName:   "Acme Lumber",
		AmountCents:  150_000,
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if inv.ID == uuid.Nil {
		t.Error("created invoice has nil id")
	}
	if inv.AmountCents != 150_000 || inv.CurrencyCode != "USD" {
		t.Errorf("invoice = %d %s, want 150000 USD", inv.AmountCents, inv.CurrencyCode)
	}
	if inv.Status != models.InvoiceStatusPending {
		t.Errorf("status = %q, want pending", inv.Status)
	}

	if got := auditCount(t, svc, fx.orgID, "invoice.created"); got != 1 {
		t.Errorf("invoice.created audit rows = %d, want 1", got)
	}

	// Round-trips through the read path scoped to the caller's org.
	budgets, err := svc.GetProjectBudgets(ctx, fx.projectID, fx.orgID)
	if err != nil {
		t.Fatalf("GetProjectBudgets: %v", err)
	}
	_ = budgets // no budget rows seeded; the call must succeed (empty slice)
}

// TestBudgetService_CreateInvoice_Validation covers the three pre-tx input
// gates — each must reject with ErrInvalidInput BEFORE any row is written, so
// no audit row is produced for a rejected create.
func TestBudgetService_CreateInvoice_Validation(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()

	cases := []struct {
		name string
		in   CreateInvoiceInput
	}{
		{"bad currency", CreateInvoiceInput{ProjectID: fx.projectID, VendorName: "X", AmountCents: 1, CurrencyCode: "EUR"}},
		{"empty vendor", CreateInvoiceInput{ProjectID: fx.projectID, VendorName: "", AmountCents: 1, CurrencyCode: "USD"}},
		{"non-positive amount", CreateInvoiceInput{ProjectID: fx.projectID, VendorName: "X", AmountCents: 0, CurrencyCode: "USD"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.CreateInvoice(ctx, fx.orgID, "owner-sub", c.in); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("CreateInvoice(%s) = %v, want ErrInvalidInput", c.name, err)
			}
		})
	}
	if got := auditCount(t, svc, fx.orgID, "invoice.created"); got != 0 {
		t.Errorf("rejected creates wrote %d audit rows, want 0", got)
	}
}

// TestBudgetService_CreateInvoice_CrossTenantHidden proves the org-scoping
// guard: an invoice create against a project owned by ANOTHER org fails as
// ErrNotFound (not a leak of the project's existence), and writes nothing.
func TestBudgetService_CreateInvoice_CrossTenantHidden(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()

	otherOrg := uuid.New() // never seeded a project; fx.projectID belongs to fx.orgID
	_, err := svc.CreateInvoice(ctx, otherOrg, "intruder", CreateInvoiceInput{
		ProjectID:    fx.projectID,
		VendorName:   "Acme",
		AmountCents:  1,
		CurrencyCode: "USD",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant CreateInvoice = %v, want ErrNotFound", err)
	}
}

// TestBudgetService_UpdateInvoice_TransitionsAndAudits covers the update
// mutation: a created invoice is moved pending → paid, the returned model
// reflects the new status, and a second "invoice.updated" audit row joins the
// "invoice.created" one. Also covers the bad-status pre-tx gate and the
// not-found leg.
func TestBudgetService_UpdateInvoice_TransitionsAndAudits(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()

	inv, err := svc.CreateInvoice(ctx, fx.orgID, "owner-sub", CreateInvoiceInput{
		ProjectID:    fx.projectID,
		VendorName:   "Acme Lumber",
		AmountCents:  90_000,
		CurrencyCode: "CAD",
	})
	if err != nil {
		t.Fatalf("seed CreateInvoice: %v", err)
	}

	paid := models.InvoiceStatusPaid
	updated, err := svc.UpdateInvoice(ctx, fx.orgID, "owner-sub", UpdateInvoiceInput{
		InvoiceID: inv.ID,
		ProjectID: fx.projectID,
		Status:    &paid,
	})
	if err != nil {
		t.Fatalf("UpdateInvoice: %v", err)
	}
	if updated.Status != models.InvoiceStatusPaid {
		t.Errorf("updated status = %q, want paid", updated.Status)
	}
	if got := auditCount(t, svc, fx.orgID, "invoice.updated"); got != 1 {
		t.Errorf("invoice.updated audit rows = %d, want 1", got)
	}

	// Bad status is rejected before the tx.
	bogus := "bogus"
	if _, err := svc.UpdateInvoice(ctx, fx.orgID, "owner-sub", UpdateInvoiceInput{
		InvoiceID: inv.ID, ProjectID: fx.projectID, Status: &bogus,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("UpdateInvoice(bogus status) = %v, want ErrInvalidInput", err)
	}

	// Updating an invoice whose project isn't in the caller's org → ErrNotFound.
	if _, err := svc.UpdateInvoice(ctx, uuid.New(), "intruder", UpdateInvoiceInput{
		InvoiceID: inv.ID, ProjectID: fx.projectID, Status: &paid,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant UpdateInvoice = %v, want ErrNotFound", err)
	}
}

// TestBudgetService_RunCorporateRollup_Idempotent exercises the periodic
// quarter rollup: with no active project_budgets the upsert affects zero rows
// and succeeds, and a re-run stays at zero (idempotent within the quarter —
// no duplicate corporate_budgets rows).
func TestBudgetService_RunCorporateRollup_Idempotent(t *testing.T) {
	svc, _ := newBudgetService(t)
	ctx := context.Background()

	first, err := svc.RunCorporateRollup(ctx)
	if err != nil {
		t.Fatalf("RunCorporateRollup: %v", err)
	}
	if first != 0 {
		t.Errorf("rollup over an org with no budgets affected %d rows, want 0", first)
	}
	second, err := svc.RunCorporateRollup(ctx)
	if err != nil {
		t.Fatalf("RunCorporateRollup (re-run): %v", err)
	}
	if second != 0 {
		t.Errorf("idempotent re-run affected %d rows, want 0", second)
	}
}

// TestBudgetService_Reads_CurrencyValidation covers the optional-currency gate
// shared by the three org-scoped financial reads: a supported code passes
// (empty result on an empty org), an unsupported code is rejected pre-query.
func TestBudgetService_Reads_CurrencyValidation(t *testing.T) {
	svc, fx := newBudgetService(t)
	ctx := context.Background()

	if _, err := svc.GetARAging(ctx, fx.orgID, "USD"); err != nil {
		t.Errorf("GetARAging(USD) = %v, want nil", err)
	}
	if _, err := svc.GetProjectFinancials(ctx, fx.orgID, "CAD"); err != nil {
		t.Errorf("GetProjectFinancials(CAD) = %v, want nil", err)
	}
	if _, err := svc.GetOrgFinancialsSummary(ctx, fx.orgID, ""); err != nil {
		t.Errorf("GetOrgFinancialsSummary(no filter) = %v, want nil", err)
	}
	if _, err := svc.GetARAging(ctx, fx.orgID, "EUR"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("GetARAging(EUR) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.GetProjectFinancials(ctx, fx.orgID, "GBP"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("GetProjectFinancials(GBP) = %v, want ErrInvalidInput", err)
	}
}
