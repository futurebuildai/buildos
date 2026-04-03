package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

func TestValidateBudgetCurrencies_AllUSD(t *testing.T) {
	b := &models.ProjectBudget{
		EstimatedCostCurrencyCode: "USD",
		CommittedCostCurrencyCode: "USD",
		ActualCostCurrencyCode:    "USD",
	}
	if err := ValidateBudgetCurrencies(b); err != nil {
		t.Errorf("expected no error for all-USD budget, got: %v", err)
	}
}

func TestValidateBudgetCurrencies_AllCAD(t *testing.T) {
	b := &models.ProjectBudget{
		EstimatedCostCurrencyCode: "CAD",
		CommittedCostCurrencyCode: "CAD",
		ActualCostCurrencyCode:    "CAD",
	}
	if err := ValidateBudgetCurrencies(b); err != nil {
		t.Errorf("expected no error for all-CAD budget, got: %v", err)
	}
}

func TestValidateBudgetCurrencies_CrossCurrency(t *testing.T) {
	tests := []struct {
		name      string
		estimated string
		committed string
		actual    string
	}{
		{"estimated USD, committed CAD", "USD", "CAD", "USD"},
		{"estimated USD, actual CAD", "USD", "USD", "CAD"},
		{"all different", "USD", "CAD", "USD"},
		{"committed different", "CAD", "USD", "CAD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &models.ProjectBudget{
				EstimatedCostCurrencyCode: tc.estimated,
				CommittedCostCurrencyCode: tc.committed,
				ActualCostCurrencyCode:    tc.actual,
			}
			err := ValidateBudgetCurrencies(b)
			if !errors.Is(err, ErrCrossCurrency) {
				t.Errorf("expected ErrCrossCurrency, got: %v", err)
			}
		})
	}
}

func TestValidateBudgetCurrencies_UnsupportedCurrency(t *testing.T) {
	b := &models.ProjectBudget{
		EstimatedCostCurrencyCode: "EUR",
		CommittedCostCurrencyCode: "EUR",
		ActualCostCurrencyCode:    "EUR",
	}
	err := ValidateBudgetCurrencies(b)
	if !errors.Is(err, ErrInvalidCurrency) {
		t.Errorf("expected ErrInvalidCurrency, got: %v", err)
	}
}

func TestValidateCurrencyMatch(t *testing.T) {
	if err := ValidateCurrencyMatch("USD", "USD"); err != nil {
		t.Errorf("USD == USD should not error: %v", err)
	}
	if err := ValidateCurrencyMatch("CAD", "CAD"); err != nil {
		t.Errorf("CAD == CAD should not error: %v", err)
	}
	if err := ValidateCurrencyMatch("USD", "CAD"); !errors.Is(err, ErrCrossCurrency) {
		t.Errorf("USD != CAD should return ErrCrossCurrency: %v", err)
	}
}

func TestProjectBudget_VarianceCents(t *testing.T) {
	b := &models.ProjectBudget{
		EstimatedCostCents: 100000,
		ActualCostCents:    85000,
	}
	if v := b.VarianceCents(); v != 15000 {
		t.Errorf("VarianceCents() = %d, want 15000", v)
	}
}

func TestProjectBudget_VarianceCents_OverBudget(t *testing.T) {
	b := &models.ProjectBudget{
		EstimatedCostCents: 100000,
		ActualCostCents:    120000,
	}
	if v := b.VarianceCents(); v != -20000 {
		t.Errorf("VarianceCents() = %d, want -20000", v)
	}
}

func TestSupportedCurrencies(t *testing.T) {
	if !SupportedCurrencies["USD"] {
		t.Error("USD should be supported")
	}
	if !SupportedCurrencies["CAD"] {
		t.Error("CAD should be supported")
	}
	if SupportedCurrencies["EUR"] {
		t.Error("EUR should not be supported")
	}
	if SupportedCurrencies["GBP"] {
		t.Error("GBP should not be supported")
	}
}

func TestNewBudgetService(t *testing.T) {
	// Just verify it doesn't panic with nil (no-op test for coverage)
	svc := NewBudgetService(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestBudgetService_ListBudgets_InvalidCurrency(t *testing.T) {
	svc := NewBudgetService(nil)
	_, err := svc.ListBudgets(nil, uuid.New(), "EUR")
	if !errors.Is(err, ErrInvalidCurrency) {
		t.Errorf("expected ErrInvalidCurrency, got: %v", err)
	}
}

func TestBudgetService_CreateInvoice_InvalidCurrency(t *testing.T) {
	svc := NewBudgetService(nil)
	_, err := svc.CreateInvoice(nil, &models.Invoice{
		CurrencyCode: "EUR",
		AmountCents:  1000,
	})
	if !errors.Is(err, ErrInvalidCurrency) {
		t.Errorf("expected ErrInvalidCurrency, got: %v", err)
	}
}

func TestBudgetService_CreateInvoice_NegativeAmount(t *testing.T) {
	svc := NewBudgetService(nil)
	_, err := svc.CreateInvoice(nil, &models.Invoice{
		CurrencyCode: "USD",
		AmountCents:  -500,
	})
	if err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestBudgetService_CreateInvoice_ZeroAmount(t *testing.T) {
	svc := NewBudgetService(nil)
	_, err := svc.CreateInvoice(nil, &models.Invoice{
		CurrencyCode: "USD",
		AmountCents:  0,
	})
	if err == nil {
		t.Error("expected error for zero amount")
	}
}
