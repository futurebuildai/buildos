package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

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
	EventReviewMaterialQuote   = "review_material_quote"
	EventReviewLaborBid        = "review_labor_bid"
	EventUpdateSchedule        = "update_schedule"
	EventDeliveryConfirmation  = "delivery_confirmation"
	EventCreateFeedCard        = "create_feed_card"
	EventLocalblueLeadCaptured = "localblue.lead_captured"
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
// BuildOS domain actions. Today most events land as feed_cards; the
// localblue.lead_captured event additionally inserts a prospect row.
// Future PRs add specialized handlers (e.g., update_schedule will
// trigger a CPM recalc job in addition to the feed card).
type A2AService struct {
	pool          *pgxpool.Pool
	a2aStore      *store.A2AStore
	feedStore     *store.FeedCardsStore
	pipelineStore *store.PipelineStore
	defaultOrgID  *uuid.UUID
}

// NewA2AService constructs the service. defaultOrgID is the fallback
// org_id used when an envelope arrives without one (single-tenant fork
// mode). Pass nil to require Brain-supplied org_id on every event.
// pipelineStore is required for the localblue.lead_captured handler;
// pass nil if that event won't be received (other handlers don't need it).
func NewA2AService(pool *pgxpool.Pool, a2aStore *store.A2AStore, feedStore *store.FeedCardsStore, pipelineStore *store.PipelineStore, defaultOrgID *uuid.UUID) *A2AService {
	return &A2AService{
		pool:          pool,
		a2aStore:      a2aStore,
		feedStore:     feedStore,
		pipelineStore: pipelineStore,
		defaultOrgID:  defaultOrgID,
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
// Each handler parses its payload, builds a feed card (and other domain
// rows when applicable), and inserts. Unknown event types return
// ErrUnknownEvent so the caller can 400.
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
	case EventLocalblueLeadCaptured:
		return s.handleLocalblueLeadCaptured(ctx, tx, orgID, env.Payload)
	default:
		return uuid.Nil, fmt.Errorf("%w: %q", ErrUnknownEvent, env.EventType)
	}
}

// ---------- handlers ----------

