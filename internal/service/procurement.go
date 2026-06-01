package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/currency"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

// Compile-time check that ProcurementService satisfies the worker
// package's interface. Catches signature drift at build time rather
// than at the first scheduled tick.
var _ worker.ProcurementChecker = (*ProcurementService)(nil)

// Sentinel errors specific to ProcurementService. ErrInvalidInput and
// ErrNotFound are reused from budget.go (the package-level sentinels
// shared across the service layer). ErrProcurementItemNotFound is
// kept distinct so handlers can surface a procurement-specific 404
// message if they want to.
var (
	// ErrProcurementItemNotFound is returned when an item lookup misses
	// (id + project_id + org_id mismatch). Mirrors the store sentinel.
	ErrProcurementItemNotFound = errors.New("procurement: item not found")

	// ErrAIUnavailable is returned by RecommendVendors when the
	// service was constructed without a ProcurementRecommender (e.g.
	// the worker binary that only runs RecomputeStatuses). Lets
	// handlers surface a clean 503 rather than panicking on a nil call.
	ErrAIUnavailable = errors.New("procurement: ai client not configured")

	// ErrVendorReviewUnavailable is returned by RequestVendorReview
	// when the service was constructed without a feed-card store. Same
	// "worker binary doesn't need it" rationale as ErrAIUnavailable —
	// only the API server triggers operator-driven review flows.
	ErrVendorReviewUnavailable = errors.New("procurement: vendor review feed not configured")
)

// ProcurementRecommender is the consumer-side interface
// ProcurementService needs from the native AI client. Defined here so
// tests can substitute a fake without spinning up an HTTP server, and
// so ProcurementService doesn't transitively pin the entire ai.Client
// surface.
//
// Wraps the typed procurement_recommend task dispatched natively to
// Anthropic (internal/ai). The per-org Anthropic key is resolved from
// the context (ai.ContextWithOrgID), which the caller sets before the
// call.
type ProcurementRecommender interface {
	ProcurementRecommend(ctx context.Context, req ai.ProcurementRecommendRequest) (*ai.ProcurementRecommendResponse, error)
}

// CreateProcurementItemInput is the validated input for Create. The
// service performs cross-org isolation, currency validation, and
// non-negative checks before opening a transaction.
type CreateProcurementItemInput struct {
	ProjectID                 uuid.UUID
	Name                      string
	WBSCode                   string
	EstimatedCostCents        int64
	EstimatedCostCurrencyCode string
	LeadTimeDays              int
	WeatherBufferDays         int
	VendorID                  *uuid.UUID
	NeedByDate                *time.Time
}

// UpdateProcurementItemInput is the validated input for Update. All
// fields are optional pointers — nil means "leave unchanged". A status
// transition to ORDERED requires PONumber to be non-empty (the human
// user shouldn't mark something ordered without a PO).
type UpdateProcurementItemInput struct {
	ItemID    uuid.UUID
	ProjectID uuid.UUID
	Status    *string
	PONumber  *string
	OrderedAt *time.Time
}

// ProcurementService is the business-logic surface for procurement
// items. Reads + writes flow through here; this service is the only
// writer of procurement_items.
type ProcurementService struct {
	pool        *pgxpool.Pool
	store       *store.ProcurementStore
	recommender ProcurementRecommender
	feedStore   *store.FeedCardsStore
	audit       AuditRecorder
}

// NewProcurementService creates a service bound to a pool + store.
// recommender may be nil — RecommendVendors then returns
// ErrAIUnavailable (worker-only deployments don't need it).
// feedStore may be nil — RequestVendorReview then returns
// ErrVendorReviewUnavailable (same worker-only rationale).
// audit may be nil; nil falls back to a no-op recorder.
func NewProcurementService(pool *pgxpool.Pool, items *store.ProcurementStore, recommender ProcurementRecommender, feedStore *store.FeedCardsStore, audit AuditRecorder) *ProcurementService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &ProcurementService{pool: pool, store: items, recommender: recommender, feedStore: feedStore, audit: audit}
}

