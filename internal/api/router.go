package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/service"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

// RouterConfig holds all dependencies needed to build the API router.
type RouterConfig struct {
	Pool         *pgxpool.Pool
	JWKS         *mw.JWKSProvider
	IssuerURL    string
	DevBypass    bool
	Logger       *slog.Logger
}

// NewRouter creates the Chi router with all route groups and middleware.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Logger)

	// Health check (no auth)
	r.Get("/health", healthHandler(cfg.Pool))

	// Instantiate service layer
	financialStore := store.NewFinancialStore(cfg.Pool)
	budgetSvc := service.NewBudgetService(financialStore)
	corporateSvc := service.NewCorporateFinancialsService(financialStore)
	pipelineStore := store.NewPipelineStore(cfg.Pool)
	pipelineSvc := service.NewPipelineService(pipelineStore)

	// Instantiate handlers
	projects := &ProjectHandler{}
	schedule := &ScheduleHandler{}
	financials := NewFinancialsHandler(budgetSvc, corporateSvc)
	pipeline := NewPipelineHandler(pipelineSvc)
	procurement := &ProcurementHandler{}
	feed := &FeedHandler{}
	field := &FieldHandler{}
	fleet := &FleetHandler{}
	hr := &HRHandler{}
	a2a := NewA2AHandler(cfg.JWKS, cfg.Logger)

	// Auth middleware
	authMiddleware := mw.Auth(cfg.JWKS, cfg.IssuerURL, cfg.DevBypass, cfg.Logger)

	// ============================================================
	// A2A Webhook — NO JWT auth (uses JWS signature instead)
	// ============================================================
	r.Post("/api/v1/a2a/webhook", a2a.ReceiveWebhook)

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

func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unhealthy","error":"database unreachable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"0.1.0"}`))
	}
}
