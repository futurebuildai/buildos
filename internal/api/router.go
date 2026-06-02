package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/auth"
	"github.com/futurebuildai/buildos/internal/obs"
)

// MetricsRecorder is the consumer-side surface the router needs from
// internal/obs.Metrics. Two responsibilities: HTTP middleware (request
// count + duration) and a /metrics handler. Defined here so the
// router stays free of a Prometheus import — and so tests can pass
// nil to skip both.
type MetricsRecorder interface {
	HTTPMiddleware(next http.Handler) http.Handler
	Handler() http.Handler
}

// RouterConfig holds all dependencies needed to build the API router.
type RouterConfig struct {
	Pool                *pgxpool.Pool
	Verifier            *auth.Verifier
	DevAuthMode         string
	Logger              *slog.Logger
	AuthService         AuthServicer
	ProjectService      ProjectServicer
	BudgetService       BudgetServicer
	PipelineService     PipelineServicer
	ScheduleService     ScheduleServicer
	FeedService         FeedServicer
	ProcurementService  ProcurementServicer
	FleetService        FleetServicer
	HRService           HRServicer
	FieldService        FieldServicer
	SetupService        SetupServicer        // optional — when nil, /setup/* routes don't mount AND SetupGate is skipped
	IntegrationsService IntegrationsServicer // optional — when nil, /integrations/* routes don't mount (vault disabled)
	AgentsService       AgentsServicer       // optional — when nil, /agents/* routes don't mount
	Metrics             MetricsRecorder      // optional — when nil, /metrics doesn't mount and HTTP middleware is skipped
	SentryEnabled       bool                 // when true, the Sentry HTTP middleware is mounted to capture panics
	RateLimiter         *mw.IPRateLimiter    // optional — when nil, no rate limiting is applied
}

