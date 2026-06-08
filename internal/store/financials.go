package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// FinancialsStore provides raw SQL queries for financial domain tables.
//
// All methods take a pgx.Tx so callers control transaction scope. For
// read-only endpoints, the service layer opens a short-lived read tx
// to keep the type signature uniform with write paths.
type FinancialsStore struct{}

// NewFinancialsStore creates a new FinancialsStore.
func NewFinancialsStore() *FinancialsStore { return &FinancialsStore{} }

// ---------- Project budgets ----------

// ListProjectBudgets returns every budget row for a project, ordered by
// WBS code. The chk_budget_currency_match CHECK constraint guarantees
// that within each row the three monetary pairs share the same currency.
func (s *FinancialsStore) ListProjectBudgets(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) ([]models.ProjectBudget, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, project_id, wbs_code, phase_name,
		       estimated_cost_cents, estimated_cost_currency_code,
		       committed_cost_cents, committed_cost_currency_code,
		       actual_cost_cents, actual_cost_currency_code,
		       created_at, updated_at
		FROM project_budgets
		WHERE project_id = $1
		ORDER BY wbs_code`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project_budgets: %w", err)
	}
	defer rows.Close()

	out := make([]models.ProjectBudget, 0)
	for rows.Next() {
		var b models.ProjectBudget
		if err := rows.Scan(
			&b.ID, &b.ProjectID, &b.WBSCode, &b.PhaseName,
			&b.EstimatedCostCents, &b.EstimatedCostCurrencyCode,
			&b.CommittedCostCents, &b.CommittedCostCurrencyCode,
			&b.ActualCostCents, &b.ActualCostCurrencyCode,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project_budgets: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---------- Corporate budgets ----------

// ListCorporateBudgets returns rollup rows for an org. If currency is
// non-empty, results are filtered to that currency; otherwise every
// currency the org has ever produced rollups for is returned.
func (s *FinancialsStore) ListCorporateBudgets(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, currency string) ([]models.CorporateBudget, error) {
	q := `
		SELECT id, org_id, fiscal_year, quarter, currency_code,
		       total_estimated_cents, total_committed_cents, total_actual_cents,
		       project_count, last_rollup_at, created_at, updated_at
		FROM corporate_budgets
		WHERE org_id = $1`
	args := []any{orgID}
	if currency != "" {
		q += ` AND currency_code = $2`
		args = append(args, currency)
	}
	q += ` ORDER BY fiscal_year DESC, quarter DESC, currency_code`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query corporate_budgets: %w", err)
	}
	defer rows.Close()

	out := make([]models.CorporateBudget, 0)
	for rows.Next() {
		var b models.CorporateBudget
		if err := rows.Scan(
			&b.ID, &b.OrgID, &b.FiscalYear, &b.Quarter, &b.CurrencyCode,
			&b.TotalEstimatedCents, &b.TotalCommittedCents, &b.TotalActualCents,
			&b.ProjectCount, &b.LastRollupAt, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan corporate_budgets: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---------- Project financials (per-project rollup) ----------

// ListProjectFinancials returns one row per (project, currency_code) for
// the given org, summing project_budgets across all WBS phases. Projects
// with zero budget rows are omitted (nothing to summarize). If currency
// is non-empty, results are filtered to that single currency.
//
// The chk_budget_currency_match constraint guarantees that within a single
// project_budgets row all three monetary pairs share a currency, so we can
// group by estimated_cost_currency_code as the canonical project currency.
func (s *FinancialsStore) ListProjectFinancials(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, currency string) ([]models.ProjectFinancial, error) {
	q := `
		SELECT
			p.id,
			p.name,
			pb.estimated_cost_currency_code AS currency_code,
			SUM(pb.estimated_cost_cents)::BIGINT AS total_estimated_cents,
			SUM(pb.committed_cost_cents)::BIGINT AS total_committed_cents,
			SUM(pb.actual_cost_cents)::BIGINT  AS total_actual_cents,
			COUNT(*)                           AS phase_count
		FROM projects p
		INNER JOIN project_budgets pb ON pb.project_id = p.id
		WHERE p.org_id = $1`
	args := []any{orgID}
	if currency != "" {
		q += ` AND pb.estimated_cost_currency_code = $2`
		args = append(args, currency)
	}
	q += `
		GROUP BY p.id, p.name, pb.estimated_cost_currency_code
		ORDER BY p.name, pb.estimated_cost_currency_code`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query project_financials: %w", err)
	}
	defer rows.Close()

	out := make([]models.ProjectFinancial, 0)
	for rows.Next() {
		var pf models.ProjectFinancial
		if err := rows.Scan(
			&pf.ProjectID, &pf.ProjectName, &pf.CurrencyCode,
			&pf.TotalEstimatedCents, &pf.TotalCommittedCents, &pf.TotalActualCents,
			&pf.PhaseCount,
		); err != nil {
			return nil, fmt.Errorf("scan project_financials: %w", err)
		}
		out = append(out, pf)
	}
	return out, rows.Err()
}

// ---------- AR aging ----------

// ListLatestARAgingSnapshots returns the most recent snapshot per currency
// for an org. If currency is non-empty, results are filtered to that
// single currency. The summary endpoint expects "latest per currency"
// rather than the full historical series.
func (s *FinancialsStore) ListLatestARAgingSnapshots(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, currency string) ([]models.ARAgingSnapshot, error) {
	q := `
		SELECT DISTINCT ON (currency_code)
		       id, org_id, snapshot_date, currency_code,
		       current_cents, days_30_cents, days_60_cents, days_90_plus_cents,
		       total_receivable_cents, created_at
		FROM ar_aging_snapshots
		WHERE org_id = $1`
	args := []any{orgID}
	if currency != "" {
		q += ` AND currency_code = $2`
		args = append(args, currency)
	}
	// DISTINCT ON requires currency_code to lead the ORDER BY.
	q += ` ORDER BY currency_code, snapshot_date DESC`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query ar_aging_snapshots: %w", err)
	}
	defer rows.Close()

	out := make([]models.ARAgingSnapshot, 0)
	for rows.Next() {
		var snap models.ARAgingSnapshot
		if err := rows.Scan(
			&snap.ID, &snap.OrgID, &snap.SnapshotDate, &snap.CurrencyCode,
			&snap.CurrentCents, &snap.Days30Cents, &snap.Days60Cents, &snap.Days90PlusCents,
			&snap.TotalReceivableCents, &snap.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ar_aging_snapshots: %w", err)
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

// ---------- Corporate rollup (write) ----------

// UpsertCorporateRollupForQuarter aggregates active project_budgets across
// every org and writes one corporate_budgets row per (org, fiscal_year,
// quarter, currency_code). Existing rows for the given (year, quarter)
// are updated in place.
//
// Aggregation rules:
//   - Only projects with status IN ('active', 'completed') contribute.
//     Archived projects are excluded — they're historical and shouldn't
//     skew current-quarter totals.
//   - Each project_budgets row contributes to the bucket matching its
//     own currency_code. The chk_budget_currency_match constraint
//     guarantees within-row consistency, so estimated_cost_currency_code
//     is the canonical bucket key.
//   - project_count is COUNT(DISTINCT project_id) within each
//     (org, currency_code) bucket — a single project with budgets in
//     two currencies counts in both buckets.
//
// Returns rows-affected (inserts + updates).
func (s *FinancialsStore) UpsertCorporateRollupForQuarter(ctx context.Context, tx pgx.Tx, fiscalYear, quarter int) (int64, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO corporate_budgets (
			org_id, fiscal_year, quarter, currency_code,
			total_estimated_cents, total_committed_cents, total_actual_cents,
			project_count, last_rollup_at, updated_at
		)
		SELECT
			p.org_id,
			$1::INTEGER,
			$2::INTEGER,
			pb.estimated_cost_currency_code,
			SUM(pb.estimated_cost_cents)::BIGINT,
			SUM(pb.committed_cost_cents)::BIGINT,
			SUM(pb.actual_cost_cents)::BIGINT,
			COUNT(DISTINCT p.id),
			now(),
			now()
		FROM projects p
		INNER JOIN project_budgets pb ON pb.project_id = p.id
		WHERE p.status IN ('active', 'completed')
		GROUP BY p.org_id, pb.estimated_cost_currency_code
		ON CONFLICT (org_id, fiscal_year, quarter, currency_code)
		DO UPDATE SET
			total_estimated_cents = EXCLUDED.total_estimated_cents,
			total_committed_cents = EXCLUDED.total_committed_cents,
			total_actual_cents    = EXCLUDED.total_actual_cents,
			project_count         = EXCLUDED.project_count,
			last_rollup_at        = now(),
			updated_at            = now()`,
		fiscalYear, quarter,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert corporate_budgets: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ---------- Invoices ----------

// ListInvoices returns invoices for a project, newest first.
func (s *FinancialsStore) ListInvoices(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) ([]models.Invoice, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, project_id, org_id, vendor_name, invoice_number,
		       amount_cents, currency_code, wbs_code, status,
		       due_date, paid_date, created_at
		FROM invoices
		WHERE project_id = $1
		ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query invoices: %w", err)
	}
	defer rows.Close()

	out := make([]models.Invoice, 0)
	for rows.Next() {
		var inv models.Invoice
		if err := rows.Scan(
			&inv.ID, &inv.ProjectID, &inv.OrgID, &inv.VendorName, &inv.InvoiceNumber,
			&inv.AmountCents, &inv.CurrencyCode, &inv.WBSCode, &inv.Status,
			&inv.DueDate, &inv.PaidDate, &inv.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invoices: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// CreateInvoiceParams is the input for CreateInvoice. CurrencyCode must
// already be validated by the caller (use currency.Validate).
type CreateInvoiceParams struct {
	ProjectID     uuid.UUID
	OrgID         uuid.UUID
	VendorName    string
	InvoiceNumber *string
	AmountCents   int64
	CurrencyCode  string
	WBSCode       *string
	DueDate       *time.Time
	// Source is the invoice provenance: "manual" | "ai_ingest". Nil leaves the
	// column at its DEFAULT 'manual' (migration 014), so the manual-entry path
	// passes nil and the ingestion path passes ptr("ai_ingest").
	Source *string
}

// CreateInvoice inserts a new invoice and returns the persisted row.
// Status defaults to "pending" via the table DEFAULT.
func (s *FinancialsStore) CreateInvoice(ctx context.Context, tx pgx.Tx, p CreateInvoiceParams) (models.Invoice, error) {
	var inv models.Invoice
	err := tx.QueryRow(ctx, `
		INSERT INTO invoices (
			project_id, org_id, vendor_name, invoice_number,
			amount_cents, currency_code, wbs_code, due_date, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, 'manual'))
		RETURNING id, project_id, org_id, vendor_name, invoice_number,
		          amount_cents, currency_code, wbs_code, status,
		          due_date, paid_date, created_at`,
		p.ProjectID, p.OrgID, p.VendorName, p.InvoiceNumber,
		p.AmountCents, p.CurrencyCode, p.WBSCode, p.DueDate, p.Source,
	).Scan(
		&inv.ID, &inv.ProjectID, &inv.OrgID, &inv.VendorName, &inv.InvoiceNumber,
		&inv.AmountCents, &inv.CurrencyCode, &inv.WBSCode, &inv.Status,
		&inv.DueDate, &inv.PaidDate, &inv.CreatedAt,
	)
	if err != nil {
		return models.Invoice{}, fmt.Errorf("insert invoice: %w", err)
	}
	return inv, nil
}

// UpdateInvoiceParams is the input for UpdateInvoice. Only non-nil fields
// are written; nil fields preserve the existing value via COALESCE.
type UpdateInvoiceParams struct {
	ID       uuid.UUID
	Status   *string
	PaidDate *time.Time
}

// UpdateInvoice modifies an invoice's status and/or paid_date. Returns
// ErrNotFound if no row matched. Status, if provided, must already be
// validated by the caller (use models.IsValidInvoiceStatus).
func (s *FinancialsStore) UpdateInvoice(ctx context.Context, tx pgx.Tx, p UpdateInvoiceParams) (models.Invoice, error) {
	var inv models.Invoice
	err := tx.QueryRow(ctx, `
		UPDATE invoices
		SET status    = COALESCE($2, status),
		    paid_date = COALESCE($3, paid_date)
		WHERE id = $1
		RETURNING id, project_id, org_id, vendor_name, invoice_number,
		          amount_cents, currency_code, wbs_code, status,
		          due_date, paid_date, created_at`,
		p.ID, p.Status, p.PaidDate,
	).Scan(
		&inv.ID, &inv.ProjectID, &inv.OrgID, &inv.VendorName, &inv.InvoiceNumber,
		&inv.AmountCents, &inv.CurrencyCode, &inv.WBSCode, &inv.Status,
		&inv.DueDate, &inv.PaidDate, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Invoice{}, ErrNotFound
		}
		return models.Invoice{}, fmt.Errorf("update invoice: %w", err)
	}
	return inv, nil
}
