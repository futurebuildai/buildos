package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// FeedHandler handles /api/v1/feed/* endpoints.
type FeedHandler struct {
	svc *service.FeedService
}

// NewFeedHandler creates a new FeedHandler with the given service.
func NewFeedHandler(svc *service.FeedService) *FeedHandler {
	return &FeedHandler{svc: svc}
}

// List returns feed cards for the authenticated user's org.
// GET /api/v1/feed
func (h *FeedHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := mw.ClaimsFromContext(r.Context())
	if !ok {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth claims")
		return
	}

	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG", "invalid org_id in token")
		return
	}

	var userID *uuid.UUID
	if uid, err := uuid.Parse(claims.Sub); err == nil {
		userID = &uid
	}

	filter := models.FeedFilter{
		Priority: r.URL.Query().Get("priority"),
		Status:   r.URL.Query().Get("status"),
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		filter.Limit, _ = strconv.Atoi(limitStr)
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		filter.Offset, _ = strconv.Atoi(offsetStr)
	}

	if filter.Limit > 100 {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_LIMIT", "limit cannot exceed 100")
		return
	}

	cards, total, err := h.svc.ListCards(r.Context(), orgID, userID, claims.Role, filter)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]any{
		"cards": cards,
		"total": total,
	})
}

// Action processes a feed card action.
// POST /api/v1/feed/{cardID}/action
func (h *FeedHandler) Action(w http.ResponseWriter, r *http.Request) {
	cardID, err := uuid.Parse(chi.URLParam(r, "cardID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid card ID")
		return
	}

	var body struct {
		Action  string          `json:"action"`
		Details json.RawMessage `json:"details,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	if err := h.svc.ActionCard(r.Context(), cardID); err != nil {
		if errors.Is(err, service.ErrCardNotFound) {
			writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "card not found or not active")
			return
		}
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]any{
		"id":     cardID,
		"status": "actioned",
	})
}

// Dismiss dismisses a feed card.
// POST /api/v1/feed/{cardID}/dismiss
func (h *FeedHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	cardID, err := uuid.Parse(chi.URLParam(r, "cardID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid card ID")
		return
	}

	if err := h.svc.DismissCard(r.Context(), cardID); err != nil {
		if errors.Is(err, service.ErrCardNotFound) {
			writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "card not found or already dismissed")
			return
		}
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
