package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// ProcurementServicer is the subset of *service.ProcurementService
// consumed by ProcurementHandler. Defined consumer-side so the handler
// can be unit-tested without spinning up a database.
type ProcurementServicer interface {
	ListProcurement(ctx context.Context, projectID, callerOrgID uuid.UUID, statusFilter []string) ([]models.ProcurementItem, error)
	CreateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in service.CreateProcurementItemInput) (models.ProcurementItem, error)
	UpdateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in service.UpdateProcurementItemInput) (models.ProcurementItem, error)
	RequestVendorReview(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in service.RequestVendorReviewInput) (uuid.UUID, error)
}

// ProcurementHandler handles /api/v1/projects/{projectID}/procurement/* endpoints.
type ProcurementHandler struct {
	svc ProcurementServicer
}

// NewProcurementHandler creates a handler bound to the given service.
func NewProcurementHandler(svc ProcurementServicer) *ProcurementHandler {
	return &ProcurementHandler{svc: svc}
}

// List returns procurement items for a project.
//
// GET /api/v1/projects/{projectID}/procurement[?status=WARNING,CRITICAL]
func (h *ProcurementHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	statusFilter := splitCSVParam(r, "status")

	items, err := h.svc.ListProcurement(r.Context(), projectID, callerOrg, statusFilter)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type createProcurementRequest struct {
	Name                      string     `json:"name"`
	WBSCode                   string     `json:"wbs_code"`
	EstimatedCostCents        int64      `json:"estimated_cost_cents"`
	EstimatedCostCurrencyCode string     `json:"estimated_cost_currency_code"`
	LeadTimeDays              int        `json:"lead_time_days"`
	WeatherBufferDays         int        `json:"weather_buffer_days"`
	VendorID                  *uuid.UUID `json:"vendor_id,omitempty"`
	NeedByDate                *string    `json:"need_by_date,omitempty"`
}

// Create adds a new procurement item.
//
// POST /api/v1/projects/{projectID}/procurement
func (h *ProcurementHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var body createProcurementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	needBy, err := parseOptionalDate(body.NeedByDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "need_by_date must be RFC3339 or YYYY-MM-DD")
		return
	}

	claims := mw.MustClaimsFromContext(r.Context())
	item, err := h.svc.CreateProcurementItem(r.Context(), callerOrg, claims.Sub, service.CreateProcurementItemInput{
		ProjectID:                 projectID,
		Name:                      body.Name,
		WBSCode:                   body.WBSCode,
		EstimatedCostCents:        body.EstimatedCostCents,
		EstimatedCostCurrencyCode: body.EstimatedCostCurrencyCode,
		LeadTimeDays:              body.LeadTimeDays,
		WeatherBufferDays:         body.WeatherBufferDays,
		VendorID:                  body.VendorID,
		NeedByDate:                needBy,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"item": item})
}

type updateProcurementRequest struct {
	Status    *string `json:"status,omitempty"`
	PONumber  *string `json:"po_number,omitempty"`
	OrderedAt *string `json:"ordered_at,omitempty"`
}

// Update modifies a procurement item.
//
// PUT /api/v1/projects/{projectID}/procurement/{itemID}
func (h *ProcurementHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	itemID, ok := parseUUIDFromURL(w, r, "itemID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var body updateProcurementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	orderedAt, err := parseOptionalRFC3339(body.OrderedAt)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "ordered_at must be RFC3339")
		return
	}

	claims := mw.MustClaimsFromContext(r.Context())
	item, err := h.svc.UpdateProcurementItem(r.Context(), callerOrg, claims.Sub, service.UpdateProcurementItemInput{
		ItemID:    itemID,
		ProjectID: projectID,
		Status:    body.Status,
		PONumber:  body.PONumber,
		OrderedAt: orderedAt,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"item": item})
}

type requestVendorReviewRequest struct {
	Vendor       string     `json:"vendor"`
	TotalCents   int64      `json:"total_cents"`
	CurrencyCode string     `json:"currency_code"`
	RFQID        *uuid.UUID `json:"rfq_id,omitempty"`
	Reasoning    string     `json:"reasoning,omitempty"`
}

// RequestVendorReview surfaces a vendor's material quote for human
// review by creating a local `vendor_review_requested` feed card. The
// service opens one tx and runs the ownership check + feed-card insert
// + audit row atomically.
//
// POST /api/v1/projects/{projectID}/procurement/{itemID}/request-review
//
// Role gate: superintendent or higher (operator-driven action).
// Applied at the route in router.go.
//
// Body: {vendor, total_cents, currency_code, rfq_id?, reasoning?}.
//   - vendor: required, non-empty.
//   - total_cents: required, non-negative.
//   - currency_code: required, USD or CAD (Composite Currency Pattern).
//   - rfq_id: optional; uuid.Nil when omitted (AI-driven flow with no formal RFQ).
//   - reasoning: optional AI narrative.
//
// Errors:
//
//   - 400 VALIDATION_ERROR: invalid project_id / item_id / JSON body /
//     vendor empty, negative total_cents, unsupported currency_code →
//     ErrInvalidInput.
//   - 404 NOT_FOUND: item missing or belongs to another org →
//     ErrProcurementItemNotFound. (Cross-org item access surfaces as
//     404 by design — an attacker probing for existence can't
//     distinguish "no such item" from "item in different org".)
//   - 503 SERVICE_UNAVAILABLE: ProcurementService constructed without
//     the feed-card store (worker binary path) →
//     ErrVendorReviewUnavailable.
//
// On success: 201 Created with {feed_card_id} — the id of the feed
// card the operator will see and action.
func (h *ProcurementHandler) RequestVendorReview(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDFromURL(w, r, "projectID"); !ok {
		return
	}
	itemID, ok := parseUUIDFromURL(w, r, "itemID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var body requestVendorReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	rfqID := uuid.Nil
	if body.RFQID != nil {
		rfqID = *body.RFQID
	}

	claims := mw.MustClaimsFromContext(r.Context())
	feedCardID, err := h.svc.RequestVendorReview(r.Context(), callerOrg, claims.Sub, service.RequestVendorReviewInput{
		ProcurementItemID: itemID,
		RFQID:             rfqID,
		Vendor:            body.Vendor,
		TotalCents:        body.TotalCents,
		CurrencyCode:      body.CurrencyCode,
		Reasoning:         body.Reasoning,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"feed_card_id": feedCardID})
}

// writeServiceError maps ProcurementService sentinels to HTTP responses.
func (h *ProcurementHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	// ProcurementService nil-dep sentinel — RequestVendorReview
	// returns this when the service was constructed without a
	// feed-card store (worker binary path). Map to 503 so callers know
	// to retry against a server binary rather than treating it as
	// a permanent input error.
	if errors.Is(err, service.ErrVendorReviewUnavailable) {
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "vendor review flow not available on this binary")
		return
	}
	switch {
	case errors.Is(err, service.ErrProcurementItemNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "procurement item not found")
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "project not found")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// parseOptionalRFC3339 differs from parseOptionalDate (financials.go) in
// that it ONLY accepts full RFC 3339 timestamps. Used for ordered_at,
// which carries a timestamp not just a date.
func parseOptionalRFC3339(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
