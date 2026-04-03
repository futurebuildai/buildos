package service

import (
	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ValidateBudgetCurrencies ensures all currency codes within a ProjectBudget are
// identical and supported. Returns ErrCrossCurrency if they differ, or
// ErrInvalidCurrency if any code is not USD or CAD.
func ValidateBudgetCurrencies(b *models.ProjectBudget) error {
	codes := []string{
		b.EstimatedCostCurrencyCode,
		b.CommittedCostCurrencyCode,
		b.ActualCostCurrencyCode,
	}

	first := codes[0]
	if !SupportedCurrencies[first] {
		return ErrInvalidCurrency
	}

	for _, code := range codes[1:] {
		if code != first {
			return ErrCrossCurrency
		}
		if !SupportedCurrencies[code] {
			return ErrInvalidCurrency
		}
	}

	return nil
}

// ValidateCurrencyMatch checks that two currency codes are identical.
// Returns ErrCrossCurrency if they differ.
func ValidateCurrencyMatch(a, b string) error {
	if a != b {
		return ErrCrossCurrency
	}
	return nil
}
