package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

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
	// ObserveErrorResponse counts a JSON error response by {code, status-class};
	// the error writers (this package + middleware) call it via a package-level
	// observer the router wires below.
	ObserveErrorResponse(code, status string)
}

// RouterConfig holds all dependencies needed to build the API router.
type RouterConfig struct {
	Pool                *pgxpool.Pool
	Verifier            *auth.Verifier
	DevAuthMode         string
	Logger              *slog.Logger
	AuthService         AuthServicer
	AuthRefreshTTL      time.Duration // refresh-cookie Max-Age; 0 = AuthService default (30d)
	CookieSecure        bool          // stamps Secure on the HttpOnly refresh cookie (default-on in prod; off for local http rigs)
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
	AgentConfigService  AgentConfigServicer  // optional — when nil, /admin/agents/* routes don't mount (agent config registry)
	ConnectorService    ConnectorServicer    // optional — when nil, /admin/connectors/* routes don't mount (connector registry)
	FeedbackService     FeedbackServicer     // optional — when nil, /feedback + /admin/feedback routes don't mount
	Assistant           AssistantConverser   // optional — when nil, POST /agents/chat doesn't mount (no AI client)
	IngestionService    InvoiceIngestor      // optional — when nil, the /invoices/ingest route doesn't mount (AI unconfigured)
	AssetService        AssetServicer        // optional — when nil, /assets + /projects/{id}/assets routes don't mount (object storage)
	Metrics             MetricsRecorder      // optional — when nil, /metrics doesn't mount and HTTP middleware is skipped
	SentryEnabled       bool                 // when true, the Sentry HTTP middleware is mounted to capture panics
	RateLimiter         *mw.IPRateLimiter    // optional — when nil, no rate limiting is applied
	TrustedProxyCIDRs   []*net.IPNet         // forwarding headers (XFF) are honored ONLY from these peers; empty = ignore XFF, use the TCP peer (fail-safe)
	WebDistDir          string               // optional — built web console dir (web/dist); empty = no SPA serving (dev rigs proxy via Vite)
	Version             string               // build version surfaced by GET /health (ldflags-stamped; empty = "dev") — deploy smoke asserts the rolled binary
}

