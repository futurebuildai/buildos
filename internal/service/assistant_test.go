package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/authz"
	"github.com/futurebuildai/buildos/internal/connectors"
)

// ---- fake connectorToolSource (Phase 3b buildRegistry merge tests) ------

type fakeConnectorSource struct {
	tools []agentic.Tool
	err   error
	calls int
}

func (f *fakeConnectorSource) ToolsFor(_ context.Context, _ connectors.Caller) ([]agentic.Tool, error) {
	f.calls++
	return f.tools, f.err
}

// noopExecutor satisfies agentic.ToolExecutor for merge tests.
type noopExecutor struct{}

func (noopExecutor) Execute(_ context.Context, _ json.RawMessage) (agentic.ToolResult, error) {
	return agentic.ToolResult{Content: "{}"}, nil
}

// panicExecutor models a buggy/hostile (e.g. future 3b-ii MCP) executor.
type panicExecutor struct{}

func (panicExecutor) Execute(_ context.Context, _ json.RawMessage) (agentic.ToolResult, error) {
	panic("executor boom")
}

// A panicking tool executor must be recovered into a soft IsError result (the
// loop contract: a tool failure never aborts the loop / 500s the chat), not
// propagate as a panic. This hardens the connector seam for 3b-ii.
func TestRegistryInvoker_ExecutorPanic_RecoversAsSoftError(t *testing.T) {
	reg := agentic.NewAssistantRegistry()
	reg.Add(agentic.Tool{Spec: agentic.ToolSpec{Name: "conn__x__boom"}, Executor: panicExecutor{}})
	inv := registryInvoker{reg: reg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	content, isErr, err := inv.Invoke(context.Background(), "conn__x__boom", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("a panicking executor must NOT propagate a Go error: %v", err)
	}
	if !isErr {
		t.Error("a panicking executor must surface as a soft IsError result")
	}
	if !strings.Contains(content, "error") {
		t.Errorf("content = %q, want a soft error payload", content)
	}
}

// ---- fake assistantAI ---------------------------------------------------

// fakeAssistantAI is the test double for the bounded tool-use loop. It can
// either replay a scripted error/response, or drive the invoker with scripted
// tool calls and capture the results the executors return — letting us assert
// RBAC behavior end-to-end without an HTTP server or a database.
type fakeAssistantAI struct {
	// gotReq captures the last request RunToolLoop received.
	gotReq *ai.ToolLoopRequest
	// scriptedCalls are tool calls to drive through the invoker before
	// returning resp. Each is (name, input).
	scriptedCalls []scriptedToolCall
	// capturedResults receives (content, isError) for each driven call.
	capturedResults []invokeResult
	// resp / err are returned verbatim after driving the scripted calls.
	resp *ai.ToolLoopResponse
	err  error
}

type scriptedToolCall struct {
	name  string
	input json.RawMessage
}

type invokeResult struct {
	content string
	isError bool
}

func (f *fakeAssistantAI) RunToolLoop(ctx context.Context, kind string, req ai.ToolLoopRequest) (*ai.ToolLoopResponse, error) {
	f.gotReq = &req
	for _, c := range f.scriptedCalls {
		content, isErr, _ := req.Invoker.Invoke(ctx, c.name, c.input)
		f.capturedResults = append(f.capturedResults, invokeResult{content: content, isError: isErr})
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.resp == nil {
		return &ai.ToolLoopResponse{FinalText: "ok"}, nil
	}
	return f.resp, nil
}

// newTestAssistantService builds a service with the given fake AI and nil
// backing services. nil services are intentional: the RBAC tests assert the
// in-executor role gate returns BEFORE any service is touched, so a nil service
// (which would panic if dereferenced) is the strongest possible "no service
// call" assertion.
func newTestAssistantService(t *testing.T, fake assistantAI) *AssistantService {
	t.Helper()
	return &AssistantService{
		ai:     fake,
		model:  defaultExperienceModel,
		bounds: agentic.LoopBounds{},
		audit:  NoopAuditRecorder{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		// pool + services left nil — never reached in these unit tests.
	}
}

// ---- buildRegistry role filtering (layer 4) -----------------------------

func TestAssistant_BuildRegistry_RoleFiltering(t *testing.T) {
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	org := uuid.New()

	cases := []struct {
		role  string
		tools []string
	}{
		{authz.RoleSuperintendent, []string{
			"get_project", "get_schedule_gantt", "list_feed_cards",
			"list_procurement", "list_project_tasks", "list_projects",
		}},
		{authz.RoleAdmin, []string{
			"get_org_financials", "get_project", "get_project_budgets",
			"get_schedule_gantt", "list_feed_cards", "list_procurement",
			"list_project_tasks", "list_projects",
		}},
		{authz.RoleOwner, []string{
			"get_org_financials", "get_project", "get_project_budgets",
			"get_schedule_gantt", "list_feed_cards", "list_procurement",
			"list_project_tasks", "list_projects",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			reg := svc.buildRegistry(context.Background(), org, tc.role, "sub-1")
			if reg.Len() != len(tc.tools) {
				t.Fatalf("role %q: got %d tools, want %d", tc.role, reg.Len(), len(tc.tools))
			}
			got := make([]string, 0, len(reg.Specs()))
			for _, s := range reg.Specs() {
				got = append(got, s.Name)
			}
			if strings.Join(got, ",") != strings.Join(tc.tools, ",") {
				t.Fatalf("role %q: tools = %v, want %v", tc.role, got, tc.tools)
			}
		})
	}
}

// --- Phase 3b: connector-tool merge into buildRegistry ---

func adminInternalToolCount(t *testing.T) int {
	t.Helper()
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	return svc.buildRegistry(context.Background(), uuid.New(), authz.RoleAdmin, "sub-1").Len()
}

func TestAssistant_BuildRegistry_MergesEnabledConnectorTool(t *testing.T) {
	base := adminInternalToolCount(t)
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	svc.connectorSvc = &fakeConnectorSource{tools: []agentic.Tool{{
		Spec:     agentic.ToolSpec{Name: "conn__reference__glossary", Description: "d", InputSchema: json.RawMessage(`{}`)},
		MinRole:  authz.RoleAdmin, // floored by ToolsFor in production
		Executor: noopExecutor{},
	}}}

	reg := svc.buildRegistry(context.Background(), uuid.New(), authz.RoleAdmin, "sub-1")
	if reg.Len() != base+1 {
		t.Fatalf("admin registry = %d tools, want %d (internal + 1 connector)", reg.Len(), base+1)
	}
	if !reg.Has("conn__reference__glossary") {
		t.Error("the enabled connector tool must be mounted for an admin caller")
	}
}

func TestAssistant_BuildRegistry_ConnectorErrorFailsClosed(t *testing.T) {
	base := adminInternalToolCount(t)
	fake := &fakeConnectorSource{err: errors.New("connectors_config db down")}
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	svc.connectorSvc = fake

	// A connector resolve error must NOT break chat: serve internal tools only.
	reg := svc.buildRegistry(context.Background(), uuid.New(), authz.RoleAdmin, "sub-1")
	if fake.calls != 1 {
		t.Fatalf("ToolsFor calls = %d, want 1", fake.calls)
	}
	if reg.Len() != base {
		t.Fatalf("fail-closed registry = %d tools, want %d (internal only)", reg.Len(), base)
	}
}

func TestAssistant_BuildRegistry_ConnectorToolRoleFiltered(t *testing.T) {
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	svc.connectorSvc = &fakeConnectorSource{tools: []agentic.Tool{{
		Spec:     agentic.ToolSpec{Name: "conn__reference__glossary", Description: "d", InputSchema: json.RawMessage(`{}`)},
		MinRole:  authz.RoleAdmin, // connector tools are floored to admin
		Executor: noopExecutor{},
	}}}

	// A superintendent caller is below the admin floor → the connector tool is
	// filtered out by layer-4 (RoleAtLeast), same as the internal admin tools.
	reg := svc.buildRegistry(context.Background(), uuid.New(), authz.RoleSuperintendent, "sub-1")
	if reg.Has("conn__reference__glossary") {
		t.Error("a connector tool floored at admin must NOT be visible to a superintendent")
	}
}

// A field_worker meets no tool's MinRole (and is route-gated out anyway): the
// registry is empty rather than panicking or exposing tools.
func TestAssistant_BuildRegistry_FieldWorkerGetsNoTools(t *testing.T) {
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	reg := svc.buildRegistry(context.Background(), uuid.New(), authz.RoleFieldWorker, "sub-1")
	if reg.Len() != 0 {
		t.Fatalf("field_worker registry Len = %d, want 0", reg.Len())
	}
}

// ---- in-executor MinRole re-check (layer 3) -----------------------------
//
// A superintendent driving a financial tool must get a forbidden IsError result
// with NO service call. The backing BudgetService is nil here, so any dispatch
// would panic — proving the gate returns first.

func TestAssistant_FinancialTool_BelowMinRole_ForbiddenNoServiceCall(t *testing.T) {
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	org := uuid.New()

	for _, name := range []string{"get_project_budgets", "get_org_financials"} {
		t.Run(name, func(t *testing.T) {
			// Build the executor exactly as buildRegistry would for an admin tool,
			// but bind a superintendent role (below admin). The registry would
			// normally not even .Add this for a superintendent (layer 4); here we
			// construct it directly to prove layer 3 alone closes the hole.
			var exec agentic.ToolExecutor
			switch name {
			case "get_project_budgets":
				exec = svc.newGetProjectBudgetsExecutor(org, authz.RoleSuperintendent, authz.RoleAdmin)
			case "get_org_financials":
				exec = svc.newGetOrgFinancialsExecutor(org, authz.RoleSuperintendent, authz.RoleAdmin)
			}
			input := json.RawMessage(`{"project_id":"` + uuid.New().String() + `"}`)
			res, err := exec.Execute(context.Background(), input)
			if err != nil {
				t.Fatalf("Execute returned unexpected error: %v", err)
			}
			if !res.IsError {
				t.Fatalf("expected IsError result for below-MinRole caller, got %+v", res)
			}
			if !strings.Contains(res.Content, "forbidden") {
				t.Fatalf("expected forbidden content, got %q", res.Content)
			}
		})
	}
}

// An admin driving a financial tool passes the gate (and would proceed to the
// service). We can't run the real BudgetService without a DB, but we CAN assert
// the gate does not short-circuit: with a nil BudgetService the dispatch panics,
// which we recover and treat as "gate passed". (The integration test covers the
// real dispatch.)
func TestAssistant_FinancialTool_AtMinRole_PassesGate(t *testing.T) {
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	exec := svc.newGetProjectBudgetsExecutor(uuid.New(), authz.RoleAdmin, authz.RoleAdmin)
	input := json.RawMessage(`{"project_id":"` + uuid.New().String() + `"}`)

	defer func() {
		// A nil-pointer panic from the nil BudgetService means the gate let the
		// call through to dispatch — exactly what we want to prove.
		if r := recover(); r == nil {
			t.Fatalf("expected dispatch to be attempted (panic on nil service), but none occurred")
		}
	}()
	_, _ = exec.Execute(context.Background(), input)
}

// ---- model-supplied identity is ignored (invariant #1) ------------------
//
// The arg structs have no org_id/role/sub field, so a prompt-injected model
// emitting one has it silently dropped by json.Unmarshal. There is no code path
// that reads identity from input.

func TestAssistant_ArgStructs_DropModelSuppliedIdentity(t *testing.T) {
	// list_projects: a malicious org_id/role in the input must not appear on the
	// decoded struct (there is no field to receive it).
	raw := json.RawMessage(`{"status":"active","org_id":"00000000-0000-0000-0000-000000000009","role":"owner","per_page":7}`)
	var args listProjectsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if args.Status != "active" || args.PerPage != 7 {
		t.Fatalf("query-shaping args not parsed: %+v", args)
	}
	// Re-marshal and confirm no org_id/role leaked onto the struct's wire form.
	out, _ := json.Marshal(args)
	if strings.Contains(string(out), "org_id") || strings.Contains(string(out), "role") {
		t.Fatalf("identity field survived onto arg struct: %s", out)
	}
}

// ---- arg clamping -------------------------------------------------------

func TestAssistant_ClampPerPage(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, assistantToolMaxPerPage},
		{-5, assistantToolMaxPerPage},
		{1, 1},
		{49, 49},
		{50, 50},
		{51, assistantToolMaxPerPage},
		{10000, assistantToolMaxPerPage},
	}
	for _, c := range cases {
		if got := clampPerPage(c.in); got != c.want {
			t.Errorf("clampPerPage(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// ---- registryInvoker ----------------------------------------------------

func TestAssistant_RegistryInvoker_UnknownTool_IsErrorNoAbort(t *testing.T) {
	reg := agentic.NewAssistantRegistry()
	inv := registryInvoker{reg: reg}
	content, isErr, err := inv.Invoke(context.Background(), "no_such_tool", nil)
	if err != nil {
		t.Fatalf("Invoke returned a hard error (should never abort the loop): %v", err)
	}
	if !isErr {
		t.Fatalf("unknown tool should be IsError")
	}
	if !strings.Contains(content, "unknown_tool") {
		t.Fatalf("expected unknown_tool content, got %q", content)
	}
}

func TestAssistant_RegistryInvoker_ForwardsExecutorResult(t *testing.T) {
	reg := agentic.NewAssistantRegistry()
	reg.Add(agentic.Tool{
		Spec:    agentic.ToolSpec{Name: "echo"},
		MinRole: authz.RoleSuperintendent,
		Executor: executorFunc(func(_ context.Context, _ json.RawMessage) (agentic.ToolResult, error) {
			return agentic.ToolResult{Content: `{"ok":true}`, IsError: false}, nil
		}),
	})
	inv := registryInvoker{reg: reg}
	content, isErr, err := inv.Invoke(context.Background(), "echo", nil)
	if err != nil || isErr || content != `{"ok":true}` {
		t.Fatalf("unexpected: content=%q isErr=%v err=%v", content, isErr, err)
	}
}

// ---- chatPlanner soft-fail mapping --------------------------------------

func TestAssistant_ChatPlanner_ErrUnconfigured_MapsToUnavailable(t *testing.T) {
	fake := &fakeAssistantAI{err: ai.ErrUnconfigured}
	p := chatPlanner{ai: fake, orgID: uuid.New(), model: defaultExperienceModel}
	reg := agentic.NewAssistantRegistry()
	_, err := p.Plan(context.Background(), "sys", agentic.ChatInput{}, reg, agentic.LoopBounds{})
	if !errors.Is(err, agentic.ErrAssistantUnavailable) {
		t.Fatalf("ErrUnconfigured should map to ErrAssistantUnavailable, got %v", err)
	}
}

func TestAssistant_ChatPlanner_OtherError_PassesThrough(t *testing.T) {
	sentinel := errors.New("rate limited")
	fake := &fakeAssistantAI{err: sentinel}
	p := chatPlanner{ai: fake, orgID: uuid.New(), model: defaultExperienceModel}
	reg := agentic.NewAssistantRegistry()
	_, err := p.Plan(context.Background(), "sys", agentic.ChatInput{}, reg, agentic.LoopBounds{})
	if errors.Is(err, agentic.ErrAssistantUnavailable) {
		t.Fatalf("a non-ErrUnconfigured error must NOT map to ErrAssistantUnavailable")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the underlying error to pass through, got %v", err)
	}
}

func TestAssistant_ChatPlanner_MapsResultAndStampsOrg(t *testing.T) {
	fake := &fakeAssistantAI{resp: &ai.ToolLoopResponse{
		FinalText:  "the answer",
		ToolCalls:  []ai.ToolCallRecord{{Name: "get_project", IsError: false}},
		Iterations: 2,
		Truncated:  true,
	}}
	org := uuid.New()
	p := chatPlanner{ai: fake, orgID: org, model: defaultExperienceModel}
	reg := agentic.NewAssistantRegistry()

	in := agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "hi"}}}
	res, err := p.Plan(context.Background(), "sys", in, reg, agentic.LoopBounds{MaxIterations: 3})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if res.Reply != "the answer" || res.Iterations != 2 || !res.Truncated {
		t.Fatalf("result not mapped: %+v", res)
	}
	if len(res.ToolCallsMade) != 1 || res.ToolCallsMade[0].Name != "get_project" {
		t.Fatalf("tool traces not mapped: %+v", res.ToolCallsMade)
	}
	// The request must carry the model, system prompt, and mapped messages.
	if fake.gotReq == nil {
		t.Fatal("RunToolLoop was not called")
	}
	if fake.gotReq.Model != defaultExperienceModel || fake.gotReq.System != "sys" {
		t.Fatalf("model/system not threaded: %+v", fake.gotReq)
	}
	if len(fake.gotReq.Messages) != 1 || fake.gotReq.Messages[0].Role != "user" {
		t.Fatalf("messages not mapped: %+v", fake.gotReq.Messages)
	}
}

// ---- Converse soft-fail when no AI client -------------------------------

func TestAssistant_Converse_NilAI_ReturnsUnavailable(t *testing.T) {
	// NewAssistantService with a nil *ai.Client leaves s.ai unset (typed-nil
	// guard). Converse must soft-fail to ErrAssistantUnavailable, not panic.
	svc := NewAssistantService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.Converse(context.Background(), uuid.New(), authz.RoleAdmin, "sub-1", agentic.ChatInput{})
	if !errors.Is(err, agentic.ErrAssistantUnavailable) {
		t.Fatalf("nil AI client should yield ErrAssistantUnavailable, got %v", err)
	}
}

func TestAssistant_Converse_RejectsNilOrg(t *testing.T) {
	svc := newTestAssistantService(t, &fakeAssistantAI{})
	_, err := svc.Converse(context.Background(), uuid.Nil, authz.RoleAdmin, "sub-1", agentic.ChatInput{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil org should be ErrInvalidInput, got %v", err)
	}
}

// ---- Converse end-to-end with a scripted tool call (RBAC enforced) ------
//
// Drive Converse as a superintendent; the fake AI scripts a call to the
// admin-only get_org_financials tool. Because buildRegistry never adds it for a
// superintendent (layer 4), the invoker returns an unknown_tool IsError — proving
// the cross-role hole is closed even when the model attempts the financial tool.
// The nil pool means audit is skipped; no service is ever dispatched.

func TestAssistant_Converse_SuperintendentCannotReachFinancialTool(t *testing.T) {
	fake := &fakeAssistantAI{
		scriptedCalls: []scriptedToolCall{
			{name: "get_org_financials", input: json.RawMessage(`{}`)},
		},
		resp: &ai.ToolLoopResponse{FinalText: "done"},
	}
	svc := newTestAssistantService(t, fake)

	_, err := svc.Converse(context.Background(), uuid.New(), authz.RoleSuperintendent, "sub-1",
		agentic.ChatInput{Messages: []agentic.ChatTurn{{Role: "user", Text: "show me org financials"}}})
	if err != nil {
		t.Fatalf("Converse error: %v", err)
	}
	if len(fake.capturedResults) != 1 {
		t.Fatalf("expected 1 driven tool call, got %d", len(fake.capturedResults))
	}
	got := fake.capturedResults[0]
	if !got.isError || !strings.Contains(got.content, "unknown_tool") {
		t.Fatalf("superintendent reaching get_org_financials should be unknown_tool IsError, got %+v", got)
	}
}
