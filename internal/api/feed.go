package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// FeedServicer is the subset of *service.FeedService consumed by
// FeedHandler. Defined here (consumer side) so handlers can be unit-
// tested with a mock and so the handler doesn't transitively import
// the database pool.
type FeedServicer interface {
	ListFeed(ctx context.Context, opts service.FeedListOptions) (service.FeedListResult, error)
	DismissCard(ctx context.Context, callerOrgID, cardID uuid.UUID) (models.FeedCard, error)
	ActionCard(ctx context.Context, callerOrgID, cardID uuid.UUID, in service.FeedActionInput) (models.FeedCard, error)
}

// FeedHandler handles /api/v1/feed/* endpoints.
type FeedHandler struct {
	svc FeedServicer
}

// NewFeedHandler creates a handler bound to the given service.
func NewFeedHandler(svc FeedServicer) *FeedHandler {
	return &FeedHandler{svc: svc}
}

// List returns feed cards visible to the authenticated user.
// Targeting is "target_user_id matches caller's user row OR
// target_role matches caller's role".
//
// GET /api/v1/feed
//
// Query params:
//   - status:   comma-separated, default "active". Valid: active, dismissed, actioned, expired.
//   - priority: comma-separated, optional. Valid: critical, urgent, normal, low.
//   - page, per_page: pagination, default 1 / 50, per_page capped at 200.
func (h *FeedHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := mw.MustClaimsFromContext(r.Context())
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid org_id claim")
		return
	}

	statusFilter := splitCSVParam(r, "status")
	priorityFilter := splitCSVParam(r, "priority")
	page, perPage := parsePagination(r)

	res, err := h.svc.ListFeed(r.Context(), service.FeedListOptions{
		CallerOrgID:       orgID,
		CallerOIDCSubject: claims.Sub,
		CallerRole:        claims.Role,
		StatusFilter:      statusFilter,
		PriorityFilter:    priorityFilter,
		Page:              page,
		PerPage:           perPage,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	totalPages := 0
	if perPage > 0 {
		totalPages = (res.Total + perPage - 1) / perPage
	}
	writeJSONWithPagination(w, r, http.StatusOK,
		map[string]any{"cards": res.Cards},
		paginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      res.Total,
			TotalPages: totalPages,
		},
	)
}

type feedActionRequest struct {
	ActionType string          `json:"action_type"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

// Action processes a feed card action — transitions status to
// 'actioned' and records the action_type/payload in the audit log.
// Side-effect dispatch (e.g., approve_quote → outbound A2A) is wired
// in a later sprint.
//
// POST /api/v1/feed/{cardID}/action
func (h *FeedHandler) Action(w http.ResponseWriter, r *http.Request) {
	cardID, ok := parseUUIDFromURL(w, r, "cardID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var body feedActionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	card, err := h.svc.ActionCard(r.Context(), callerOrg, cardID, service.FeedActionInput{
		ActionType: body.ActionType,
		Payload:    body.Payload,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"card": card})
}

// Dismiss transitions a card to status='dismissed'.
//
// POST /api/v1/feed/{cardID}/dismiss
func (h *FeedHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	cardID, ok := parseUUIDFromURL(w, r, "cardID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	card, err := h.svc.DismissCard(r.Context(), callerOrg, cardID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"card": card})
}

// writeServiceError maps FeedService sentinel errors to HTTP responses.
func (h *FeedHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrFeedCardNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "feed card not found")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// splitCSVParam reads a comma-separated query param into a slice.
// Empty segments and whitespace-only segments are dropped. Missing
// param returns nil so the service layer applies its own defaults.
func splitCSVParam(r *http.Request, key string) []string {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
