package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/futurebuildai/buildos/internal/a2asigner"
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

	// Outbound A2A: optional. When A2A_SIGNING_KEY_PATH is unset the
	// worker registry falls back to a no-op handler that logs and
	// discards. Customer-fork deployments must provision a signing
	// key (see internal/a2asigner package docs) and add the matching
	// public key to their JWKS for Brain to verify signatures.
	var a2aOutbound worker.A2AWebhookDeliverer
	if cfg.A2ASigningKeyPath != "" {
		signer, err := a2asigner.NewSignerFromFile(cfg.A2ASigningKeyPath, cfg.A2AKeyID)
		if err != nil {
			return fmt.Errorf("loading a2a signing key: %w", err)
		}
		targetURL := cfg.BrainOutboundURL
		if targetURL == "" {
			// Fallback: same host as the issuer, /api/v1/a2a/webhook
			// path. The IssuerURL is the OIDC issuer, which lives on
			// the same Brain deployment.
			targetURL = strings.TrimRight(cfg.BrainIssuerURL, "/") + "/api/v1/a2a/webhook"
		}
		outboundStore := store.NewA2AOutboundStore()
		a2aOutbound = service.NewA2AOutboundService(pool, outboundStore, signer, targetURL,
			&http.Client{Timeout: 30 * time.Second}, logger)
		logger.Info("a2a outbound dispatcher wired",
			"target_url", targetURL, "key_id", cfg.A2AKeyID)
	} else {
		logger.Warn("a2a outbound dispatcher NOT wired — A2A_SIGNING_KEY_PATH unset; outbound events will be discarded")
	}

	registry, err := worker.NewRegistry(pool, logger, worker.Dependencies{
		BudgetRunner:          budgetService,
		NotificationDeliverer: notifService,
		ProcurementChecker:    procurementService,
		A2AWebhookDeliverer:   a2aOutbound,
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
