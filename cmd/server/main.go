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
	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/config"
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

	// Brain client — typed wrapper for The Brain's REST API (Maestro
	// AI, billing, future Hub/MCP). Each method takes a ctx that carries
	// the caller's Bearer token (auth middleware stashes it). The Ping
	// method is unauth and powers the /ready readiness probe; service-
	// layer agents land in Phase B.
	brainClient, err := brain.NewClient(brain.Config{
		BaseURL: cfg.BrainIssuerURL, // Brain's API + OIDC live on the same host
		Logger:  logger,
	})
	if err != nil {
		return fmt.Errorf("creating brain client: %w", err)
	}
	logger.Info("brain client initialized", "base_url", cfg.BrainIssuerURL)

	// Stores + services
	financialsStore := store.NewFinancialsStore()
	budgetService := service.NewBudgetService(pool, financialsStore)
	pipelineStore := store.NewPipelineStore()
	pipelineService := service.NewPipelineService(pool, pipelineStore, riverClient)
	scheduleStore := store.NewScheduleStore()
	scheduleService := service.NewScheduleService(pool, scheduleStore, riverClient)
	a2aStore := store.NewA2AStore()
	feedCardsStore := store.NewFeedCardsStore()
	feedService := service.NewFeedService(pool, feedCardsStore, logger, riverClient)
	procurementStore := store.NewProcurementStore()
	procurementService := service.NewProcurementService(pool, procurementStore)
	fleetStore := store.NewFleetStore()
	fleetService := service.NewFleetService(pool, fleetStore)
	hrStore := store.NewHRStore()
	hrService := service.NewHRService(pool, hrStore)
	fieldStore := store.NewFieldStore()
	fieldService := service.NewFieldService(pool, fieldStore, feedCardsStore)
	agentsService := service.NewAgentsService(pool, fieldStore, feedCardsStore, brainClient.Maestro)
	a2aService := service.NewA2AService(pool, a2aStore, feedCardsStore, pipelineStore, cfg.DefaultOrgID)
	a2aVerifier := api.NewJWKSVerifier(jwks) // verifies Brain's JWS using the same JWKS used for JWT validation

	// Build the router with all route groups
	router := api.NewRouter(api.RouterConfig{
		Pool:               pool,
		JWKS:               jwks,
		IssuerURL:          cfg.BrainIssuerURL,
		DevAuthMode:        cfg.DevAuthMode,
		Logger:             logger,
		BudgetService:      budgetService,
		PipelineService:    pipelineService,
		ScheduleService:    scheduleService,
		FeedService:        feedService,
		ProcurementService: procurementService,
		FleetService:       fleetService,
		HRService:          hrService,
		FieldService:       fieldService,
		A2AService:         a2aService,
		A2AVerifier:        a2aVerifier,
		BrainPinger:        brainClient,
		JWKSReporter:       jwks,
		BillingClient:      brainClient.Billing,
		AgentsService:      agentsService,
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
