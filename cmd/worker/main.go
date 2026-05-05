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
	"github.com/futurebuildai/buildos/internal/obs"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

func main() {
	// Same correlating handler as cmd/server — when a River job runs
	// with a request_id in ctx (rare, but can happen for jobs enqueued
	// from a request scope), the worker's log line stamps it too.
	jsonH := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(obs.NewCorrelatingHandler(jsonH))
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

	// Sentry up first so subsequent init failures land. Empty DSN is
	// a no-op; the flush is always safe.
	flushSentry, _ := obs.InitSentry(obs.SentryConfig{
		DSN:              cfg.SentryDSN,
		Environment:      cfg.SentryEnvironment,
		Release:          cfg.SentryRelease,
		TracesSampleRate: cfg.SentryTracesRate,
	}, logger)
	defer flushSentry()

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
	// CorporateRollupWorker writes via the budget service; rollups are
	// system-actor mutations (no human caller), so we pass a no-op
	// audit recorder. Per-row audit for those rollups would be huge
	// volume + low signal; the daily aggregate count goes to the log
	// line at completion time, which is sufficient observability.
	financialsStore := store.NewFinancialsStore()
	budgetService := service.NewBudgetService(pool, financialsStore, service.NewNoopAuditRecorder())

	// Notification delivery: the LoggingSender just logs and succeeds
	// — fine until Sprint 5 PR 3 swaps in real Twilio/FCM senders.
	// Even with the no-op sender wired, the DLQ infra + River retry
	// machinery is exercised at startup.
	notifStore := store.NewNotificationsStore()
	notifService := service.NewNotificationDeliveryService(pool, service.NewLoggingSender(logger), notifStore, logger)

	// Worker-side procurement: the daily check runs server-managed
	// (no human caller), so audit rows would have a NULL user_sub.
	// We pass a no-op recorder for now — the cron runs every 24h,
	// already logs its row count, and per-row mutations are bulk
	// SQL UPDATEs rather than per-item RPC. Once the agent emits
	// per-item feed cards we'll wire the real audit there.
	procurementStore := store.NewProcurementStore()
	// Worker only runs RecomputeStatuses (the daily sweep) — no
	// human caller, no Maestro recommendation, no outbound review
	// requests. Pass nil for both the MaestroProcurementRecommender
	// and the VendorReviewEmitter; RecommendVendors and
	// RequestVendorReview guard with their respective "unavailable"
	// sentinels but are never invoked from this binary.
	procurementService := service.NewProcurementService(pool, procurementStore, nil, nil, service.NewNoopAuditRecorder())

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

	// River semantics: a cancelled Start(ctx) is a HARD stop — in-flight
	// jobs have their contexts cancelled, then the client waits for
	// them to return. We want a GRACEFUL shutdown: stop fetching new
	// jobs, wait for in-progress jobs to finish naturally, then exit.
	//
	// To get that, Start() runs against a never-cancelling background
	// context, and SIGTERM (the signal-cancelled `ctx` above) triggers
	// an explicit Stop() with a 30s budget. The 30s ceiling is the
	// k8s default terminationGracePeriodSeconds; aligning here avoids
	// kubelet sending SIGKILL while we're still draining.
	logger.Info("starting river worker, waiting for jobs")
	startCtx := context.Background()
	startErrCh := make(chan error, 1)
	go func() {
		if err := registry.Start(startCtx); err != nil {
			startErrCh <- err
		}
	}()

	// Block until either River errors out on its own or we get SIGTERM.
	select {
	case err := <-startErrCh:
		return fmt.Errorf("river worker error: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining in-flight jobs")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := registry.Client.Stop(stopCtx); err != nil {
		// Log but don't fail — we still want to return cleanly so
		// the deferred pool.Close runs and the process exits 0
		// rather than 1 (a 1 exit on a clean SIGTERM is a noisy
		// signal in alerting).
		logger.Warn("river graceful stop did not complete in budget",
			"error", err, "timeout_seconds", 30)
	}

	logger.Info("worker stopped gracefully")
	return nil
}
