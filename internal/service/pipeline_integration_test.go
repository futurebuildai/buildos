//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// newPipelineService wires a PipelineService to a fresh migrated pool with a
// real audit recorder and a seeded org. The River client is intentionally nil:
// every entrypoint EXCEPT the PERMIT_ISSUED Kanban→CPM transition is
// river-independent, and the nil-river ErrNotImplemented guard is already
// covered by the default-tag pipeline_test.go. Returns the service + org id;
// svc.pool is reachable for direct audit_log asserts.
func newPipelineService(t *testing.T) (*PipelineService, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Foothills Custom Homes")

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewPipelineService(pool, store.NewPipelineStore(), nil, audit)
	return svc, orgID
}

func pipelineAuditCount(t *testing.T, s *PipelineService, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, action).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// seedProspect creates a LEAD-stage prospect under the org and returns it.
func seedProspect(t *testing.T, svc *PipelineService, orgID uuid.UUID, name string) models.Prospect {
	t.Helper()
	gsf := 3200
	p, err := svc.CreateProspect(context.Background(), CreateProspectInput{
		OrgID:      orgID,
		Name:       name,
		ClientName: "Jordan Client",
		GSF:        &gsf,
	})
	if err != nil {
		t.Fatalf("seed prospect %q: %v", name, err)
	}
	return p
}

// TestPipelineService_ProspectCRUD_Lifecycle is the canonical prospect
// round-trip: create at LEAD (probability 10), read back with empty
// estimates/permits, partial-update, and see it on the listing.
func TestPipelineService_ProspectCRUD_Lifecycle(t *testing.T) {
	svc, orgID := newPipelineService(t)
	ctx := context.Background()

	p := seedProspect(t, svc, orgID, "Birch Hollow Residence")
	if p.PipelineStage != models.StageLead {
		t.Errorf("stage = %s, want LEAD", p.PipelineStage)
	}
	if p.ProbabilityPct != 10 {
		t.Errorf("probability = %d, want 10", p.ProbabilityPct)
	}

	details, err := svc.GetProspectWithDetails(ctx, p.ID, orgID)
	if err != nil {
		t.Fatalf("GetProspectWithDetails: %v", err)
	}
	if details.Prospect.ID != p.ID || len(details.Estimates) != 0 || len(details.Permits) != 0 {
		t.Errorf("details = %+v, want the prospect with empty estimates/permits", details)
	}

	updated, err := svc.UpdateProspect(ctx, UpdateProspectInput{
		ProspectID: p.ID,
		OrgID:      orgID,
		Name:       strptr("Birch Hollow (Phase 2)"),
		Notes:      strptr("client wants a walkout basement"),
	})
	if err != nil {
		t.Fatalf("UpdateProspect: %v", err)
	}
	if updated.Name != "Birch Hollow (Phase 2)" {
		t.Errorf("name = %q, want renamed", updated.Name)
	}

	page, err := svc.ListProspects(ctx, ListProspectsInput{OrgID: orgID})
	if err != nil {
		t.Fatalf("ListProspects: %v", err)
	}
	if page.Total != 1 || len(page.Prospects) != 1 {
		t.Errorf("page total/len = %d/%d, want 1/1", page.Total, len(page.Prospects))
	}

	// Cross-tenant read is hidden.
	if _, err := svc.GetProspectWithDetails(ctx, p.ID, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant GetProspectWithDetails = %v, want ErrNotFound", err)
	}
}

