package ai

import (
	"errors"
	"testing"
)

// TestHTTPError_Is exercises the status-code → sentinel mapping that lets
// callers write errors.Is(err, ai.ErrRateLimited) against a *HTTPError
// without a type assertion. The method reads 0% in the package profile
// because the transport returns the bare sentinels after exhausting its
// retry budget, so errors.Is short-circuits on identity before ever
// dispatching to HTTPError.Is.
func TestHTTPError_Is(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		rateLimited    bool // want errors.Is(err, ErrRateLimited)
		transient      bool // want errors.Is(err, ErrTransient)
		circuitMatches bool // want errors.Is(err, ErrCircuitOpen) — always false
	}{
		{"429 is rate-limited only", 429, true, false, false},
		{"500 is transient only", 500, false, true, false},
		{"503 is transient only", 503, false, true, false},
		{"400 matches nothing", 400, false, false, false},
		{"404 matches nothing", 404, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &HTTPError{StatusCode: tc.status, Type: "x", Message: "y"}
			if got := errors.Is(err, ErrRateLimited); got != tc.rateLimited {
				t.Errorf("errors.Is(%d, ErrRateLimited) = %v, want %v", tc.status, got, tc.rateLimited)
			}
			if got := errors.Is(err, ErrTransient); got != tc.transient {
				t.Errorf("errors.Is(%d, ErrTransient) = %v, want %v", tc.status, got, tc.transient)
			}
			// Sentinels the method doesn't switch on never match.
			if got := errors.Is(err, ErrCircuitOpen); got != tc.circuitMatches {
				t.Errorf("errors.Is(%d, ErrCircuitOpen) = %v, want false", tc.status, got)
			}
		})
	}
}
