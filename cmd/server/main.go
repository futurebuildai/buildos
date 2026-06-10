package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/api"
	"github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/auth"
	"github.com/futurebuildai/buildos/internal/config"
	"github.com/futurebuildai/buildos/internal/connectors"
	"github.com/futurebuildai/buildos/internal/cryptobox"
	"github.com/futurebuildai/buildos/internal/mailer"
	"github.com/futurebuildai/buildos/internal/obs"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
)

func main() {
	// Wrap the JSON handler with obs.CorrelatingHandler so every
	// context-scoped log line auto-stamps request_id (matching the
	// X-Request-ID we propagate to Brain). Logs without a ctx (e.g.
	// process boot) get no extra fields.
	jsonH := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(obs.NewCorrelatingHandler(jsonH))
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

	// Sentry first so subsequent init failures are captured. Empty
	// DSN is a no-op; flush is always safe to call.
	flushSentry, sentryOK := obs.InitSentry(obs.SentryConfig{
		DSN:              cfg.SentryDSN,
		Environment:      cfg.SentryEnvironment,
		Release:          cfg.SentryRelease,
		TracesSampleRate: cfg.SentryTracesRate,
	}, logger)
	defer flushSentry()

	// OpenTelemetry tracing. Empty endpoint = no-op tracer + W3C
	// propagator only (so inbound trace_ids still flow into logs
	// even without a collector configured).
	shutdownOTel, _ := obs.InitTracing(ctx, obs.TracingConfig{
		Endpoint:    cfg.OTelEndpoint,
		ServiceName: "buildos-server",
		Environment: cfg.SentryEnvironment, // share env tag with Sentry
		Release:     cfg.SentryRelease,
		SampleRate:  cfg.OTelSampleRate,
		Insecure:    cfg.OTelInsecure,
	}, logger)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownOTel(shutdownCtx)
	}()

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

	if cfg.DevAuthMode != "" {
		if middleware.IsProdBuild() {
			// Fail-fast: prod build with dev auth set is almost
			// certainly a deployment mistake. The dev-auth header
			// path is compiled out, so every request would 401 —
			// better to refuse to start than serve traffic that's
			// uniformly broken.
			return fmt.Errorf("DEV_AUTH_MODE=%q set in a prod-tagged build; rebuild without -tags=prod or unset DEV_AUTH_MODE", cfg.DevAuthMode)
		}
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

	// Process-level metrics. One Prometheus registry per process.
	// Metrics are wired into the AI client (per-attempt counters +
	// duration), the HTTP middleware stack (request count + duration
	// by route), and exposed via GET /metrics.
	metrics := obs.NewMetrics()
	// DB connection-pool gauges (Phase 4b-iii). The closure keeps internal/obs
	// free of a pgxpool import; GaugeFunc reads pool.Stat() at scrape time.
	metrics.RegisterPoolGauges(func() (acquired, idle, total, maxConns int32) {
		s := pool.Stat()
		return s.AcquiredConns(), s.IdleConns(), s.TotalConns(), s.MaxConns()
	})

	// Stores + services. Audit service first so domain services can
	// receive it as a dependency.
	auditStore := store.NewAuditStore()
	auditService := service.NewAuditService(auditStore, logger)

	// ----------------------------------------------------------------
	// Encrypted BYOK vault (WS3). When VAULT_MASTER_KEY is configured
	// the vault feeds the native AI client (Anthropic key) and the
	// Resend mailer (Resend key), both resolving the per-org key at
	// call time. When unset everything soft-fails: no AI, no email,
	// no /integrations routes — the server still boots and serves the
	// core domain. This is the "missing key → soft-fail / 503" posture.
	// ----------------------------------------------------------------
	var (
		vaultService  *service.VaultService
		aiClient      *ai.Client
		resendMailer  mailer.Mailer
		aiRecommender service.ProcurementRecommender
		aiBriefer     service.DailyBriefer
		aiAdjuster    service.ScheduleAdjuster
	)
	if cfg.VaultMasterKey != "" {
		masterKey, err := cryptobox.ParseMasterKey(cfg.VaultMasterKey)
		if err != nil {
			return fmt.Errorf("parsing vault master key: %w", err)
		}
		cipher, err := cryptobox.NewCipher(masterKey, cfg.VaultKeyVersion)
		if err != nil {
			return fmt.Errorf("building vault cipher: %w", err)
		}
		credStore := store.NewIntegrationCredentialStore()
		vaultService = service.NewVaultService(pool, credStore, cipher, auditService, logger, nil)

		aiClient, err = ai.NewClient(ai.Config{KeyResolver: vaultService, Metrics: metrics})
		if err != nil {
			return fmt.Errorf("building ai client: %w", err)
		}
		aiRecommender = aiClient
		aiBriefer = aiClient
		aiAdjuster = aiClient

		resendMailer = mailer.NewResendMailer(vaultService, cfg.MailFrom, cfg.MailFromName, mailer.WithLogger(logger))
		logger.Info("vault enabled", "key_version", cfg.VaultKeyVersion)
	} else {
		logger.Warn("VAULT_MASTER_KEY not set — AI, email, and integrations vault are disabled")
	}

	projectStore := store.NewProjectStore()
	projectService := service.NewProjectService(pool, projectStore, auditService)
	financialsStore := store.NewFinancialsStore()
	budgetService := service.NewBudgetService(pool, financialsStore, auditService)
	pipelineStore := store.NewPipelineStore()
	pipelineService := service.NewPipelineService(pool, pipelineStore, riverClient, auditService)
	scheduleStore := store.NewScheduleStore()
	scheduleService := service.NewScheduleService(pool, scheduleStore, riverClient, auditService)
	feedCardsStore := store.NewFeedCardsStore()
	feedService := service.NewFeedService(pool, feedCardsStore, logger, auditService)

	procurementStore := store.NewProcurementStore()
	// recommender soft-fails to ErrAIUnavailable when the vault (hence
	// AI client) is unconfigured; the vendor-review feed-card path
	// works regardless via feedCardsStore.
	procurementService := service.NewProcurementService(pool, procurementStore, aiRecommender, feedCardsStore, auditService)
	fleetStore := store.NewFleetStore()
	fleetService := service.NewFleetService(pool, fleetStore, auditService)
	hrStore := store.NewHRStore()
	hrService := service.NewHRService(pool, hrStore)
	fieldStore := store.NewFieldStore()
	fieldService := service.NewFieldService(pool, fieldStore, feedCardsStore, auditService)
	agentsService := service.NewAgentsService(pool, fieldStore, feedCardsStore, scheduleStore, scheduleService, aiBriefer, aiAdjuster, auditService)

	// Agent config registry (Phase 3a). The DB-backed per-org enable/tune
	// surface for the agentic capabilities. One service, two faces: the admin
	// CRUD face wires into the router (/api/v1/admin/agents), and the
	// agentic.ConfigResolver face injects into the assistant (Experience gate).
	// Always constructed (needs only the pool + store), so it is never nil.
	agentConfigService := service.NewAgentConfigService(pool, store.NewAgentConfigStore(), auditService, logger)

	// Connector registry (Phase 3b). DB-backed per-org enable/config for the
	// integration connectors (default-OFF). Two faces: the admin CRUD face wires
	// into the router (/api/v1/admin/connectors), and the ToolsFor face injects
	// into the assistant so enabled connector tools mount into chat. The vault
	// supplies per-org MCP credentials (Phase 3b-ii); pass a nil SecretResolver
	// (typed-nil-safe) when the vault is unconfigured so MCP calls run
	// unauthenticated rather than dereferencing a nil VaultService.
	var connectorSecret connectors.SecretResolver
	if vaultService != nil {
		connectorSecret = vaultService
	}
	connectorService := service.NewConnectorService(pool, store.NewConnectorConfigStore(), store.NewConnectorToolsStore(), connectorSecret, auditService, logger)

	// Conversational ERP assistant (Phase 2c). Typed-nil-safe: a nil
	// aiClient (vault/AI unconfigured) leaves the service's AI seam unset
	// so Converse soft-fails with agentic.ErrAssistantUnavailable (503)
	// rather than panicking. The read services are passed by concrete type;
	// pipelineService is threaded now so the Phase-3 pipeline tool is purely
	// additive. agentConfigService gates the Experience capability per org
	// (Phase 3a). Server-only — the worker binary never constructs this.
	assistantService := service.NewAssistantService(
		aiClient, pool,
		scheduleService, budgetService, procurementService,
		projectService, feedService, pipelineService,
		agentConfigService,
		connectorService,
		auditService, logger,
	)

	// Invoice ingestion (Phase 2a). NewIngestionService is typed-nil-safe:
	// a nil aiClient (vault/AI unconfigured) leaves the internal extractor
	// seam nil so the pipeline soft-fails with ai.ErrUnconfigured (503)
	// rather than panicking. Reuses budgetService.createInvoiceTx as the
	// single money-validation chokepoint.
	ingestionService := service.NewIngestionService(pool, aiClient, budgetService, store.NewInvoiceIngestionStore(), feedCardsStore, auditService)

	// Onboarding wizard (MB-7). The same *service.SetupService
	// satisfies both api.SetupServicer (wizard handlers) and
	// middleware.OnboardingChecker (SetupGate). Wiring it here flips
	// the gate on automatically.
	setupStore := store.NewSetupStore()
	setupService := service.NewSetupService(pool, setupStore, auditService, nil)

	// Materialize a one-shot owner-claim bootstrap row from
	// BUILDOS_BOOTSTRAP_TOKEN. Idempotent — re-boots with the same
	// env value are a silent no-op. Empty env is also a no-op.
	// Operators who skip this can still bring up the fork by issuing
	// a token out-of-band (see cmd/buildos-fork-init); this is the
	// happy path for the canonical fork-onboarding runbook.
	if seeded, err := setupService.SeedBootstrapTokenIfNeeded(ctx, cfg.BootstrapToken, uuid.Nil, 0); err != nil {
		// A bad env value is loud at boot, not at first request.
		return fmt.Errorf("seed bootstrap token: %w", err)
	} else if seeded {
		logger.Info("bootstrap token seeded for first-run wizard claim")
	}

	// ----------------------------------------------------------------
	// Native auth (WS1). BuildOS mints + validates its own RS256 JWTs
	// against a per-fork keypair. The TokenIssuer signs; the Verifier
	// powers the Auth middleware. The AuthService owns the
	// claim/login/refresh/logout/password-reset surface. The keypair
	// is REQUIRED in production; the dev-header rig (DEV_AUTH_MODE=
	// header) injects claims directly and needs no keys.
	// ----------------------------------------------------------------
	var (
		verifier    *auth.Verifier
		authService api.AuthServicer
	)
	if cfg.JWTPrivateKeyPEM != "" || cfg.JWTPublicKeyPEM != "" {
		priv, err := auth.ParseRSAPrivateKeyPEM([]byte(cfg.JWTPrivateKeyPEM))
		if err != nil {
			return fmt.Errorf("parsing JWT private key: %w", err)
		}
		pub, err := auth.ParseRSAPublicKeyPEM([]byte(cfg.JWTPublicKeyPEM))
		if err != nil {
			return fmt.Errorf("parsing JWT public key: %w", err)
		}
		issuer, err := auth.NewTokenIssuer(priv, cfg.JWTKeyID, cfg.JWTIssuer, cfg.JWTAudience)
		if err != nil {
			return fmt.Errorf("building token issuer: %w", err)
		}
		verifier, err = auth.NewVerifier(pub, cfg.JWTIssuer, cfg.JWTAudience)
		if err != nil {
			return fmt.Errorf("building token verifier: %w", err)
		}
		userStore := store.NewUserStore()
		as, err := service.NewAuthService(service.AuthServiceConfig{
			Pool:       pool,
			Users:      userStore,
			Setup:      setupStore,
			Issuer:     issuer,
			Mailer:     resendMailer, // nil → AuthService falls back to no-op mailer
			Audit:      auditService,
			Logger:     logger,
			RefreshTTL: cfg.AuthRefreshTTL,
			ResetTTL:   cfg.AuthResetTTL,
			AppBaseURL: cfg.AppBaseURL,
		})
		if err != nil {
			return fmt.Errorf("building auth service: %w", err)
		}
		authService = as
		logger.Info("native auth enabled", "issuer", cfg.JWTIssuer, "audience", cfg.JWTAudience)
	} else if cfg.DevAuthMode != "header" {
		return fmt.Errorf("JWT_PRIVATE_KEY_PEM and JWT_PUBLIC_KEY_PEM are required unless DEV_AUTH_MODE=header")
	}

	// Integrations vault servicer — interface-typed so a nil vault
	// stays a nil interface (router guards on != nil).
	var integrationsSvc api.IntegrationsServicer
	if vaultService != nil {
		integrationsSvc = vaultService
	}

	// Same-origin SPA serving (Phase 0a). Validate eagerly: a set-but-
	// broken WEB_DIST_DIR is a deployment mistake (the production image
	// bakes the console at a fixed path), and failing at boot beats a
	// blank page at first login. Empty = disabled — dev rigs run Vite
	// with an /api proxy instead.
	if cfg.WebDistDir != "" {
		indexPath := filepath.Join(cfg.WebDistDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			return fmt.Errorf("WEB_DIST_DIR=%q set but %s is unreadable (%w); unset WEB_DIST_DIR or fix the path", cfg.WebDistDir, indexPath, err)
		}
		logger.Info("same-origin SPA serving enabled", "web_dist_dir", cfg.WebDistDir)
	} else {
		logger.Info("same-origin SPA serving disabled (WEB_DIST_DIR unset)")
	}

	// Surface the X-Forwarded-For trust posture once at boot: with no trusted
	// proxies, the per-IP rate limiter keys on the direct TCP peer — correct for
	// a directly-exposed fork, but behind a load balancer it collapses ALL
	// clients into one shared bucket. Set TRUSTED_PROXY_CIDRS to the LB subnet
	// if proxied.
	if len(cfg.TrustedProxyCIDRs) == 0 {
		logger.Warn("TRUSTED_PROXY_CIDRS is empty: X-Forwarded-For is ignored; the rate limiter keys on the direct peer. If behind a load balancer, set it to the LB subnet or all clients share one rate-limit bucket.")
	}

	// Build the router with all route groups
	router := api.NewRouter(api.RouterConfig{
		Pool:                pool,
		Verifier:            verifier,
		DevAuthMode:         cfg.DevAuthMode,
		Logger:              logger,
		AuthService:         authService,
		ProjectService:      projectService,
		BudgetService:       budgetService,
		PipelineService:     pipelineService,
		ScheduleService:     scheduleService,
		FeedService:         feedService,
		ProcurementService:  procurementService,
		FleetService:        fleetService,
		HRService:           hrService,
		FieldService:        fieldService,
		IntegrationsService: integrationsSvc,
		Metrics:             metrics,
		AgentsService:       agentsService,
		AgentConfigService:  agentConfigService,
		ConnectorService:    connectorService,
		Assistant:           assistantService,
		IngestionService:    ingestionService,
		SetupService:        setupService,
		SentryEnabled:       sentryOK,
		RateLimiter:         middleware.NewIPRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst),
		TrustedProxyCIDRs:   cfg.TrustedProxyCIDRs,
		WebDistDir:          cfg.WebDistDir,
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
