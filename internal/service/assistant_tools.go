package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/authz"
)

// This file holds the read-only agentic.ToolExecutor implementations for the
// conversational assistant (Phase 2c §5). Each tool is built as a caller-bound
// CLOSURE: buildRegistry (in assistant.go) captures the caller's org_id, role,
// and oidc subject from the JWT claims and seals them into the executor. The
// model-supplied tool input carries ONLY query-shaping args (project_id,
// status, currency_code, paging) — it has NO org_id/role/sub field, so a
// prompt-injected model cannot escape its org or role no matter what it emits.
//
// RBAC defense-in-depth lives here as layer 3: every executor re-checks
// authz.RoleAtLeast(boundRole, minRole) BEFORE dispatching to the underlying
// deterministic service, returning a uniform forbidden IsError tool_result (no
// service call) when the caller's role is insufficient. This is the load-bearing
// fix for the role-ungated financial services (GetProjectBudgets /
// GetProjectFinancials / GetOrgFinancialsSummary), whose only other role gate is
// route-level middleware.
//
// EXACT/FUZZY split: these tools return the deterministic service output
// VERBATIM (integer cents + currency_code intact). The model plans + phrases
// only; no tool computes or writes a number. All tools are read-only.

// assistantToolMaxPerPage is the hard clamp on the model-supplied per_page for
// any listing tool. The model's paging args are untrusted; cap them so a chatty
// or adversarial model cannot pull an unbounded page into the loop's byte
// budget.
const assistantToolMaxPerPage = 50

// toolForbiddenContent is the uniform tool_result content returned by the
// in-executor MinRole re-check (layer 3). It is intentionally generic so it
// leaks no information about what the higher-privilege tool would have returned.
const toolForbiddenContent = `{"error":"forbidden","detail":"your role is not permitted to use this tool"}`

// executorFunc adapts a plain function to agentic.ToolExecutor so each tool can
// be a closure over the caller's sealed identity + the backing services.
type executorFunc func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error)

func (f executorFunc) Execute(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
	return f(ctx, input)
}

// roleGate is the in-executor MinRole re-check (layer 3). It returns a uniform
// forbidden tool_result and ok=false when boundRole does not meet minRole, so
// the caller returns BEFORE making any service call. ok=true means dispatch may
// proceed.
func roleGate(boundRole, minRole string) (agentic.ToolResult, bool) {
	if !authz.RoleAtLeast(boundRole, minRole) {
		return agentic.ToolResult{Content: toolForbiddenContent, IsError: true}, false
	}
	return agentic.ToolResult{}, true
}

// clampPerPage clamps an untrusted model-supplied per_page into [1, max]. A
// zero/negative value falls back to the max (a sane default page).
func clampPerPage(perPage int) int {
	if perPage <= 0 || perPage > assistantToolMaxPerPage {
		return assistantToolMaxPerPage
	}
	return perPage
}

// toolResultJSON marshals an engine fact into a tool_result. A marshal failure
// (programmer error — the value is always a concrete service type) surfaces as
// an IsError result so the loop keeps running rather than aborting.
func toolResultJSON(v any) (agentic.ToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return agentic.ToolResult{Content: `{"error":"internal","detail":"failed to encode result"}`, IsError: true}, nil
	}
	return agentic.ToolResult{Content: string(b)}, nil
}

// toolResultFromServiceErr maps a service error onto a soft IsError tool_result.
// NotFound (incl. cross-org via VerifyProjectInOrg) and InvalidInput (bad model
// args) become IsError results the model recovers from in prose; the loop never
// aborts. Anything else is also surfaced as IsError (read-only tools never need
// to bubble a hard error up the loop).
func toolResultFromServiceErr(err error) agentic.ToolResult {
	switch {
	case errors.Is(err, ErrNotFound):
		return agentic.ToolResult{Content: `{"error":"not_found","detail":"no such resource in your organization"}`, IsError: true}
	case errors.Is(err, ErrInvalidInput):
		return agentic.ToolResult{Content: fmt.Sprintf(`{"error":"invalid_input","detail":%q}`, err.Error()), IsError: true}
	default:
		return agentic.ToolResult{Content: `{"error":"tool_error","detail":"the tool could not complete the request"}`, IsError: true}
	}
}