// materialQuoteLineItem mirrors Brain's a2a.MaterialQuoteLineItem
// (see futurebuild-brain/internal/a2a/types.go). LineItems are
// optional on the wire today — Brain may emit a top-level total only,
// or the total plus an itemized breakdown.
type materialQuoteLineItem struct {
	Name           string `json:"name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	CurrencyCode   string `json:"currency_code"`
}

type reviewMaterialQuotePayload struct {
	RFQID        uuid.UUID               `json:"rfq_id"`
	TotalCents   int64                   `json:"total_cents"`
	CurrencyCode string                  `json:"currency_code"`
	Vendor       string                  `json:"vendor"`
	LineItems    []materialQuoteLineItem `json:"line_items,omitempty"`
}

func (s *A2AService) handleReviewMaterialQuote(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p reviewMaterialQuotePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: review_material_quote payload: %v", ErrInvalidInput, err)
	}
	if p.Vendor == "" || p.CurrencyCode == "" {
		return uuid.Nil, fmt.Errorf("%w: review_material_quote requires vendor + currency_code", ErrInvalidInput)
	}
	// Each line item must share the envelope-level currency_code. Mixed
	// currencies inside a single quote would violate the composite-
	// currency invariant (`cents` paired with `currency_code` at one
	// scope) and downstream aggregation has no way to interpret. Also
	// guard against negative quantity / unit_price_cents from a buggy
	// Brain emitter — those would survive JSON decode silently and
	// corrupt the feed-card preview.
	for i, li := range p.LineItems {
		if li.CurrencyCode != p.CurrencyCode {
			return uuid.Nil, fmt.Errorf("%w: review_material_quote line_items[%d] currency_code=%q mismatches envelope currency_code=%q", ErrInvalidInput, i, li.CurrencyCode, p.CurrencyCode)
		}
		if li.Quantity < 0 {
			return uuid.Nil, fmt.Errorf("%w: review_material_quote line_items[%d] quantity must be >= 0", ErrInvalidInput, i)
		}
		if li.UnitPriceCents < 0 {
			return uuid.Nil, fmt.Errorf("%w: review_material_quote line_items[%d] unit_price_cents must be >= 0", ErrInvalidInput, i)
		}
	}

	body := formatMoney(p.TotalCents, p.CurrencyCode)
	if n := len(p.LineItems); n > 0 {
		body += fmt.Sprintf(" · %d %s", n, pluralize("item", n))
	}

	role := "owner"
	card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
		OrgID:      orgID,
		CardType:   "procurement.material_quote",
		Title:      "Review material quote from " + p.Vendor,
		Body:       body,
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
	AIAnalysis   string    `json:"ai_analysis,omitempty"`
}

// aiAnalysisPreviewMaxRunes caps the analysis preview rendered in the
// feed-card body. Cards are notification surfaces, not the
// authoritative view of the analysis — the full text round-trips
// through a2a_inbound_log.payload for any caller that needs it.
const aiAnalysisPreviewMaxRunes = 200

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
	if p.AIAnalysis != "" {
		body += " · analysis: " + truncateRunes(p.AIAnalysis, aiAnalysisPreviewMaxRunes)
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

// Defensive bounds on update_schedule fields. Brain's emit path does
// not validate format (DeliveryDate is whatever the AI Maestro task
// returned), so the receiver must cap unbounded strings/arrays before
// they land in feed_cards body text or risk inflating notification UI.
const (
	updateScheduleDeliveryDateMaxLen = 64
	updateScheduleWBSCodeMaxLen      = 64
	updateScheduleWBSCodesMax        = 256
)

func (s *A2AService) handleUpdateSchedule(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p updateSchedulePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: update_schedule payload: %v", ErrInvalidInput, err)
	}
	if len(p.DeliveryDate) > updateScheduleDeliveryDateMaxLen {
		return uuid.Nil, fmt.Errorf("%w: update_schedule delivery_date exceeds %d bytes", ErrInvalidInput, updateScheduleDeliveryDateMaxLen)
	}
	if len(p.Constraints.WBSCodes) > updateScheduleWBSCodesMax {
		return uuid.Nil, fmt.Errorf("%w: update_schedule wbs_codes count %d exceeds limit %d", ErrInvalidInput, len(p.Constraints.WBSCodes), updateScheduleWBSCodesMax)
	}
	for i, code := range p.Constraints.WBSCodes {
		if len(code) > updateScheduleWBSCodeMaxLen {
			return uuid.Nil, fmt.Errorf("%w: update_schedule wbs_codes[%d] exceeds %d bytes", ErrInvalidInput, i, updateScheduleWBSCodeMaxLen)
		}
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
	// recalculation kicks off automatically. Blocked on Brain wire
	// shape — `UpdateSchedulePayload` currently does not carry the
	// project_id this fork would need to scope the recalc to one
	// project. Tracked at ADR-003 backlog. For now the card prompts a
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

// Defensive cap on convergence_status to keep feed-card body text
// bounded. Brain's emit side does not enforce a vocabulary today, so
// the receiver clamps length; an empty string is normalized to
// "in_progress" below.
const deliveryConfirmationStatusMaxLen = 64

func (s *A2AService) handleDeliveryConfirmation(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	var p deliveryConfirmationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: delivery_confirmation payload: %v", ErrInvalidInput, err)
	}
	if len(p.ConvergenceStatus) > deliveryConfirmationStatusMaxLen {
		return uuid.Nil, fmt.Errorf("%w: delivery_confirmation convergence_status exceeds %d bytes", ErrInvalidInput, deliveryConfirmationStatusMaxLen)
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

	// target_role is optional on the wire (ADR-003 P1; Brain-side
	// Stage 12 added it as `json:"target_role,omitempty"`). Empty
	// falls back to "owner" — that's the legacy default before Brain
	// could express a role. A NON-empty value that isn't in the
	// BuildOS RBAC vocabulary is a wire-shape violation: reject with
	// ErrInvalidInput so the surrounding tx rolls back, Brain's
	// at-least-once retry can succeed once the upstream payload is
	// corrected, and the typo doesn't silently get muted to "owner".
	var rolePtr *string
	if p.Role != "" {
		if !isAllowedTargetRole(p.Role) {
			return uuid.Nil, fmt.Errorf("%w: create_feed_card target_role=%q must be one of owner|admin|superintendent|field_worker", ErrInvalidInput, p.Role)
		}
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

// ---------- LocalBlue lead handler ----------

// localblueLeadCapturedPayload mirrors Brain's LocalblueLeadCapturedPayload
// (see futurebuild-brain/internal/a2a/types.go). The contractor org id
// lives on the payload because LocalBlue knows which contractor's site
// captured the lead via the embedded chatbot's site_id binding.
type localblueLeadCapturedPayload struct {
	LeadID          uuid.UUID `json:"lead_id"`
	ContractorOrgID uuid.UUID `json:"contractor_org_id"`
	LeadName        string    `json:"lead_name"`
	ContactName     string    `json:"contact_name"`
	ContactEmail    string    `json:"contact_email,omitempty"`
	ContactPhone    string    `json:"contact_phone,omitempty"`
	Address         string    `json:"address,omitempty"`
	EstimatedGSF    *int      `json:"estimated_gsf,omitempty"`
	Source          string    `json:"source,omitempty"`
	CapturedAt      time.Time `json:"captured_at"`
}

func (s *A2AService) handleLocalblueLeadCaptured(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, payload []byte) (uuid.UUID, error) {
	if s.pipelineStore == nil {
		return uuid.Nil, fmt.Errorf("%w: A2AService constructed without pipelineStore; localblue.lead_captured handler unavailable", ErrInvalidInput)
	}
	var p localblueLeadCapturedPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, fmt.Errorf("%w: localblue.lead_captured payload: %v", ErrInvalidInput, err)
	}
	if p.LeadName == "" || p.ContactName == "" {
		return uuid.Nil, fmt.Errorf("%w: localblue.lead_captured requires lead_name + contact_name", ErrInvalidInput)
	}

	// Insert the prospect at stage='LEAD'. Source is annotated as
	// "localblue:<orig source>" so the contractor's pipeline analytics
	// can attribute leads to the platform.
	source := localblueSourceTag(p.Source)
	prospect, err := s.pipelineStore.CreateProspect(ctx, tx, store.CreateProspectParams{
		OrgID:       orgID,
		Name:        p.LeadName,
		ClientName:  p.ContactName,
		ClientEmail: stringPtrOrNil(p.ContactEmail),
		ClientPhone: stringPtrOrNil(p.ContactPhone),
		Address:     stringPtrOrNil(p.Address),
		GSF:         p.EstimatedGSF,
		Source:      &source,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create prospect from localblue lead: %w", err)
	}
	_ = prospect // prospect.ID is in the dedup log via the raw payload + trace_id

	// Surface the new lead to the owner immediately. Urgent priority —
	// fast follow-up on inbound leads is the contractor's competitive edge.
	role := "owner"
	body := fmt.Sprintf("Lead: %s · Contact: %s", p.LeadName, p.ContactName)
	if p.ContactEmail != "" {
		body += " · " + p.ContactEmail
	}
	if p.ContactPhone != "" {
		body += " · " + p.ContactPhone
	}
	card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
		OrgID:      orgID,
		CardType:   "pipeline.lead_captured",
		Title:      "New lead from LocalBlue",
		Body:       body,
		Priority:   models.FeedPriorityUrgent,
		TargetRole: &role,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("emit lead feed card: %w", err)
	}
	return card.ID, nil
}

// stringPtrOrNil promotes a string to a pointer, mapping "" → nil so
// nullable columns receive SQL NULL rather than an empty string.
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// localblueSourceTag annotates the prospect's source field with a
// "localblue:" prefix so the contractor's pipeline analytics can
// distinguish LocalBlue-sourced leads from referrals/walk-ins.
func localblueSourceTag(orig string) string {
	if orig == "" {
		return "localblue"
	}
	return "localblue:" + orig
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
	// Per-event extractors — some events carry orgID inside the payload
	// (LocalBlue knows the contractor org via its chatbot's site_id
	// binding; the lead-capture envelope intentionally has no OrgID).
	if env.EventType == EventLocalblueLeadCaptured {
		if id, ok := orgIDFromLocalblueLead(env.Payload); ok {
			return id, nil
		}
		return uuid.Nil, fmt.Errorf("%w: localblue.lead_captured payload missing contractor_org_id", ErrInvalidInput)
	}
	if s.defaultOrgID != nil {
		return *s.defaultOrgID, nil
	}
	return uuid.Nil, fmt.Errorf("%w: envelope has no org_id and no DEFAULT_ORG_ID configured", ErrInvalidInput)
}

// orgIDFromLocalblueLead does a tiny pre-parse of the payload to pull
// just the contractor_org_id out for tenant routing. The full payload
// is re-parsed inside the handler — that's two parses but the json is
// small and this keeps the org-resolution decision in one place.
func orgIDFromLocalblueLead(payload []byte) (uuid.UUID, bool) {
	var p struct {
		ContractorOrgID uuid.UUID `json:"contractor_org_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return uuid.Nil, false
	}
	return p.ContractorOrgID, p.ContractorOrgID != uuid.Nil
}

