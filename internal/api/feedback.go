package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
)

// FeedbackServicer is the subset of *service.FeedbackService consumed
// by FeedbackHandler. The handler depends on the interface so unit
// tests can substitute a fake (matches AgentConfigServicer).
type FeedbackServicer interface {
	Submit(ctx context.Context, in service.SubmitFeedbackInput) (models.Feedback, error)
	ListForAdmin(ctx context.Context, in service.ListFeedbackInput) (store.FeedbackPage, error)
	Triage(ctx context.Context, in service.TriageFeedbackInput) (models.Feedback, error)
}

// FeedbackHandler exposes the feedback loop (Phase 0b): any
// authenticated role submits from the web-console widget; admins (and
// the buildos-operations command center) list + triage under
// /api/v1/admin/feedback.
type FeedbackHandler struct {
	svc FeedbackServicer
}

// NewFeedbackHandler binds the handler to a FeedbackServicer.
func NewFeedbackHandler(svc FeedbackServicer) *FeedbackHandler {
	return &FeedbackHandler{svc: svc}
}

// submitFeedbackRequest is the POST body. Context is the widget's
// client-captured environment (route, role, app_version, user_agent,
// viewport) — passed through as an opaque JSON object; the service
// validates shape + size. Identity comes from claims, never the body.
type submitFeedbackRequest struct {
	Category string          `json:"category"`
	Message  string          `json:"message"`
	Context  json.RawMessage `json:"context,omitempty"`
}

// triageFeedbackRequest is the PATCH body. Status is required; a nil
// (omitted) triage_note keeps the existing note, an empty string
// clears it.
type triageFeedbackRequest struct {
	Status     string  `json:"status"`
	TriageNote *string `json:"triage_note,omitempty"`
}

// ---------- POST /api/v1/feedback ----------

// Submit files one feedback report for the caller's org. Any
// authenticated role — field workers included; the global per-IP rate
// limiter and body-size cap bound abuse.
func (h *FeedbackHandler) Submit(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())

	var body submitFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	fb, err := h.svc.Submit(r.Context(), service.SubmitFeedbackInput{
		OrgID:    orgID,
		UserSub:  claims.Sub,
		Category: body.Category,
		Message:  body.Message,
		Context:  body.Context,
	})
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"feedback": fb})
}

// ---------- GET /api/v1/admin/feedback?status=&page=&per_page= ----------

// List returns one page of the org's feedback, newest first, optionally
// filtered by ?status=, with pagination meta (API_CONTRACT §2.3) so the
// command-center poller can drain a backlog without a truncation blind
// spot. Admin RBAC enforced by the route group.
func (h *FeedbackHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	page, perPage := parsePagination(r)
	result, err := h.svc.ListForAdmin(r.Context(), service.ListFeedbackInput{
		OrgID:   orgID,
		Status:  r.URL.Query().Get("status"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	rows := result.Feedback
	if rows == nil {
		rows = []models.Feedback{}
	}
	writeJSONWithPagination(w, r, http.StatusOK,
		map[string]any{"feedback": rows},
		paginationMeta{Page: result.Page, PerPage: result.PerPage, Total: result.Total, TotalPages: result.TotalPages})
}

// ---------- PATCH /api/v1/admin/feedback/{feedbackID} ----------

// Triage moves a report through the lifecycle. 404 when the id is
// unknown OR belongs to another org (indistinguishable on purpose);
// 400 on an unknown status.
func (h *FeedbackHandler) Triage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDFromURL(w, r, "feedbackID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())

	var body triageFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	fb, err := h.svc.Triage(r.Context(), service.TriageFeedbackInput{
		OrgID:      orgID,
		ID:         id,
		Status:     body.Status,
		TriageNote: body.TriageNote,
		UserSub:    claims.Sub,
	})
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"feedback": fb})
}

// feedbackThrottleRetryAfter is the Retry-After hint for the
// per-(org,user) submit throttle's 429 (a fraction of the 1h window —
// precise expiry would leak per-row timing for no operator benefit).
const feedbackThrottleRetryAfter = 15 * time.Minute

// writeFeedbackError maps service sentinels to HTTP responses:
//   - service.ErrNotFound     -> 404 (unknown id / foreign org)
//   - service.ErrInvalidInput -> 400 (bad category/status/message/context)
//   - service.ErrRateLimited  -> 429 + Retry-After (per-user submit throttle)
//   - anything else           -> 500 (opaque)
func writeFeedbackError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrRateLimited):
		writeErrorResponseRetry(w, r, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), feedbackThrottleRetryAfter)
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountFeedbackRoutes registers the feedback surfaces. The submit
// route is auth-only (every role files feedback); the triage surface
// is admin-gated and OFF the submit path so the RBAC boundary is
// structural. Mounted inside the authenticated + SetupGate group by
// NewRouter.
func MountFeedbackRoutes(r chi.Router, h *FeedbackHandler) {
	r.Post("/api/v1/feedback", h.Submit)
	r.Route("/api/v1/admin/feedback", func(r chi.Router) {
		r.Use(mw.RequireMinRole(mw.RoleAdmin))
		r.Get("/", h.List)
		r.Patch("/{feedbackID}", h.Triage)
	})
}
