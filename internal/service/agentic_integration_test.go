//go:build integration

package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// fakeCascadeReasoner is a deterministic agentic.Reasoner used in CI: no
// live AI. plan is returned verbatim when err is nil; otherwise err is
// returned (used to script the ErrReasonerUnavailable soft-fail leg). It
// records the context it was handed so a test can prove the workspace
// projected the engine-computed snapshot before reasoning.
type fakeCascadeReasoner struct {
	plan    agentic.CascadePlan
	err     error
	lastCtx agentic.CascadeContext
	calls   int
}

func (r *fakeCascadeReasoner) PlanCascade(_ context.Context, c agentic.CascadeContext) (agentic.CascadePlan, error) {
	r.calls++
	r.lastCtx = c
	if r.err != nil {
		return agentic.CascadePlan{}, r.err
	}
	return r.plan, nil
}

// multiModulePlan is the canonical deterministic cascade plan: one impact
// per module (schedule / procurement / crew / budget) at distinct
// severities, so the assertions can verify the full card_type / priority /
// target_role mapping in one pass.
func multiModulePlan() agentic.CascadePlan {
	return agentic.CascadePlan{Impacts: []agentic.CascadeImpact{
		{
			Module:            "schedule",
			Severity:          "critical",
			Title:             "Foundation slip pushes the critical path",
			Body:              "The foundation cure overran; downstream framing now starts late.",
			RecommendedAction: "Resequence framing crew to recover two days.",
		},
		{
			Module:            "procurement",
			Severity:          "high",
			Title:             "Window order at risk",
			Body:              "Must-order date now falls behind the slipped finish.",
			RecommendedAction: "Expedite the window PO today.",
		},
		{
			Module:            "crew",
			Severity:          "normal",
			Title:             "Framing crew idle window",
			Body:              "Framing crew has a one-day gap created by the slip.",
			RecommendedAction: "Reassign the framing crew to punch-list work.",
		},
		{
			Module:            "budget",
			Severity:          "low",
			Title:             "Cure-overtime cost exposure",
			Body:              "Extended cure may add overtime against the foundation phase.",
			RecommendedAction: "Owner to review the foundation phase contingency.",
		},
	}}
}

// cascadeFixture bundles the seeded ids + the real workspace under test.
type cascadeFixture struct {
	pool      *pgxpool.Pool
	workspace *CascadeWorkspace
	orgID     uuid.UUID
	projectID uuid.UUID
}

// newCascadeWorkspaceFixture wires a REAL service.CascadeWorkspace over a
// fresh migrated pool with all six stores plus a REAL AuditService (so the
// in-tx audit writes actually hit audit_log and the assertions can count
// them). It seeds an org + project but no schedule rows; callers seed the
// task / procurement / budget graph they need.
func newCascadeWorkspaceFixture(t *testing.T) *cascadeFixture {
	t.Helper()
	pool := testdb.NewPool(t)

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ws := NewCascadeWorkspace(
		pool,
		store.NewScheduleStore(),
		store.NewProcurementStore(),
		store.NewFinancialsStore(),
		store.NewProjectStore(),
		store.NewFeedCardsStore(),
		audit,
	)

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Stoneridge Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Maple Ridge Custom")

	return &cascadeFixture{pool: pool, workspace: ws, orgID: orgID, projectID: projectID}
}

// seedCascadeTask inserts a project_tasks row with explicit is_critical /
// total_float so a fixture can construct a real critical path (the
// orchestrator no-ops on a context with no critical slipped task).
func seedCascadeTask(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, name string, durationDays, totalFloat int, isCritical bool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, status, percent_complete, total_float, is_critical)
		VALUES ($1, $2, $3, $4, 'pending', 0, $5, $6)
		RETURNING id`, projectID, wbs, name, durationDays, totalFloat, isCritical).Scan(&id); err != nil {
		t.Fatalf("seed cascade task %s: %v", wbs, err)
	}
	return id
}

// seedCascadeProcurement inserts a procurement_items row in the project's
// orbit (the cascade context ferries these as ordering pressure).
func seedCascadeProcurement(t *testing.T, pool *pgxpool.Pool, projectID, orgID uuid.UUID, name, wbs, status string, leadTimeDays int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO procurement_items (project_id, org_id, name, wbs_code, lead_time_days, status)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		projectID, orgID, name, wbs, leadTimeDays, status); err != nil {
		t.Fatalf("seed procurement %s: %v", name, err)
	}
}

// seedCascadeBudget inserts a single project_budgets row. All three
// monetary pairs share currency to satisfy chk_budget_currency_match (the
// cascade context ferries these cents-paired figures, never floats).
func seedCascadeBudget(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, currency string, est, com, act int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO project_budgets (
			project_id, wbs_code, phase_name,
			estimated_cost_cents, estimated_cost_currency_code,
			committed_cost_cents, committed_cost_currency_code,
			actual_cost_cents, actual_cost_currency_code
		) VALUES ($1, $2, $3, $4, $5, $6, $5, $7, $5)`,
		projectID, wbs, "phase "+wbs, est, currency, com, act); err != nil {
		t.Fatalf("seed project_budget %s: %v", wbs, err)
	}
}

