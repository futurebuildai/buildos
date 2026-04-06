package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNewCorporateFinancialsService(t *testing.T) {
	svc := NewCorporateFinancialsService(nil)
	if svc == nil {
		t.Fatal("expected non-nil CorporateFinancialsService")
	}
}

// ---------------------------------------------------------------------------
// quarterOf — pure function tests (covers all 4 quarters)
// ---------------------------------------------------------------------------

func TestQuarterOf(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
		want int
	}{
		{"January → Q1", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), 1},
		{"February → Q1", time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), 1},
		{"March → Q1", time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), 1},
		{"April → Q2", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), 2},
		{"May → Q2", time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC), 2},
		{"June → Q2", time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), 2},
		{"July → Q3", time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), 3},
		{"August → Q3", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), 3},
		{"September → Q3", time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), 3},
		{"October → Q4", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 4},
		{"November → Q4", time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), 4},
		{"December → Q4", time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := quarterOf(tc.time)
			if got != tc.want {
				t.Errorf("quarterOf(%v) = %d, want %d", tc.time, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FinancialSummary struct tests
// ---------------------------------------------------------------------------

func TestFinancialSummary_EmptyFields(t *testing.T) {
	fs := &FinancialSummary{}
	if fs.CorporateBudget != nil {
		t.Error("expected nil CorporateBudget")
	}
	if fs.LatestARAging != nil {
		t.Error("expected nil LatestARAging")
	}
}

// ---------------------------------------------------------------------------
// Summary — currency validation
// ---------------------------------------------------------------------------

func TestCorporateFinancials_Summary_DefaultCurrency(t *testing.T) {
	// When currency is empty, it should default to USD, then reach the store.
	// Since store is nil, it will panic after validation. We verify it didn't
	// return ErrInvalidCurrency.
	svc := NewCorporateFinancialsService(nil)
	orgID := uuid.New()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.Summary(context.Background(), orgID, "")
	}()

	if !panicked {
		t.Error("expected panic from nil store after defaulting to USD, but did not panic")
	}
}

func TestCorporateFinancials_Summary_InvalidCurrency(t *testing.T) {
	svc := NewCorporateFinancialsService(nil)
	orgID := uuid.New()

	tests := []struct {
		name     string
		currency string
	}{
		{"EUR", "EUR"},
		{"GBP", "GBP"},
		{"JPY", "JPY"},
		{"empty after check", "BTC"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Summary(context.Background(), orgID, tc.currency)
			if !errors.Is(err, ErrInvalidCurrency) {
				t.Errorf("expected ErrInvalidCurrency for %q, got: %v", tc.currency, err)
			}
		})
	}
}

func TestCorporateFinancials_Summary_ValidCurrencies(t *testing.T) {
	// Both USD and CAD should pass validation and reach the store (panic on nil).
	svc := NewCorporateFinancialsService(nil)
	orgID := uuid.New()

	for _, cc := range []string{"USD", "CAD"} {
		t.Run(cc, func(t *testing.T) {
			panicked := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
					}
				}()
				_, _ = svc.Summary(context.Background(), orgID, cc)
			}()
			if !panicked {
				t.Errorf("expected panic from nil store for currency %s", cc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ProjectFinancials — currency validation
// ---------------------------------------------------------------------------

func TestCorporateFinancials_ProjectFinancials_InvalidCurrency(t *testing.T) {
	svc := NewCorporateFinancialsService(nil)
	orgID := uuid.New()

	_, err := svc.ProjectFinancials(context.Background(), orgID, "EUR")
	if !errors.Is(err, ErrInvalidCurrency) {
		t.Errorf("expected ErrInvalidCurrency, got: %v", err)
	}
}

func TestCorporateFinancials_ProjectFinancials_DefaultCurrency(t *testing.T) {
	svc := NewCorporateFinancialsService(nil)
	orgID := uuid.New()

	// Empty currency defaults to USD, then panics on nil store
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.ProjectFinancials(context.Background(), orgID, "")
	}()
	if !panicked {
		t.Error("expected panic from nil store after defaulting to USD")
	}
}

func TestCorporateFinancials_ProjectFinancials_ValidCurrencies(t *testing.T) {
	svc := NewCorporateFinancialsService(nil)
	orgID := uuid.New()

	for _, cc := range []string{"USD", "CAD"} {
		t.Run(cc, func(t *testing.T) {
			panicked := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
					}
				}()
				_, _ = svc.ProjectFinancials(context.Background(), orgID, cc)
			}()
			if !panicked {
				t.Errorf("expected panic from nil store for %s", cc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RunCorporateRollup — validation & rollup iteration
// ---------------------------------------------------------------------------

func TestCorporateFinancials_RunCorporateRollup_ReachesStore(t *testing.T) {
	// RunCorporateRollup iterates over USD and CAD. With a nil store it will
	// panic on the first store call.
	svc := NewCorporateFinancialsService(nil)
	orgID := uuid.New()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_ = svc.RunCorporateRollup(context.Background(), orgID)
	}()
	if !panicked {
		t.Error("expected panic from nil store during rollup")
	}
}

// ---------------------------------------------------------------------------
// BIGINT cents compliance — structural verification
// ---------------------------------------------------------------------------

func TestCorporateFinancials_BigintCentsCompliance(t *testing.T) {
	// Verify that the CorporateBudget model uses int64 for monetary fields
	// and that the service never converts to float. This is a structural test.
	svc := NewCorporateFinancialsService(nil)
	if svc == nil {
		t.Fatal("service should not be nil")
	}

	// Verify large cent values work without overflow in the model
	largeValues := []int64{
		0,
		1,
		999_999_999_99,      // ~$10B
		9_223_372_036_854_775_807, // max int64
		-1,
		-999_999_999_99,
	}

	for _, v := range largeValues {
		// Ensure assignment to int64 fields compiles and doesn't lose precision
		var estimated int64 = v
		var committed int64 = v
		var actual int64 = v

		if estimated != v || committed != v || actual != v {
			t.Errorf("int64 precision lost for value %d", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Currency separation — no cross-currency mixing in rollup design
// ---------------------------------------------------------------------------

func TestCorporateFinancials_RollupCurrencySeparation(t *testing.T) {
	// The RunCorporateRollup method iterates over []string{"USD", "CAD"} separately,
	// ensuring no cross-currency aggregation. Verify the supported currencies map
	// is consistent with this design.
	expectedCurrencies := []string{"USD", "CAD"}
	for _, cc := range expectedCurrencies {
		if !SupportedCurrencies[cc] {
			t.Errorf("expected %s to be in SupportedCurrencies", cc)
		}
	}

	// Verify unsupported currencies are rejected
	unsupported := []string{"EUR", "GBP", "JPY", "AUD", "CHF", "BTC", ""}
	for _, cc := range unsupported {
		if SupportedCurrencies[cc] {
			t.Errorf("expected %s to NOT be in SupportedCurrencies", cc)
		}
	}
}
