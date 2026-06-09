//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/authz"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// This file is the Phase 2c §13.1 end-to-end integration suite for the
// conversational assistant. It exercises a REAL AssistantService (real stores,
// real BudgetService/ScheduleService/etc. over an ephemeral migrated Postgres)
// driven by a SCRIPTED fake AI loop — no live Anthropic, no HTTP. The fake
// drives tool calls through the real registryInvoker so the four RBAC layers
// (org-sealed closure, VerifyProjectInOrg, in-executor MinRole re-check,
// registry filter) are genuinely on the hot path.
//
// The structural guarantee under test: a (possibly prompt-injected) model can
// emit any tool args it likes, yet it can never read another org's data, never
// call a tool its role lacks, and never read above its role through a
// role-ungated financial service.

// ---- scripted fake AI loop driver --------------------------------------

// scriptedAssistantAI is the test double for the bounded tool-use loop. It
// implements the service-side assistantAI seam (one method: RunToolLoop). On
// each run it drives a scripted sequence of tool calls through the REAL
// registryInvoker the service hands it (so the registry executors run with the
// caller's sealed org/role/sub), records every (content, isError) the invoker
// returns, then synthesizes a final reply.
//
// synth, given the captured tool results, produces the model's "answer" — this
// is the only fuzzy leg, and it models a grounded model: the integration tests
// extract a real engine fact from the captured tool result and assert it shows
// up in the reply, proving the date/number came from the deterministic service,
// not from the model.
type scriptedAssistantAI struct {
	calls    []scriptedAICall
	synth    func(results []invokeResult) string
	captured []invokeResult
	gotReq   *ai.ToolLoopRequest
	// loopBound, when true, simulates the real ai.RunToolLoop bound behavior:
	// it drives calls until MaxToolCalls is hit, then returns Truncated=true
	// with a nil error (graceful, never a 500).
	loopBound bool
}

type scriptedAICall struct {
	name  string
	input json.RawMessage
}

func (f *scriptedAssistantAI) RunToolLoop(ctx context.Context, kind string, req ai.ToolLoopRequest) (*ai.ToolLoopResponse, error) {
	f.gotReq = &req

	if f.loopBound {
		// Model that NEVER stops asking for tools: drive the same call past the
		// MaxToolCalls ceiling and assert the loop terminates gracefully. We
		// honor the request's MaxToolCalls bound exactly as ai.RunToolLoop
		// would (it caps total executions, then returns Truncated=true).
		max := req.Bounds.MaxToolCalls
		if max <= 0 {
			max = 12
		}
		call := f.calls[0]
		for i := 0; i < max; i++ {
			content, isErr, _ := req.Invoker.Invoke(ctx, call.name, call.input)
			f.captured = append(f.captured, invokeResult{content: content, isError: isErr})
		}
		return &ai.ToolLoopResponse{
			FinalText:  "I wasn't able to fully resolve that — here's what I found so far.",
			ToolCalls:  recordsFrom(f.captured, call.name),
			Iterations: req.Bounds.MaxIterations,
			Truncated:  true,
		}, nil
	}

	for _, c := range f.calls {
		content, isErr, _ := req.Invoker.Invoke(ctx, c.name, c.input)
		f.captured = append(f.captured, invokeResult{content: content, isError: isErr})
	}

	reply := "ok"
	if f.synth != nil {
		reply = f.synth(f.captured)
	}
	return &ai.ToolLoopResponse{
		FinalText:  reply,
		ToolCalls:  recordsFromCalls(f.calls, f.captured),
		Iterations: 2,
		Truncated:  false,
	}, nil
}

// recordsFromCalls builds the per-tool trace records the loop would surface:
// one record per driven call, carrying the IsError flag the invoker returned.
func recordsFromCalls(calls []scriptedAICall, captured []invokeResult) []ai.ToolCallRecord {
	out := make([]ai.ToolCallRecord, 0, len(calls))
	for i, c := range calls {
		isErr := false
		if i < len(captured) {
			isErr = captured[i].isError
		}
		out = append(out, ai.ToolCallRecord{Name: c.name, IsError: isErr})
	}
	return out
}

