package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// A2A-specific sentinel errors. Reuses ErrInvalidInput / ErrNotFound
// from the budget service file (same service package).
var (
	// ErrUnknownEvent is returned when the envelope's event_type is not
	// one of the five Brain-emitted types this service knows how to
	// handle. Surface as 400 VALIDATION_ERROR.
	ErrUnknownEvent = errors.New("a2a: unknown event_type")
)

// WebhookEnvelope mirrors Brain's a2a.WebhookEvent struct (see
// futurebuild-brain/internal/a2a/types.go) plus an OrgID field that the
// service requires for tenant routing.
//
// Brain currently does NOT populate OrgID; the matching change ships in
// the cross-repo PR after this one. Until then, the receiver falls back
// to A2AService.defaultOrgID (single-tenant fork mode). If neither is
// available the service returns ErrInvalidInput with a clear message.
type WebhookEnvelope struct {
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	TraceID        string          `json:"trace_id"`
	IdempotencyKey uuid.UUID       `json:"idempotency_key"`
	Timestamp      time.Time       `json:"timestamp"`
	Issuer         string          `json:"iss"`
	OrgID          uuid.UUID       `json:"org_id,omitempty"` // ADDED — Brain coordinated change pending
}

// Webhook event types — must match the constants in
// futurebuild-brain/internal/a2a/types.go.
const (
	EventReviewMaterialQuote  = "review_material_quote"
	EventReviewLaborBid       = "review_labor_bid"
	EventUpdateSchedule       = "update_schedule"
	EventDeliveryConfirmation = "delivery_confirmation"
	EventCreateFeedCard       = "create_feed_card"
)

// ProcessResult is the outcome of A2AService.ProcessWebhook. Both an
// already-processed event and a newly-handled event are 200-success
// outcomes — the caller (HTTP handler) returns 200 in both cases.
// AlreadyProcessed=true gives observability that the source retried.
type ProcessResult struct {
	AlreadyProcessed bool
	FeedCardID       *uuid.UUID
	EventType        string
}

// A2AService dispatches verified webhook events from The Brain into
// BuildOS domain actions. Today every event lands in feed_cards; future
// PRs add specialized handlers (e.g., update_schedule will trigger a
// CPM recalc job).
type A2AService struct {
	pool         *pgxpool.Pool
	a2aStore     *store.A2AStore
	feedStore    *store.FeedCardsStore
	defaultOrgID *uuid.UUID
}

// NewA2AService constructs the service. defaultOrgID is the fallback
// org_id used when an envelope arrives without one (single-tenant fork
// mode). Pass nil to require Brain-supplied org_id on every event.
func NewA2AService(pool *pgxpool.Pool, a2aStore *store.A2AStore, feedStore *store.FeedCardsStore, defaultOrgID *uuid.UUID) *A2AService {
	return &A2AService{
		pool:         pool,
		a2aStore:     a2aStore,
		feedStore:    feedStore,
		defaultOrgID: defaultOrgID,
	}
}

// ProcessWebhook dedups the envelope and dispatches to a typed handler.
// The whole operation runs in a single transaction — if anything fails
// (dedup insert, payload parse, feed-card insert), nothing commits, so
// Brain's at-least-once retry semantics work correctly.
//
// Returns AlreadyProcessed=true when the idempotency_key was already
// present; the inner dispatch is skipped.
func (s *A2AService) ProcessWebhook(ctx context.Context, env WebhookEnvelope) (ProcessResult, error) {
	if err := s.validateEnvelope(env); err != nil {
		return ProcessResult{}, err
	}
	orgID, err := s.resolveOrgID(env)
	if err != nil {
		return ProcessResult{}, err
	}

	out := ProcessResult{EventType: env.EventType}
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		alreadyProcessed, err := s.a2aStore.InsertInboundLog(ctx, tx, store.InsertInboundLogParams{
			IdempotencyKey: env.IdempotencyKey,
			EventType:      env.EventType,
			TraceID:        env.TraceID,
			Issuer:         env.Issuer,
			Payload:        env.Payload,
		})
		if err != nil {
			return err
		}
		if alreadyProcessed {
			out.AlreadyProcessed = true
			return nil
		}

		cardID, err := s.dispatch(ctx, tx, orgID, env)
		if err != nil {
			return err
		}
		out.FeedCardID = &cardID
		return nil
	})
	if err != nil {
		return ProcessResult{}, err
	}
	return out, nil
}

