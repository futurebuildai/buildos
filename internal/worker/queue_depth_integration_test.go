//go:build integration

package worker

import (
	"context"
	"testing"

	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/futurebuildai/buildos/internal/testdb"
)

func TestRegistry_QueueDepth(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	// River's own tables aren't in migrations/*.up.sql — apply them.
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("river migrator: %v", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("river migrate: %v", err)
	}

	r := &Registry{pool: pool}

	// Empty queue → 0 depth, 0 oldest (the COALESCE guard).
	depth, oldest, err := r.queueDepth(ctx)
	if err != nil {
		t.Fatalf("queueDepth (empty): %v", err)
	}
	if depth != 0 || oldest != 0 {
		t.Errorf("empty queue: depth=%d oldest=%v, want 0/0", depth, oldest)
	}

	// One available job scheduled 10s ago.
	if _, err := pool.Exec(ctx, `
		INSERT INTO river_job (state, kind, queue, args, metadata, max_attempts, priority, scheduled_at)
		VALUES ('available', 'test_kind', 'default', '{}'::jsonb, '{}'::jsonb, 5, 1, now() - interval '10 seconds')`); err != nil {
		t.Fatalf("insert available job: %v", err)
	}
	// A non-available job must NOT count toward the ready backlog.
	if _, err := pool.Exec(ctx, `
		INSERT INTO river_job (state, kind, queue, args, metadata, max_attempts, priority, scheduled_at, finalized_at)
		VALUES ('completed', 'done_kind', 'default', '{}'::jsonb, '{}'::jsonb, 5, 1, now() - interval '1 hour', now())`); err != nil {
		t.Fatalf("insert completed job: %v", err)
	}

	depth, oldest, err = r.queueDepth(ctx)
	if err != nil {
		t.Fatalf("queueDepth: %v", err)
	}
	if depth != 1 {
		t.Errorf("depth = %d, want 1 (only the available job)", depth)
	}
	if oldest < 9 {
		t.Errorf("oldest = %v seconds, want >= ~10", oldest)
	}
}