// NewRouter creates the Chi router with all route groups and middleware.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Global middleware. Order matters:
	//   - RequestID first so chi assigns one before any logging.
	//   - RealIP next so downstream sees the right remote.
	//   - Recoverer wraps before metrics: panics still get counted.
	//   - Metrics middleware before Logger so a panicked handler's
	//     5xx still ticks the counter even if Logger swallows it.
	//   - Logger emits the request line, with request_id already set.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	// OTel HTTP middleware sits HERE — after RequestID/RealIP (so
	// the span tags include both) but before everything else (so
	// every downstream middleware's behavior is captured under the
	// span). The wrapper is a no-op when no global tracer is
	// configured; dev rigs without OTel pay no cost.
	r.Use(func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "buildos.http",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				// Default span name is the HTTP method which doesn't
				// distinguish routes. Postpone naming to chi's
				// post-routing hook so we get the matched pattern
				// (e.g. "GET /api/v1/projects/{projectID}") rather
				// than the raw URL.
				return r.Method + " " + r.URL.Path
			}),
		)
	})
	// Rate limiter mounts EARLY in the stack so rejected requests
	// don't pay the cost of downstream middleware (auth, metrics,
	// audit). Mounted after RealIP so the per-IP bucket sees the
	// real client IP, not the LB's.
	if cfg.RateLimiter != nil {
		r.Use(cfg.RateLimiter.Middleware)
	}
	// Sentry middleware sits BEFORE chi.Recoverer so it sees the
	// panic before Recoverer's defer swallows it. The SDK's Repanic
	// option re-throws the panic on its way back up, then Recoverer
	// catches it and writes the 500. The order — Sentry inside,
	// Recoverer outside — is what every chi+Sentry recipe uses.
	if cfg.SentryEnabled {
		r.Use(obs.SentryHTTPMiddleware())
	}
	r.Use(chimw.Recoverer)
	if cfg.Metrics != nil {
		r.Use(cfg.Metrics.HTTPMiddleware)
	}
	// Default body-size cap. Mounted before Logger so the access log
	// already shows a 413 status when a handler rejects oversized
	// payloads. Routes that need a looser cap (future file uploads)
	// override per-group below.
	r.Use(mw.MaxBodySize(mw.DefaultMaxBodyBytes))
	r.Use(chimw.Logger)

	// Probes (no auth):
	//
	//   /health — liveness. The process is up. Always returns 200 once
	//             the server has started; used by orchestrators to
	//             decide whether to restart the container.
	//
	//   /ready  — readiness. The process can serve traffic right now.
	//             Pings the database. Used by load balancers / k8s
	//             readiness gates.
	r.Get("/health", livenessHandler())
	r.Get("/ready", readinessHandler(cfg.Pool, cfg.Logger))

	// Prometheus scrape endpoint. No auth — Prometheus convention.
	// Lock down via network policy / IP allowlist at the LB if your
	// scrape source isn't private.
	if cfg.Metrics != nil {
		r.Method(http.MethodGet, "/metrics", cfg.Metrics.Handler())
	}

	// Instantiate handlers
	projects := NewProjectHandler(cfg.ProjectService)
	schedule := NewScheduleHandler(cfg.ScheduleService)
	financials := NewFinancialsHandler(cfg.BudgetService)
	pipeline := NewPipelineHandler(cfg.PipelineService)
	procurement := NewProcurementHandler(cfg.ProcurementService)
	feed := NewFeedHandler(cfg.FeedService)
	field := NewFieldHandler(cfg.FieldService)
	fleet := NewFleetHandler(cfg.FleetService)
	hr := NewHRHandler(cfg.HRService)
	authHandler := NewAuthHandler(cfg.AuthService)
	var agents *AgentsHandler
	if cfg.AgentsService != nil {
		agents = NewAgentsHandler(cfg.AgentsService)
	}
	var setup *SetupHandler
	if cfg.SetupService != nil {
		setup = NewSetupHandler(cfg.SetupService)
	}
	var integrations *IntegrationsHandler
	if cfg.IntegrationsService != nil {
		integrations = NewIntegrationsHandler(cfg.IntegrationsService)
	}

	// Auth middleware
	authMiddleware := mw.Auth(cfg.Verifier, cfg.DevAuthMode, cfg.Logger)

	// ============================================================
	// Native auth — UNAUTHENTICATED. These routes mint the
	// credentials the rest of the API requires (claim/login/refresh/
	// logout/password-reset), so they mount OUTSIDE the auth group
	// and are exempt from the SetupGate (the first-owner claim must
	// work on a fresh fork before onboarding completes).
	// ============================================================
	MountAuthRoutes(r, authHandler)

	// ============================================================
	// All authenticated routes
	// ============================================================
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

		// SetupGate guards every authenticated route. It exempts
		// /api/v1/setup, /health, /ready, /metrics (per
		// DefaultSetupGateExemptPrefixes) and 403s every other path
		// until onboarding_complete=true. When SetupService is nil
		// (tests, dev rigs without the wizard) the gate is skipped
		// entirely — operational routes remain reachable.
		//
		// It must be installed via r.Use BEFORE any route is mounted
		// on this group: chi panics if Use() is called after a route
		// is registered on a mux. The wizard routes mounted just below
		// are still reachable while onboarding is incomplete because
		// the gate exempts the /api/v1/setup prefix by PATH, not by
		// mount order.
		if cfg.SetupService != nil {
			r.Use(mw.SetupGate(mw.SetupGateConfig{Checker: cfg.SetupService}))
		}

		// --------------------------------------------------------
		// 0. Onboarding wizard — gated by Auth (so we know the
		//    caller's org_id) but exempt from SetupGate by prefix
		//    (this is the surface operators use to finish setup from
		//    a fresh fork). Mounts only when SetupService is wired so
		//    tests / dev rigs without it stay green.
		// --------------------------------------------------------
		if setup != nil {
			MountSetupRoutes(r, setup)
		}

		// --------------------------------------------------------
		// 0.5 Integrations vault — admin-gated BYOK credential store
		//     (Anthropic, Resend, named vendors). RBAC is enforced
		//     inside MountIntegrationRoutes. Mounts only when the
		//     vault is wired (VAULT_MASTER_KEY configured).
		// --------------------------------------------------------
		if integrations != nil {
			MountIntegrationRoutes(r, integrations)
			// Capabilities (GET /api/v1/capabilities) — auth-only, NOT
			// admin-gated: every role's UI gates AI/email affordances on
			// these flags. Derived from active-credential presence in the
			// same vault. When the vault is nil the frontend keeps its
			// assume-on fallback (unchanged behavior).
			MountCapabilitiesRoutes(r, integrations)
		}

		// --------------------------------------------------------
		// 1. Projects — owner, admin: full; superintendent, field_worker: read-only
		// --------------------------------------------------------
		r.Route("/api/v1/projects", func(r chi.Router) {
			r.Get("/", projects.List)
			r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/", projects.Create)

			r.Route("/{projectID}", func(r chi.Router) {
				r.Get("/", projects.Get)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Put("/", projects.Update)

				// Schedule sub-routes
				r.Route("/schedule", func(r chi.Router) {
					r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
						Post("/recalculate", schedule.Recalculate)
					r.Get("/gantt", schedule.Gantt)

					// Maestro-driven duration adjustments. Same role
					// gate as /recalculate (CPM-affecting); plus
					// pro-tier plan gate (consumes metered AI tokens).
					// Mounts only when AgentsService is wired —
					// matches the conditional under /api/v1/agents.
					if agents != nil {
						r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
							With(mw.RequirePlanTier(mw.PlanTierPro)).
							Post("/recommend-adjustments", agents.RecommendScheduleAdjustments)
					}
				})

				// Tasks sub-routes
				r.Get("/tasks", schedule.ListTasks)
				r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
					Put("/tasks/{taskID}", schedule.UpdateTask)

				// Budgets (financial — owner, admin only)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).
					Get("/budgets", financials.ListBudgets)

				// Invoices (financial — owner, admin only)
				r.Route("/invoices", func(r chi.Router) {
					r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
					r.Post("/", financials.CreateInvoice)
					r.Put("/{invoiceID}", financials.UpdateInvoice)
				})

				// Procurement — owner, admin: full; superintendent: read + request review
				r.Route("/procurement", func(r chi.Router) {
					r.Get("/", procurement.List)
					r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/", procurement.Create)
					r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Put("/{itemID}", procurement.Update)
					// Vendor-review request — creates a local
					// vendor_review_requested feed card for an operator to
					// action. Superintendent gate (operator-driven; matches
					// the field-level supervision boundary). No plan-tier
					// gate — it doesn't itself consume AI tokens.
					r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
						Post("/{itemID}/request-review", procurement.RequestVendorReview)
				})
			})
		})

		// --------------------------------------------------------
		// 2. Org-scoped routes — /api/v1/org/{orgID}/*
		// --------------------------------------------------------
		r.Route("/api/v1/org/{orgID}", func(r chi.Router) {
			// Financials — owner, admin: full; superintendent: read-only
			r.Route("/financials", func(r chi.Router) {
				r.Use(mw.RequireMinRole(mw.RoleSuperintendent))
				r.Get("/summary", financials.Summary)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Get("/ar-aging", financials.ARAging)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Get("/projects", financials.ProjectFinancials)
			})

			// Pipeline — owner, admin: full; superintendent: read-only
			r.Route("/pipeline", func(r chi.Router) {
				r.Use(mw.RequireMinRole(mw.RoleSuperintendent))

				r.Route("/prospects", func(r chi.Router) {
					r.Get("/", pipeline.ListProspects)
					r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/", pipeline.CreateProspect)

					r.Route("/{prospectID}", func(r chi.Router) {
						r.Get("/", pipeline.GetProspect)
						r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Put("/", pipeline.UpdateProspect)
						r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/advance", pipeline.AdvanceProspect)
						r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/lose", pipeline.LoseProspect)
						r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/estimates", pipeline.CreateEstimate)
						r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/permits", pipeline.CreatePermit)
					})
				})

				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).
					Put("/estimates/{estimateID}", pipeline.UpdateEstimate)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).
					Put("/permits/{permitID}", pipeline.UpdatePermit)

				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).
					Get("/analytics", pipeline.Analytics)
			})

			// Fleet — owner, admin: full; superintendent: allocate + read
			r.Route("/fleet", func(r chi.Router) {
				r.Use(mw.RequireMinRole(mw.RoleSuperintendent))
				r.Get("/", fleet.ListAssets)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/", fleet.CreateAsset)
				r.Post("/{assetID}/allocate", fleet.AllocateAsset)
			})

			// HR — owner, admin only
			r.Route("/employees", func(r chi.Router) {
				r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
				r.Get("/", hr.ListEmployees)
				r.Get("/{employeeID}/certifications", hr.ListCertifications)
			})
		})

		// --------------------------------------------------------
		// 3. Feed — all authenticated roles
		// --------------------------------------------------------
		r.Route("/api/v1/feed", func(r chi.Router) {
			r.Get("/", feed.List)
			r.Post("/{cardID}/action", feed.Action)
			r.Post("/{cardID}/dismiss", feed.Dismiss)
		})

		// --------------------------------------------------------
		// 3.6 AI Agents — native-AI-backed endpoints. Pro tier and up.
		//      Each agent is a separate route so we can gate them
		//      individually if pricing tiers diverge.
		// --------------------------------------------------------
		if agents != nil {
			r.Route("/api/v1/agents", func(r chi.Router) {
				r.Use(mw.RequirePlanTier(mw.PlanTierPro))
				r.Post("/daily-briefing", agents.DailyBriefing)
			})
		}

		// --------------------------------------------------------
		// 4. Field Sync — all authenticated roles (primarily field_worker)
		// --------------------------------------------------------
		r.Route("/api/v1/field", func(r chi.Router) {
			r.Get("/sync", field.Sync)
			r.Post("/progress", field.ReportProgress)
			r.Post("/checkin", field.Checkin)
			r.Post("/daily-log", field.DailyLog)
		})
	})

	return r
}

