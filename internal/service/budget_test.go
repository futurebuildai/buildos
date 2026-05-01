package service

import (
	"testing"
	"time"
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
