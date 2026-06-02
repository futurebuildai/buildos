//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// fakeProcurementRecommender is an in-test ProcurementRecommender that returns
// a canned ranked list — lets the RecommendVendors persistence + audit path be
// exercised against a real Postgres without an HTTP server or a live Anthropic
// key. It records the request it saw so the test can assert the budget/currency
// context was threaded through.
type fakeProcurementRecommender struct {
	gotReq ai.ProcurementRecommendRequest
	resp   *ai.ProcurementRecommendResponse
	err    error
}

func (f *fakeProcurementRecommender) ProcurementRecommend(ctx context.Context, req ai.ProcurementRecommendRequest) (*ai.ProcurementRecommendResponse, error) {
	f.gotReq = req
	return f.resp, f.err
}

// newProcurementService wires a ProcurementService to a fresh migrated pool
// with a real audit recorder (so the in-tx procurement.* writes hit audit_log),
// a real feed-card store (so RequestVendorReview persists a card), a fake AI
// recommender, and a seeded org + project. Returns the service, the fake
// recommender (for response injection), and the org/project fixture.
func newProcurementService(t *testing.T) (*ProcurementService, *fakeProcurementRecommender, *procFixture) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Summit Procurement Co")
	testdb.SeedProject(t, pool, projectID, orgID, "Lakeview Renovation")

	rec := &fakeProcurementRecommender{}
	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewProcurementService(pool, store.NewProcurementStore(), rec, store.NewFeedCardsStore(), audit)
	return svc, rec, &procFixture{orgID: orgID, projectID: projectID}
}

type procFixture struct {
	orgID     uuid.UUID
	projectID uuid.UUID
}

func procAuditCount(t *testing.T, s *ProcurementService, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, action).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// seedItem creates a procurement item under the fixture's org/project and
// returns it. Shared setup for the update / recommend / review tests.
func seedItem(t *testing.T, svc *ProcurementService, fx *procFixture, name string) models.ProcurementItem {
	t.Helper()
	it, err := svc.CreateProcurementItem(context.Background(), fx.orgID, "owner-sub", CreateProcurementItemInput{
		ProjectID:                 fx.projectID,
		Name:                      name,
		WBSCode:                   "06.10.00",
		EstimatedCostCents:        250_000,
		EstimatedCostCurrencyCode: "USD",
		LeadTimeDays:              14,
		WeatherBufferDays:         2,
	})
	if err != nil {
		t.Fatalf("seed item %q: %v", name, err)
	}
	return it
}