// TestPipelineService_ProspectCRUD_Validation covers the pre-tx gates on the
// create/update + list reads.
func TestPipelineService_ProspectCRUD_Validation(t *testing.T) {
	svc, orgID := newPipelineService(t)
	ctx := context.Background()

	if _, err := svc.CreateProspect(ctx, CreateProspectInput{OrgID: orgID, ClientName: "x"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateProspect(blank name) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateProspect(ctx, CreateProspectInput{OrgID: orgID, Name: "x"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateProspect(blank client) = %v, want ErrInvalidInput", err)
	}
	bad := -5
	if _, err := svc.CreateProspect(ctx, CreateProspectInput{OrgID: orgID, Name: "x", ClientName: "y", GSF: &bad}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateProspect(neg gsf) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.UpdateProspect(ctx, UpdateProspectInput{ProspectID: uuid.New(), OrgID: orgID, GSF: &bad}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("UpdateProspect(neg gsf) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.ListProspects(ctx, ListProspectsInput{OrgID: orgID, Stage: "BOGUS"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ListProspects(bad stage) = %v, want ErrInvalidInput", err)
	}
}

// TestPipelineService_AdvanceAndLose walks the stage state-machine: a prospect
// advances LEAD→QUALIFIED→ESTIMATE_SENT→VERBAL_COMMITMENT→PERMIT_APPLIED (each
// a valid single hop, probability rising, one audit row per hop), an
// out-of-order hop is rejected as ErrInvalidTransition, and a separate prospect
// is marked LOST then refuses further advancement (ErrTerminalStage).
func TestPipelineService_AdvanceAndLose(t *testing.T) {
	svc, orgID := newPipelineService(t)
	ctx := context.Background()

	p := seedProspect(t, svc, orgID, "Cedar Ridge Build")
	chain := []models.PipelineStage{
		models.StageQualified,
		models.StageEstimateSent,
		models.StageVerbalCommitment,
		models.StagePermitApplied,
	}
	lastProb := p.ProbabilityPct
	for _, target := range chain {
		adv, err := svc.AdvanceProspect(ctx, "owner-sub", AdvanceProspectInput{
			ProspectID: p.ID, OrgID: orgID, Target: target,
		})
		if err != nil {
			t.Fatalf("AdvanceProspect → %s: %v", target, err)
		}
		if adv.PipelineStage != target {
			t.Errorf("stage = %s, want %s", adv.PipelineStage, target)
		}
		if adv.ProbabilityPct <= lastProb {
			t.Errorf("probability %d did not rise past %d at %s", adv.ProbabilityPct, lastProb, target)
		}
		lastProb = adv.ProbabilityPct
	}
	if got := pipelineAuditCount(t, svc, orgID, "pipeline.stage_transitioned"); got != len(chain) {
		t.Errorf("stage_transitioned audit rows = %d, want %d", got, len(chain))
	}

	// An out-of-order hop (PERMIT_APPLIED → QUALIFIED) is not permitted.
	if _, err := svc.AdvanceProspect(ctx, "owner-sub", AdvanceProspectInput{
		ProspectID: p.ID, OrgID: orgID, Target: models.StageQualified,
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("backward advance = %v, want ErrInvalidTransition", err)
	}

	// Cross-tenant advance is hidden as not-found.
	if _, err := svc.AdvanceProspect(ctx, "intruder", AdvanceProspectInput{
		ProspectID: p.ID, OrgID: uuid.New(), Target: models.StagePermitApplied,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant advance = %v, want ErrNotFound", err)
	}

	// LoseProspect: a fresh prospect → LOST, then a further advance is terminal.
	lost := seedProspect(t, svc, orgID, "Falling-Through Deal")
	if _, err := svc.LoseProspect(ctx, "owner-sub", LoseProspectInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("LoseProspect(no reason) = %v, want ErrInvalidInput", err)
	}
	marked, err := svc.LoseProspect(ctx, "owner-sub", LoseProspectInput{
		ProspectID: lost.ID, OrgID: orgID, Reason: "chose another builder",
	})
	if err != nil {
		t.Fatalf("LoseProspect: %v", err)
	}
	if marked.PipelineStage != models.StageLost {
		t.Errorf("stage = %s, want LOST", marked.PipelineStage)
	}
	if _, err := svc.AdvanceProspect(ctx, "owner-sub", AdvanceProspectInput{
		ProspectID: lost.ID, OrgID: orgID, Target: models.StageQualified,
	}); !errors.Is(err, ErrTerminalStage) {
		t.Errorf("advance from LOST = %v, want ErrTerminalStage", err)
	}
}

// TestPipelineService_Estimates covers the estimate sub-surface: create
// (currency + line items, total computed from the items), a valid status
// transition, the pre-tx validation gates, and cross-tenant hiding.
func TestPipelineService_Estimates(t *testing.T) {
	svc, orgID := newPipelineService(t)
	ctx := context.Background()

	p := seedProspect(t, svc, orgID, "Maple Court Estimate")
	items := models.PipelineEstimateLineItems{
		{WBSCode: "03.30.00", Description: "Foundation", EstimatedCents: 1_200_000},
		{WBSCode: "06.10.00", Description: "Framing", EstimatedCents: 800_000},
	}
	est, err := svc.CreateEstimate(ctx, CreateEstimateInput{
		ProspectID: p.ID, OrgID: orgID, CurrencyCode: "USD", LineItems: items, MarginPct: 15,
	})
	if err != nil {
		t.Fatalf("CreateEstimate: %v", err)
	}
	if est.TotalEstimatedCents != 2_000_000 {
		t.Errorf("total = %d, want 2000000 (sum of line items)", est.TotalEstimatedCents)
	}

	sent, err := svc.UpdateEstimateStatus(ctx, UpdateEstimateStatusInput{
		EstimateID: est.ID, ProspectID: p.ID, OrgID: orgID, NewStatus: models.EstimateStatusSent,
	})
	if err != nil {
		t.Fatalf("UpdateEstimateStatus: %v", err)
	}
	if sent.Status != models.EstimateStatusSent {
		t.Errorf("status = %q, want sent", sent.Status)
	}

	// Validation gates.
	if _, err := svc.CreateEstimate(ctx, CreateEstimateInput{ProspectID: p.ID, OrgID: orgID, CurrencyCode: "EUR", LineItems: items}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateEstimate(bad currency) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateEstimate(ctx, CreateEstimateInput{ProspectID: p.ID, OrgID: orgID, CurrencyCode: "USD"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateEstimate(no items) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateEstimate(ctx, CreateEstimateInput{ProspectID: p.ID, OrgID: orgID, CurrencyCode: "USD", LineItems: items, MarginPct: 150}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateEstimate(margin 150) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.UpdateEstimateStatus(ctx, UpdateEstimateStatusInput{EstimateID: est.ID, ProspectID: p.ID, OrgID: orgID, NewStatus: "bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("UpdateEstimateStatus(bad status) = %v, want ErrInvalidInput", err)
	}

	// Cross-tenant create is hidden as not-found.
	if _, err := svc.CreateEstimate(ctx, CreateEstimateInput{ProspectID: p.ID, OrgID: uuid.New(), CurrencyCode: "USD", LineItems: items}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant CreateEstimate = %v, want ErrNotFound", err)
	}
}

// TestPipelineService_Permits covers the permit sub-surface: create at
// not_submitted, a valid status transition, the validation gates, and
// cross-tenant hiding.
func TestPipelineService_Permits(t *testing.T) {
	svc, orgID := newPipelineService(t)
	ctx := context.Background()

	p := seedProspect(t, svc, orgID, "Permit Lane Project")
	permit, err := svc.CreatePermit(ctx, CreatePermitInput{
		ProspectID: p.ID, OrgID: orgID, PermitType: "building", Jurisdiction: "Boulder County",
		FeeCents: 45_000, FeeCurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("CreatePermit: %v", err)
	}
	if permit.Status != models.PermitStatusNotSubmitted {
		t.Errorf("status = %q, want not_submitted", permit.Status)
	}

	submitted := models.PermitStatusSubmitted
	upd, err := svc.UpdatePermit(ctx, UpdatePermitInput{
		PermitID: permit.ID, ProspectID: p.ID, OrgID: orgID, NewStatus: &submitted,
	})
	if err != nil {
		t.Fatalf("UpdatePermit: %v", err)
	}
	if upd.Status != models.PermitStatusSubmitted {
		t.Errorf("status = %q, want submitted", upd.Status)
	}

	// Validation gates.
	if _, err := svc.CreatePermit(ctx, CreatePermitInput{ProspectID: p.ID, OrgID: orgID, Jurisdiction: "X", FeeCurrencyCode: "USD"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreatePermit(no type) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreatePermit(ctx, CreatePermitInput{ProspectID: p.ID, OrgID: orgID, PermitType: "building", FeeCurrencyCode: "USD"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreatePermit(no jurisdiction) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreatePermit(ctx, CreatePermitInput{ProspectID: p.ID, OrgID: orgID, PermitType: "building", Jurisdiction: "X", FeeCurrencyCode: "GBP"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreatePermit(bad currency) = %v, want ErrInvalidInput", err)
	}
	bogus := "bogus"
	if _, err := svc.UpdatePermit(ctx, UpdatePermitInput{PermitID: permit.ID, ProspectID: p.ID, OrgID: orgID, NewStatus: &bogus}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("UpdatePermit(bad status) = %v, want ErrInvalidInput", err)
	}

	// Cross-tenant create is hidden.
	if _, err := svc.CreatePermit(ctx, CreatePermitInput{ProspectID: p.ID, OrgID: uuid.New(), PermitType: "building", Jurisdiction: "X", FeeCurrencyCode: "USD"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant CreatePermit = %v, want ErrNotFound", err)
	}
}

// TestPipelineService_Analytics exercises the weighted-revenue aggregation
// read: it must succeed (empty or populated) for an org with prospects.
func TestPipelineService_Analytics(t *testing.T) {
	svc, orgID := newPipelineService(t)
	ctx := context.Background()

	_ = seedProspect(t, svc, orgID, "Analytics Prospect A")
	_ = seedProspect(t, svc, orgID, "Analytics Prospect B")

	if _, err := svc.GetPipelineAnalytics(ctx, orgID); err != nil {
		t.Errorf("GetPipelineAnalytics: %v", err)
	}
}
