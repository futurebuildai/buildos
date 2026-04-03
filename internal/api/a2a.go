package api

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

// A2AHandler handles the POST /api/v1/a2a/webhook endpoint.
// This endpoint uses JWS detached signature verification (NOT JWT Bearer auth).
type A2AHandler struct {
	jwks     *mw.JWKSProvider
	a2aStore *store.A2AStore
	feedSvc  *service.FeedService
	procSvc  *service.ProcurementService
	logger   *slog.Logger
}

// NewA2AHandler creates a handler for A2A webhook events from FB-Brain.
func NewA2AHandler(jwks *mw.JWKSProvider, a2aStore *store.A2AStore, feedSvc *service.FeedService, procSvc *service.ProcurementService, logger *slog.Logger) *A2AHandler {
	return &A2AHandler{
		jwks:     jwks,
		a2aStore: a2aStore,
		feedSvc:  feedSvc,
		procSvc:  procSvc,
		logger:   logger,
	}
}

// a2aWebhookPayload represents the incoming webhook body from FB-Brain.
type a2aWebhookPayload struct {
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	TraceID        string          `json:"trace_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Timestamp      string          `json:"timestamp"`
	Issuer         string          `json:"iss"`
}

// Supported event types from FB-Brain A2A webhooks.
const (
	EventReviewMaterialQuote  = "review_material_quote"
	EventReviewLaborBid       = "review_labor_bid"
	EventUpdateSchedule       = "update_schedule"
	EventDeliveryConfirmation = "delivery_confirmation"
	EventCreateFeedCard       = "create_feed_card"
)

// ReceiveWebhook processes A2A webhook events from FB-Brain.
// POST /api/v1/a2a/webhook
//
// Auth: JWS detached signature via X-JWS-Signature header.
// This route BYPASSES the standard JWT middleware.
func (h *A2AHandler) ReceiveWebhook(w http.ResponseWriter, r *http.Request) {
	// Read the raw body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Extract JWS signature header
	jwsSig := r.Header.Get("X-JWS-Signature")
	if jwsSig == "" {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-JWS-Signature header")
		return
	}

	// Extract idempotency key
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "missing X-Idempotency-Key header")
		return
	}

	// Verify JWS detached signature
	if err := h.verifyJWSSignature(r.Context(), body, jwsSig); err != nil {
		h.logger.Warn("JWS verification failed", "error", err, "trace_id", r.Header.Get("X-Trace-ID"))
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid JWS signature")
		return
	}

	// Check idempotency — reject duplicates with 409
	isDuplicate, err := h.a2aStore.CheckIdempotencyKey(r.Context(), idempotencyKey)
	if err != nil {
		h.logger.Error("idempotency check failed", "error", err)
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "idempotency check failed")
		return
	}
	if isDuplicate {
		writeErrorResponse(w, r, http.StatusConflict, "DUPLICATE", "idempotency key already processed")
		return
	}

	// Parse the webhook payload
	var payload a2aWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	// Log the webhook for idempotency
	_, err = h.a2aStore.LogWebhook(r.Context(), &store.A2AWebhookLog{
		IdempotencyKey: idempotencyKey,
		EventType:      payload.EventType,
		Payload:        payload.Payload,
		TraceID:        payload.TraceID,
		Issuer:         payload.Issuer,
		Status:         "processed",
	})
	if err != nil {
		h.logger.Error("failed to log webhook", "error", err)
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to log webhook")
		return
	}

	// Route to event-specific handler
	if err := h.routeEvent(r.Context(), &payload); err != nil {
		h.logger.Error("event handler failed",
			"event_type", payload.EventType,
			"trace_id", payload.TraceID,
			"error", err,
		)
		// Still return 200 — the webhook was received and logged.
		// Failures are logged for investigation.
	}

	h.logger.Info("a2a webhook processed",
		"event_type", payload.EventType,
		"trace_id", payload.TraceID,
		"idempotency_key", payload.IdempotencyKey,
	)

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted"})
}

// routeEvent dispatches the webhook payload to the appropriate handler.
func (h *A2AHandler) routeEvent(ctx context.Context, payload *a2aWebhookPayload) error {
	switch payload.EventType {
	case EventReviewMaterialQuote:
		return h.handleMaterialQuote(ctx, payload)
	case EventReviewLaborBid:
		return h.handleLaborBid(ctx, payload)
	case EventUpdateSchedule:
		return h.handleUpdateSchedule(ctx, payload)
	case EventDeliveryConfirmation:
		return h.handleDeliveryConfirmation(ctx, payload)
	case EventCreateFeedCard:
		return h.handleCreateFeedCard(ctx, payload)
	default:
		h.logger.Warn("unknown event type", "event_type", payload.EventType)
		return nil // Don't fail on unknown events — forward compatibility
	}
}

// --- Event Handlers ---

// materialQuotePayload matches Brain's review_material_quote event.
type materialQuotePayload struct {
	RFQID        string              `json:"rfq_id"`
	LineItems    []materialLineItem  `json:"line_items"`
	TotalCents   int64               `json:"total_cents"`
	CurrencyCode string              `json:"currency_code"`
	Vendor       string              `json:"vendor"`
}

type materialLineItem struct {
	Name           string `json:"name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	CurrencyCode   string `json:"currency_code"`
}