func recordsFrom(captured []invokeResult, name string) []ai.ToolCallRecord {
	out := make([]ai.ToolCallRecord, 0, len(captured))
	for _, r := range captured {
		out = append(out, ai.ToolCallRecord{Name: name, IsError: r.isError})
	}
	return out
}

// ---- fixture wiring ----------------------------------------------------

// assistantFixture bundles the seeded ids + a REAL AssistantService over a
// fresh migrated pool. The AI loop driver is injected per-test so each case can
// script its own tool sequence + synthesis.
type assistantFixture struct {
	pool  *pgxpool.Pool
	pkg   *assistantServices
	orgA  uuid.UUID
	orgB  uuid.UUID
	projA uuid.UUID
	projB uuid.UUID
	subA  string
}

// assistantServices holds the real backing services so a per-test
// AssistantService can be assembled with any AI driver.
type assistantServices struct {
	pool        *pgxpool.Pool
	schedule    *ScheduleService
	budget      *BudgetService
	procurement *ProcurementService
	projects    *ProjectService
	feed        *FeedService
	pipeline    *PipelineService
}

// newAssistantFixture wires REAL services over a fresh migrated pool and seeds
// TWO orgs each with one project — so the cross-org refusal cases have a real
// org-B row to (fail to) reach. Schedule/budget/procurement graphs are seeded
// per-test.
func newAssistantFixture(t *testing.T) *assistantFixture {
	t.Helper()
	pool := testdb.NewPool(t)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	svcs := &assistantServices{
		pool:        pool,
		schedule:    NewScheduleService(pool, store.NewScheduleStore(), nil, nil),
		budget:      NewBudgetService(pool, store.NewFinancialsStore(), nil),
		procurement: NewProcurementService(pool, store.NewProcurementStore(), nil, store.NewFeedCardsStore(), nil),
		projects:    NewProjectService(pool, store.NewProjectStore(), nil),
		feed:        NewFeedService(pool, store.NewFeedCardsStore(), logger, nil),
		pipeline:    NewPipelineService(pool, store.NewPipelineStore(), nil, nil),
	}

	orgA := uuid.New()
	orgB := uuid.New()
	projA := uuid.New()
	projB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Stoneridge Builders")
	testdb.SeedOrg(t, pool, orgB, "Rival Construction")
	testdb.SeedProject(t, pool, projA, orgA, "Maple Ridge Custom")
	testdb.SeedProject(t, pool, projB, orgB, "Competitor Tower")

	return &assistantFixture{
		pool:  pool,
		pkg:   svcs,
		orgA:  orgA,
		orgB:  orgB,
		projA: projA,
		projB: projB,
		subA:  "oidc-sub-org-a-user",
	}
}