// parseProjectID extracts and validates a required project_id from untrusted
// model input. A malformed/missing id is a soft IsError (the model self-corrects).
func parseProjectID(raw uuid.UUID) (uuid.UUID, *agentic.ToolResult) {
	if raw == uuid.Nil {
		res := agentic.ToolResult{Content: `{"error":"invalid_input","detail":"project_id is required and must be a UUID"}`, IsError: true}
		return uuid.Nil, &res
	}
	return raw, nil
}

// ---- model-facing arg structs (query-shaping ONLY — never identity) --------
//
// CRITICAL: none of these structs has an org_id/role/sub field. json.Unmarshal
// silently drops any such field a prompt-injected model emits, so there is no
// code path that reads identity from model input. Identity is bound in the
// closure from claims.

type listProjectsArgs struct {
	Status  string `json:"status,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

type getProjectArgs struct {
	ProjectID uuid.UUID `json:"project_id"`
}

type getScheduleGanttArgs struct {
	ProjectID uuid.UUID `json:"project_id"`
}

type listProjectTasksArgs struct {
	ProjectID  uuid.UUID `json:"project_id"`
	Status     string    `json:"status,omitempty"`
	IsCritical *bool     `json:"is_critical,omitempty"`
}

type listProcurementArgs struct {
	ProjectID    uuid.UUID `json:"project_id"`
	StatusFilter []string  `json:"status_filter,omitempty"`
}

type listFeedCardsArgs struct {
	StatusFilter   []string `json:"status_filter,omitempty"`
	PriorityFilter []string `json:"priority_filter,omitempty"`
	Page           int      `json:"page,omitempty"`
	PerPage        int      `json:"per_page,omitempty"`
}

type getProjectBudgetsArgs struct {
	ProjectID uuid.UUID `json:"project_id"`
}

type getOrgFinancialsArgs struct {
	CurrencyCode string `json:"currency_code,omitempty"`
}

// unmarshalArgs decodes untrusted model input into a typed arg struct. An empty
// input ("" / "{}") is valid (a no-arg tool call); only malformed JSON is an
// error. A decode error surfaces as a soft IsError tool_result.
func unmarshalArgs(input json.RawMessage, dst any) *agentic.ToolResult {
	if len(input) == 0 {
		return nil
	}
	if err := json.Unmarshal(input, dst); err != nil {
		res := agentic.ToolResult{Content: `{"error":"invalid_input","detail":"could not parse tool arguments as JSON"}`, IsError: true}
		return &res
	}
	return nil
}

// ---- the 8 read-only executors (caller-bound closures) ---------------------
//
// Each constructor captures boundOrgID/boundRole/boundSub + the backing service
// and returns an agentic.ToolExecutor. The minRole arg is the tool's declared
// MinRole; it is re-checked here (layer 3) independently of registry filtering
// (layer 4).

func (s *AssistantService) newListProjectsExecutor(boundOrgID uuid.UUID, boundRole, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args listProjectsArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		projects, err := s.projects.ListProjects(ctx, ListProjectsInput{
			OrgID:   boundOrgID, // SEALED from claims — never from model input
			Status:  args.Status,
			Page:    args.Page,
			PerPage: clampPerPage(args.PerPage),
		})
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		return toolResultJSON(map[string]any{"projects": projects, "count": len(projects)})
	})
}

func (s *AssistantService) newGetProjectExecutor(boundOrgID uuid.UUID, boundRole, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args getProjectArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		pid, bad := parseProjectID(args.ProjectID)
		if bad != nil {
			return *bad, nil
		}
		project, err := s.projects.GetProject(ctx, boundOrgID, pid)
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		return toolResultJSON(map[string]any{"project": project})
	})
}

func (s *AssistantService) newGetScheduleGanttExecutor(boundOrgID uuid.UUID, boundRole, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args getScheduleGanttArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		pid, bad := parseProjectID(args.ProjectID)
		if bad != nil {
			return *bad, nil
		}
		// GetGantt runs VerifyProjectInOrg(pid, boundOrgID); a cross-org id
		// surfaces as ErrNotFound -> IsError, no probe leak.
		gantt, err := s.schedule.GetGantt(ctx, pid, boundOrgID)
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		return toolResultJSON(map[string]any{"gantt": gantt})
	})
}

func (s *AssistantService) newListProjectTasksExecutor(boundOrgID uuid.UUID, boundRole, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args listProjectTasksArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		pid, bad := parseProjectID(args.ProjectID)
		if bad != nil {
			return *bad, nil
		}
		tasks, err := s.schedule.ListProjectTasks(ctx, ListProjectTasksInput{
			ProjectID:  pid,
			OrgID:      boundOrgID, // SEALED from claims
			Status:     args.Status,
			IsCritical: args.IsCritical,
		})
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		return toolResultJSON(map[string]any{"tasks": tasks, "count": len(tasks)})
	})
}

func (s *AssistantService) newListProcurementExecutor(boundOrgID uuid.UUID, boundRole, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args listProcurementArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		pid, bad := parseProjectID(args.ProjectID)
		if bad != nil {
			return *bad, nil
		}
		items, err := s.procurement.ListProcurement(ctx, pid, boundOrgID, args.StatusFilter)
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		return toolResultJSON(map[string]any{"procurement": items, "count": len(items)})
	})
}

func (s *AssistantService) newListFeedCardsExecutor(boundOrgID uuid.UUID, boundRole, boundSub, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args listFeedCardsArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		// ListFeed enforces per-user/role visibility in SQL via the sealed
		// CallerOIDCSubject + CallerRole — the model cannot widen its scope.
		res, err := s.feed.ListFeed(ctx, FeedListOptions{
			CallerOrgID:       boundOrgID,
			CallerOIDCSubject: boundSub,
			CallerRole:        boundRole,
			StatusFilter:      args.StatusFilter,
			PriorityFilter:    args.PriorityFilter,
			Page:              args.Page,
			PerPage:           clampPerPage(args.PerPage),
		})
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		return toolResultJSON(map[string]any{"feed_cards": res.Cards, "total": res.Total})
	})
}

func (s *AssistantService) newGetProjectBudgetsExecutor(boundOrgID uuid.UUID, boundRole, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		// MinRole=admin re-check (layer 3) — GetProjectBudgets takes no role
		// param, so this in-executor gate is the real second layer.
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args getProjectBudgetsArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		pid, bad := parseProjectID(args.ProjectID)
		if bad != nil {
			return *bad, nil
		}
		budgets, err := s.budget.GetProjectBudgets(ctx, pid, boundOrgID)
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		// Integer cents + currency_code returned VERBATIM; the model never sums.
		return toolResultJSON(map[string]any{"budgets": budgets, "count": len(budgets)})
	})
}

func (s *AssistantService) newGetOrgFinancialsExecutor(boundOrgID uuid.UUID, boundRole, minRole string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		// MinRole=admin re-check (layer 3) — GetProjectFinancials/GetARAging
		// take no role param; this gate is the load-bearing cross-role fix.
		if denied, ok := roleGate(boundRole, minRole); !ok {
			return denied, nil
		}
		var args getOrgFinancialsArgs
		if bad := unmarshalArgs(input, &args); bad != nil {
			return *bad, nil
		}
		financials, err := s.budget.GetProjectFinancials(ctx, boundOrgID, args.CurrencyCode)
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		arAging, err := s.budget.GetARAging(ctx, boundOrgID, args.CurrencyCode)
		if err != nil {
			return toolResultFromServiceErr(err), nil
		}
		// Engine-grouped by currency; integer cents intact. The system prompt
		// forbids the model from summing or converting across currencies.
		return toolResultJSON(map[string]any{
			"project_financials": financials,
			"ar_aging":           arAging,
		})
	})
}