func (h *A2AHandler) handleMaterialQuote(ctx context.Context, webhook *a2aWebhookPayload) error {
	var p materialQuotePayload
	if err := json.Unmarshal(webhook.Payload, &p); err != nil {
		return fmt.Errorf("parsing material quote payload: %w", err)
	}

	// Build a descriptive body with currency
	body := fmt.Sprintf("Quote from %s — Total: %d cents %s (%d items)",
		p.Vendor, p.TotalCents, p.CurrencyCode, len(p.LineItems))

	actions, _ := json.Marshal([]map[string]any{
		{"label": "Review Quote", "action_type": "open_quote", "payload": map[string]string{"rfq_id": p.RFQID}},
		{"label": "Dismiss", "action_type": "dismiss"},
	})

	// Use a default org for system-generated cards
	orgID := defaultOrgID()

	card := &models.FeedCard{
		OrgID:    orgID,
		CardType: models.CardTypeProcurement,
		Title:    fmt.Sprintf("Material Quote Ready: %s", p.Vendor),
		Body:     body,
		Priority: models.PriorityUrgent,
		Actions:  actions,
		Status:   models.FeedStatusActive,
	}

	_, err := h.feedSvc.CreateCard(ctx, card)
	return err
}

// laborBidPayload matches Brain's review_labor_bid event.
type laborBidPayload struct {
	RFQID        string `json:"rfq_id"`
	Bidder       string `json:"bidder"`
	AmountCents  int64  `json:"amount_cents"`
	CurrencyCode string `json:"currency_code"`
	Timeline     string `json:"timeline"`
	AIAnalysis   string `json:"ai_analysis"`
}

func (h *A2AHandler) handleLaborBid(ctx context.Context, webhook *a2aWebhookPayload) error {
	var p laborBidPayload
	if err := json.Unmarshal(webhook.Payload, &p); err != nil {
		return fmt.Errorf("parsing labor bid payload: %w", err)
	}

	body := fmt.Sprintf("Bid from %s — %d cents %s, Timeline: %s. AI: %s",
		p.Bidder, p.AmountCents, p.CurrencyCode, p.Timeline, p.AIAnalysis)

	actions, _ := json.Marshal([]map[string]any{
		{"label": "Review Bid", "action_type": "open_bid", "payload": map[string]string{"rfq_id": p.RFQID}},
		{"label": "Dismiss", "action_type": "dismiss"},
	})

	orgID := defaultOrgID()

	card := &models.FeedCard{
		OrgID:    orgID,
		CardType: "labor_bid",
		Title:    fmt.Sprintf("Labor Bid: %s", p.Bidder),
		Body:     body,
		Priority: models.PriorityUrgent,
		Actions:  actions,
		Status:   models.FeedStatusActive,
	}

	_, err := h.feedSvc.CreateCard(ctx, card)
	return err
}

// updateSchedulePayload matches Brain's update_schedule event.
type updateSchedulePayload struct {
	EventType    string   `json:"event_type"`
	DeliveryDate string   `json:"delivery_date"`
	Constraints  struct {
		WBSCodes []string `json:"wbs_codes"`
	} `json:"constraints"`
}

