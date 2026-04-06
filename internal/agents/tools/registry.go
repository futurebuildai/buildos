// Package tools provides a registry of Claude-callable tools that wrap
// existing FutureBuild service interfaces. Each tool has a JSON schema
// definition (for Claude) and a handler function (for execution).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// Handler executes a tool call and returns a JSON-serialized result string.
// The ctx carries project/org scope. Input is the raw JSON from Claude's tool_use block.
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Tool pairs a Claude-visible definition with its server-side handler.
type Tool struct {
	Definition ai.ToolDefinition
	Handler    Handler
}

// Registry holds all tools available to Claude agents.
// Thread-safe for concurrent reads after initialization.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. Panics on duplicate names to fail fast at startup.
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[tool.Definition.Name]; exists {
		panic(fmt.Sprintf("duplicate tool registration: %s", tool.Definition.Name))
	}
	r.tools[tool.Definition.Name] = tool
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Definitions returns all tool definitions for sending to Claude.
func (r *Registry) Definitions() []ai.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ai.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition)
	}
	return defs
}

// Execute runs a tool by name and returns the result string.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Handler(ctx, input)
}

// --- Scope Context ---

// scopeKey is the context key for project/org scope.
type scopeKey struct{}

// Scope carries project and org context through tool execution.
type Scope struct {
	ProjectID uuid.UUID
	OrgID     uuid.UUID
	UserID    uuid.UUID
}

// WithScope injects scope into context.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// GetScope extracts scope from context.
func GetScope(ctx context.Context) (Scope, bool) {
	s, ok := ctx.Value(scopeKey{}).(Scope)
	return s, ok
}

// MustGetScope extracts scope or panics (for use inside tool handlers where scope is guaranteed).
func MustGetScope(ctx context.Context) Scope {
	s, ok := GetScope(ctx)
	if !ok {
		panic("tools: scope not found in context — did you forget WithScope?")
	}
	return s
}