// ListProcurement returns all items on a project visible to the caller's
// org. Cross-org access surfaces as ErrNotFound (we never leak existence
// across tenants).
func (s *ProcurementService) ListProcurement(ctx context.Context, projectID, callerOrgID uuid.UUID, statusFilter []string) ([]models.ProcurementItem, error) {
	if projectID == uuid.Nil || callerOrgID == uuid.Nil {
		return nil, fmt.Errorf("%w: project_id and caller org_id are required", ErrInvalidInput)
	}
	for _, st := range statusFilter {
		if !models.IsValidProcurementStatus(st) {
			return nil, fmt.Errorf("%w: unknown procurement status %q", ErrInvalidInput, st)
		}
	}

	var items []models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.ListProcurementItems(ctx, tx, store.ListProcurementItemsParams{
			ProjectID:    projectID,
			OrgID:        callerOrgID,
			StatusFilter: statusFilter,
		})
		if err != nil {
			return err
		}
		items = got
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("list procurement: %w", err)
	}
	return items, nil
}

// CreateProcurementItem inserts a new item with status='OK'. Validates:
//   - Name and WBSCode non-empty.
//   - EstimatedCostCents >= 0.
//   - LeadTimeDays >= 0, WeatherBufferDays >= 0.
//   - Currency code in the supported set (USD/CAD).
//   - ProjectID belongs to callerOrgID (cross-org isolation).
//
// callerUserSub is recorded on the audit row.
func (s *ProcurementService) CreateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in CreateProcurementItemInput) (models.ProcurementItem, error) {
	if callerOrgID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if in.ProjectID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Name) == "" {
		return models.ProcurementItem{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.WBSCode) == "" {
		return models.ProcurementItem{}, fmt.Errorf("%w: wbs_code is required", ErrInvalidInput)
	}
	if in.EstimatedCostCents < 0 {
		return models.ProcurementItem{}, fmt.Errorf("%w: estimated_cost_cents must be non-negative", ErrInvalidInput)
	}
	if in.LeadTimeDays < 0 || in.WeatherBufferDays < 0 {
		return models.ProcurementItem{}, fmt.Errorf("%w: lead_time_days and weather_buffer_days must be non-negative", ErrInvalidInput)
	}
	if err := currency.Validate(in.EstimatedCostCurrencyCode); err != nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var item models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.CreateProcurementItem(ctx, tx, store.CreateProcurementItemParams{
			ProjectID:                 in.ProjectID,
			OrgID:                     callerOrgID,
			Name:                      strings.TrimSpace(in.Name),
			WBSCode:                   strings.TrimSpace(in.WBSCode),
			EstimatedCostCents:        in.EstimatedCostCents,
			EstimatedCostCurrencyCode: in.EstimatedCostCurrencyCode,
			LeadTimeDays:              in.LeadTimeDays,
			WeatherBufferDays:         in.WeatherBufferDays,
			VendorID:                  in.VendorID,
			NeedByDate:                in.NeedByDate,
		})
		if err != nil {
			return err
		}
		item = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "procurement.item.created",
			ResourceType: AuditResourceProcurementItem,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"project_id": in.ProjectID,
				"wbs_code":   got.WBSCode,
				"cost_cents": got.EstimatedCostCents,
				"currency":   got.EstimatedCostCurrencyCode,
			}),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return models.ProcurementItem{}, ErrNotFound
		}
		return models.ProcurementItem{}, fmt.Errorf("create procurement item: %w", err)
	}
	return item, nil
}

// UpdateProcurementItem applies a partial update. Validates:
//   - At least one field is being changed.
//   - Status, if provided, is a valid value.
//   - Status transition to ORDERED requires non-empty PONumber.
//   - Item belongs to ProjectID belongs to callerOrgID.
//
// We don't enforce a strict status FSM (OK→WARNING→CRITICAL→ORDERED)
// because the agent overwrites time-based statuses on every tick — a
// brief stale-state read shouldn't block a human marking ORDERED.
//
// callerUserSub is recorded on the audit row.
func (s *ProcurementService) UpdateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in UpdateProcurementItemInput) (models.ProcurementItem, error) {
	if callerOrgID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if in.ItemID == uuid.Nil || in.ProjectID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: item_id and project_id are required", ErrInvalidInput)
	}
	if in.Status == nil && in.PONumber == nil && in.OrderedAt == nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: at least one updatable field is required", ErrInvalidInput)
	}
	if in.Status != nil {
		if !models.IsValidProcurementStatus(*in.Status) {
			return models.ProcurementItem{}, fmt.Errorf("%w: unknown procurement status %q", ErrInvalidInput, *in.Status)
		}
		if *in.Status == models.ProcurementStatusOrdered {
			if in.PONumber == nil || strings.TrimSpace(*in.PONumber) == "" {
				return models.ProcurementItem{}, fmt.Errorf("%w: po_number is required when transitioning to ORDERED", ErrInvalidInput)
			}
		}
	}

	var item models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.UpdateProcurementItem(ctx, tx, store.UpdateProcurementItemParams{
			ItemID:    in.ItemID,
			ProjectID: in.ProjectID,
			OrgID:     callerOrgID,
			Status:    in.Status,
			PONumber:  in.PONumber,
			OrderedAt: in.OrderedAt,
		})
		if err != nil {
			return err
		}
		item = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "procurement.item.updated",
			ResourceType: AuditResourceProcurementItem,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"status":     in.Status,
				"po_number":  in.PONumber,
				"ordered_at": in.OrderedAt,
			}),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrProcurementItemNotFound):
			return models.ProcurementItem{}, ErrProcurementItemNotFound
		case errors.Is(err, store.ErrNotFound):
			return models.ProcurementItem{}, ErrNotFound
		}
		return models.ProcurementItem{}, fmt.Errorf("update procurement item: %w", err)
	}
	return item, nil
}

