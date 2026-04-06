package skills

import (
	"context"
	"sync"
	"testing"
)

// mockSkill is a test implementation of Skill interface.
type mockSkill struct {
	id        string
	shouldErr bool
}

func (m *mockSkill) ID() string { return m.id }

func (m *mockSkill) Execute(_ context.Context, _ map[string]any) (Result, error) {
	if m.shouldErr {
		return Result{Success: false, Summary: "mock error"}, context.Canceled
	}
	return Result{Success: true, Summary: "mock success"}, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	skill := &mockSkill{id: "test_skill"}

	r.Register(skill)

	got, ok := r.Get("test_skill")
	if !ok {
		t.Fatal("expected skill to be found")
	}
	if got.ID() != "test_skill" {
		t.Errorf("expected ID 'test_skill', got %q", got.ID())
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected skill to not be found")
	}
}

func TestRegistryHas(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockSkill{id: "existing"})

	if !r.Has("existing") {
		t.Error("expected Has to return true for registered skill")
	}
	if r.Has("nonexistent") {
		t.Error("expected Has to return false for unregistered skill")
	}
}

func TestRegistryPanicOnDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockSkill{id: "dup"})

	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()

	r.Register(&mockSkill{id: "dup"})
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockSkill{id: "skill_a"})
	r.Register(&mockSkill{id: "skill_b"})

	ids := r.List()
	if len(ids) != 2 {
		t.Errorf("expected 2 skills, got %d", len(ids))
	}

	// Check both exist (order not guaranteed)
	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found["skill_a"] || !found["skill_b"] {
		t.Error("expected both skill_a and skill_b in list")
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	// Pre-register some skills
	for i := 0; i < 10; i++ {
		r.Register(&mockSkill{id: "preregged_" + string(rune('a'+i))})
	}

	// Concurrent reads should not panic
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Mix of Has and Get calls
			if idx%2 == 0 {
				r.Has("preregged_a")
			} else {
				r.Get("preregged_b")
			}
			r.List()
		}(i)
	}
	wg.Wait()
}

func TestResultStructure(t *testing.T) {
	r := Result{
		Success: true,
		Summary: "test summary",
		Data: map[string]any{
			"key": "value",
		},
	}

	if !r.Success {
		t.Error("expected Success to be true")
	}
	if r.Summary != "test summary" {
		t.Errorf("expected Summary 'test summary', got %q", r.Summary)
	}
	if r.Data["key"] != "value" {
		t.Error("expected Data to contain key->value")
	}
}
