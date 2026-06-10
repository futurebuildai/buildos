package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(logger); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	direction, dryRun := parseArgs(os.Args[1:])

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Step 1: Run River migrations (creates river_job, river_queue, etc.)
	riverMigrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("creating river migrator: %w", err)
	}
	if dryRun {
		// Don't mutate river_migration in a dry run; the app-migration list
		// below (Step 2) is the actionable preview.
		logger.Info("[dry-run] skipping river migrations", "direction", direction)
	} else {
		logger.Info("running river migrations", "direction", direction)
		if direction == "up" {
			res, err := riverMigrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
			if err != nil {
				return fmt.Errorf("river migrate up: %w", err)
			}
			for _, v := range res.Versions {
				logger.Info("river migration applied", "version", v.Version)
			}
		} else {
			res, err := riverMigrator.Migrate(ctx, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{TargetVersion: -1})
			if err != nil {
				return fmt.Errorf("river migrate down: %w", err)
			}
			for _, v := range res.Versions {
				logger.Info("river migration rolled back", "version", v.Version)
			}
		}
	}

	// Step 2: Run application SQL migrations
	logger.Info("running application migrations", "direction", direction, "dry_run", dryRun)
	migrationDir := "migrations"
	if envDir := os.Getenv("MIGRATIONS_DIR"); envDir != "" {
		migrationDir = envDir
	}

	if direction == "up" {
		return runAppMigrationsUp(ctx, pool, migrationDir, dryRun, logger)
	}
	if dryRun {
		logger.Info("[dry-run] down migrations are not enumerated; run without --dry-run to roll back")
		return nil
	}
	return runAppMigrationsDown(ctx, pool, migrationDir, logger)
}

// parseArgs resolves the direction (the first non-flag positional, default "up")
// and the --dry-run/-n flag. Without this, the old positional parse made
// `migrate --dry-run` set direction="--dry-run" and fall into the DOWN branch.
func parseArgs(args []string) (direction string, dryRun bool) {
	direction = "up"
	gotDir := false
	for _, a := range args {
		switch a {
		case "--dry-run", "-n":
			dryRun = true
		default:
			if !gotDir && !strings.HasPrefix(a, "-") {
				direction = a
				gotDir = true
			}
		}
	}
	return direction, dryRun
}

func runAppMigrationsUp(ctx context.Context, pool *pgxpool.Pool, dir string, dryRun bool, logger *slog.Logger) error {
	// Create tracking table if it doesn't exist. Idempotent; needed even for a
	// dry run so the "already applied?" query below works on a fresh DB. This is
	// the runner's own bookkeeping table, not a schema migration.
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("listing migration files: %w", err)
	}
	// An empty glob is ALWAYS a deployment mistake (wrong MIGRATIONS_DIR or
	// working directory), never a valid state — this repo ships migrations
	// from 001. Exiting 0 here would let a CD pipeline's migrate-before-roll
	// gate pass while applying nothing (a green deploy with no schema).
	if len(files) == 0 {
		return fmt.Errorf("no *.up.sql files found in %q — set MIGRATIONS_DIR or run from the repo root; refusing to report success having applied nothing", dir)
	}
	sort.Strings(files)

	for _, f := range files {
		version := extractVersion(f)

		// Check if already applied
		var exists bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		if dryRun {
			logger.Info("[dry-run] pending migration", "version", version, "file", filepath.Base(f))
			continue
		}

		sql, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("executing %s: %w", f, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("recording %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("committing %s: %w", version, err)
		}

		logger.Info("migration applied", "version", version, "file", filepath.Base(f))
	}

	return nil
}

func runAppMigrationsDown(ctx context.Context, pool *pgxpool.Pool, dir string, logger *slog.Logger) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.down.sql"))
	if err != nil {
		return fmt.Errorf("listing migration files: %w", err)
	}
	// Reverse order for rollback
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	for _, f := range files {
		version := extractVersion(f)

		// Check if applied
		var exists bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
		if err != nil {
			// Table might not exist on full rollback
			break
		}
		if !exists {
			continue
		}

		sql, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("executing %s: %w", f, err)
		}

		if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("removing %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("committing %s: %w", version, err)
		}

		logger.Info("migration rolled back", "version", version, "file", filepath.Base(f))
	}

	return nil
}

// extractVersion gets "001" from "migrations/001_initial_schema.up.sql"
func extractVersion(path string) string {
	base := filepath.Base(path)
	parts := strings.SplitN(base, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return base
}
