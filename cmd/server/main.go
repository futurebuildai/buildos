package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/futurebuildai/buildos/internal/api"
	"github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/config"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	pool, err := store.NewPool(ctx, store.PoolConfig{
		DatabaseURL:    cfg.DatabaseURL,
		MaxConns:       cfg.DBPoolMax,
		MinConns:       cfg.DBPoolMin,
		ConnectTimeout: cfg.DBTimeout,
	})
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	logger.Info("database connected", "max_conns", cfg.DBPoolMax)

	// JWKS provider for JWT validation and JWS verification
	jwks := middleware.NewJWKSProvider(cfg.BrainJWKSURL, logger)

	if cfg.DevAuthMode != "" {
		logger.Warn("DEV_AUTH_MODE is set — JWT validation may be bypassed",
			"mode", cfg.DevAuthMode,
			"production_safe", false)
	}

	// River insert-only client. The API server enqueues jobs from inside
	// service-layer transactions (river.InsertTx); workers run separately
	// via cmd/worker. No Workers / Queues config here — this client only
	// writes to river_job rows; cmd/worker drains them.
	riverClient, err := river.NewClient[pgx.Tx](riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return fmt.Errorf("creating river insert client: %w", err)
	}

	// Stores + services
	financialsStore := store.NewFinancialsStore()
	budgetService := service.NewBudgetService(pool, financialsStore)
	pipelineStore := store.NewPipelineStore()
	pipelineService := service.NewPipelineService(pool, pipelineStore, riverClient)
	scheduleStore := store.NewScheduleStore()
	scheduleService := service.NewScheduleService(pool, scheduleStore, riverClient)

	// Build the router with all route groups
	router := api.NewRouter(api.RouterConfig{
		Pool:            pool,
		JWKS:            jwks,
		IssuerURL:       cfg.BrainIssuerURL,
		DevAuthMode:     cfg.DevAuthMode,
		Logger:          logger,
		BudgetService:   budgetService,
		PipelineService: pipelineService,
		ScheduleService: scheduleService,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	logger.Info("server stopped gracefully")
	return nil
}
