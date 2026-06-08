package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/currency"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Sentinel errors returned by BudgetService. Handlers map these to
// HTTP status codes:
//
//	ErrNotFound       → 404
//	ErrCrossCurrency  → 422 CROSS_CURRENCY_ERROR (re-export from currency pkg)
//	ErrInvalidInput   → 400 VALIDATION_ERROR
var (
	ErrNotFound      = errors.New("budget: resource not found")
	ErrCrossCurrency = currency.ErrCrossCurrency
	ErrInvalidInput  = errors.New("budget: invalid input")
)

// BudgetService orchestrates financial reads and writes. It enforces
// cross-tenant isolation (every project-scoped operation verifies the
// project belongs to the caller's org) and validates currency codes
// before any data hits the database.
type BudgetService struct {
	pool  *pgxpool.Pool
	store *store.FinancialsStore
	audit AuditRecorder
}

// NewBudgetService creates a new BudgetService.
// audit may be nil; nil falls back to a no-op recorder.
func NewBudgetService(pool *pgxpool.Pool, fs *store.FinancialsStore, audit AuditRecorder) *BudgetService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &BudgetService{pool: pool, store: fs, audit: audit}
}

// ---------- Reads ----------

// GetProjectBudgets returns budget rows for a project, scoped to the
// caller's org. Returns ErrNotFound if the project does not exist or
// belongs to a different org.
func (s *BudgetService) GetProjectBudgets(ctx context.Context, projectID, callerOrgID uuid.UUID) ([]models.ProjectBudget, error) {
	var out []models.ProjectBudget
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}
		var qErr error
		out, qErr = s.store.ListProjectBudgets(ctx, tx, projectID)
		return qErr
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return out, nil
}

// FinancialsSummary bundles corporate budget rollups and AR aging snapshots.
type FinancialsSummary struct {
	CorporateBudgets []models.CorporateBudget `json:"corporate_budgets"`
	ARAging          []models.ARAgingSnapshot `json:"ar_aging"`
}

// GetOrgFinancialsSummary returns corporate rollups + AR aging snapshots
// for an org. If currencyCode is non-empty, results are filtered to that
// single currency. Returns ErrInvalidInput if currencyCode is provided
// but unsupported.
func (s *BudgetService) GetOrgFinancialsSummary(ctx context.Context, orgID uuid.UUID, currencyCode string) (FinancialsSummary, error) {
	if err := validateOptionalCurrency(currencyCode); err != nil {
		return FinancialsSummary{}, err
	}
	var summary FinancialsSummary
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		cbs, err := s.store.ListCorporateBudgets(ctx, tx, orgID, currencyCode)
		if err != nil {
			return err
		}
		ar, err := s.store.ListLatestARAgingSnapshots(ctx, tx, orgID, currencyCode)
		if err != nil {
			return err
		}
		summary.CorporateBudgets = cbs
		summary.ARAging = ar
		return nil
	})
	if err != nil {
		return FinancialsSummary{}, err
	}
	return summary, nil
}

// GetARAging returns the most recent AR aging snapshot per currency for
// an org. If currencyCode is non-empty, results are filtered to that
// single currency.
func (s *BudgetService) GetARAging(ctx context.Context, orgID uuid.UUID, currencyCode string) ([]models.ARAgingSnapshot, error) {
	if err := validateOptionalCurrency(currencyCode); err != nil {
		return nil, err
	}
	var out []models.ARAgingSnapshot
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var qErr error
		out, qErr = s.store.ListLatestARAgingSnapshots(ctx, tx, orgID, currencyCode)
		return qErr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetProjectFinancials returns per-project rollups (one row per project
// per currency) for an org. If currencyCode is non-empty, results are
// filtered to that single currency.
func (s *BudgetService) GetProjectFinancials(ctx context.Context, orgID uuid.UUID, currencyCode string) ([]models.ProjectFinancial, error) {
	if err := validateOptionalCurrency(currencyCode); err != nil {
		return nil, err
	}
	var out []models.ProjectFinancial
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var qErr error
		out, qErr = s.store.ListProjectFinancials(ctx, tx, orgID, currencyCode)
		return qErr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------- Writes ----------

// CreateInvoiceInput is the service-layer input for creating an invoice.
// CurrencyCode and AmountCents are required; other fields are optional.
type CreateInvoiceInput struct {
	ProjectID     uuid.UUID
	VendorName    string
	InvoiceNumber *string
	AmountCents   int64
	CurrencyCode  string
	WBSCode       *string
	DueDate       *time.Time
	// Source tags the invoice provenance ("manual" | "ai_ingest"). nil →
	// the store resolves it to the column DEFAULT 'manual'. The manual
	// CreateInvoice path leaves it nil; the AI ingestion path passes
	// ptr("ai_ingest").
	Source *string
}

// CreateInvoice records a new invoice for a project, scoped to the
// caller's org. Validates currency code and that the project belongs
// to the org. Returns the persisted invoice.
//
// callerUserSub is recorded on the audit row. The body delegates to
// createInvoiceTx so the manual path and the AI ingestion path share one
// money-validation chokepoint (§7.5(a)).
func (s *BudgetService) CreateInvoice(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in CreateInvoiceInput) (models.Invoice, error) {
	var inv models.Invoice
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		created, err := s.createInvoiceTx(ctx, tx, callerOrgID, callerUserSub, in)
		if err != nil {
			return err
		}
		inv = created
		return nil
	})
	if err != nil {
		return models.Invoice{}, mapStoreError(err)
	}
	return inv, nil
}

// createInvoiceTx is the single money-validation chokepoint for invoice
// creation. It re-applies the three invariants (currency in {USD,CAD},
// vendor non-empty, amount > 0), verifies the project belongs to the
// caller's org, writes the invoice via the store, and records the audit
// row — all on the caller-supplied tx. Both the public CreateInvoice
// (which opens its own tx) and IngestionService.IngestInvoiceFromDocument
// (which passes its existing write tx) route through here so no caller can
// bypass the invariants (§7.5(a)).
//
// Validation errors return ErrInvalidInput BEFORE any row or audit is
// written, so a rejected create leaves no state. The tx ownership stays
// with the caller (this never commits or rolls back).
func (s *BudgetService) createInvoiceTx(ctx context.Context, tx pgx.Tx, callerOrgID uuid.UUID, callerUserSub string, in CreateInvoiceInput) (models.Invoice, error) {
	if err := currency.Validate(in.CurrencyCode); err != nil {
		return models.Invoice{}, fmt.Errorf("%w: currency_code: %v", ErrInvalidInput, err)
	}
	if in.VendorName == "" {
		return models.Invoice{}, fmt.Errorf("%w: vendor_name is required", ErrInvalidInput)
	}
	if in.AmountCents <= 0 {
		return models.Invoice{}, fmt.Errorf("%w: amount_cents must be positive", ErrInvalidInput)
	}

	if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
		return models.Invoice{}, err
	}
	created, err := s.store.CreateInvoice(ctx, tx, store.CreateInvoiceParams{
		ProjectID:     in.ProjectID,
		OrgID:         callerOrgID,
		VendorName:    in.VendorName,
		InvoiceNumber: in.InvoiceNumber,
		AmountCents:   in.AmountCents,
		CurrencyCode:  in.CurrencyCode,
		WBSCode:       in.WBSCode,
		DueDate:       in.DueDate,
		Source:        in.Source, // nil → store resolves to 'manual'
	})
	if err != nil {
		return models.Invoice{}, err
	}
	s.audit.Record(ctx, tx, AuditEntry{
		OrgID:        callerOrgID,
		UserSub:      callerUserSub,
		Action:       "invoice.created",
		ResourceType: AuditResourceInvoice,
		ResourceID:   created.ID,
		After:        marshalAudit(created),
		Metadata: marshalAudit(map[string]any{
			"project_id":   in.ProjectID,
			"vendor_name":  in.VendorName,
			"amount_cents": in.AmountCents,
			"currency":     in.CurrencyCode,
			"wbs_code":     in.WBSCode,
		}),
	})
	return created, nil
}

