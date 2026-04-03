package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// FinancialStore provides raw SQL access to financial tables via pgx.
type FinancialStore struct {
	pool *pgxpool.Pool
}

// NewFinancialStore creates a new FinancialStore.
func NewFinancialStore(pool *pgxpool.Pool) *FinancialStore {
	return &FinancialStore{pool: pool}
}

// --- Project Budgets ---

// ListBudgetsByProject returns all budget rows for a project, filtered by currency.
func (s *FinancialStore) ListBudgetsByProject(ctx context.Context, projectID uuid.UUID, currencyCode string) ([]models.ProjectBudget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, wbs_code, phase_name,
			estimated_cost_cents, estimated_cost_currency_code,
			committed_cost_cents, committed_cost_currency_code,
			actual_cost_cents, actual_cost_currency_code,
			created_at, updated_at
		FROM project_budgets
		WHERE project_id = $1 AND estimated_cost_currency_code = $2
		ORDER BY wbs_code`, projectID, currencyCode)
	if err != nil {
		return nil, fmt.Errorf("querying project budgets: %w", err)
	}
	defer rows.Close()

	return collectBudgets(rows)
}

// UpsertBudget inserts or updates a budget for a project/wbs_code combination.
func (s *FinancialStore) UpsertBudget(ctx context.Context, b *models.ProjectBudget) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_budgets (
			project_id, wbs_code, phase_name,
			estimated_cost_cents, estimated_cost_currency_code,
			committed_cost_cents, committed_cost_currency_code,
			actual_cost_cents, actual_cost_currency_code
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (project_id, wbs_code) DO UPDATE SET
			phase_name = EXCLUDED.phase_name,
			estimated_cost_cents = EXCLUDED.estimated_cost_cents,
			estimated_cost_currency_code = EXCLUDED.estimated_cost_currency_code,
			committed_cost_cents = EXCLUDED.committed_cost_cents,
			committed_cost_currency_code = EXCLUDED.committed_cost_currency_code,
			actual_cost_cents = EXCLUDED.actual_cost_cents,
			actual_cost_currency_code = EXCLUDED.actual_cost_currency_code,
			updated_at = now()`,
		b.ProjectID, b.WBSCode, b.PhaseName,
		b.EstimatedCostCents, b.EstimatedCostCurrencyCode,
		b.CommittedCostCents, b.CommittedCostCurrencyCode,
		b.ActualCostCents, b.ActualCostCurrencyCode,
	)
	if err != nil {
		return fmt.Errorf("upserting budget: %w", err)
	}
	return nil
}

// --- Invoices ---

// CreateInvoice inserts a new invoice and returns the generated ID.
func (s *FinancialStore) CreateInvoice(ctx context.Context, inv *models.Invoice) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO invoices (project_id, org_id, vendor_name, invoice_number,
			amount_cents, currency_code, wbs_code, status, due_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		inv.ProjectID, inv.OrgID, inv.VendorName, inv.InvoiceNumber,
		inv.AmountCents, inv.CurrencyCode, inv.WBSCode, inv.Status, inv.DueDate,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating invoice: %w", err)
	}
	return id, nil
}

// UpdateInvoiceStatus changes an invoice's status (e.g., pending → approved → paid).
func (s *FinancialStore) UpdateInvoiceStatus(ctx context.Context, invoiceID uuid.UUID, status string, paidDate *time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE invoices SET status = $2, paid_date = $3 WHERE id = $1`,
		invoiceID, status, paidDate)
	if err != nil {
		return fmt.Errorf("updating invoice status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetInvoice returns a single invoice by ID.
func (s *FinancialStore) GetInvoice(ctx context.Context, invoiceID uuid.UUID) (*models.Invoice, error) {
	var inv models.Invoice
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, org_id, vendor_name, invoice_number,
			amount_cents, currency_code, wbs_code, status, due_date, paid_date, created_at
		FROM invoices WHERE id = $1`, invoiceID,
	).Scan(
		&inv.ID, &inv.ProjectID, &inv.OrgID, &inv.VendorName, &inv.InvoiceNumber,
		&inv.AmountCents, &inv.CurrencyCode, &inv.WBSCode, &inv.Status,
		&inv.DueDate, &inv.PaidDate, &inv.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting invoice: %w", err)
	}
	return &inv, nil
}