// formatMoney is a quick display helper for feed-card body text. NOT a
// canonical money formatter (no thousands separators, no symbol). The
// frontend renders proper currency strings — this is the bare-minimum
// the contractor sees in a notification preview.
func formatMoney(cents int64, code string) string {
	return fmt.Sprintf("%.2f %s", float64(cents)/100.0, code)
}

// truncateRunes returns s if it fits in n runes; otherwise returns the
// first n runes followed by a single-rune ellipsis. Rune-aware so we
// never split a multi-byte UTF-8 character.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "…"
}

// pluralize returns the singular form for n == 1 and base+"s" otherwise.
// Tiny helper kept local because the only caller is the material-quote
// line-item preview.
func pluralize(base string, n int) string {
	if n == 1 {
		return base
	}
	return base + "s"
}

// allowedTargetRoles is the BuildOS RBAC role vocabulary that
// create_feed_card events may target. Mirrors the constants in
// internal/api/middleware/rbac.go (RoleOwner/RoleAdmin/...). Kept as a
// local copy rather than imported because middleware is an inbound-HTTP
// concern and the service layer must not depend on it.
var allowedTargetRoles = map[string]struct{}{
	"owner":          {},
	"admin":          {},
	"superintendent": {},
	"field_worker":   {},
}

func isAllowedTargetRole(s string) bool {
	_, ok := allowedTargetRoles[s]
	return ok
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
