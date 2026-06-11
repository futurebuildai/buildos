package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// ReportsServicer is the consumer-side interface ReportsHandler needs. Defined
// here so the handler stays free of a transitive db-pool import and tests can
// substitute a fake.
type ReportsServicer interface {
	ListProjectReports(ctx context.Context, orgID, projectID uuid.UUID, since, until time.Time) ([]models.DailyReportSummary, error)
	GetProjectReport(ctx context.Context, orgID, projectID uuid.UUID, day time.Time) (models.DailyReport, error)
	GenerateDigest(ctx context.Context, orgID uuid.UUID, userSub string, projectID uuid.UUID, day time.Time) (string, error)
	DraftClientUpdate(ctx context.Context, orgID uuid.UUID, userSub string, projectID uuid.UUID, day time.Time) (service.ClientUpdateDraft, error)
}

// ReportsHandler exposes the operator daily-reports surface (Chunk C): the
// derived read model (list + one day) plus the two AI compositions (the
// internal office digest and the client-safe homeowner draft). The reads are
// derived from daily_logs + crew_checkins + task_progress — there is no
// daily_reports table.
type ReportsHandler struct {
	svc ReportsServicer
}

// NewReportsHandler binds the handler to a ReportsServicer.
func NewReportsHandler(svc ReportsServicer) *ReportsHandler {
	return &ReportsHandler{svc: svc}
}

// List returns daily-report summaries for a project over a date window.
// GET /api/v1/projects/{projectID}/daily-reports?since=&until= — minRole superintendent.
func (h *ReportsHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var since, until time.Time
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := parseReportDate(v)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "since must be YYYY-MM-DD")
			return
		}
		since = t
	}
	if v := r.URL.Query().Get("until"); v != "" {
		t, err := parseReportDate(v)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "until must be YYYY-MM-DD")
			return
		}
		until = t
	}
	// No bounds supplied → default last-14-days window (deterministic).
	if since.IsZero() && until.IsZero() {
		since, until = service.DefaultReportWindow(time.Now())
	}

	reports, err := h.svc.ListProjectReports(r.Context(), orgID, projectID, since, until)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if reports == nil {
		reports = []models.DailyReportSummary{}
	}
	writeJSON(w, r, http.StatusOK, reports)
}

// Get returns one day's derived daily report (incl. photo thumbnails via the
// Chunk A signed-GET when storage is configured).
// GET /api/v1/projects/{projectID}/daily-reports/{date} — minRole superintendent.
func (h *ReportsHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	day, ok := parseReportDateParam(w, r)
	if !ok {
		return
	}
	report, err := h.svc.GetProjectReport(r.Context(), orgID, projectID, day)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, report)
}

// Digest generates the AI internal office digest for a day's report.
// POST /api/v1/projects/{projectID}/daily-reports/{date}/digest — minRole superintendent.
func (h *ReportsHandler) Digest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	day, ok := parseReportDateParam(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	digest, err := h.svc.GenerateDigest(r.Context(), orgID, claims.Sub, projectID, day)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"digest": digest})
}

// ClientUpdateDraft generates the client-safe homeowner progress DRAFT for a
// day's report. The draft is returned for the operator to edit; Chunk C does NOT
// persist or send (that's Chunk D).
// POST /api/v1/projects/{projectID}/daily-reports/{date}/client-update-draft — owner/admin.
func (h *ReportsHandler) ClientUpdateDraft(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	day, ok := parseReportDateParam(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	draft, err := h.svc.DraftClientUpdate(r.Context(), orgID, claims.Sub, projectID, day)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, draft)
}

// writeServiceError maps ReportsService + native-AI sentinels. AI soft-fail
// (ErrReportsAIUnavailable / ai.ErrUnconfigured / transient) reuses the shared
// writeAIServiceError map so the daily-reports AI surface degrades identically
// to the briefing / assistant.
func (h *ReportsHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, service.ErrReportsAIUnavailable) {
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "AI is not available for daily reports")
		return
	}
	// Defer to the shared AI/notfound/validation map for everything else
	// (ai.ErrUnconfigured, ErrRateLimited, ErrNotFound, ErrInvalidInput, ...).
	writeAIServiceError(w, r, err)
}

// parseReportDate parses a YYYY-MM-DD date as midnight UTC.
func parseReportDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// parseReportDateParam reads + validates the {date} URL param (YYYY-MM-DD).
func parseReportDateParam(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	raw := chi.URLParam(r, "date")
	day, err := parseReportDate(raw)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "date must be YYYY-MM-DD")
		return time.Time{}, false
	}
	return day, true
}

// MountReportsRoutes wires the daily-reports endpoints under the project
// subtree. List/Get/Digest are minRole superintendent (internal office surface);
// the client-update DRAFT is owner/admin (external-comms trust, §9-1). Caller
// must place this INSIDE the auth group.
func MountReportsRoutes(r chi.Router, h *ReportsHandler) {
	r.Route("/api/v1/projects/{projectID}/daily-reports", func(r chi.Router) {
		r.With(mw.RequireMinRole(mw.RoleSuperintendent)).Get("/", h.List)
		r.With(mw.RequireMinRole(mw.RoleSuperintendent)).Get("/{date}", h.Get)
		r.With(mw.RequireMinRole(mw.RoleSuperintendent)).Post("/{date}/digest", h.Digest)
		r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/{date}/client-update-draft", h.ClientUpdateDraft)
	})
}