// DefaultProcurementWarningWindowDays is the lead-time horizon at which
// an OK row flips to WARNING. Picked at 7 days based on the typical
// residential build cadence — far enough to react, close enough to be
// actionable. ProcurementCheckWorker scheduled daily; one full warning
// → critical → expedite cycle gives the contractor a ~7-day runway.
const DefaultProcurementWarningWindowDays = 7

// RecomputeStatuses runs the daily procurement health sweep, flipping
// every non-ORDERED row to OK / WARNING / CRITICAL per its
// must_order_date relative to today + the warning window. Returns the
// number of rows whose status actually changed (useful for
// observability — a healthy fleet should mostly produce zero or one
// per day; thousands of transitions in one tick is a smell).
//
// This is the worker-side entrypoint. The worker lives in
// internal/worker; this method satisfies its consumer-side interface.
func (s *ProcurementService) RecomputeStatuses(ctx context.Context) (int64, error) {
	var changed int64
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.store.RecomputeStatuses(ctx, tx, store.RecomputeStatusesParams{
			WarningWindowDays: DefaultProcurementWarningWindowDays,
		})
		if err != nil {
			return err
		}
		changed = got
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("recompute procurement statuses: %w", err)
	}
	return changed, nil
}

// ProcurementRecommendationSet is the response shape from
// RecommendVendors: the persisted recommendation rows plus the locally
// minted batch RunID that correlates every row produced by a single
// AI call (so callers can group "what did this call produce" without a
// second query).
type ProcurementRecommendationSet struct {
	Items []models.ProcurementRecommendation
	RunID uuid.UUID
}