// NewRouter creates the Chi router with all route groups and middleware.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Wire the error-response counter into both error writers (this package +
	// the middleware package). Reset to nil when metrics are disabled so a
	// later metrics-less router build doesn't keep a stale observer.
	if cfg.Metrics != nil {
		errResponseObserver = cfg.Metrics.ObserveErrorResponse
		mw.SetErrorObserver(cfg.Metrics.ObserveErrorResponse)
	} else {
		errResponseObserver = nil
		mw.SetErrorObserver(nil)
	}

	// Global middleware. Order matters:
	//   - RequestID first so chi assigns one before any logging.
	//   - RealIP next so downstream sees the right remote. Unlike chi's
	//     RealIP, mw.RealIP trusts X-Forwarded-For ONLY from configured
	//     trusted-proxy CIDRs (else the per-IP rate limiter is bypassable by
	//     header spoofing). Empty allowlist = use the real TCP peer.
	//   - Recoverer wraps before metrics: panics still get counted.
	//   - Metrics middleware before Logger so a panicked handler's
	//     5xx still ticks the counter even if Logger swallows it.
	//   - Logger emits the request line, with request_id already set.
	r.Use(chimw.RequestID)
	r.Use(mw.RealIP(cfg.TrustedProxyCIDRs))
	// OTel HTTP middleware sits HERE — after RequestID/RealIP (so
	// the span tags include both) but before everything else (so
	// every downstream middleware's behavior is captured under the
	// span). The wrapper is a no-op when no global tracer is
	// configured; dev rigs without OTel pay no cost.
	r.Use(func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "buildos.http",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				// Provisional name only — the formatter runs BEFORE
				// routing, so the matched pattern isn't known yet. The
				// middleware just below renames the span post-routing.
				return r.Method + " " + r.URL.Path
			}),
		)
	})
	// Rename the server span AFTER routing so it carries chi's matched
	// pattern (e.g. "GET /api/v1/projects/{projectID}") instead of the
	// raw URL. Unmatched paths — which, with the SPA catch-all, include
	// every client-routed deep link and hashed asset URL — collapse to
	// one "(unmatched)" name: raw URLs would make span-name cardinality
	// unbounded for any backend deriving per-name metrics (same failure
	// mode the Prometheus middleware guards against). SetName runs
	// before otelhttp ends the span on unwind, so the rename sticks.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			span := trace.SpanFromContext(req.Context())
			if !span.IsRecording() {
				return
			}
			if pattern := chi.RouteContext(req.Context()).RoutePattern(); pattern != "" {
				span.SetName(req.Method + " " + pattern)
			} else {
				span.SetName(req.Method + " (unmatched)")
			}
		})
	})
	// Baseline security headers on every response (incl. 429s/errors below).
	r.Use(mw.SecurityHeaders)
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
	r.Get("/health", livenessHandler(cfg.Version))
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
	authHandler := NewAuthHandler(cfg.AuthService, WithRefreshCookie(cfg.CookieSecure, cfg.AuthRefreshTTL))
	var agents *AgentsHandler
	if cfg.AgentsService != nil {
		agents = NewAgentsHandler(cfg.AgentsService)
	}
	var assistant *AssistantHandler
	if cfg.Assistant != nil {
		assistant = NewAssistantHandler(cfg.Assistant)
	}
	var ingest *IngestHandler
	if cfg.IngestionService != nil {
		ingest = NewIngestHandler(cfg.IngestionService)
	}
	var setup *SetupHandler
	if cfg.SetupService != nil {
		setup = NewSetupHandler(cfg.SetupService)
	}
	var integrations *IntegrationsHandler
	if cfg.IntegrationsService != nil {
		integrations = NewIntegrationsHandler(cfg.IntegrationsService)
	}
	var agentConfig *AgentConfigHandler
	if cfg.AgentConfigService != nil {
		agentConfig = NewAgentConfigHandler(cfg.AgentConfigService)
	}
	var connectorAdmin *ConnectorHandler
	if cfg.ConnectorService != nil {
		connectorAdmin = NewConnectorHandler(cfg.ConnectorService)
	}
	var feedback *FeedbackHandler
	if cfg.FeedbackService != nil {
		feedback = NewFeedbackHandler(cfg.FeedbackService)
	}
	var assets *AssetHandler
	if cfg.AssetService != nil {
		assets = NewAssetHandler(cfg.AssetService)
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
		// 0.6 Agent config registry — admin-gated enable/tune of the
		//     agentic capabilities (delay_cascade, foresight, experience)
		//     per org, post-deploy, no redeploy (Phase 3a). Mounted under
		//     /api/v1/admin/agents — deliberately OFF the pro-tier
		//     /api/v1/agents tree so the kill-switch is reachable
		//     regardless of plan tier. RBAC (admin+) enforced inside
		//     MountAgentConfigRoutes. Behind the SetupGate (config is
		//     operational, not bootstrap).
		// --------------------------------------------------------
		if agentConfig != nil {
			MountAgentConfigRoutes(r, agentConfig)
		}

		// --------------------------------------------------------
		// 0.7 Connector registry — admin-gated enable/config of the
		//     integration connectors (Phase 3b). Default-OFF per org;
		//     mounted under /api/v1/admin/connectors, OFF the pro tree
		//     so the kill-switch is reachable regardless of plan tier.
		//     RBAC (admin+) enforced inside MountConnectorRoutes;
		//     behind the SetupGate.
		// --------------------------------------------------------
		if connectorAdmin != nil {
			MountConnectorRoutes(r, connectorAdmin)
		}

		// --------------------------------------------------------
		// 0.8 Feedback loop (Phase 0b) — POST /api/v1/feedback is
		//     auth-only (every role files reports from the widget);
		//     /api/v1/admin/feedback (list + triage) is admin-gated —
		//     the harvest surface the buildos-operations command
		//     center polls. Behind the SetupGate (operational, not
		//     bootstrap).
		// --------------------------------------------------------
		if feedback != nil {
			MountFeedbackRoutes(r, feedback)
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
					// Batch task+dependency import → CPM (operator ingress).
					// Same role gate as /recalculate (CPM-affecting structural data).
					r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
						Post("/import", schedule.Import)
					r.Get("/gantt", schedule.Gantt)

					// Maestro-driven duration adjustments. Same role
					// gate as /recalculate (CPM-affecting). The pro-tier
					// plan gate was removed (ESC-002): post-pivot billing
					// is gone (ESC-001), so the gate had no backing system
					// and 402-walled every real self-minted token.
					// Mounts only when AgentsService is wired —
					// matches the conditional under /api/v1/agents.
					if agents != nil {
						r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
							Post("/recommend-adjustments", agents.RecommendScheduleAdjustments)
						// PREVIEW-FIRST (ESC-AUX-01): apply the
						// user-selected duration proposals from a
						// dry-run preview. Same superintendent+ gate.
						r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
							Post("/adjustments/apply", agents.ApplyScheduleAdjustments)
					}
				})

				// Tasks sub-routes
				r.Get("/tasks", schedule.ListTasks)
				r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
					Post("/tasks", schedule.CreateTask)
				r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
					Put("/tasks/{taskID}", schedule.UpdateTask)

				// Budgets (financial — owner, admin only)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).
					Get("/budgets", financials.ListBudgets)
				r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).
					Post("/budgets", financials.CreateBudgets)

				// Invoices (financial — owner, admin only)
				r.Route("/invoices", func(r chi.Router) {
					r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
					r.Post("/", financials.CreateInvoice)
					r.Put("/{invoiceID}", financials.UpdateInvoice)
					// AI invoice ingestion (Phase 2a). Mounts only when
					// the IngestionService is wired — matches the
					// conditional-handler pattern used for agents.
					if ingest != nil {
						r.Post("/ingest", ingest.IngestInvoice)
					}
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
				r.Post("/", hr.CreateEmployee)
				r.Get("/{employeeID}/certifications", hr.ListCertifications)
				r.Post("/{employeeID}/certifications", hr.CreateCertification)
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
		// 3.6 AI Agents — native-AI-backed endpoints. Authenticated
		//      (any role; daily-briefing is for the caller themselves).
		//      The pro-tier plan gate was removed (ESC-002): post-pivot
		//      billing is gone (ESC-001), so the gate had no backing
		//      system and 402-walled every real self-minted token. Each
		//      agent is a separate route so per-route gates can diverge
		//      later if a tiering model returns.
		// --------------------------------------------------------
		if agents != nil {
			r.Route("/api/v1/agents", func(r chi.Router) {
				r.Post("/daily-briefing", agents.DailyBriefing)
			})
		}

		// Conversational ERP assistant (Phase 2c). Mounted in a sibling
		// block — it depends on the AssistantService (AI client + read
		// services), NOT on AgentsService, so the two AI surfaces wire
		// independently. The bounded Claude tool-use loop runs read-only
		// tools scoped to the caller's org+role.
		//
		// Route gate: RequireMinRole(superintendent) — field_worker has no
		// conversational surface in 2c. (The pro-tier plan gate was removed —
		// ESC-002 — since post-pivot billing is gone; role gates stay.) RBAC
		// invariant #1 (caller org/role/sub sealed into per-request executor
		// closures) is enforced in AssistantService.Converse; the handler reads
		// identity from claims only.
		if assistant != nil {
			r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
				Post("/api/v1/agents/chat", assistant.Converse)
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

		// --------------------------------------------------------
		// 4.5 Object-storage substrate (Chunk A) — presigned-PUT upload,
		//     confirm, signed-GET, and the project gallery. RBAC (minRole
		//     superintendent) is enforced inside MountAssetRoutes. Mounts
		//     only when the AssetService is wired (R2 configured for the
		//     fork) so a storage-less fork stays green; the field-facing
		//     variant for field_worker lands in Chunk B.
		// --------------------------------------------------------
		if assets != nil {
			MountAssetRoutes(r, assets)
		}
	})

	// ------------------------------------------------------------
	// NotFound handling + same-origin SPA serving (Phase 0a).
	//
	// chi propagates a root NotFound handler into every mounted
	// subrouter, so this fires both for top-level unmatched paths AND
	// for unmatched paths inside API subtrees (where the group's
	// middleware — auth, SetupGate — runs first). It executes inside
	// the global middleware chain (security headers, rate limit,
	// metrics) but outside the auth group for top-level paths — the
	// login page must load unauthenticated.
	//
	// With WebDistDir set (production image) the SPA handler serves the
	// web console and keeps /api/* misses as JSON 404s. With it empty
	// (dev rigs — Vite proxies /api) there is no console to serve, but
	// the JSON 404 is registered anyway so unmatched-path response
	// bodies are identical in both modes (API clients should never see
	// chi's text/plain default in one environment and the envelope in
	// the other).
	// ------------------------------------------------------------
	if cfg.WebDistDir != "" {
		r.NotFound(newSPAHandler(cfg.WebDistDir).ServeHTTP)
	} else {
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			writeErrorResponse(w, req, http.StatusNotFound, "NOT_FOUND", "route not found")
		})
	}

	return r
}

// livenessHandler returns the liveness probe. Always 200 — if the
// HTTP listener can answer, the process is alive. We deliberately do
// NOT check the database here; an unreachable DB is a readiness
// problem, not a liveness one (restarting the process won't help).
//
// version is the ldflags-stamped build version (e.g. "staging-<sha>").
// The deploy pipeline's smoke asserts it after a roll: status codes
// alone can't distinguish the NEW deployment from the OLD one still
// draining, but the version string can.
func livenessHandler(version string) http.HandlerFunc {
	if version == "" {
		version = "dev"
	}
	// Pre-marshal once — version is fixed for the process lifetime and
	// must be JSON-escaped, not string-concatenated, since it arrives
	// from a build arg.
	body, err := json.Marshal(map[string]string{"status": "ok", "version": version})
	if err != nil {
		body = []byte(`{"status":"ok","version":"unknown"}`)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
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
