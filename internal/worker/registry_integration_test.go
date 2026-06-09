//go:build integration

package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

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
		CascadeOrchestrator:   &fakeCascadeOrchestrator{},
		ForesightRunner:       &fakeForesightRunner{},
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

// TestRegistry_Start proves the registry actually boots the River client
// against a live database: River's own job tables (river_job, …) are not
// in migrations/*.up.sql, so they're applied here via rivermigrate (the
// same step cmd/migrate runs). Start(ctx) is non-blocking — it boots the
// producers and returns nil once running — so the test starts the client
// then gracefully drains it via Client.Stop (the cmd/worker shutdown
// path). A missing River schema or a bad client config would surface as
// a non-nil Start error.
func TestRegistry_Start(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("river migrator: %v", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("river migrate up: %v", err)
	}

	deps := Dependencies{
		BudgetRunner:          &fakeRunner{},
		NotificationDeliverer: &fakeDeliverer{},
		ProcurementChecker:    &fakeChecker{},
		CascadeOrchestrator:   &fakeCascadeOrchestrator{},
		ForesightRunner:       &fakeForesightRunner{},
	}
	reg, err := NewRegistry(pool, slog.Default(), deps)
	if err != nil {
		t.Fatalf("NewRegistry() = %v, want nil", err)
	}

	if err := reg.Start(ctx); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	// Graceful drain (mirrors cmd/worker's bounded Stop).
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := reg.Client.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
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
