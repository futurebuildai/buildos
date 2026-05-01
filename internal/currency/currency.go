// Package currency provides safe arithmetic for the Composite Currency Pattern.
//
// All monetary values in BuildOS are stored as paired BIGINT cents + VARCHAR(3)
// currency code. Cross-currency arithmetic is forbidden by both the SQL layer
// (CHECK constraints) and this package (ErrCrossCurrency).
//
// Use Money for in-memory arithmetic; persistence and wire formats use the
// flat *_cents + *_currency_code fields directly per TECH_STACK convention.
package currency

import (
	"errors"
	"fmt"
)

// Supported currency codes. BuildOS does not support any others; introducing
// a new currency requires adding the code here, updating the SQL migration
// linter's allowed set, and reconciling with The Brain's billing engine.
const (
	USD = "USD"
	CAD = "CAD"
)

// Sentinel errors. Surface ErrCrossCurrency as HTTP 422 CROSS_CURRENCY_ERROR
// per API_CONTRACT.md.
var (
	ErrCrossCurrency       = errors.New("cross-currency arithmetic forbidden")
	ErrUnsupportedCurrency = errors.New("unsupported currency code")
)

// Validate returns an error if the code is not in the supported set.
// Empty strings are rejected — callers must default explicitly.
func Validate(code string) error {
	switch code {
	case USD, CAD:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedCurrency, code)
	}
}

// Money is a paired cents amount + currency code. The zero value is invalid
// (empty currency code); use New to construct.
type Money struct {
	Cents        int64
	CurrencyCode string
}

// New constructs a Money value, validating the currency code.
func New(cents int64, code string) (Money, error) {
	if err := Validate(code); err != nil {
		return Money{}, err
	}
	return Money{Cents: cents, CurrencyCode: code}, nil
}

// Zero returns a Money of zero cents in the given currency. The currency
// must be valid; callers that already trust their code (e.g. constants)
// can ignore the error.
func Zero(code string) (Money, error) {
	return New(0, code)
}

// Add returns m + other when currencies match, ErrCrossCurrency otherwise.
func (m Money) Add(other Money) (Money, error) {
	if m.CurrencyCode != other.CurrencyCode {
		return Money{}, fmt.Errorf("%w: %s + %s", ErrCrossCurrency, m.CurrencyCode, other.CurrencyCode)
	}
	return Money{Cents: m.Cents + other.Cents, CurrencyCode: m.CurrencyCode}, nil
}

// Sub returns m - other when currencies match, ErrCrossCurrency otherwise.
func (m Money) Sub(other Money) (Money, error) {
	if m.CurrencyCode != other.CurrencyCode {
		return Money{}, fmt.Errorf("%w: %s - %s", ErrCrossCurrency, m.CurrencyCode, other.CurrencyCode)
	}
	return Money{Cents: m.Cents - other.Cents, CurrencyCode: m.CurrencyCode}, nil
}

// SameCurrency reports whether two Money values share a currency code.
// An empty CurrencyCode on either side is treated as a mismatch — never
// silently aggregate uninitialized values.
func (m Money) SameCurrency(other Money) bool {
	return m.CurrencyCode != "" && m.CurrencyCode == other.CurrencyCode
}

// SumByCurrency aggregates a slice of Money grouped by currency code.
// Never sums across currencies — the result is keyed by code so the caller
// gets a per-currency total. Empty-string currency codes are ignored.
func SumByCurrency(values []Money) map[string]int64 {
	out := make(map[string]int64)
	for _, v := range values {
		if v.CurrencyCode == "" {
			continue
		}
		out[v.CurrencyCode] += v.Cents
	}
	return out
}