// feedCardRow is the slice of feed_cards a cascade assertion reads back.
type feedCardRow struct {
	cardType   string
	priority   string
	targetRole *string
}

// readCascadeFeedCards returns every feed card the cascade wrote for the
// org, newest first, so the assertions can verify count + card_type /
// priority / target_role per impact.
func readCascadeFeedCards(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID) []feedCardRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT card_type, priority, target_role
		FROM feed_cards
		WHERE org_id = $1
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		t.Fatalf("query feed_cards: %v", err)
	}
	defer rows.Close()

	var out []feedCardRow
	for rows.Next() {
		var fc feedCardRow
		if err := rows.Scan(&fc.cardType, &fc.priority, &fc.targetRole); err != nil {
			t.Fatalf("scan feed_card: %v", err)
		}
		out = append(out, fc)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate feed_cards: %v", err)
	}
	return out
}

// cascadeImpactAuditCount counts the audit_log rows the cascade wrote
// (action agentic.delay_cascade.impact) for the org — one per applied
// impact, committed in the same tx as the feed card.
func cascadeImpactAuditCount(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, "agentic.delay_cascade.impact").Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// TestAgenticOrchestrator_RunDelayCascade_AppliesMultiModulePlan is the
// end-to-end happy path: a project with a real critical path (two critical
// tasks) plus procurement + budget context is loaded by the REAL
// CascadeWorkspace, a FAKE deterministic Reasoner returns a four-module
// plan, and the orchestrator applies it in one tx. Asserts the workspace
// projected the critical tasks into the reasoner context, the feed-card
// count equals the impact count with the correct card_type / priority /
// target_role per module, and one agentic.delay_cascade.impact audit row
// landed per impact.
func TestAgenticOrchestrator_RunDelayCascade_AppliesMultiModulePlan(t *testing.T) {
	fx := newCascadeWorkspaceFixture(t)
	ctx := context.Background()

	// Two critical tasks (float 0) form the slipped critical path; the
	// third absorbs into float and is intentionally non-critical to prove
	// only critical tasks are projected.
	seedCascadeTask(t, fx.pool, fx.projectID, "1.0", "Foundation", 5, 0, true)
	seedCascadeTask(t, fx.pool, fx.projectID, "2.0", "Framing", 3, 0, true)
	seedCascadeTask(t, fx.pool, fx.projectID, "3.0", "Landscaping", 4, 6, false)

	seedCascadeProcurement(t, fx.pool, fx.projectID, fx.orgID, "Windows", "2.0", "WARNING", 21)
	seedCascadeProcurement(t, fx.pool, fx.projectID, fx.orgID, "Roof Trusses", "3.0", "OK", 14)

	seedCascadeBudget(t, fx.pool, fx.projectID, "1.0", "USD", 500000, 480000, 510000)
	seedCascadeBudget(t, fx.pool, fx.projectID, "2.0", "USD", 300000, 290000, 0)

	reasoner := &fakeCascadeReasoner{plan: multiModulePlan()}
	orch := agentic.NewOrchestrator(reasoner, fx.workspace, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	res, err := orch.RunDelayCascade(ctx, agentic.DelayCascadeInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunDelayCascade: %v", err)
	}

	// The reasoner ran exactly once over a context that carried only the
	// two critical tasks plus the procurement + budget orbit.
	if reasoner.calls != 1 {
		t.Fatalf("reasoner calls = %d, want 1", reasoner.calls)
	}
	if got := len(reasoner.lastCtx.SlippedTasks); got != 2 {
		t.Errorf("reasoner saw %d slipped tasks, want 2 (only critical)", got)
	}
	for _, st := range reasoner.lastCtx.SlippedTasks {
		if !st.IsCritical {
			t.Errorf("reasoner saw non-critical task %q in slipped set", st.Name)
		}
	}
	if len(reasoner.lastCtx.Procurement) != 2 {
		t.Errorf("reasoner saw %d procurement lines, want 2", len(reasoner.lastCtx.Procurement))
	}
	// The WBS join key must survive into the reasoner context (bug_009):
	// without it the model can't correlate a procurement line to a slipped
	// task / budget line.
	procWBS := map[string]bool{}
	for _, p := range reasoner.lastCtx.Procurement {
		procWBS[p.WBS] = true
	}
	if !procWBS["2.0"] || !procWBS["3.0"] {
		t.Errorf("procurement WBS codes = %v, want 2.0 and 3.0 present", procWBS)
	}
	if len(reasoner.lastCtx.Budget) != 2 {
		t.Errorf("reasoner saw %d budget lines, want 2", len(reasoner.lastCtx.Budget))
	}

	// Result + persisted cards: one feed card per impact.
	wantImpacts := len(multiModulePlan().Impacts)
	if res.Impacts != wantImpacts || res.CardsCreated != wantImpacts {
		t.Errorf("result = %+v, want impacts %d / cards %d", res, wantImpacts, wantImpacts)
	}

	cards := readCascadeFeedCards(t, fx.pool, fx.orgID)
	if len(cards) != wantImpacts {
		t.Fatalf("persisted feed cards = %d, want %d (one per impact)", len(cards), wantImpacts)
	}

	// Every card is a delay_cascade card. Tally the (priority, target_role)
	// pairs so the per-module mapping is verified regardless of row order:
	//   schedule/critical   -> critical priority, superintendent
	//   procurement/high    -> urgent   priority, superintendent
	//   crew/normal         -> normal   priority, superintendent
	//   budget/low          -> low      priority, owner
	type key struct {
		priority   string
		targetRole string
	}
	tally := map[key]int{}
	for _, c := range cards {
		if c.cardType != "delay_cascade" {
			t.Errorf("card_type = %q, want delay_cascade", c.cardType)
		}
		if c.targetRole == nil {
			t.Fatalf("card has nil target_role, want a role-broadcast card")
		}
		tally[key{c.priority, *c.targetRole}]++
	}
	want := map[key]int{
		{"critical", "superintendent"}: 1,
		{"urgent", "superintendent"}:   1,
		{"normal", "superintendent"}:   1,
		{"low", "owner"}:               1,
	}
	if fmt.Sprint(tally) != fmt.Sprint(want) {
		t.Errorf("priority/role tally = %v, want %v", tally, want)
	}

	// One agentic.delay_cascade.impact audit row per impact, committed in
	// the same tx as the feed cards.
	if got := cascadeImpactAuditCount(t, fx.pool, fx.orgID); got != wantImpacts {
		t.Errorf("agentic.delay_cascade.impact audit rows = %d, want %d", got, wantImpacts)
	}
}

// TestAgenticOrchestrator_RunDelayCascade_NoCriticalPath proves the no-op
// guard: a project whose tasks all absorb into float (none critical) yields
// a context with no critical path, so the orchestrator skips reasoning and
// writes zero feed cards / zero audit rows.
func TestAgenticOrchestrator_RunDelayCascade_NoCriticalPath(t *testing.T) {
	fx := newCascadeWorkspaceFixture(t)
	ctx := context.Background()

	seedCascadeTask(t, fx.pool, fx.projectID, "1.0", "Foundation", 5, 4, false)
	seedCascadeTask(t, fx.pool, fx.projectID, "2.0", "Framing", 3, 6, false)

	reasoner := &fakeCascadeReasoner{plan: multiModulePlan()}
	orch := agentic.NewOrchestrator(reasoner, fx.workspace, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	res, err := orch.RunDelayCascade(ctx, agentic.DelayCascadeInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunDelayCascade: %v", err)
	}
	if reasoner.calls != 0 {
		t.Errorf("reasoner calls = %d, want 0 (no critical path → skip reasoning)", reasoner.calls)
	}
	if res.CardsCreated != 0 || res.Impacts != 0 {
		t.Errorf("result = %+v, want zero cards / impacts", res)
	}
	if cards := readCascadeFeedCards(t, fx.pool, fx.orgID); len(cards) != 0 {
		t.Errorf("persisted feed cards = %d, want 0", len(cards))
	}
	if got := cascadeImpactAuditCount(t, fx.pool, fx.orgID); got != 0 {
		t.Errorf("audit rows = %d, want 0", got)
	}
}

// TestAgenticOrchestrator_RunDelayCascade_ReasonerUnavailable proves the
// soft-fail leg: a real critical path loads, but the reasoner reports
// ErrReasonerUnavailable (no Anthropic key for the org). The orchestrator
// swallows the advisory gap — zero cards, zero audit rows, nil error — so a
// missing AI key never fails the deterministic job.
func TestAgenticOrchestrator_RunDelayCascade_ReasonerUnavailable(t *testing.T) {
	fx := newCascadeWorkspaceFixture(t)
	ctx := context.Background()

	seedCascadeTask(t, fx.pool, fx.projectID, "1.0", "Foundation", 5, 0, true)
	seedCascadeTask(t, fx.pool, fx.projectID, "2.0", "Framing", 3, 0, true)

	reasoner := &fakeCascadeReasoner{err: fmt.Errorf("no key: %w", agentic.ErrReasonerUnavailable)}
	orch := agentic.NewOrchestrator(reasoner, fx.workspace, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	res, err := orch.RunDelayCascade(ctx, agentic.DelayCascadeInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunDelayCascade soft-fail returned err = %v, want nil", err)
	}
	if reasoner.calls != 1 {
		t.Errorf("reasoner calls = %d, want 1 (critical path → reasoning attempted)", reasoner.calls)
	}
	if res.CardsCreated != 0 || res.Impacts != 0 {
		t.Errorf("result = %+v, want zero cards / impacts on soft-fail", res)
	}
	if cards := readCascadeFeedCards(t, fx.pool, fx.orgID); len(cards) != 0 {
		t.Errorf("persisted feed cards = %d, want 0 on soft-fail", len(cards))
	}
	if got := cascadeImpactAuditCount(t, fx.pool, fx.orgID); got != 0 {
		t.Errorf("audit rows = %d, want 0 on soft-fail", got)
	}
}