func (h *A2AHandler) handleUpdateSchedule(ctx context.Context, webhook *a2aWebhookPayload) error {
	var p updateSchedulePayload
	if err := json.Unmarshal(webhook.Payload, &p); err != nil {
		return fmt.Errorf("parsing update_schedule payload: %w", err)
	}

	// Create a feed card notifying the schedule change
	body := fmt.Sprintf("Schedule update: %s delivery on %s affecting WBS codes: %v",
		p.EventType, p.DeliveryDate, p.Constraints.WBSCodes)

	orgID := defaultOrgID()

	card := &models.FeedCard{
		OrgID:    orgID,
		CardType: "schedule_update",
		Title:    "Schedule Update from Brain",
		Body:     body,
		Priority: models.PriorityNormal,
		Status:   models.FeedStatusActive,
	}

	_, err := h.feedSvc.CreateCard(ctx, card)
	if err != nil {
		return fmt.Errorf("creating schedule update card: %w", err)
	}

	// TODO: In future sprints, trigger CPM recalculation via delay_cascade River job
	// This requires project ID resolution from WBS codes
	h.logger.Info("schedule update received, CPM recalculation deferred",
		"event_type", p.EventType,
		"delivery_date", p.DeliveryDate,
		"wbs_codes", p.Constraints.WBSCodes,
	)

	return nil
}

// deliveryConfirmationPayload matches Brain's delivery_confirmation event.
type deliveryConfirmationPayload struct {
	MaterialsOrdered   bool   `json:"materials_ordered"`
	LaborApproved      bool   `json:"labor_approved"`
	ConvergenceStatus  string `json:"convergence_status"`
}

func (h *A2AHandler) handleDeliveryConfirmation(ctx context.Context, webhook *a2aWebhookPayload) error {
	var p deliveryConfirmationPayload
	if err := json.Unmarshal(webhook.Payload, &p); err != nil {
		return fmt.Errorf("parsing delivery_confirmation payload: %w", err)
	}

	body := fmt.Sprintf("Materials ordered: %t, Labor approved: %t, Status: %s",
		p.MaterialsOrdered, p.LaborApproved, p.ConvergenceStatus)

	priority := models.PriorityNormal
	if p.ConvergenceStatus == "complete" {
		priority = models.PriorityLow
	}

	orgID := defaultOrgID()

	card := &models.FeedCard{
		OrgID:    orgID,
		CardType: models.CardTypeProgress,
		Title:    "Delivery Confirmation",
		Body:     body,
		Priority: priority,
		Status:   models.FeedStatusActive,
	}

	_, err := h.feedSvc.CreateCard(ctx, card)
	return err
}

// createFeedCardPayload matches Brain's create_feed_card event.
type createFeedCardPayload struct {
	CardType string          `json:"card_type"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Actions  json.RawMessage `json:"actions,omitempty"`
	Priority string          `json:"priority"`
}

func (h *A2AHandler) handleCreateFeedCard(ctx context.Context, webhook *a2aWebhookPayload) error {
	var p createFeedCardPayload
	if err := json.Unmarshal(webhook.Payload, &p); err != nil {
		return fmt.Errorf("parsing create_feed_card payload: %w", err)
	}

	orgID := defaultOrgID()

	card := &models.FeedCard{
		OrgID:    orgID,
		CardType: p.CardType,
		Title:    p.Title,
		Body:     p.Body,
		Priority: p.Priority,
		Actions:  p.Actions,
		Status:   models.FeedStatusActive,
	}

	_, err := h.feedSvc.CreateCard(ctx, card)
	return err
}

// defaultOrgID returns the dev placeholder org ID for system-generated cards.
// In production, org context would be extracted from the webhook payload or JWT claims.
func defaultOrgID() uuid.UUID {
	id, _ := uuid.Parse("00000000-0000-0000-0000-000000000001")
	return id
}

// verifyJWSSignature verifies the detached JWS compact signature against the
// request body using FB-Brain's public key from JWKS.
func (h *A2AHandler) verifyJWSSignature(ctx context.Context, body []byte, jwsSig string) error {
	keySet, err := h.jwks.GetKeySet(ctx)
	if err != nil {
		return fmt.Errorf("fetching JWKS for JWS verification: %w", err)
	}

	// Parse the detached JWS — the signature has format "header..signature"
	jws, err := jose.ParseDetached(jwsSig, body, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return fmt.Errorf("parsing JWS detached signature: %w", err)
	}

	// Try each key in the JWKS
	for _, key := range keySet.Keys {
		if key.Use != "sig" && key.Use != "" {
			continue
		}
		pubKey, ok := key.Key.(*rsa.PublicKey)
		if !ok {
			continue
		}
		if _, err := jws.Verify(pubKey); err == nil {
			return nil // Verification succeeded
		}
	}

	return fmt.Errorf("no matching key verified the JWS signature")
}
