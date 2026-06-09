package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

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

	// Process-level metrics for the worker (Phase 4b-ii). One Prometheus
	// registry per process — wired into the AI client (so worker-side AI calls
	// are counted), the River job-outcome subscription, and a small /metrics +
	// probe HTTP server below.
	metrics := obs.NewMetrics()

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
		aiClient, err = ai.NewClient(ai.Config{KeyResolver: vaultService, Metrics: metrics})
		if err != nil {
			return fmt.Errorf("building ai client: %w", err)
		}
		logger.Info("vault enabled for worker", "key_version", cfg.VaultKeyVersion)
	} else {
		logger.Warn("VAULT_MASTER_KEY not set — delay_cascade AI reasoning disabled (logged no-op)")
	}

	// Shared stores for the cascade + foresight workspaces (both read the same
	// schedule / project / feed tables). Hoisted so the foresight workspace below
	// reuses the exact instances rather than re-allocating.
	scheduleStore := store.NewScheduleStore()
	projectStore := store.NewProjectStore()
	feedStore := store.NewFeedCardsStore()

	// Agent config registry resolver (Phase 3a) — the per-org enable/tune
	// source the orchestrators + sweep consult. One shared instance (stateless;
	// reads agents_config), injected as the agentic.ConfigResolver into both the
	// cascade factory and the foresight sweep. Never nil.
	agentConfigResolver := service.NewAgentConfigService(pool, store.NewAgentConfigStore(), auditService, logger)

	cascadeWorkspace := service.NewCascadeWorkspace(
		pool,
		scheduleStore,
		procurementStore,
		financialsStore,
		projectStore,
		feedStore,
		auditService,
	)
	cascadeOrchestrator := &cascadeOrchestratorFactory{
		aiClient:  aiClient,
		workspace: cascadeWorkspace,
		config:    agentConfigResolver,
		logger:    logger,
	}

	// ----------------------------------------------------------------
	// Foresight-sweep wiring. Same shape as the cascade orchestrator: the
	// deterministic ForesightWorkspace carries no per-org state and is built
	// once over the (reused) stores; the per-org reasoner bakes in the org id
	// (the AI key resolves per org) so a fresh per-org ForesightOrchestrator is
	// built on every project from p.OrgID — that happens in the
	// foresightOrchestratorFactory closure. A nil aiClient (vault unconfigured)
	// makes each per-project JudgeRisks soft-fail with ErrReasonerUnavailable,
	// which the orchestrator swallows: the sweep runs deterministically and
	// emits no cards. ForesightThresholds{} takes the documented defaults
	// (schedule float <=2 days, burn >=80%).
	// ----------------------------------------------------------------
	foresightWorkspace := service.NewForesightWorkspace(
		pool,
		scheduleStore,
		procurementStore,
		financialsStore,
		projectStore,
		feedStore,
		auditService,
	)
	foresightFactory := &foresightOrchestratorFactory{
		aiClient:  aiClient,
		workspace: foresightWorkspace,
		logger:    logger,
	}
	// procurementService is the ProcurementRecomputer (R3: RecomputeStatuses runs
	// first so the procurement dimension reads FRESH statuses). It is the same
	// instance wired as Dependencies.ProcurementChecker.
	foresightSweep := service.NewForesightSweepService(
		pool,
		projectStore,
		procurementService,
		agentConfigResolver,
		foresightFactory.RunForesight,
		logger,
	)

	registry, err := worker.NewRegistry(pool, logger, worker.Dependencies{
		BudgetRunner:          budgetService,
		NotificationDeliverer: notifService,
		ProcurementChecker:    procurementService,
		CascadeOrchestrator:   cascadeOrchestrator,
		ForesightRunner:       foresightSweep,
	})
	if err != nil {
		return fmt.Errorf("creating worker registry: %w", err)
	}

	// Observability (Phase 4b-ii): record every job's terminal outcome into
	// buildos_river_job_runs_total via a River event subscription, and serve
	// /metrics + /health + /ready on PORT. Subscribe before Start so no early
	// completion is missed; the stop func is deferred so it runs on any return.
	stopJobMetrics := registry.ObserveJobMetrics(metrics, logger)
	defer stopJobMetrics()

	httpSrv := newWorkerHTTPServer(":"+cfg.Port, metrics, pool)
	httpErrCh := make(chan error, 1)
	go func() {
		// ListenAndServe blocks until Shutdown (→ ErrServerClosed, ignored) or a
		// bind failure — e.g. PORT already in use when the worker is co-located
		// with the server on one host. Surface the bind error (below, via the
		// select) so the worker fails LOUDLY rather than silently running with no
		// observability surface. In production the two roles are separate pods,
		// each with its own PORT, so this never fires.
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErrCh <- err
		}
	}()
	logger.Info("worker observability up", "addr", ":"+cfg.Port, "paths", "/metrics /health /ready")

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

	// Block until River errors out on its own, the observability server fails to
	// bind (fail-fast — don't run blind), or we get SIGTERM.
	select {
	case err := <-startErrCh:
		return fmt.Errorf("river worker error: %w", err)
	case err := <-httpErrCh:
		return fmt.Errorf("worker metrics/probe server: %w", err)
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

	// Stop serving /metrics + probes after the jobs have drained.
	httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer httpCancel()
	if err := httpSrv.Shutdown(httpShutdownCtx); err != nil {
		logger.Warn("worker http server shutdown did not complete", "error", err)
	}

	logger.Info("worker stopped gracefully")
	return nil
}