// dispatch routes the envelope to a typed handler based on event_type.
// Each handler parses its payload, builds a feed card, and inserts.
// Unknown event types return ErrUnknownEvent so the caller can 400.
func (s *A2AService) dispatch(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, env WebhookEnvelope) (uuid.UUID, error) {
	switch env.EventType {
	case EventReviewMaterialQuote:
		return s.handleReviewMaterialQuote(ctx, tx, orgID, env.Payload)
	case EventReviewLaborBid:
		return s.handleReviewLaborBid(ctx, tx, orgID, env.Payload)
	case EventUpdateSchedule:
		return s.handleUpdateSchedule(ctx, tx, orgID, env.Payload)
	case EventDeliveryConfirmation:
		return s.handleDeliveryConfirmation(ctx, tx, orgID, env.Payload)
	case EventCreateFeedCard:
		return s.handleCreateFeedCard(ctx, tx, orgID, env.Payload)
	default:
		return uuid.Nil, fmt.Errorf("%w: %q", ErrUnknownEvent, env.EventType)
	}
}

// ---------- handlers ----------

type reviewMaterialQuotePayload struct {
	RFQID        uuid.UUID `json:"rfq_id"`
	TotalCents   int64     `json:"total_cents"`
	CurrencyCode string    `json:"currency_code"`
	Vendor       string    `json:"vendor"`
}

func (s *A2AService) handleReviewMaterialQuote(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p reviewMaterialQuotePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: review_material_quote payload: %v", ErrInvalidInput, err)
	}
	if p.Vendor == "" || p.CurrencyCode == "" {
		return uuid.Nil, fmt.Errorf("%w: review_material_quote requires vendor + currency_code", ErrInvalidInput)
	}

	role := "owner"
	card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
		OrgID:      orgID,
		CardType:   "procurement.material_quote",
		Title:      "Review material quote from " + p.Vendor,
		Body:       formatMoney(p.TotalCents, p.CurrencyCode),
		Priority:   models.FeedPriorityUrgent,
		TargetRole: &role,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return card.ID, nil
}

type reviewLaborBidPayload struct {
	RFQID        uuid.UUID `json:"rfq_id"`
	Bidder       string    `json:"bidder"`
	AmountCents  int64     `json:"amount_cents"`
	CurrencyCode string    `json:"currency_code"`
	Timeline     string    `json:"timeline"`
}

func (s *A2AService) handleReviewLaborBid(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p reviewLaborBidPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: review_labor_bid payload: %v", ErrInvalidInput, err)
	}
	if p.Bidder == "" || p.CurrencyCode == "" {
		return uuid.Nil, fmt.Errorf("%w: review_labor_bid requires bidder + currency_code", ErrInvalidInput)
	}

	role := "owner"
	body := formatMoney(p.AmountCents, p.CurrencyCode)
	if p.Timeline != "" {
		body += " · timeline: " + p.Timeline
	}
	card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
		OrgID:      orgID,
		CardType:   "procurement.labor_bid",
		Title:      "Review labor bid from " + p.Bidder,
		Body:       body,
		Priority:   models.FeedPriorityUrgent,
		TargetRole: &role,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return card.ID, nil
}

type updateSchedulePayload struct {
	EventType    string `json:"event_type"`
	DeliveryDate string `json:"delivery_date"`
	Constraints  struct {
		WBSCodes []string `json:"wbs_codes"`
	} `json:"constraints"`
}