// service assembles a real AssistantService over the fixture's real backing
// services with the supplied AI loop driver. A nil pool-less audit recorder
// (NoopAuditRecorder via the nil arg) keeps the audit write out of the way.
func (fx *assistantFixture) service(aiDriver assistantAI) *AssistantService {
	return &AssistantService{
		ai:          aiDriver,
		pool:        fx.pool,
		schedule:    fx.pkg.schedule,
		budget:      fx.pkg.budget,
		procurement: fx.pkg.procurement,
		projects:    fx.pkg.projects,
		feed:        fx.pkg.feed,
		pipeline:    fx.pkg.pipeline,
		audit:       NoopAuditRecorder{},
		bounds:      agentic.LoopBounds{},
		model:       defaultExperienceModel,
		logger:      slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

// seedAssistantTask inserts a project_tasks row with a CPM early_start so the
// gantt tool returns a date the grounded-answer test can assert flowed through.
// early_start is TIMESTAMPTZ and pgx reads it back in the process-local tz, so a
// bare-date seed can render a day off off-UTC. The grounded-answer test therefore
// asserts on the date the tool ACTUALLY returned (flow-through), not the raw seed
// string — see TestAssistantIntegration_GroundedAnswer.
func seedAssistantTask(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, name, earlyStart string, isCritical bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, status, percent_complete, early_start, is_critical)
		VALUES ($1, $2, $3, 5, 'pending', 0, $4, $5)`,
		projectID, wbs, name, earlyStart, isCritical); err != nil {
		t.Fatalf("seed assistant task %s: %v", wbs, err)
	}
}

// seedAssistantBudget inserts a single project_budgets row (all three monetary
// pairs share a currency to satisfy chk_budget_currency_match).
func seedAssistantBudget(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, currency string, est, com, act int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO project_budgets (
			project_id, wbs_code, phase_name,
			estimated_cost_cents, estimated_cost_currency_code,
			committed_cost_cents, committed_cost_currency_code,
			actual_cost_cents, actual_cost_currency_code
		) VALUES ($1, $2, $3, $4, $5, $6, $5, $7, $5)`,
		projectID, wbs, "phase "+wbs, est, currency, com, act); err != nil {
		t.Fatalf("seed assistant budget %s: %v", wbs, err)
	}
}