// RecommendVendors asks the native AI `procurement_recommend` task for
// a ranked list of vendors for the given procurement item, then
// persists each recommendation row inside one tx alongside the audit
// entry. The whole batch shares a locally minted RunID so future
// evaluation can correlate "what did this AI call produce" with "what
// was actually ordered" once procurement_items.vendor_id is wired.
//
// Validates:
//   - callerOrgID + procurementItemID non-zero.
//   - The item belongs to callerOrgID (cross-org isolation).
//   - The service was constructed with a non-nil AI recommender.
//
// The model's float64 Confidence (0.0–1.0) is rounded * 100 into the
// SMALLINT confidence_pct column to match the codebase no-floats
// culture (CHECK constraint enforces 0..100).
//
// callerUserSub is recorded on the audit row.
func (s *ProcurementService) RecommendVendors(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, procurementItemID uuid.UUID) (ProcurementRecommendationSet, error) {
	if callerOrgID == uuid.Nil {
		return ProcurementRecommendationSet{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if procurementItemID == uuid.Nil {
		return ProcurementRecommendationSet{}, fmt.Errorf("%w: procurement_item_id is required", ErrInvalidInput)
	}
	if s.recommender == nil {
		return ProcurementRecommendationSet{}, ErrAIUnavailable
	}

	// One run id minted locally per batch — the native AI client is
	// single-shot and returns no server-side run id, so BuildOS owns
	// the correlation key now.
	batchRunID := uuid.New()

	var result ProcurementRecommendationSet
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Ownership check + budget context. Returns
		// ErrProcurementItemNotFound on miss / cross-org.
		item, err := s.store.GetProcurementItem(ctx, tx, procurementItemID, callerOrgID)
		if err != nil {
			return err
		}

		// AI call rides outside the SQL round-trip but inside the
		// outer tx — if persistence fails we don't want a phantom
		// recommendation. The trade-off is that a slow model response
		// holds a tx open briefly; acceptable for an interactive
		// recommend flow. The per-org Anthropic key is resolved from
		// the context.
		aiCtx := ai.ContextWithOrgID(ctx, callerOrgID.String())
		resp, err := s.recommender.ProcurementRecommend(aiCtx, ai.ProcurementRecommendRequest{
			MaterialRequestID: item.ID,
			BudgetCents:       item.EstimatedCostCents,
			CurrencyCode:      item.EstimatedCostCurrencyCode,
		})
		if err != nil {
			return fmt.Errorf("ai procurement_recommend: %w", err)
		}
		if resp == nil {
			return fmt.Errorf("ai procurement_recommend: nil response")
		}

		recs := make([]models.ProcurementRecommendation, 0, len(resp.Recommendations))
		for _, r := range resp.Recommendations {
			// vendor_id is nullable: the model may recommend vendors
			// that don't exist in BuildOS's vendor table yet. Treat
			// uuid.Nil as "no canonical id".
			var vendorIDPtr *uuid.UUID
			if r.VendorID != uuid.Nil {
				v := r.VendorID
				vendorIDPtr = &v
			}
			var reasoningPtr *string
			if strings.TrimSpace(r.Reasoning) != "" {
				rp := r.Reasoning
				reasoningPtr = &rp
			}

			rec, err := s.store.CreateProcurementRecommendation(ctx, tx, store.CreateProcurementRecommendationParams{
				ProcurementItemID:          item.ID,
				OrgID:                      callerOrgID,
				RunID:                      batchRunID,
				VendorID:                   vendorIDPtr,
				VendorName:                 r.VendorName,
				PredictedSpendCents:        r.PredictedSpendCents,
				PredictedSpendCurrencyCode: r.CurrencyCode,
				ConfidencePct:              confidenceToPct(r.Confidence),
				Reasoning:                  reasoningPtr,
			})
			if err != nil {
				return err
			}
			recs = append(recs, rec)
		}

		// One audit row for the whole batch — the resource is the
		// recommendation set (keyed by procurement_item_id; run_id
		// goes in metadata for replay). Per-row audit would create
		// 3-5 nearly-identical rows per AI call with no extra signal.
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "procurement.recommendations.created",
			ResourceType: AuditResourceProcurementRecommendation,
			ResourceID:   item.ID,
			Metadata: marshalAudit(map[string]any{
				"run_id":               batchRunID,
				"recommendation_count": len(recs),
			}),
		})

		result = ProcurementRecommendationSet{
			Items: recs,
			RunID: batchRunID,
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrProcurementItemNotFound):
			return ProcurementRecommendationSet{}, ErrProcurementItemNotFound
		case errors.Is(err, store.ErrNotFound):
			return ProcurementRecommendationSet{}, ErrNotFound
		}
		return ProcurementRecommendationSet{}, fmt.Errorf("recommend vendors: %w", err)
	}
	return result, nil
}

// RequestVendorReviewInput is the validated input for
// RequestVendorReview. RFQID and Reasoning are optional (uuid.Nil /
// empty string omit them).
type RequestVendorReviewInput struct {
	ProcurementItemID uuid.UUID
	RFQID             uuid.UUID // optional — uuid.Nil for AI-driven (no formal RFQ)
	Vendor            string
	TotalCents        int64
	CurrencyCode      string
	Reasoning         string // optional AI narrative
}

// vendorReviewAction is the single feed-card action attached to a
// vendor_review_requested card. The frontend renders an "approve /
// route" affordance from it; the payload carries everything needed to
// act on the quote without a second fetch.
type vendorReviewAction struct {
	Label             string    `json:"label"`
	Action            string    `json:"action"`
	ProcurementItemID uuid.UUID `json:"procurement_item_id"`
	RFQID             uuid.UUID `json:"rfq_id,omitempty"`
	Vendor            string    `json:"vendor"`
	TotalCents        int64     `json:"total_cents"`
	CurrencyCode      string    `json:"currency_code"`
	Reasoning         string    `json:"reasoning,omitempty"`
}