func (s *A2AService) handleUpdateSchedule(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p updateSchedulePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: update_schedule payload: %v", ErrInvalidInput, err)
	}

	role := "superintendent"
	body := "Brain requested a schedule update."
	if p.DeliveryDate != "" {
		body += " New delivery: " + p.DeliveryDate + "."
	}
	if len(p.Constraints.WBSCodes) > 0 {
		body += " Affected WBS: " + joinStrings(p.Constraints.WBSCodes, ", ") + "."
	}

	// Future PR: also enqueue a DelayCascade River job here so the CPM
	// recalculation kicks off automatically. For now the card prompts a
	// human to run /schedule/recalculate.
	card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
		OrgID:      orgID,
		CardType:   "schedule.update_requested",
		Title:      "Schedule update requested",
		Body:       body,
		Priority:   models.FeedPriorityNormal,
		TargetRole: &role,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return card.ID, nil
}

type deliveryConfirmationPayload struct {
	MaterialsOrdered  bool   `json:"materials_ordered"`
	LaborApproved     bool   `json:"labor_approved"`
	ConvergenceStatus string `json:"convergence_status"`
}

func (s *A2AService) handleDeliveryConfirmation(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p deliveryConfirmationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: delivery_confirmation payload: %v", ErrInvalidInput, err)
	}

	role := "superintendent"
	status := p.ConvergenceStatus
	if status == "" {
		status = "in_progress"
	}
	body := fmt.Sprintf("Materials %s · Labor %s · Convergence: %s",
		boolToStatus(p.MaterialsOrdered), boolToStatus(p.LaborApproved), status)

	card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
		OrgID:      orgID,
		CardType:   "procurement.delivery_confirmation",
		Title:      "Delivery + labor confirmation",
		Body:       body,
		Priority:   models.FeedPriorityNormal,
		TargetRole: &role,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return card.ID, nil
}

type createFeedCardPayload struct {
	CardType string          `json:"card_type"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Actions  json.RawMessage `json:"actions,omitempty"`
	Priority string          `json:"priority"`
	Role     string          `json:"target_role,omitempty"`
}

func (s *A2AService) handleCreateFeedCard(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p createFeedCardPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: create_feed_card payload: %v", ErrInvalidInput, err)
	}
	if p.Title == "" {
		return uuid.Nil, fmt.Errorf("%w: create_feed_card requires title", ErrInvalidInput)
	}
	if p.CardType == "" {
		p.CardType = "brain.generic"
	}
	priority := p.Priority
	if !models.IsValidFeedPriority(priority) {
		priority = models.FeedPriorityNormal
	}

	var rolePtr *string
	if p.Role != "" {
		rolePtr = &p.Role
	} else {
		role := "owner"
		rolePtr = &role
	}

	card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
		OrgID:      orgID,
		CardType:   p.CardType,
		Title:      p.Title,
		Body:       p.Body,
		Priority:   priority,
		TargetRole: rolePtr,
		Actions:    p.Actions,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return card.ID, nil
}

// ---------- helpers ----------

func (s *A2AService) validateEnvelope(env WebhookEnvelope) error {
	if env.EventType == "" {
		return fmt.Errorf("%w: event_type is required", ErrInvalidInput)
	}
	if env.IdempotencyKey == uuid.Nil {
		return fmt.Errorf("%w: idempotency_key is required", ErrInvalidInput)
	}
	if len(env.Payload) == 0 {
		return fmt.Errorf("%w: payload is required", ErrInvalidInput)
	}
	return nil
}

func (s *A2AService) resolveOrgID(env WebhookEnvelope) (uuid.UUID, error) {
	if env.OrgID != uuid.Nil {
		return env.OrgID, nil
	}
	if s.defaultOrgID != nil {
		return *s.defaultOrgID, nil
	}
	return uuid.Nil, fmt.Errorf("%w: envelope has no org_id and no DEFAULT_ORG_ID configured", ErrInvalidInput)
}

// formatMoney is a quick display helper for feed-card body text. NOT a
// canonical money formatter (no thousands separators, no symbol). The
// frontend renders proper currency strings — this is the bare-minimum
// the contractor sees in a notification preview.
func formatMoney(cents int64, code string) string {
	return fmt.Sprintf("%.2f %s", float64(cents)/100.0, code)
}

func boolToStatus(b bool) string {
	if b {
		return "ordered"
	}
	return "pending"
}

func joinStrings(ss []string, sep string) string {
	switch len(ss) {
	case 0:
		return ""
	case 1:
		return ss[0]
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += sep + s
	}
	return out
}
