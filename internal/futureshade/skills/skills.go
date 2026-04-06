// Package skills defines the Skill interface and Registry for FutureShade Action Bridge.
package skills

import (
	"context"
	"fmt"
	"sync"
)

// Result represents the outcome of a Skill execution.
type Result struct {
	// Success indicates if the skill executed successfully.
	Success bool `json:"success"`
	// Summary is a human-readable description of what happened.
	Summary string `json:"summary"`
	// Data contains any structured output from the skill (optional).
	Data map[string]any `json:"data,omitempty"`
}

// Skill is the interface that all executable skills must implement.
// Skills wrap existing agent/service functionality for Tribunal-triggered execution.
type Skill interface {
	// ID returns the unique identifier for this skill (e.g., "procurement_sync").
	ID() string
	// Execute runs the skill with the given parameters.
	// Returns a Result with success/failure and summary information.
	Execute(ctx context.Context, params map[string]any) (Result, error)
}

// Registry provides thread-safe registration and lookup of Skills.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]Skill
}

// NewRegistry creates a new empty skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]Skill),
	}
}

// Register adds a skill to the registry.
// Panics if a skill with the same ID is already registered (fail-fast for duplicates).
func (r *Registry) Register(skill Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := skill.ID()
	if _, exists := r.skills[id]; exists {
		panic(fmt.Sprintf("skill %q already registered", id))
	}
	r.skills[id] = skill
}

// Get retrieves a skill by ID.
// Returns the skill and true if found, or nil and false if not found.
func (r *Registry) Get(id string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, ok := r.skills[id]
	return skill, ok
}

// Has checks if a skill with the given ID is registered.
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.skills[id]
	return ok
}

// List returns all registered skill IDs.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.skills))
	for id := range r.skills {
		ids = append(ids, id)
	}
	return ids
}
