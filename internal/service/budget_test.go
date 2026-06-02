package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/futurebuildai/buildos/internal/store"
)

func TestCurrentFiscalQuarter(t *testing.T) {
	cases := []struct {
		date        string // RFC3339
		wantYear    int
		wantQuarter int
	}{
		// Q1 boundary: first instant of Jan, mid-Q1, last instant of Mar
		{"2026-01-01T00:00:00Z", 2026, 1},
		{"2026-02-15T12:00:00Z", 2026, 1},
		{"2026-03-31T23:59:59Z", 2026, 1},
		// Q2 boundary
		{"2026-04-01T00:00:00Z", 2026, 2},
		{"2026-06-30T23:59:59Z", 2026, 2},
		// Q3
		{"2026-07-01T00:00:00Z", 2026, 3},
		{"2026-09-30T23:59:59Z", 2026, 3},
		// Q4
		{"2026-10-01T00:00:00Z", 2026, 4},
		{"2026-12-31T23:59:59Z", 2026, 4},
		// Year rollover Q4 → Q1
		{"2027-01-01T00:00:00Z", 2027, 1},
		// Leap year Feb 29
		{"2028-02-29T00:00:00Z", 2028, 1},
	}
	for _, c := range cases {
		ts, err := time.Parse(time.RFC3339, c.date)
		if err != nil {
			t.Fatalf("parse %q: %v", c.date, err)
		}
		gotYear, gotQuarter := currentFiscalQuarter(ts)
		if gotYear != c.wantYear || gotQuarter != c.wantQuarter {
			t.Errorf("currentFiscalQuarter(%s) = (%d,Q%d), want (%d,Q%d)",
				c.date, gotYear, gotQuarter, c.wantYear, c.wantQuarter)
		}
	}
}

// TestValidateOptionalCurrency covers the BYO-currency gate that runs before
// any financials write. Empty is allowed (the column defaults to USD); a
// supported code passes; an unsupported/garbage code is wrapped as
// ErrInvalidInput so the handler returns 400 rather than persisting bad data.
func TestValidateOptionalCurrency(t *testing.T) {
	t.Run("empty is allowed", func(t *testing.T) {
		if err := validateOptionalCurrency(""); err != nil {
			t.Fatalf("validateOptionalCurrency(\"\") = %v, want nil", err)
		}
	})
	t.Run("supported codes pass", func(t *testing.T) {
		for _, code := range []string{"USD", "CAD"} {
			if err := validateOptionalCurrency(code); err != nil {
				t.Errorf("validateOptionalCurrency(%q) = %v, want nil", code, err)
			}
		}
	})
	t.Run("unsupported code is ErrInvalidInput", func(t *testing.T) {
		err := validateOptionalCurrency("EUR")
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("validateOptionalCurrency(\"EUR\") = %v, want ErrInvalidInput", err)
		}
	})
}

// TestMapStoreError covers the package-shared store→service error translation:
// nil stays nil, store.ErrNotFound becomes the service ErrNotFound sentinel the
// handler matches with errors.Is, and any other error passes through unchanged
// (so unexpected failures surface as 500, not a misleading 404).
func TestMapStoreError(t *testing.T) {
	if got := mapStoreError(nil); got != nil {
		t.Errorf("mapStoreError(nil) = %v, want nil", got)
	}
	if got := mapStoreError(store.ErrNotFound); !errors.Is(got, ErrNotFound) {
		t.Errorf("mapStoreError(store.ErrNotFound) = %v, want ErrNotFound", got)
	}
	other := fmt.Errorf("connection reset")
	if got := mapStoreError(other); !errors.Is(got, other) {
		t.Errorf("mapStoreError(other) = %v, want passthrough", got)
	}
}