// RequestVendorReview surfaces a vendor's material quote for human
// review by creating a local `vendor_review_requested` feed card.
// The whole flow runs in one tx: ownership check, feed-card insert,
// audit row. Returns the new feed card id so the caller can correlate
// the request with the card the operator will action.
//
// Validates:
//   - callerOrgID + ProcurementItemID non-zero.
//   - vendor non-empty, total_cents non-negative, currency_code in USD/CAD.
//   - The item belongs to callerOrgID (cross-org isolation via store.GetProcurementItem).
//   - The service was constructed with a non-nil feed-card store.
//
// The card targets the `owner` role — vendor selection is an
// owner-level decision — and carries the quote details in its action
// payload. callerUserSub is recorded on the audit row.
func (s *ProcurementService) RequestVendorReview(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in RequestVendorReviewInput) (uuid.UUID, error) {
	if callerOrgID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if in.ProcurementItemID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: procurement_item_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Vendor) == "" {
		return uuid.Nil, fmt.Errorf("%w: vendor is required", ErrInvalidInput)
	}
	if in.TotalCents < 0 {
		return uuid.Nil, fmt.Errorf("%w: total_cents must be non-negative", ErrInvalidInput)
	}
	if err := currency.Validate(in.CurrencyCode); err != nil {
		return uuid.Nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if s.feedStore == nil {
		return uuid.Nil, ErrVendorReviewUnavailable
	}

	var cardID uuid.UUID
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Ownership check + cross-org isolation. Returns
		// ErrProcurementItemNotFound if the item doesn't exist or
		// belongs to a different org. The project_id is read off the
		// item so the card scopes to the right project.
		item, err := s.store.GetProcurementItem(ctx, tx, in.ProcurementItemID, callerOrgID)
		if err != nil {
			return err
		}

		actions, err := json.Marshal([]vendorReviewAction{{
			Label:             "Review quote",
			Action:            "review_material_quote",
			ProcurementItemID: in.ProcurementItemID,
			RFQID:             in.RFQID,
			Vendor:            in.Vendor,
			TotalCents:        in.TotalCents,
			CurrencyCode:      in.CurrencyCode,
			Reasoning:         in.Reasoning,
		}})
		if err != nil {
			return fmt.Errorf("marshal vendor review action: %w", err)
		}

		projectID := item.ProjectID
		// Target the owner role — vendor selection is an owner-level
		// decision. Role strings are the RBAC vocabulary ("owner" >
		// "admin" > …); the literal avoids importing the api/middleware
		// constant into the service layer.
		ownerRole := "owner"
		card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
			OrgID:      callerOrgID,
			ProjectID:  &projectID,
			CardType:   "vendor_review_requested",
			Title:      "Vendor quote ready for review: " + in.Vendor,
			Body:       vendorReviewBody(item.Name, in.Vendor, in.TotalCents, in.CurrencyCode),
			Priority:   models.FeedPriorityUrgent,
			TargetRole: &ownerRole,
			Actions:    actions,
		})
		if err != nil {
			return err
		}
		cardID = card.ID

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "procurement.vendor_review.requested",
			ResourceType: AuditResourceProcurementItem,
			ResourceID:   in.ProcurementItemID,
			Metadata: marshalAudit(map[string]any{
				"feed_card_id":  card.ID,
				"vendor":        in.Vendor,
				"total_cents":   in.TotalCents,
				"currency_code": in.CurrencyCode,
				"rfq_id":        in.RFQID,
				"has_reasoning": strings.TrimSpace(in.Reasoning) != "",
			}),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrProcurementItemNotFound):
			return uuid.Nil, ErrProcurementItemNotFound
		case errors.Is(err, store.ErrNotFound):
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("request vendor review: %w", err)
	}
	return cardID, nil
}

// vendorReviewBody renders the human-readable feed-card body for a
// vendor review request. Kept tiny + dependency-free; the structured
// payload lives in the card's action JSON. The amount is rendered from
// integer cents (no floats) per the Composite Currency Pattern.
func vendorReviewBody(itemName, vendor string, totalCents int64, currencyCode string) string {
	return fmt.Sprintf("%s quoted %s %s for %q. Review and approve or route to another vendor.",
		vendor, formatCents(totalCents), currencyCode, itemName)
}

// formatCents renders integer cents as a fixed-2-decimal string
// (e.g. 50000 → "500.00") without ever converting to a float.
func formatCents(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if neg {
		return "-" + s
	}
	return s
}

// confidenceToPct converts the model's float64 confidence (0.0–1.0) to
// the SMALLINT 0..100 column the schema uses. Rounds half-up; clamps
// out-of-range inputs so a buggy Brain response can't violate the
// CHECK constraint and roll back the whole tx.
func confidenceToPct(c float64) int {
	if math.IsNaN(c) || c <= 0 {
		return 0
	}
	if c >= 1 {
		return 100
	}
	return int(math.Round(c * 100))
}
