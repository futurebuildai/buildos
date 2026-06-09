package connectors

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/futurebuildai/buildos/internal/agentic"
)

// referenceConnector is the in-process, read-only, NO-NETWORK built-in connector
// (Phase 3b-i). It proves the connector seam end-to-end without any egress: its
// tools are deterministic static lookups, identity-independent, and soft-fail a
// bad arg as an IsError result rather than a hard error. It declares a modest
// natural MinRole; service.ConnectorService floors every connector tool to admin.
type referenceConnector struct{}

func newReferenceConnector() *referenceConnector { return &referenceConnector{} }

func (c *referenceConnector) Name() string { return "reference" }

func (c *referenceConnector) Description() string {
	return "Read-only, in-process reference lookups (construction/CPM/ERP glossary; supported currencies). No external calls."
}

// referenceMinRole is the connector's NATURAL MinRole; the service floors it up
// to admin (a connector tool is never available below admin in 3b). Declared as
// a literal so the connectors package needn't import internal/authz.
const referenceMinRole = "superintendent"

func (c *referenceConnector) BuildTools(_ context.Context, _ Caller) ([]agentic.Tool, error) {
	return []agentic.Tool{
		{
			Spec: agentic.ToolSpec{
				Name:        "reference_glossary",
				Description: "Look up construction / CPM / ERP terms used by BuildOS. Omit 'term' to list every term.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"term":{"type":"string","description":"a term to define (case-insensitive); omit for all"}}}`),
			},
			MinRole:  referenceMinRole,
			Executor: executorFunc(referenceGlossaryExecute),
		},
		{
			Spec: agentic.ToolSpec{
				Name:        "reference_supported_currencies",
				Description: "List the currency codes BuildOS supports and the composite-currency rule.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			},
			MinRole:  referenceMinRole,
			Executor: executorFunc(referenceCurrenciesExecute),
		},
	}, nil
}

// referenceGlossary is the static, in-code term catalog. Deterministic; no I/O.
var referenceGlossary = map[string]string{
	"wbs":                     "Work Breakdown Structure — the hierarchical cost-code key (e.g. 1.2.3) that ties schedule tasks, procurement items, and budget lines together.",
	"critical path":           "The longest dependency chain of tasks with zero total float; any slip on it slips the whole project finish.",
	"total float":             "The whole days a task can slip without delaying the project finish. Zero float = on the critical path.",
	"gsf":                     "Gross Square Footage — the area metric the deterministic engine scales task durations by (see internal/physics/dhsm.go).",
	"cpm":                     "Critical Path Method — the deterministic forward/backward-pass scheduling engine at the core of BuildOS.",
	"feed card":               "An actionable item surfaced to operators (risks, reviews, recommendations) — the harness's surfacing mechanism.",
	"procurement criticality": "A foresight risk: a material/long-lead item whose order window is closing relative to its dependent tasks.",
	"budget burn":             "A foresight risk: a cost-coded budget line whose actual spend has crossed its estimate threshold (integer percent).",
}

type referenceGlossaryArgs struct {
	Term string `json:"term"`
}

func referenceGlossaryExecute(_ context.Context, input json.RawMessage) (agentic.ToolResult, error) {
	var args referenceGlossaryArgs
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return softError("invalid arguments: expected an object with an optional string 'term'"), nil
		}
	}
	term := strings.ToLower(strings.TrimSpace(args.Term))
	if term == "" {
		// List every term, sorted for determinism.
		keys := make([]string, 0, len(referenceGlossary))
		for k := range referenceGlossary {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]map[string]string, 0, len(keys))
		for _, k := range keys {
			out = append(out, map[string]string{"term": k, "definition": referenceGlossary[k]})
		}
		return toolJSON(map[string]any{"terms": out, "count": len(out)})
	}
	def, ok := referenceGlossary[term]
	if !ok {
		return softError("no glossary entry for term " + jsonQuote(args.Term) + "; omit 'term' to list all"), nil
	}
	return toolJSON(map[string]string{"term": term, "definition": def})
}

func referenceCurrenciesExecute(_ context.Context, _ json.RawMessage) (agentic.ToolResult, error) {
	return toolJSON(map[string]any{
		"supported": []string{"USD", "CAD"},
		"rule":      "All monetary values are integer cents paired with a currency_code. Cross-currency arithmetic is forbidden; aggregations group by currency_code.",
	})
}

// --- result helpers ---

func toolJSON(v any) (agentic.ToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return softError("failed to encode result"), nil
	}
	return agentic.ToolResult{Content: string(b)}, nil
}

func softError(msg string) agentic.ToolResult {
	return agentic.ToolResult{Content: msg, IsError: true}
}

// jsonQuote renders an untrusted term safely (quoted, escaped) for a soft-error
// message without importing strconv just for one quote.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