// TestProcurementService_CreateItem_PersistsAndAudits is the canonical create
// round-trip: a valid item is persisted with the default 'OK' status, the
// name/wbs are trimmed, exactly one procurement.item.created audit row is
// written in the same tx, and the item round-trips through the org-scoped list.
func TestProcurementService_CreateItem_PersistsAndAudits(t *testing.T) {
	svc, _, fx := newProcurementService(t)
	ctx := context.Background()

	item, err := svc.CreateProcurementItem(ctx, fx.orgID, "owner-sub", CreateProcurementItemInput{
		ProjectID:                 fx.projectID,
		Name:                      "  Engineered Trusses  ", // trimmed
		WBSCode:                   "  06.17.00  ",           // trimmed
		EstimatedCostCents:        180_000,
		EstimatedCostCurrencyCode: "CAD",
		LeadTimeDays:              21,
		WeatherBufferDays:         3,
	})
	if err != nil {
		t.Fatalf("CreateProcurementItem: %v", err)
	}
	if item.ID == uuid.Nil {
		t.Fatal("created item has nil id")
	}
	if item.Name != "Engineered Trusses" || item.WBSCode != "06.17.00" {
		t.Errorf("item = %q/%q, want trimmed name+wbs", item.Name, item.WBSCode)
	}
	if item.Status != models.ProcurementStatusOK {
		t.Errorf("status = %q, want default OK", item.Status)
	}
	if item.EstimatedCostCents != 180_000 || item.EstimatedCostCurrencyCode != "CAD" {
		t.Errorf("cost = %d %s, want 180000 CAD", item.EstimatedCostCents, item.EstimatedCostCurrencyCode)
	}
	if got := procAuditCount(t, svc, fx.orgID, "procurement.item.created"); got != 1 {
		t.Errorf("procurement.item.created audit rows = %d, want 1", got)
	}

	items, err := svc.ListProcurement(ctx, fx.projectID, fx.orgID, nil)
	if err != nil {
		t.Fatalf("ListProcurement: %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Errorf("ListProcurement = %+v, want the created item", items)
	}
}

// TestProcurementService_CreateItem_CrossTenantHidden proves the org-scoping
// guard: a create against a project owned by ANOTHER org fails as ErrNotFound
// (not a leak of the project's existence) and writes nothing. The read path is
// likewise hidden cross-tenant.
func TestProcurementService_CreateItem_CrossTenantHidden(t *testing.T) {
	svc, _, fx := newProcurementService(t)
	ctx := context.Background()

	otherOrg := uuid.New() // never owns fx.projectID
	if _, err := svc.CreateProcurementItem(ctx, otherOrg, "intruder", CreateProcurementItemInput{
		ProjectID:                 fx.projectID,
		Name:                      "Sneaky Order",
		WBSCode:                   "01.00.00",
		EstimatedCostCents:        1,
		EstimatedCostCurrencyCode: "USD",
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant create = %v, want ErrNotFound", err)
	}

	// Seed a real item under fx, then prove the list is hidden cross-tenant.
	_ = seedItem(t, svc, fx, "Real Item")
	if _, err := svc.ListProcurement(ctx, fx.projectID, otherOrg, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant ListProcurement = %v, want ErrNotFound", err)
	}
	if got := procAuditCount(t, svc, otherOrg, "procurement.item.created"); got != 0 {
		t.Errorf("foreign-org create wrote %d audit rows, want 0", got)
	}
}

// TestProcurementService_UpdateItem_TransitionsAndAudits covers the update
// mutation: a created item is moved to ORDERED (with the required PO number),
// the returned model reflects it, and an procurement.item.updated audit row is
// written. Also covers the item-not-found and cross-tenant legs.
func TestProcurementService_UpdateItem_TransitionsAndAudits(t *testing.T) {
	svc, _, fx := newProcurementService(t)
	ctx := context.Background()

	item := seedItem(t, svc, fx, "Roof Membrane")

	ordered := models.ProcurementStatusOrdered
	updated, err := svc.UpdateProcurementItem(ctx, fx.orgID, "owner-sub", UpdateProcurementItemInput{
		ItemID:    item.ID,
		ProjectID: fx.projectID,
		Status:    &ordered,
		PONumber:  ptrString("PO-2026-0042"),
	})
	if err != nil {
		t.Fatalf("UpdateProcurementItem: %v", err)
	}
	if updated.Status != models.ProcurementStatusOrdered {
		t.Errorf("status = %q, want ORDERED", updated.Status)
	}
	if got := procAuditCount(t, svc, fx.orgID, "procurement.item.updated"); got != 1 {
		t.Errorf("procurement.item.updated audit rows = %d, want 1", got)
	}

	// A real project but a non-existent item id → ErrProcurementItemNotFound.
	if _, err := svc.UpdateProcurementItem(ctx, fx.orgID, "owner-sub", UpdateProcurementItemInput{
		ItemID:    uuid.New(),
		ProjectID: fx.projectID,
		Status:    ptrString(models.ProcurementStatusWarning),
	}); !errors.Is(err, ErrProcurementItemNotFound) {
		t.Errorf("update missing item = %v, want ErrProcurementItemNotFound", err)
	}

	// Updating from a foreign org → ErrNotFound (project guard fires first).
	if _, err := svc.UpdateProcurementItem(ctx, uuid.New(), "intruder", UpdateProcurementItemInput{
		ItemID:    item.ID,
		ProjectID: fx.projectID,
		Status:    ptrString(models.ProcurementStatusWarning),
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant update = %v, want ErrNotFound", err)
	}
}

// TestProcurementService_RecomputeStatuses_FlipsThenIdempotent exercises the
// worker sweep: an item whose must_order_date is well in the past flips from
// the default OK to CRITICAL on the first run (one change), and a re-run is a
// no-op (already CRITICAL). Uses a far-past horizon so the result is
// deterministic regardless of the real wall-clock date the suite runs on.
func TestProcurementService_RecomputeStatuses_FlipsThenIdempotent(t *testing.T) {
	svc, _, fx := newProcurementService(t)
	ctx := context.Background()

	// need_by = today, lead 30d → must_order ~30 days ago → CRITICAL.
	needBy := time.Now().UTC().Truncate(24 * time.Hour)
	if _, err := svc.CreateProcurementItem(ctx, fx.orgID, "owner-sub", CreateProcurementItemInput{
		ProjectID:                 fx.projectID,
		Name:                      "Long-lead Steel",
		WBSCode:                   "05.10.00",
		EstimatedCostCents:        500_000,
		EstimatedCostCurrencyCode: "USD",
		LeadTimeDays:              30,
		WeatherBufferDays:         0,
		NeedByDate:                &needBy,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	first, err := svc.RecomputeStatuses(ctx)
	if err != nil {
		t.Fatalf("RecomputeStatuses: %v", err)
	}
	if first != 1 {
		t.Errorf("first recompute changed %d rows, want 1 (OK→CRITICAL)", first)
	}
	second, err := svc.RecomputeStatuses(ctx)
	if err != nil {
		t.Fatalf("RecomputeStatuses (re-run): %v", err)
	}
	if second != 0 {
		t.Errorf("idempotent re-run changed %d rows, want 0", second)
	}
}

// TestProcurementService_RecommendVendors_PersistsAndAudits drives the AI
// recommend path with a fake recommender: the canned ranked list is persisted
// (each row sharing the batch RunID), the budget/currency context is threaded
// into the request, and one batch audit row is written. The cross-tenant item
// leg returns ErrProcurementItemNotFound.
func TestProcurementService_RecommendVendors_PersistsAndAudits(t *testing.T) {
	svc, rec, fx := newProcurementService(t)
	ctx := context.Background()

	item := seedItem(t, svc, fx, "Cabinetry Package")

	rec.resp = &ai.ProcurementRecommendResponse{
		Recommendations: []ai.ProcurementVendorRec{
			{VendorName: "Northwind Millwork", PredictedSpendCents: 240_000, CurrencyCode: "USD", Confidence: 0.92, Reasoning: "in budget, fast lead"},
			{VendorName: "Cascade Cabinets", PredictedSpendCents: 260_000, CurrencyCode: "USD", Confidence: 0.5},
		},
	}

	set, err := svc.RecommendVendors(ctx, fx.orgID, "owner-sub", item.ID)
	if err != nil {
		t.Fatalf("RecommendVendors: %v", err)
	}
	if set.RunID == uuid.Nil {
		t.Error("recommendation set has nil run id")
	}
	if len(set.Items) != 2 {
		t.Fatalf("persisted %d recommendations, want 2", len(set.Items))
	}
	for _, r := range set.Items {
		if r.RunID != set.RunID {
			t.Errorf("rec run id = %s, want batch %s", r.RunID, set.RunID)
		}
	}
	// The item's budget context was threaded into the AI request.
	if rec.gotReq.MaterialRequestID != item.ID || rec.gotReq.BudgetCents != item.EstimatedCostCents {
		t.Errorf("recommender req = %+v, want item budget context", rec.gotReq)
	}
	if got := procAuditCount(t, svc, fx.orgID, "procurement.recommendations.created"); got != 1 {
		t.Errorf("procurement.recommendations.created audit rows = %d, want 1 (one batch row)", got)
	}

	// A foreign org can't recommend against fx's item → not found.
	if _, err := svc.RecommendVendors(ctx, uuid.New(), "intruder", item.ID); !errors.Is(err, ErrProcurementItemNotFound) {
		t.Errorf("cross-tenant RecommendVendors = %v, want ErrProcurementItemNotFound", err)
	}
}

// TestProcurementService_RequestVendorReview_CreatesCardAndAudits drives the
// operator-review flow against a real feed-card store: a vendor quote produces
// a vendor_review_requested feed card (id returned) and one audit row. The
// cross-tenant item leg returns ErrProcurementItemNotFound and writes nothing.
func TestProcurementService_RequestVendorReview_CreatesCardAndAudits(t *testing.T) {
	svc, _, fx := newProcurementService(t)
	ctx := context.Background()

	item := seedItem(t, svc, fx, "HVAC Rooftop Unit")

	cardID, err := svc.RequestVendorReview(ctx, fx.orgID, "owner-sub", RequestVendorReviewInput{
		ProcurementItemID: item.ID,
		Vendor:            "Apex Mechanical",
		TotalCents:        310_000,
		CurrencyCode:      "USD",
		Reasoning:         "lowest quote, in-region",
	})
	if err != nil {
		t.Fatalf("RequestVendorReview: %v", err)
	}
	if cardID == uuid.Nil {
		t.Error("returned feed card id is nil")
	}
	// The card actually landed in feed_cards under the org.
	var cardType string
	if err := svc.pool.QueryRow(ctx,
		`SELECT card_type FROM feed_cards WHERE id = $1 AND org_id = $2`, cardID, fx.orgID).Scan(&cardType); err != nil {
		t.Fatalf("read feed card: %v", err)
	}
	if cardType != "vendor_review_requested" {
		t.Errorf("card_type = %q, want vendor_review_requested", cardType)
	}
	if got := procAuditCount(t, svc, fx.orgID, "procurement.vendor_review.requested"); got != 1 {
		t.Errorf("procurement.vendor_review.requested audit rows = %d, want 1", got)
	}

	// Foreign org can't request review against fx's item.
	if _, err := svc.RequestVendorReview(ctx, uuid.New(), "intruder", RequestVendorReviewInput{
		ProcurementItemID: item.ID,
		Vendor:            "Ghost Vendor",
		TotalCents:        1,
		CurrencyCode:      "USD",
	}); !errors.Is(err, ErrProcurementItemNotFound) {
		t.Errorf("cross-tenant RequestVendorReview = %v, want ErrProcurementItemNotFound", err)
	}
}
