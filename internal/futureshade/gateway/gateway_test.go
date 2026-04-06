package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/futureshade"
	"github.com/futurebuild/futurebuild-os/internal/futureshade/skills"
	"github.com/google/uuid"
)

func TestValidatePlanSuccess(t *testing.T) {
	registry := skills.NewRegistry()
	registry.Register(&mockSkill{id: "skill_a"})
	registry.Register(&mockSkill{id: "skill_b"})

	g := &ExecutionGateway{
		config:   futureshade.Config{Enabled: true},
		registry: registry,
	}

	planJSON := `{"actions": [
		{"skill_id": "skill_a", "params": {}},
		{"skill_id": "skill_b", "params": {"key": "value"}}
	]}`

	if err := g.ValidatePlan([]byte(planJSON)); err != nil {
		t.Errorf("expected valid plan, got error: %v", err)
	}
}

func TestValidatePlanUnknownSkill(t *testing.T) {
	registry := skills.NewRegistry()
	registry.Register(&mockSkill{id: "skill_a"})

	g := &ExecutionGateway{
		config:   futureshade.Config{Enabled: true},
		registry: registry,
	}

	planJSON := `{"actions": [
		{"skill_id": "skill_a", "params": {}},
		{"skill_id": "unknown_skill", "params": {}}
	]}`

	err := g.ValidatePlan([]byte(planJSON))
	if err == nil {
		t.Error("expected error for unknown skill, got nil")
	}
}

func TestValidatePlanInvalidJSON(t *testing.T) {
	g := &ExecutionGateway{
		config:   futureshade.Config{Enabled: true},
		registry: skills.NewRegistry(),
	}

	err := g.ValidatePlan([]byte("invalid json"))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestExecutePlan_DisabledSkipsExecution(t *testing.T) {
	registry := skills.NewRegistry()
	registry.Register(&mockSkill{id: "skill_a"})

	g := NewExecutionGateway(
		futureshade.Config{Enabled: false},
		registry,
		nil, // no repo needed
		nil, // default logger
	)

	planJSON := `{"actions": [{"skill_id": "skill_a", "params": {}}]}`
	err := g.ExecutePlan(context.Background(), uuid.New(), []byte(planJSON))
	if err != nil {
		t.Errorf("expected nil error when disabled, got: %v", err)
	}
}

func TestExecutePlan_SuccessNoRepo(t *testing.T) {
	registry := skills.NewRegistry()
	registry.Register(&mockSkill{id: "skill_a"})
	registry.Register(&mockSkill{id: "skill_b"})

	g := NewExecutionGateway(
		futureshade.Config{Enabled: true},
		registry,
		nil, // no repo - exercises the no-repo path
		nil,
	)

	planJSON := `{"actions": [
		{"skill_id": "skill_a", "params": {}},
		{"skill_id": "skill_b", "params": {"key": "value"}}
	]}`

	err := g.ExecutePlan(context.Background(), uuid.New(), []byte(planJSON))
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRemediationPlanParsing(t *testing.T) {
	planJSON := `{
		"actions": [
			{"skill_id": "schedule_recalc", "params": {"project_id": "abc-123", "org_id": "org-456"}},
			{"skill_id": "procurement_sync", "params": {}}
		]
	}`

	var plan RemediationPlan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		t.Fatalf("failed to parse plan: %v", err)
	}

	if len(plan.Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(plan.Actions))
	}

	if plan.Actions[0].SkillID != "schedule_recalc" {
		t.Errorf("expected first action skill_id 'schedule_recalc', got %q", plan.Actions[0].SkillID)
	}

	if plan.Actions[0].Params["project_id"] != "abc-123" {
		t.Errorf("expected project_id 'abc-123', got %v", plan.Actions[0].Params["project_id"])
	}
}

func TestPlanActionSerialization(t *testing.T) {
	action := PlanAction{
		SkillID: "schedule_recalc",
		Params: map[string]any{
			"project_id": "test-uuid",
			"org_id":     "org-uuid",
		},
	}

	data, err := json.Marshal(action)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded PlanAction
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.SkillID != action.SkillID {
		t.Errorf("skill_id mismatch: expected %q, got %q", action.SkillID, decoded.SkillID)
	}
}

// mockSkill is a test implementation of Skill interface.
type mockSkill struct {
	id string
}

func (m *mockSkill) ID() string { return m.id }

func (m *mockSkill) Execute(_ context.Context, _ map[string]any) (skills.Result, error) {
	return skills.Result{Success: true, Summary: "mock"}, nil
}
