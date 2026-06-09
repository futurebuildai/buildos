package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/ai"
	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
)

// InvoiceIngestor is the narrow surface the IngestHandler consumes from
// *service.IngestionService. Defined as an interface (mirrors
// BudgetServicer) so the handler is unit-testable against a fake without a
// database or an HTTP-backed AI client. Optional in the router — when the
// service is nil the /ingest route does not mount.
type InvoiceIngestor interface {
	IngestInvoiceFromDocument(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in service.IngestInvoiceInput) (service.IngestInvoiceResult, error)
}

// IngestHandler handles POST /api/v1/projects/{projectID}/invoices/ingest —
// the Phase 2a fuzzy→exact invoice ingestion entry point.
type IngestHandler struct {
	svc InvoiceIngestor
}

// NewIngestHandler creates a handler bound to the given ingestion service.
func NewIngestHandler(svc InvoiceIngestor) *IngestHandler {
	return &IngestHandler{svc: svc}
}

// ingestInvoiceRequest is the 2a JSON body (§5.3). DocumentURL XOR Text —
// the handler enforces exactly one is set (400 otherwise).
type ingestInvoiceRequest struct {
	IdempotencyKey string  `json:"idempotency_key"`
	DocumentURL    string  `json:"document_url,omitempty"`
	Text           string  `json:"text,omitempty"`
	CurrencyCode   *string `json:"currency_code,omitempty"`
	WBSCode        *string `json:"wbs_code,omitempty"`
}

// IngestInvoice runs the ingestion pipeline for one document.
// POST /api/v1/projects/{projectID}/invoices/ingest (owner/admin).
//
// Error → HTTP mapping (§5.4):
//   - success                            → 201 { data: { invoice, review_card } }
//   - store.ErrIdempotencyConflict       → 409 (bare error body, no record echo)
//   - service.ErrInvoiceExtractionInvalid → 422 (also covers unsupported media)
//   - service.ErrNotFound                → 404 (cross-tenant project)
//   - ai.ErrUnconfigured                 → 503 (pipeline degraded, nothing written)
//   - other (AI transport)               → 502
//   - bad JSON / key / XOR               → 400
func (h *IngestHandler) IngestInvoice(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var body ingestInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	// idempotency_key must be present and parse as a UUID.
	idempotencyKey, err := uuid.Parse(body.IdempotencyKey)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "idempotency_key must be a valid UUID")
		return
	}

	// Exactly one of document_url / text. Neither-or-both → 400.
	hasURL := body.DocumentURL != ""
	hasText := body.Text != ""
	if hasURL == hasText {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "exactly one of document_url or text is required")
		return
	}

	claims := mw.MustClaimsFromContext(r.Context())
	result, err := h.svc.IngestInvoiceFromDocument(r.Context(), callerOrg, claims.Sub, service.IngestInvoiceInput{
		ProjectID:        projectID,
		IdempotencyKey:   idempotencyKey,
		DocumentURL:      body.DocumentURL,
		Text:             body.Text,
		CurrencyOverride: body.CurrencyCode,
		WBSCode:          body.WBSCode,
	})
	if err != nil {
		h.writeIngestError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{
		"invoice":     result.Invoice,
		"review_card": result.ReviewCard,
	})
}

// writeIngestError maps an ingestion-pipeline error to an HTTP response per
// §5.4. Ordered most-specific first.
func (h *IngestHandler) writeIngestError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrIdempotencyConflict):
		// 409 bare — matches the field-sync idempotency contract (§6):
		// no record echo, since the conflicting tx rolled back and we
		// never read the prior row.
		writeErrorResponse(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key already used")
	case errors.Is(err, service.ErrInvoiceExtractionInvalid):
		// 422 — fuzzy AI output failed the deterministic gate, or the
		// document was an unsupported media type.
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "EXTRACTION_INVALID", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		// 422 — the defensive createInvoiceTx chokepoint rejected the
		// extracted content (bad currency / empty vendor / non-positive
		// total): the same "unprocessable extracted content" class as the
		// gate above, not a malformed client request.
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "EXTRACTION_INVALID", err.Error())
	case errors.Is(err, service.ErrNotFound):
		// 404 — project not in caller's org (cross-tenant guard).
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, ai.ErrUnconfigured):
		// 503 — no Anthropic key for this org; pipeline degraded,
		// nothing written.
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "AI_UNCONFIGURED", "AI extraction is not configured")
	default:
		// AI transport error (timeout, 5xx, circuit open) → 502.
		writeErrorResponse(w, r, http.StatusBadGateway, "AI_UPSTREAM_ERROR", "AI extraction upstream error")
	}
}