// UpdateInvoiceInput is the service-layer input for updating an invoice.
// All fields are optional; nil fields preserve existing values.
type UpdateInvoiceInput struct {
	InvoiceID uuid.UUID
	ProjectID uuid.UUID
	Status    *string
	PaidDate  *time.Time
}

// UpdateInvoice modifies an invoice's status and/or paid_date. Verifies
// the invoice belongs to the project, which belongs to the caller's org.
// Validates status transitions if a new status is provided.
//
// callerUserSub is recorded on the audit row.
func (s *BudgetService) UpdateInvoice(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in UpdateInvoiceInput) (models.Invoice, error) {
	if in.Status != nil && !models.IsValidInvoiceStatus(*in.Status) {
		return models.Invoice{}, fmt.Errorf("%w: status %q is not one of {pending, approved, rejected, paid}", ErrInvalidInput, *in.Status)
	}

	var inv models.Invoice
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		updated, err := s.store.UpdateInvoice(ctx, tx, store.UpdateInvoiceParams{
			ID:       in.InvoiceID,
			Status:   in.Status,
			PaidDate: in.PaidDate,
		})
		if err != nil {
			return err
		}
		inv = updated
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "invoice.updated",
			ResourceType: AuditResourceInvoice,
			ResourceID:   updated.ID,
			After:        marshalAudit(updated),
			Metadata: marshalAudit(map[string]any{
				"status":    in.Status,
				"paid_date": in.PaidDate,
			}),
		})
		return nil
	})
	if err != nil {
		return models.Invoice{}, mapStoreError(err)
	}
	return inv, nil
}

// ---------- Periodic rollup ----------

// RunCorporateRollup aggregates active project_budgets across every org
// into one corporate_budgets row per (org, current calendar quarter,
// currency_code). Idempotent within a quarter: re-runs upsert the same
// rows. Quarter rollover creates new rows automatically.
//
// Returns rows-affected for telemetry (insert + update count).
func (s *BudgetService) RunCorporateRollup(ctx context.Context) (int64, error) {
	year, quarter := currentFiscalQuarter(time.Now().UTC())
	var rows int64
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		rows, qErr = s.store.UpsertCorporateRollupForQuarter(ctx, tx, year, quarter)
		return qErr
	})
	if err != nil {
		return 0, err
	}
	return rows, nil
}

// currentFiscalQuarter returns the calendar year and quarter (1–4) for t.
// BuildOS uses calendar quarters: Q1=Jan–Mar, Q2=Apr–Jun, Q3=Jul–Sep,
// Q4=Oct–Dec. If a tenant adopts a non-calendar fiscal year later, the
// rollup logic will need to learn per-tenant fiscal start months.
func currentFiscalQuarter(t time.Time) (year, quarter int) {
	return t.Year(), int(t.Month()-1)/3 + 1
}

// ---------- helpers ----------

// validateOptionalCurrency validates a currency code if non-empty.
// Empty string means "no filter" and is always valid.
func validateOptionalCurrency(code string) error {
	if code == "" {
		return nil
	}
	if err := currency.Validate(code); err != nil {
		return fmt.Errorf("%w: currency: %v", ErrInvalidInput, err)
	}
	return nil
}

// mapStoreError translates store-layer errors into service-layer sentinels
// the API handler can match with errors.Is.
func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
