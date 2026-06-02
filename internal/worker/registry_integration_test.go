//go:build integration

package worker

import (
	"log/slog"
	"testing"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestNewRegistry_Integration proves the River client + worker registry build
// cleanly with every job worker registered (a duplicate kind or bad config
// would fail here) against a real pool.
func TestNewRegistry_Integration(t *testing.T) {
	pool := testdb.NewPool(t)
	deps := Dependencies{
		BudgetRunner:          &fakeRunner{},
		NotificationDeliverer: &fakeDeliverer{},
		ProcurementChecker:    &fakeChecker{},
	}

	reg, err := NewRegistry(pool, slog.Default(), deps)
	if err != nil {
		t.Fatalf("NewRegistry() = %v, want nil", err)
	}
	if reg.Client == nil {
		t.Error("registry Client is nil")
	}
	if reg.Workers == nil {
		t.Error("registry Workers is nil")
	}
}

// TestNewRegistry_PanicsOnNilDeps confirms the wiring fails fast at
// construction when a required service dependency is missing, rather than at
// the first scheduled tick.
func TestNewRegistry_PanicsOnNilDeps(t *testing.T) {
	pool := testdb.NewPool(t)
	defer func() {
		if recover() == nil {
			t.Error("expected panic with nil dependencies, got none")
		}
	}()
	_, _ = NewRegistry(pool, slog.Default(), Dependencies{})
}