// ListInvoicesByProject returns all invoices for a project.
func (s *FinancialStore) ListInvoicesByProject(ctx context.Context, projectID uuid.UUID) ([]models.Invoice, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, org_id, vendor_name, invoice_number,
			amount_cents, currency_code, wbs_code, status, due_date, paid_date, created_at
		FROM invoices WHERE project_id = $1
		ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing invoices: %w", err)
	}
	defer rows.Close()

	var invoices []models.Invoice
	for rows.Next() {
		var inv models.Invoice
		if err := rows.Scan(
			&inv.ID, &inv.ProjectID, &inv.OrgID, &inv.VendorName, &inv.InvoiceNumber,
			&inv.AmountCents, &inv.CurrencyCode, &inv.WBSCode, &inv.Status,
			&inv.DueDate, &inv.PaidDate, &inv.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning invoice: %w", err)
		}
		invoices = append(invoices, inv)
	}
	return invoices, rows.Err()
}

// --- AR Aging ---

// CreateARAgingSnapshot inserts a new aging snapshot.
func (s *FinancialStore) CreateARAgingSnapshot(ctx context.Context, snap *models.ARAgingSnapshot) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ar_aging_snapshots (org_id, snapshot_date, currency_code,
			current_cents, days_30_cents, days_60_cents, days_90_plus_cents, total_receivable_cents)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		snap.OrgID, snap.SnapshotDate, snap.CurrencyCode,
		snap.CurrentCents, snap.Days30Cents, snap.Days60Cents,
		snap.Days90PlusCents, snap.TotalReceivableCents,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating AR aging snapshot: %w", err)
	}
	return id, nil
}

// LatestARAgingByOrg returns the most recent aging snapshot for an org and currency.
func (s *FinancialStore) LatestARAgingByOrg(ctx context.Context, orgID uuid.UUID, currencyCode string) (*models.ARAgingSnapshot, error) {
	var snap models.ARAgingSnapshot
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, snapshot_date, currency_code,
			current_cents, days_30_cents, days_60_cents,
			days_90_plus_cents, total_receivable_cents, created_at
		FROM ar_aging_snapshots
		WHERE org_id = $1 AND currency_code = $2
		ORDER BY snapshot_date DESC
		LIMIT 1`, orgID, currencyCode,
	).Scan(
		&snap.ID, &snap.OrgID, &snap.SnapshotDate, &snap.CurrencyCode,
		&snap.CurrentCents, &snap.Days30Cents, &snap.Days60Cents,
		&snap.Days90PlusCents, &snap.TotalReceivableCents, &snap.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting latest AR aging: %w", err)
	}
	return &snap, nil
}

// --- Corporate Budget ---

// UpsertCorporateBudget inserts or updates a corporate rollup row.
func (s *FinancialStore) UpsertCorporateBudget(ctx context.Context, cb *models.CorporateBudget) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO corporate_budgets (org_id, fiscal_year, quarter, currency_code,
			total_estimated_cents, total_committed_cents, total_actual_cents,
			project_count, last_rollup_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (org_id, fiscal_year, quarter, currency_code) DO UPDATE SET
			total_estimated_cents = EXCLUDED.total_estimated_cents,
			total_committed_cents = EXCLUDED.total_committed_cents,
			total_actual_cents = EXCLUDED.total_actual_cents,
			project_count = EXCLUDED.project_count,
			last_rollup_at = now(),
			updated_at = now()`,
		cb.OrgID, cb.FiscalYear, cb.Quarter, cb.CurrencyCode,
		cb.TotalEstimatedCents, cb.TotalCommittedCents, cb.TotalActualCents,
		cb.ProjectCount,
	)
	if err != nil {
		return fmt.Errorf("upserting corporate budget: %w", err)
	}
	return nil
}