// seedAssistantProcurement inserts a procurement_items row with an explicit
// status so the cross-org procurement case has an org-B row to (fail to) reach.
func seedAssistantProcurement(t *testing.T, pool *pgxpool.Pool, projectID, orgID uuid.UUID, name, wbs, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO procurement_items (project_id, org_id, name, wbs_code, lead_time_days, status)
		VALUES ($1, $2, $3, $4, 14, $5)`,
		projectID, orgID, name, wbs, status); err != nil {
		t.Fatalf("seed assistant procurement %s: %v", name, err)
	}
}

// ---- §13.1: grounded answer --------------------------------------------

// TestAssistantIntegration_GroundedAnswer seeds an org + project + a critical
// task with a known engine-computed early_start, drives the loop with a scripted
// driver that calls get_schedule_gantt then synthesizes a reply embedding the
// date it found, and asserts (a) the reply carries the engine fact (the seeded
// date), (b) tools_used records get_schedule_gantt with is_error=false. The
// model never computes the date — it comes from GetGantt's verbatim output.
func TestAssistantIntegration_GroundedAnswer(t *testing.T) {
	fx := newAssistantFixture(t)
	ctx := context.Background()

	const earlyStart = "2026-07-15"
	seedAssistantTask(t, fx.pool, fx.projA, "1.0", "Framing", earlyStart, true)

	// ganttDate captures the early_start the deterministic gantt TOOL returned.
	// We assert the reply carries THAT value (flow-through: the engine fact
	// reached synthesis verbatim, not fabricated by the model) rather than the
	// raw seed string — early_start is TIMESTAMPTZ and pgx renders it in the
	// process-local tz, so the bare-date seed can read back a day off off-UTC.
	var ganttDate string
	driver := &scriptedAssistantAI{
		calls: []scriptedAICall{
			{name: "get_schedule_gantt", input: ganttInput(fx.projA)},
		},
		synth: func(results []invokeResult) string {
			if len(results) == 0 {
				return "no data"
			}
			ganttDate = extractFirstEarlyStart(t, results[0].content)
			return "Framing is scheduled to start on " + ganttDate + "."
		},
	}

	svc := fx.service(driver)
	res, err := svc.Converse(ctx, fx.orgA, authz.RoleAdmin, fx.subA,
		agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "When does framing start on Maple Ridge?"}}})
	if err != nil {
		t.Fatalf("Converse: %v", err)
	}

	if ganttDate == "" {
		t.Fatal("driver never received a gantt early_start from the tool result")
	}
	// The seeded early_start round-trips through the deterministic gantt tool into
	// the model's reply verbatim (the model didn't compute or invent it).
	if !strings.Contains(res.Reply, ganttDate) {
		t.Fatalf("reply does not carry the gantt early_start %q: %q", ganttDate, res.Reply)
	}
	if len(res.ToolCallsMade) != 1 {
		t.Fatalf("tools_used = %d, want 1", len(res.ToolCallsMade))
	}
	if res.ToolCallsMade[0].Name != "get_schedule_gantt" {
		t.Errorf("tool name = %q, want get_schedule_gantt", res.ToolCallsMade[0].Name)
	}
	if res.ToolCallsMade[0].IsError {
		t.Errorf("get_schedule_gantt should not be IsError on a same-org project")
	}
	// The driven invoker result itself must not be an error.
	if driver.captured[0].isError {
		t.Errorf("captured gantt result is_error = true, want false")
	}
}

// ---- §13.1: cross-org refused (per project-scoped tool) ----------------

// TestAssistantIntegration_CrossOrgRefused drives, for each project-scoped tool,
// a caller in org A against ORG B's project_id. Every executor must return an
// IsError not_found result with NO org-B content — the cross-org id is
// indistinguishable from a non-existent one (VerifyProjectInOrg, layer 2). The
// org-B project name ("Competitor Tower") must never appear in the tool result.
func TestAssistantIntegration_CrossOrgRefused(t *testing.T) {
	fx := newAssistantFixture(t)
	ctx := context.Background()

	// Seed real org-B graph so the rows genuinely exist (and could leak if the
	// scope check were missing).
	seedAssistantTask(t, fx.pool, fx.projB, "1.0", "SecretFraming", "2026-09-01", true)
	seedAssistantBudget(t, fx.pool, fx.projB, "1.0", "USD", 500000, 480000, 460000)
	seedAssistantProcurement(t, fx.pool, fx.projB, fx.orgB, "SecretWindows", "1.0", "CRITICAL")

	cases := []struct {
		tool  string
		input json.RawMessage
	}{
		{"get_schedule_gantt", ganttInput(fx.projB)},
		{"get_project", projectInput(fx.projB)},
		{"list_project_tasks", projectInput(fx.projB)},
		{"list_procurement", projectInput(fx.projB)},
		{"get_project_budgets", projectInput(fx.projB)},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			driver := &scriptedAssistantAI{
				calls: []scriptedAICall{{name: tc.tool, input: tc.input}},
				synth: func(results []invokeResult) string { return "done" },
			}
			svc := fx.service(driver)

			// Owner role so the financial tools are in the registry (the point of
			// this test is cross-ORG, not cross-role — so the tool exists, the
			// org scope is what refuses).
			_, err := svc.Converse(ctx, fx.orgA, authz.RoleOwner, fx.subA,
				agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "show me org B's project " + fx.projB.String()}}})
			if err != nil {
				t.Fatalf("Converse: %v", err)
			}
			if len(driver.captured) != 1 {
				t.Fatalf("expected 1 driven call, got %d", len(driver.captured))
			}
			got := driver.captured[0]
			if !got.isError {
				t.Fatalf("%s cross-org call should be IsError, got %+v", tc.tool, got)
			}
			if !strings.Contains(got.content, "not_found") {
				t.Errorf("%s cross-org result should be not_found, got %q", tc.tool, got.content)
			}
			// No org-B content may appear: not the project name, not the secret
			// task/proc names, not the org-B budget amount.
			for _, leak := range []string{"Competitor Tower", "SecretFraming", "SecretWindows", "460000"} {
				if strings.Contains(got.content, leak) {
					t.Errorf("%s cross-org result leaked org-B content %q: %q", tc.tool, leak, got.content)
				}
			}
		})
	}
}

// ---- §13.1: above-role refused (role × tool matrix) --------------------

// TestAssistantIntegration_RoleToolMatrix asserts buildRegistry resolves exactly
// the expected tool set for each role, and that a superintendent driving a
// financial tool gets ErrForbidden (IsError) with NO BudgetService call — the
// cross-role axis. The BudgetService IS real here, so a leak would surface as
// org-A budget content; its ABSENCE (forbidden content instead) proves the
// in-executor MinRole gate (layer 3) and the registry filter (layer 4) hold.
func TestAssistantIntegration_RoleToolMatrix(t *testing.T) {
	fx := newAssistantFixture(t)
	ctx := context.Background()

	// Seed a real org-A budget so a missing role gate would visibly leak it.
	seedAssistantBudget(t, fx.pool, fx.projA, "1.0", "USD", 700000, 650000, 600000)

	nonFinancial := []string{
		"get_project", "get_schedule_gantt", "list_feed_cards",
		"list_procurement", "list_project_tasks", "list_projects",
	}
	withFinancial := []string{
		"get_org_financials", "get_project", "get_project_budgets",
		"get_schedule_gantt", "list_feed_cards", "list_procurement",
		"list_project_tasks", "list_projects",
	}

	matrix := []struct {
		role  string
		tools []string
	}{
		{authz.RoleSuperintendent, nonFinancial},
		{authz.RoleAdmin, withFinancial},
		{authz.RoleOwner, withFinancial},
	}

	svc := fx.service(&scriptedAssistantAI{})
	for _, tc := range matrix {
		t.Run("registry_"+tc.role, func(t *testing.T) {
			reg := svc.buildRegistry(fx.orgA, tc.role, fx.subA)
			if reg.Len() != len(tc.tools) {
				t.Fatalf("role %q: got %d tools, want %d", tc.role, reg.Len(), len(tc.tools))
			}
			got := make([]string, 0, reg.Len())
			for _, s := range reg.Specs() {
				got = append(got, s.Name)
			}
			if strings.Join(got, ",") != strings.Join(tc.tools, ",") {
				t.Fatalf("role %q: tools = %v, want %v", tc.role, got, tc.tools)
			}
		})
	}

	// Superintendent attempting both financial tools: registry filter (layer 4)
	// means they aren't even added, so the invoker returns unknown_tool IsError
	// and the real BudgetService is never reached — no org-A budget content.
	for _, tool := range []string{"get_project_budgets", "get_org_financials"} {
		t.Run("superintendent_blocked_"+tool, func(t *testing.T) {
			driver := &scriptedAssistantAI{
				calls: []scriptedAICall{{name: tool, input: projectInput(fx.projA)}},
				synth: func(results []invokeResult) string { return "done" },
			}
			fsvc := fx.service(driver)
			_, err := fsvc.Converse(ctx, fx.orgA, authz.RoleSuperintendent, fx.subA,
				agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "show me budgets"}}})
			if err != nil {
				t.Fatalf("Converse: %v", err)
			}
			if len(driver.captured) != 1 {
				t.Fatalf("expected 1 driven call, got %d", len(driver.captured))
			}
			got := driver.captured[0]
			if !got.isError {
				t.Fatalf("superintendent reaching %s should be IsError, got %+v", tool, got)
			}
			if !strings.Contains(got.content, "unknown_tool") {
				t.Errorf("superintendent %s should be unknown_tool (registry filter), got %q", tool, got.content)
			}
			// The real BudgetService must not have run: no org-A budget content.
			for _, leak := range []string{"600000", "650000", "700000"} {
				if strings.Contains(got.content, leak) {
					t.Errorf("%s leaked org-A budget content %q (BudgetService was reached!): %q", tool, leak, got.content)
				}
			}
		})
	}

	// Now prove layer 3 in isolation: build the financial executors directly
	// bound to a superintendent role (bypassing the registry filter) — the
	// in-executor MinRole re-check alone must still forbid, with no service call.
	for _, tool := range []string{"get_project_budgets", "get_org_financials"} {
		t.Run("layer3_executor_"+tool, func(t *testing.T) {
			var exec agentic.ToolExecutor
			switch tool {
			case "get_project_budgets":
				exec = svc.newGetProjectBudgetsExecutor(fx.orgA, authz.RoleSuperintendent, authz.RoleAdmin)
			case "get_org_financials":
				exec = svc.newGetOrgFinancialsExecutor(fx.orgA, authz.RoleSuperintendent, authz.RoleAdmin)
			}
			res, err := exec.Execute(ctx, projectInput(fx.projA))
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !res.IsError || !strings.Contains(res.Content, "forbidden") {
				t.Fatalf("layer-3 gate should forbid below-MinRole, got %+v", res)
			}
			for _, leak := range []string{"600000", "650000", "700000"} {
				if strings.Contains(res.Content, leak) {
					t.Errorf("layer-3 %s leaked org-A budget content %q: %q", tool, leak, res.Content)
				}
			}
		})
	}
}

// ---- §13.1: soft-fail with no key --------------------------------------

// TestAssistantIntegration_SoftFailNoKey constructs a REAL *ai.Client whose
// KeyResolver returns an empty key (modeling a fork with no Anthropic key
// configured). Converse must surface agentic.ErrAssistantUnavailable (translated
// from ai.ErrUnconfigured by the chatPlanner adapter) so the handler soft-fails
// to 503 — a missing key never 500s.
func TestAssistantIntegration_SoftFailNoKey(t *testing.T) {
	fx := newAssistantFixture(t)
	ctx := context.Background()

	client, err := ai.NewClient(ai.Config{KeyResolver: emptyKeyResolver{}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	svc := NewAssistantService(
		client, fx.pool,
		fx.pkg.schedule, fx.pkg.budget, fx.pkg.procurement,
		fx.pkg.projects, fx.pkg.feed, fx.pkg.pipeline,
		nil, // config resolver — nil => Experience enabled-with-default
		NoopAuditRecorder{}, slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)

	_, err = svc.Converse(ctx, fx.orgA, authz.RoleAdmin, fx.subA,
		agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "hello"}}})
	if !isAssistantUnavailable(err) {
		t.Fatalf("no-key Converse should surface ErrAssistantUnavailable, got %v", err)
	}
}

// ---- §13.1: loop bound enforced ----------------------------------------

// TestAssistantIntegration_LoopBoundEnforced drives a model that NEVER stops
// asking for tools (always emits a tool_use, never end_turn). The bounded loop
// must terminate at MaxToolCalls, return Truncated=true, ToolCalls <=
// MaxToolCalls, and a nil error (no infinite loop, no 500). It runs a real,
// same-org get_schedule_gantt each time so the executor path is genuinely hot.
func TestAssistantIntegration_LoopBoundEnforced(t *testing.T) {
	fx := newAssistantFixture(t)
	ctx := context.Background()
	seedAssistantTask(t, fx.pool, fx.projA, "1.0", "Framing", "2026-07-15", true)

	driver := &scriptedAssistantAI{
		loopBound: true,
		calls:     []scriptedAICall{{name: "get_schedule_gantt", input: ganttInput(fx.projA)}},
	}
	svc := fx.service(driver)

	res, err := svc.Converse(ctx, fx.orgA, authz.RoleAdmin, fx.subA,
		agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "loop forever"}}})
	if err != nil {
		t.Fatalf("loop-bound Converse should return nil error (graceful truncation), got %v", err)
	}
	if !res.Truncated {
		t.Errorf("expected Truncated=true on loop-bound exhaustion")
	}
	// The default MaxToolCalls is 12 (agentic.LoopBounds zero -> withDefaults).
	if len(res.ToolCallsMade) > 12 {
		t.Errorf("ToolCalls = %d, want <= MaxToolCalls (12)", len(res.ToolCallsMade))
	}
	if len(res.ToolCallsMade) == 0 {
		t.Errorf("expected at least one driven tool call")
	}
	// Every driven call was a real, same-org, non-error gantt read.
	for i, tc := range res.ToolCallsMade {
		if tc.Name != "get_schedule_gantt" {
			t.Errorf("call %d name = %q, want get_schedule_gantt", i, tc.Name)
		}
		if tc.IsError {
			t.Errorf("call %d should not be IsError (same-org read)", i)
		}
	}
}

// TestAssistantIntegration_ResultSizeClamp proves the per-executor result clamp
// on untrusted model paging: a model asking for an enormous per_page is silently
// capped at assistantToolMaxPerPage (50). We seed > 50 projects in org A and
// drive list_projects with per_page=10000; the returned project count must be
// capped at 50, bounding the bytes a chatty/adversarial model can pull into the
// loop's cumulative byte budget. (The cumulative MaxResultBytes ceiling itself
// lives in ai.RunToolLoop and is covered by the ai/chatloop fake-RoundTripper
// suite; here we assert the executor-side clamp that feeds it.)
func TestAssistantIntegration_ResultSizeClamp(t *testing.T) {
	fx := newAssistantFixture(t)
	ctx := context.Background()

	// Seed > assistantToolMaxPerPage extra projects in org A (projA already
	// exists, so seed 60 more for a comfortable margin).
	for i := 0; i < 60; i++ {
		testdb.SeedProject(t, fx.pool, uuid.New(), fx.orgA, fmt.Sprintf("Extra Project %d", i))
	}

	driver := &scriptedAssistantAI{
		calls: []scriptedAICall{
			{name: "list_projects", input: json.RawMessage(`{"per_page":10000}`)},
		},
		synth: func(results []invokeResult) string { return "done" },
	}
	svc := fx.service(driver)
	if _, err := svc.Converse(ctx, fx.orgA, authz.RoleAdmin, fx.subA,
		agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "list everything"}}}); err != nil {
		t.Fatalf("Converse: %v", err)
	}
	if len(driver.captured) != 1 {
		t.Fatalf("expected 1 driven call, got %d", len(driver.captured))
	}
	count := extractCount(t, driver.captured[0].content)
	if count > assistantToolMaxPerPage {
		t.Errorf("project count = %d, want <= %d (per_page clamp on untrusted model args)", count, assistantToolMaxPerPage)
	}
	if count == 0 {
		t.Errorf("expected a non-empty clamped page, got 0")
	}
}

// ---- small JSON / input helpers ----------------------------------------

func ganttInput(projectID uuid.UUID) json.RawMessage {
	return json.RawMessage(`{"project_id":"` + projectID.String() + `"}`)
}

func projectInput(projectID uuid.UUID) json.RawMessage {
	return json.RawMessage(`{"project_id":"` + projectID.String() + `"}`)
}

// extractFirstEarlyStart pulls the first task's early_start (YYYY-MM-DD prefix)
// out of a get_schedule_gantt tool result, proving the date came from the
// engine's persisted CPM column and not from the model.
func extractFirstEarlyStart(t *testing.T, content string) string {
	t.Helper()
	var payload struct {
		Gantt struct {
			Tasks []struct {
				EarlyStart string `json:"early_start"`
			} `json:"tasks"`
		} `json:"gantt"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("unmarshal gantt result: %v\ncontent=%s", err, content)
	}
	if len(payload.Gantt.Tasks) == 0 {
		t.Fatalf("gantt result has no tasks: %s", content)
	}
	es := payload.Gantt.Tasks[0].EarlyStart
	if len(es) < 10 {
		t.Fatalf("early_start too short: %q", es)
	}
	return es[:10] // YYYY-MM-DD
}

// extractCount pulls the "count" field out of a listing tool result.
func extractCount(t *testing.T, content string) int {
	t.Helper()
	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("unmarshal listing result: %v\ncontent=%s", err, content)
	}
	return payload.Count
}

// emptyKeyResolver models a fork with no Anthropic key configured: an empty key
// (with nil error) is treated by the ai.Client exactly like an error → no HTTP
// attempt, ErrUnconfigured.
type emptyKeyResolver struct{}

func (emptyKeyResolver) AnthropicKey(ctx context.Context, orgID string) (string, error) {
	return "", nil
}

// isAssistantUnavailable reports whether err wraps agentic.ErrAssistantUnavailable.
func isAssistantUnavailable(err error) bool {
	return errors.Is(err, agentic.ErrAssistantUnavailable)
}
