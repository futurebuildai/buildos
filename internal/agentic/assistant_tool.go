package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// ToolSpec is the model-facing declaration of one tool: stable name, prose
// description, and JSON Schema for its input. agentic owns the shape; the ai
// adapter renders it onto the Messages API tools[] array. The schema declares
// ONLY query-shaping args (project_id, status, currency_code) — NEVER org_id or
// role; those are bound from the caller, not the model.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema for the model-supplied args
}

// ToolResult is the deterministic outcome of executing one tool call. Content
// is the JSON-encoded engine fact fed back to the model as a tool_result block.
// IsError marks a soft failure (bad args, not-found, forbidden) so the model
// sees it and recovers in prose — it never aborts the loop.
type ToolResult struct {
	Content string
	IsError bool
}

// ToolExecutor runs ONE tool. input is the model-supplied JSON args
// (UNTRUSTED). The executor unmarshals + validates them, then calls the
// underlying deterministic service. CRITICAL: the caller's org_id and role are
// NOT in input — they are baked into the executor at construction time
// (per-request closure). A prompt-injected model can supply any args here and
// STILL cannot escape its org/role.
type ToolExecutor interface {
	Execute(ctx context.Context, input json.RawMessage) (ToolResult, error)
}

// Tool pairs a model-facing spec with its executor and the minimum role
// required to use it. MinRole is enforced at registry-build (the tool is not
// added for an insufficient caller) AND re-checked inside the executor.
type Tool struct {
	Spec     ToolSpec
	MinRole  string // "field_worker" | "superintendent" | "admin" | "owner"
	Executor ToolExecutor
}

// AssistantRegistry is the per-request catalog of tools the model may call. It
// is built fresh on each chat request, already filtered to the tools the
// caller's ROLE may use and bound to the caller's ORG. Not concurrency-safe:
// build once per request, then read-only.
type AssistantRegistry struct {
	tools map[string]Tool
}

// NewAssistantRegistry returns an empty registry ready for Add.
func NewAssistantRegistry() *AssistantRegistry {
	return &AssistantRegistry{tools: make(map[string]Tool)}
}

// Add registers a tool. It panics on an empty tool name or a duplicate name —
// both are programmer errors at registry-build time for the in-code internal
// tools. (Tools from runtime-configured CONNECTORS use TryAdd instead, where a
// name clash is a runtime condition that must never crash a request.)
func (r *AssistantRegistry) Add(t Tool) {
	if t.Spec.Name == "" {
		panic("agentic: AssistantRegistry.Add called with empty tool name")
	}
	if _, dup := r.tools[t.Spec.Name]; dup {
		panic(fmt.Sprintf("agentic: AssistantRegistry.Add duplicate tool name %q", t.Spec.Name))
	}
	r.tools[t.Spec.Name] = t
}

// TryAdd registers a tool if its name is non-empty and not already taken,
// returning true on success and false (WITHOUT panicking) on an empty or
// duplicate name. The connector-merge path uses this so a runtime-configured
// connector advertising a clashing or malformed tool name can never crash
// buildRegistry — it is skipped and logged by the caller instead.
func (r *AssistantRegistry) TryAdd(t Tool) bool {
	if t.Spec.Name == "" {
		return false
	}
	if _, dup := r.tools[t.Spec.Name]; dup {
		return false
	}
	r.tools[t.Spec.Name] = t
	return true
}

// Has reports whether a tool name is already registered.
func (r *AssistantRegistry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// Specs returns every registered tool's spec, stable-sorted by name, for the
// Messages API tools[] array.
func (r *AssistantRegistry) Specs() []ToolSpec {
	out := make([]ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Spec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Executor returns the executor for a tool name and whether it was registered.
func (r *AssistantRegistry) Executor(name string) (ToolExecutor, bool) {
	t, ok := r.tools[name]
	if !ok {
		return nil, false
	}
	return t.Executor, true
}

// Len reports how many tools are registered.
func (r *AssistantRegistry) Len() int { return len(r.tools) }
