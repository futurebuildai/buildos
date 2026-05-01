package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
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

// BrainPinger is the consumer-side interface for the readiness probe's
// Brain check. Defined here to keep the router free of the
// internal/brain import — and to let tests substitute a fake.
type BrainPinger interface {
	Ping(ctx context.Context) error
}

// JWKSCacheReporter exposes the JWKS provider's cache state to the
// readiness probe. The probe treats keyCount == 0 (never fetched, or
// upstream returned empty) as unhealthy and age > 2*ttl as stale —
// the upstream has been unreachable long enough that cached keys may
// be past Brain's rotation horizon.
type JWKSCacheReporter interface {
	CacheStatus() (keyCount int, age time.Duration)
	CacheTTL() time.Duration
}

// RouterConfig holds all dependencies needed to build the API router.
type RouterConfig struct {
	Pool            *pgxpool.Pool
	JWKS            *mw.JWKSProvider
	IssuerURL       string
	DevAuthMode     string
	Logger          *slog.Logger
	BudgetService      BudgetServicer
	PipelineService    PipelineServicer
	ScheduleService    ScheduleServicer
	FeedService        FeedServicer
	ProcurementService ProcurementServicer
	FleetService       FleetServicer
	HRService          HRServicer
	FieldService       FieldServicer
	A2AService         A2AServicer
	A2AVerifier        JWSVerifier
	BrainPinger        BrainPinger       // optional — when nil, /ready skips the Brain check
	JWKSReporter       JWKSCacheReporter // optional — when nil, /ready skips the JWKS check
	BillingClient      BillingClient     // optional — when nil, /billing/* routes don't mount
	AgentsService      AgentsServicer    // optional — when nil, /agents/* routes don't mount
	Metrics            MetricsRecorder   // optional — when nil, /metrics doesn't mount and HTTP middleware is skipped
	SentryEnabled      bool              // when true, the Sentry HTTP middleware is mounted to capture panics
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
	// payloads. Routes that need a tighter cap (A2A inbound) or a
	// looser one (future file uploads) override per-group below.
	r.Use(mw.MaxBodySize(mw.DefaultMaxBodyBytes))
	r.Use(chimw.Logger)

	// Probes (no auth):
	//
	//   /health — liveness. The process is up. Always returns 200 once
	//             the server has started; used by orchestrators to
	//             decide whether to restart the container.
	//
	//   /ready  — readiness. The process can serve traffic right now.
	//             Pings the database and (optionally) Brain's /health.
	//             Used by load balancers / k8s readiness gates.
	r.Get("/health", livenessHandler())
	r.Get("/ready", readinessHandler(cfg.Pool, cfg.BrainPinger, cfg.JWKSReporter, cfg.Logger))

	// Prometheus scrape endpoint. No auth — Prometheus convention.
	// Lock down via network policy / IP allowlist at the LB if your
	// scrape source isn't private.
	if cfg.Metrics != nil {
		r.Method(http.MethodGet, "/metrics", cfg.Metrics.Handler())
	}

	// Instantiate handlers
	projects := &ProjectHandler{}
	schedule := NewScheduleHandler(cfg.ScheduleService)
	financials := NewFinancialsHandler(cfg.BudgetService)
	pipeline := NewPipelineHandler(cfg.PipelineService)
	procurement := NewProcurementHandler(cfg.ProcurementService)
	feed := NewFeedHandler(cfg.FeedService)
	field := NewFieldHandler(cfg.FieldService)
	fleet := NewFleetHandler(cfg.FleetService)
	hr := NewHRHandler(cfg.HRService)
	a2a := NewA2AHandler(cfg.A2AVerifier, cfg.A2AService, cfg.Logger)
	var billing *BillingHandler
	if cfg.BillingClient != nil {
		billing = NewBillingHandler(cfg.BillingClient)
	}
	var agents *AgentsHandler
	if cfg.AgentsService != nil {
		agents = NewAgentsHandler(cfg.AgentsService)
	}

	// Auth middleware
	authMiddleware := mw.Auth(cfg.JWKS, cfg.IssuerURL, cfg.DevAuthMode, cfg.Logger)

	// ============================================================
	// A2A Webhook — NO JWT auth (uses JWS signature instead).
	// Tighter body cap than the default: Brain envelopes are <100KB
	// in practice; 1 MiB is comfortably above any plausible event
	// while restricting amplification surface for an attacker who
	// somehow forged a JWS.
	// ============================================================
	r.With(mw.MaxBodySize(mw.A2AInboundMaxBodyBytes)).
		Post("/api/v1/a2a/webhook", a2a.ReceiveWebhook)

	// ============================================================
	// All authenticated routes
	// ============================================================
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

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

				// Procurement — owner, admin: full; superintendent: read
				r.Route("/procurement", func(r chi.Router) {
					r.Get("/", procurement.List)
					r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/", procurement.Create)
					r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Put("/{itemID}", procurement.Update)
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
		// 3.5 Billing — proxy to Brain. Owner/admin only since
		//     usage data is sensitive operational/financial info.
		// --------------------------------------------------------
		if billing != nil {
			r.Route("/api/v1/billing", func(r chi.Router) {
				r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
				r.Get("/usage", billing.Usage)
				r.Get("/usage/daily", billing.DailyUsage)
			})
		}

		// --------------------------------------------------------
		// 3.6 AI Agents — Maestro-backed endpoints. Pro tier and up.
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
// actually serve traffic. Checks the database pool, (when wired)
// Brain's /health, and (when wired) the JWKS cache freshness. All
// checks run under a 2s deadline so a slow upstream doesn't block
// the load balancer's poll cadence.
//
// Response body always includes per-component status so ops can
// distinguish "DB down" from "Brain down" from "JWKS stale" from a
// single curl.
func readinessHandler(pool *pgxpool.Pool, brain BrainPinger, jwksRep JWKSCacheReporter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		dbStatus := "ok"
		var dbErr error
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unhealthy"
			dbErr = err
		}

		brainStatus := "skipped" // when no brainPinger is wired
		var brainErr error
		if brain != nil {
			if err := brain.Ping(ctx); err != nil {
				brainStatus = "unhealthy"
				brainErr = err
			} else {
				brainStatus = "ok"
			}
		}

		// JWKS cache check: report unhealthy when we have NO keys
		// (cold start, or upstream returned empty) OR the cache is
		// >2x its TTL (we've been unable to refresh long enough that
		// rotated Brain keys may have started rejecting our tokens).
		jwksStatus := "skipped"
		var jwksDetail string
		if jwksRep != nil {
			keyCount, age := jwksRep.CacheStatus()
			ttl := jwksRep.CacheTTL()
			switch {
			case keyCount == 0:
				jwksStatus = "unhealthy"
				jwksDetail = "no keys cached"
			case ttl > 0 && age > 2*ttl:
				jwksStatus = "unhealthy"
				jwksDetail = "cache stale: age " + age.Round(time.Second).String()
			default:
				jwksStatus = "ok"
			}
		}

		ready := dbStatus == "ok" &&
			brainStatus != "unhealthy" &&
			jwksStatus != "unhealthy"
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			if logger != nil {
				logger.WarnContext(ctx, "readiness probe failed",
					"db_status", dbStatus, "db_error", dbErr,
					"brain_status", brainStatus, "brain_error", brainErr,
					"jwks_status", jwksStatus, "jwks_detail", jwksDetail)
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
			`","components":{"database":"` + dbStatus +
			`","brain":"` + brainStatus +
			`","jwks":"` + jwksStatus + `"}}`
		_, _ = w.Write([]byte(body))
	}
}
