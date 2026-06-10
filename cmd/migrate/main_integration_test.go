//go:build integration

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestRunAppMigrationsUpDown exercises the application-SQL migration
// runner end-to-end against a real pool, using a throwaway temp dir of
// test-only migration files (version 900, a disposable table) so it
// never collides with the repo's real migrations. Covers the up path
// (tracking-table create, glob, apply, record, idempotent re-run skip)
// and the down path (reverse-order glob, apply, de-register).
func TestRunAppMigrationsUpDown(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "900_widget.up.sql"),
		"CREATE TABLE mig_test_widget (id INT PRIMARY KEY);")
	writeFile(t, filepath.Join(dir, "900_widget.down.sql"),
		"DROP TABLE mig_test_widget;")

	// --- Up: applies the migration and records version 900.
	if err := runAppMigrationsUp(ctx, pool, dir, false, logger); err != nil {
		t.Fatalf("runAppMigrationsUp: %v", err)
	}
	if !tableExists(t, pool, "mig_test_widget") {
		t.Fatal("mig_test_widget not created by up migration")
	}
	if versionCount(t, pool, "900") != 1 {
		t.Fatal("version 900 not recorded in schema_migrations")
	}

	// --- Up again: already-applied versions are skipped (no error, no
	// duplicate row — the `if exists { continue }` branch).
	if err := runAppMigrationsUp(ctx, pool, dir, false, logger); err != nil {
		t.Fatalf("runAppMigrationsUp (re-run): %v", err)
	}
	if n := versionCount(t, pool, "900"); n != 1 {
		t.Fatalf("schema_migrations rows for 900 = %d, want 1 (idempotent)", n)
	}

	// --- Down: rolls the migration back and de-registers version 900.
	if err := runAppMigrationsDown(ctx, pool, dir, logger); err != nil {
		t.Fatalf("runAppMigrationsDown: %v", err)
	}
	if tableExists(t, pool, "mig_test_widget") {
		t.Fatal("mig_test_widget still present after down migration")
	}
	if versionCount(t, pool, "900") != 0 {
		t.Fatal("version 900 still recorded after down migration")
	}
}

// TestRunAppMigrationsUp_DryRun proves --dry-run lists a pending migration
// WITHOUT applying it (no table created, version not recorded).
func TestRunAppMigrationsUp_DryRun(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "901_dryrun.up.sql"),
		"CREATE TABLE mig_test_dryrun (id INT PRIMARY KEY);")

	if err := runAppMigrationsUp(ctx, pool, dir, true /*dryRun*/, logger); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if tableExists(t, pool, "mig_test_dryrun") {
		t.Error("dry-run must NOT create the table")
	}
	if versionCount(t, pool, "901") != 0 {
		t.Error("dry-run must NOT record the version")
	}

	// A real run after the dry-run applies it (the dry-run left no residue).
	if err := runAppMigrationsUp(ctx, pool, dir, false, logger); err != nil {
		t.Fatalf("apply after dry-run: %v", err)
	}
	if !tableExists(t, pool, "mig_test_dryrun") {
		t.Error("real run after dry-run should apply the migration")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(),
		"SELECT to_regclass($1) IS NOT NULL", name).Scan(&ok); err != nil {
		t.Fatalf("tableExists(%s): %v", name, err)
	}
	return ok
}

func versionCount(t *testing.T, pool *pgxpool.Pool, version string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM schema_migrations WHERE version = $1", version).Scan(&n); err != nil {
		t.Fatalf("versionCount(%s): %v", version, err)
	}
	return n
}