// livenessHandler returns the liveness probe. Always 200 — if the
// HTTP listener can answer, the process is alive. We deliberately do
// NOT check the database here; an unreachable DB is a readiness
// problem, not a liveness one (restarting the process won't help).
func livenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"0.1.0"}`))
	}
}

// readinessHandler returns the readiness probe — the server can
// actually serve traffic. Checks the database pool under a 2s
// deadline so a slow DB doesn't block the load balancer's poll
// cadence. BuildOS is now self-contained (native auth + AI), so the
// only hard dependency is its own Postgres.
func readinessHandler(pool *pgxpool.Pool, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		dbStatus := "ok"
		var dbErr error
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unhealthy"
			dbErr = err
		}

		ready := dbStatus == "ok"
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			if logger != nil {
				logger.WarnContext(ctx, "readiness probe failed",
					"db_status", dbStatus, "db_error", dbErr)
			}
		} else {
			w.WriteHeader(http.StatusOK)
		}

		statusWord := "ok"
		if !ready {
			statusWord = "unhealthy"
		}
		// Hand-rolled JSON — a few fixed fields, no need for a struct +
		// encoder. Keeps this allocation-light on a hot probe path.
		body := `{"status":"` + statusWord +
			`","components":{"database":"` + dbStatus + `"}}`
		_, _ = w.Write([]byte(body))
	}
}
