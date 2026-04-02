package api

import "net/http"

// FeedHandler handles /api/v1/feed/* endpoints.
type FeedHandler struct{}

// List returns feed cards for the authenticated user's org.
// GET /api/v1/feed
func (h *FeedHandler) List(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Action processes a feed card action.
// POST /api/v1/feed/{cardID}/action
func (h *FeedHandler) Action(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Dismiss dismisses a feed card.
// POST /api/v1/feed/{cardID}/dismiss
func (h *FeedHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