// GetCorporateBudget returns the corporate rollup for an org/year/quarter/currency.
func (s *FinancialStore) GetCorporateBudget(ctx context.Context, orgID uuid.UUID, year, quarter int, currencyCode string) (*models.CorporateBudget, error) {
	var cb models.CorporateBudget
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, fiscal_year, quarter, currency_code,
			total_estimated_cents, total_committed_cents, total_actual_cents,
			project_count, last_rollup_at, created_at, updated_at
		FROM corporate_budgets
		WHERE org_id = $1 AND fiscal_year = $2 AND quarter = $3 AND currency_code = $4`,
		orgID, year, quarter, currencyCode,
	).Scan(
		&cb.ID, &cb.OrgID, &cb.FiscalYear, &cb.Quarter, &cb.CurrencyCode,
		&cb.TotalEstimatedCents, &cb.TotalCommittedCents, &cb.TotalActualCents,
		&cb.ProjectCount, &cb.LastRollupAt, &cb.CreatedAt, &cb.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting corporate budget: %w", err)
	}
	return &cb, nil
}

// --- Project Financial Summary (computed view) ---

// ProjectFinancialSummaries returns per-project financial summaries for an org, filtered by currency.
func (s *FinancialStore) ProjectFinancialSummaries(ctx context.Context, orgID uuid.UUID, currencyCode string) ([]models.ProjectFinancialSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			p.id,
			p.name,
			COALESCE(b.currency_code, $2) AS currency_code,
			COALESCE(SUM(b.estimated_cost_cents), 0) AS total_estimated_cents,
			COALESCE(SUM(b.committed_cost_cents), 0) AS total_committed_cents,
			COALESCE(SUM(b.actual_cost_cents), 0) AS total_actual_cents,
			COALESCE(SUM(b.estimated_cost_cents), 0) - COALESCE(SUM(b.actual_cost_cents), 0) AS variance_cents,
			COUNT(DISTINCT inv.id) AS invoice_count,
			COALESCE(SUM(CASE WHEN inv.status = 'pending' THEN inv.amount_cents ELSE 0 END), 0) AS pending_invoice_cents
		FROM projects p
		LEFT JOIN project_budgets b ON b.project_id = p.id AND b.estimated_cost_currency_code = $2
		LEFT JOIN invoices inv ON inv.project_id = p.id AND inv.currency_code = $2
		WHERE p.org_id = $1
		GROUP BY p.id, p.name, b.currency_code
		ORDER BY p.name`,
		orgID, currencyCode,
	)
	if err != nil {
		return nil, fmt.Errorf("querying project financials: %w", err)
	}
	defer rows.Close()

	var summaries []models.ProjectFinancialSummary
	for rows.Next() {
		var s models.ProjectFinancialSummary
		if err := rows.Scan(
			&s.ProjectID, &s.ProjectName, &s.CurrencyCode,
			&s.TotalEstimatedCents, &s.TotalCommittedCents, &s.TotalActualCents,
			&s.VarianceCents, &s.InvoiceCount, &s.PendingInvoiceCents,
		); err != nil {
			return nil, fmt.Errorf("scanning project financial: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// RollupProjectBudgets computes aggregate budget totals for an org+currency, used by corporate rollup.
func (s *FinancialStore) RollupProjectBudgets(ctx context.Context, orgID uuid.UUID, currencyCode string) (estimated, committed, actual int64, projectCount int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(b.estimated_cost_cents), 0),
			COALESCE(SUM(b.committed_cost_cents), 0),
			COALESCE(SUM(b.actual_cost_cents), 0),
			COUNT(DISTINCT b.project_id)
		FROM project_budgets b
		JOIN projects p ON p.id = b.project_id
		WHERE p.org_id = $1 AND b.estimated_cost_currency_code = $2`,
		orgID, currencyCode,
	).Scan(&estimated, &committed, &actual, &projectCount)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("rolling up project budgets: %w", err)
	}
	return
}

// --- helpers ---

func collectBudgets(rows pgx.Rows) ([]models.ProjectBudget, error) {
	var budgets []models.ProjectBudget
	for rows.Next() {
		var b models.ProjectBudget
		if err := rows.Scan(
			&b.ID, &b.ProjectID, &b.WBSCode, &b.PhaseName,
			&b.EstimatedCostCents, &b.EstimatedCostCurrencyCode,
			&b.CommittedCostCents, &b.CommittedCostCurrencyCode,
			&b.ActualCostCents, &b.ActualCostCurrencyCode,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning budget: %w", err)
		}
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}
