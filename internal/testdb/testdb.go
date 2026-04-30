// Package testdb provides Postgres test fixtures backed by Testcontainers.
//
// Use NewPool(t) in any integration test to get a fresh pgxpool.Pool
// connected to a freshly migrated database. Container + pool are torn
// down automatically when the test exits via t.Cleanup.
//
// Integration tests that import this package should sit behind the
// `//go:build integration` build tag so the default `go test ./...`
// run doesn't require Docker. Use:
//
//	go test -tags=integration ./...
//
// (or `make test-integration`).
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewPool spins up a fresh Postgres 16 container, applies all migrations,
// and returns a pgxpool.Pool wired to it. Cleanup (pool close + container
// terminate) is registered via t.Cleanup, so callers do nothing.
//
// Skips the test if Docker isn't reachable (helpful in environments
// without docker-in-docker).
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	startCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	container, err := postgres.Run(startCtx,
		"postgres:16-alpine",
		postgres.WithDatabase("buildos_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Skipf("testdb: Docker unavailable or container start failed: %v", err)
		return nil
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("testdb: container terminate failed: %v", err)
		}
	})

	dsn, err := container.ConnectionString(startCtx, "sslmode=disable")
	if err != nil {
		t.Fatalf("testdb: get connection string: %v", err)
	}

	pool, err := pgxpool.New(startCtx, dsn)
	if err != nil {
		t.Fatalf("testdb: connect to test pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := applyMigrations(startCtx, pool); err != nil {
		t.Fatalf("testdb: apply migrations: %v", err)
	}

	return pool
}

// applyMigrations runs every migrations/*.up.sql in lexicographic order.
// Runs each migration as a single Exec — pgx will auto-wrap multi-statement
// SQL in an implicit transaction at the protocol layer.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir, err := findMigrationsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %q: %w", dir, err)
	}
	var ups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(ups)

	for _, path := range ups {
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

// findMigrationsDir walks up from the current working directory until it
// finds a "migrations" subdirectory. Tests running from any package depth
// (e.g. internal/store/) end up at the repo root.
func findMigrationsDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached filesystem root
			return "", fmt.Errorf("migrations dir not found above %s", cwd)
		}
	}
}

// SeedOrg inserts a minimal organization row. Slug is derived from the
// id (stringified) so calls don't collide on the UNIQUE constraint when
// many tests share a pool. Useful when a test needs a valid org_id FK
// target before seeding projects, prospects, or invoices.
func SeedOrg(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, name string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug)
		VALUES ($1, $2, $3)`, id, name, id.String())
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
}

// SeedProject inserts a minimal active project row.
func SeedProject(t *testing.T, pool *pgxpool.Pool, projectID, orgID uuid.UUID, name string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO projects (id, org_id, name, status)
		VALUES ($1, $2, $3, 'active')`, projectID, orgID, name)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
}