// newWorkerHTTPServer builds the worker's observability HTTP server: Prometheus
// /metrics plus k8s probes — /health (liveness, no DB) and /ready (readiness,
// pings the pool). Mirrors the server's probe semantics (internal/api): a DB
// blip should fail readiness (pull from rotation) without killing liveness.
// Unauthenticated by convention; restrict /metrics at the network layer.
func newWorkerHTTPServer(addr string, metrics *obs.Metrics, pool *pgxpool.Pool) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	return &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}

// cascadeOrchestratorFactory satisfies worker.CascadeOrchestrator. The AI key
// resolves per-org (via ai.ContextWithOrgID inside the reasoner), and the
// reasoner bakes the org id in at construction — so a fresh per-org reasoner +
// orchestrator must be built on every job invocation from in.OrgID. The
// workspace carries no per-org state and is reused across invocations.
type cascadeOrchestratorFactory struct {
	aiClient  *ai.Client // may be nil when the vault is unconfigured
	workspace *service.CascadeWorkspace
	config    agentic.ConfigResolver // per-org delay_cascade enabled gate (Phase 3a)
	logger    *slog.Logger
}

// RunDelayCascade builds the per-org orchestrator and runs the flow. When
// aiClient is nil we pass an untyped-nil reasoner client (not a typed nil
// *ai.Client) so the reasoner's nil check fires and PlanCascade soft-fails with
// agentic.ErrReasonerUnavailable instead of dereferencing a nil pointer.
func (f *cascadeOrchestratorFactory) RunDelayCascade(ctx context.Context, in agentic.DelayCascadeInput) (agentic.CascadeResult, error) {
	reasoner := f.newReasoner(in.OrgID)
	orch := agentic.NewOrchestrator(reasoner, f.workspace, f.config, f.logger)
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

// foresightOrchestratorFactory is the per-org orchestrator builder for the
// foresight sweep. It mirrors cascadeOrchestratorFactory: the AI key resolves
// per-org (via ai.ContextWithOrgID inside the reasoner), and the reasoner bakes
// the org id in at construction — so a fresh per-org reasoner + orchestrator is
// built on every project the sweep visits. The workspace carries no per-org
// state and is reused across invocations. Its RunForesight method is passed to
// NewForesightSweepService as the per-org factory func.
type foresightOrchestratorFactory struct {
	aiClient  *ai.Client // may be nil when the vault is unconfigured
	workspace *service.ForesightWorkspace
	logger    *slog.Logger
}

// RunForesight builds the per-org reasoner + orchestrator. NewForesightReasoner
// takes the concrete *ai.Client and handles a nil client internally (JudgeRisks
// then soft-fails with ErrReasonerUnavailable), so a key-less / vault-
// unconfigured worker passes its nil aiClient straight through — no typed-nil
// interface hazard to guard at the call site.
func (f *foresightOrchestratorFactory) RunForesight(orgID uuid.UUID) *agentic.ForesightOrchestrator {
	reasoner := service.NewForesightReasoner(f.aiClient, orgID)
	return agentic.NewForesightOrchestrator(reasoner, f.workspace, f.logger)
}
