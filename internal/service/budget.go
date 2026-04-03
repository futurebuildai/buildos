package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

// CrossCurrencyError is returned when an operation would mix different currencies.
var ErrCrossCurrency = errors.New("CROSS_CURRENCY_ERROR: cannot mix currencies in a single financial operation")

// ErrInvalidCurrency is returned for unsupported currency codes.
var ErrInvalidCurrency = errors.New("INVALID_CURRENCY: only USD and CAD are supported")

// SupportedCurrencies are the only valid currency codes.
var SupportedCurrencies = map[string]bool{"USD": true, "CAD": true}

// BudgetService provides business logic for project budgets and invoices.
type BudgetService struct {
	store *store.FinancialStore
}

// NewBudgetService creates a new BudgetService.
func NewBudgetService(s *store.FinancialStore) *BudgetService {
	return &BudgetService{store: s}
}

// ListBudgets returns budgets for a project, filtered by currency (defaults to USD).
func (svc *BudgetService) ListBudgets(ctx context.Context, projectID uuid.UUID, currencyCode string) ([]models.ProjectBudget, error) {
	if currencyCode == "" {
		currencyCode = "USD"
	}
	if !SupportedCurrencies[currencyCode] {
		return nil, ErrInvalidCurrency
	}
	return svc.store.ListBudgetsByProject(ctx, projectID, currencyCode)
}

// UpsertBudget creates or updates a project budget entry.
// Validates that all currency codes in the budget are consistent.
func (svc *BudgetService) UpsertBudget(ctx context.Context, b *models.ProjectBudget) error {
	if err := ValidateBudgetCurrencies(b); err != nil {
		return err
	}
	return svc.store.UpsertBudget(ctx, b)
}

// CreateInvoice validates and creates a new invoice.
func (svc *BudgetService) CreateInvoice(ctx context.Context, inv *models.Invoice) (uuid.UUID, error) {
	if !SupportedCurrencies[inv.CurrencyCode] {
		return uuid.Nil, ErrInvalidCurrency
	}
	if inv.AmountCents <= 0 {
		return uuid.Nil, fmt.Errorf("amount_cents must be positive, got %d", inv.AmountCents)
	}
	if inv.Status == "" {
		inv.Status = models.InvoiceStatusPending
	}
	return svc.store.CreateInvoice(ctx, inv)
}

// ApproveInvoice transitions an invoice from pending to approved.
func (svc *BudgetService) ApproveInvoice(ctx context.Context, invoiceID uuid.UUID) error {
	inv, err := svc.store.GetInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if inv.Status != models.InvoiceStatusPending {
		return fmt.Errorf("cannot approve invoice in status %q", inv.Status)
	}
	return svc.store.UpdateInvoiceStatus(ctx, invoiceID, models.InvoiceStatusApproved, nil)
}

// PayInvoice transitions an invoice from approved to paid.
func (svc *BudgetService) PayInvoice(ctx context.Context, invoiceID uuid.UUID) error {
	inv, err := svc.store.GetInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if inv.Status != models.InvoiceStatusApproved {
		return fmt.Errorf("cannot pay invoice in status %q", inv.Status)
	}
	now := time.Now().UTC()
	return svc.store.UpdateInvoiceStatus(ctx, invoiceID, models.InvoiceStatusPaid, &now)
}

// RejectInvoice transitions an invoice from pending to rejected.
func (svc *BudgetService) RejectInvoice(ctx context.Context, invoiceID uuid.UUID) error {
	inv, err := svc.store.GetInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if inv.Status != models.InvoiceStatusPending {
		return fmt.Errorf("cannot reject invoice in status %q", inv.Status)
	}
	return svc.store.UpdateInvoiceStatus(ctx, invoiceID, models.InvoiceStatusRejected, nil)
}
