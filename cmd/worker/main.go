package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/futurebuildai/buildos/internal/config"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("worker exited with error", "error", err)
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

	logger.Info("database connected for worker", "max_conns", cfg.DBPoolMax)

	// Stores + services for workers that need service-layer access.
	financialsStore := store.NewFinancialsStore()
	budgetService := service.NewBudgetService(pool, financialsStore)

	// Notification delivery: the LoggingSender just logs and succeeds
	// — fine until Sprint 5 PR 3 swaps in real Twilio/FCM senders.
	// Even with the no-op sender wired, the DLQ infra + River retry
	// machinery is exercised at startup.
	notifStore := store.NewNotificationsStore()
	notifService := service.NewNotificationDeliveryService(pool, service.NewLoggingSender(logger), notifStore, logger)

	procurementStore := store.NewProcurementStore()
	procurementService := service.NewProcurementService(pool, procurementStore)

	registry, err := worker.NewRegistry(pool, logger, worker.Dependencies{
		BudgetRunner:          budgetService,
		NotificationDeliverer: notifService,
		ProcurementChecker:    procurementService,
	})
	if err != nil {
		return fmt.Errorf("creating worker registry: %w", err)
	}

	logger.Info("starting river worker, waiting for jobs")

	if err := registry.Start(ctx); err != nil {
		return fmt.Errorf("river worker error: %w", err)
	}

	logger.Info("worker stopped gracefully")
	return nil
}
