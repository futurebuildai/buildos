package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/config"
	"github.com/futurebuildai/buildos/internal/cryptobox"
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

	// Real audit service for worker mutations that warrant a trail.
	// Used by the delay-cascade workspace below so every cascade feed
	// card lands an audit row. (Budget rollups / procurement sweeps keep
	// their no-op recorders — see the comments at their construction.)
	auditService := service.NewAuditService(store.NewAuditStore(), logger)

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

	// ----------------------------------------------------------------
	// Delay-cascade orchestrator wiring. When VAULT_MASTER_KEY is set we
	// build the encrypted BYOK vault -> native AI client (per-org
	// Anthropic key resolution), then the deterministic CascadeWorkspace
	// over the stores. The workspace has no per-org state and is built
	// once; the reasoner bakes in the org id (the AI key resolves
	// per-org), so the orchestrator must be built per job invocation
	// from args.OrgID — that happens in cascadeOrchestratorFactory.Run.
	//
	// When the vault is unconfigured, aiClient stays nil:
	// NewCascadeReasoner(nil, …).PlanCascade soft-fails with
	// ErrReasonerUnavailable, which the orchestrator swallows. The worker
	// still boots and delay_cascade is a logged no-op.
	// ----------------------------------------------------------------
	var aiClient *ai.Client
	if cfg.VaultMasterKey != "" {
		masterKey, err := cryptobox.ParseMasterKey(cfg.VaultMasterKey)
		if err != nil {
			return fmt.Errorf("parsing vault master key: %w", err)
		}
		cipher, err := cryptobox.NewCipher(masterKey, cfg.VaultKeyVersion)
		if err != nil {
			return fmt.Errorf("building vault cipher: %w", err)
		}
		vaultService := service.NewVaultService(pool, store.NewIntegrationCredentialStore(), cipher, auditService, logger, nil)
		aiClient, err = ai.NewClient(ai.Config{KeyResolver: vaultService})
		if err != nil {
			return fmt.Errorf("building ai client: %w", err)
		}
		logger.Info("vault enabled for worker", "key_version", cfg.VaultKeyVersion)
	} else {
		logger.Warn("VAULT_MASTER_KEY not set — delay_cascade AI reasoning disabled (logged no-op)")
	}

	cascadeWorkspace := service.NewCascadeWorkspace(
		pool,
		store.NewScheduleStore(),
		procurementStore,
		financialsStore,
		store.NewProjectStore(),
		store.NewFeedCardsStore(),
		auditService,
	)
	cascadeOrchestrator := &cascadeOrchestratorFactory{
		aiClient:  aiClient,
		workspace: cascadeWorkspace,
		logger:    logger,
	}

	registry, err := worker.NewRegistry(pool, logger, worker.Dependencies{
		BudgetRunner:          budgetService,
		NotificationDeliverer: notifService,
		ProcurementChecker:    procurementService,
		CascadeOrchestrator:   cascadeOrchestrator,
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

// cascadeOrchestratorFactory satisfies worker.CascadeOrchestrator. The AI key
// resolves per-org (via ai.ContextWithOrgID inside the reasoner), and the
// reasoner bakes the org id in at construction — so a fresh per-org reasoner +
// orchestrator must be built on every job invocation from in.OrgID. The
// workspace carries no per-org state and is reused across invocations.
type cascadeOrchestratorFactory struct {
	aiClient  *ai.Client // may be nil when the vault is unconfigured
	workspace *service.CascadeWorkspace
	logger    *slog.Logger
}

// RunDelayCascade builds the per-org orchestrator and runs the flow. When
// aiClient is nil we pass an untyped-nil reasoner client (not a typed nil
// *ai.Client) so the reasoner's nil check fires and PlanCascade soft-fails with
// agentic.ErrReasonerUnavailable instead of dereferencing a nil pointer.
func (f *cascadeOrchestratorFactory) RunDelayCascade(ctx context.Context, in agentic.DelayCascadeInput) (agentic.CascadeResult, error) {
	reasoner := f.newReasoner(in.OrgID)
	orch := agentic.NewOrchestrator(reasoner, f.workspace, f.logger)
	return orch.RunDelayCascade(ctx, in)
}

// newReasoner constructs the per-org reasoner. NewCascadeReasoner takes the
// concrete *ai.Client and handles a nil client internally (PlanCascade then
// soft-fails with ErrReasonerUnavailable), so a key-less / vault-unconfigured
// worker passes its nil aiClient straight through — no typed-nil interface
// hazard to guard at the call site.
func (f *cascadeOrchestratorFactory) newReasoner(orgID uuid.UUID) agentic.Reasoner {
	return service.NewCascadeReasoner(f.aiClient, orgID)
}
