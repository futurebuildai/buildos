package physics

import (
	"testing"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/models/types"
)

// baseTemplate returns a small WBS template touching every code the scoping
// rules act on, so each rule can be exercised in isolation.
func baseTemplate() []models.WBSTask {
	return []models.WBSTask{
		{Code: "7.4", Name: "Site Prep", BaseDurationDays: 4},
		{Code: "8.1", Name: "Excavation", BaseDurationDays: 10},
		{Code: "8.2", Name: "Footing Inspection", BaseDurationDays: 1, IsInspection: true},
		{Code: "8.6", Name: "Foundation Walls", BaseDurationDays: 5},
		{Code: "8.7", Name: "Waterproofing", BaseDurationDays: 2},
		{Code: "8.8", Name: "Drain Tile", BaseDurationDays: 2},
		{Code: "9.1", Name: "First Floor Framing", BaseDurationDays: 10},
		{Code: "9.2", Name: "Second Floor Framing", BaseDurationDays: 10},
		{Code: "9.3", Name: "Roof Framing", BaseDurationDays: 10},
	}
}

// indexTasks maps result tasks by code (ApplyScope's result order is not stable
// for appended tasks, so assertions must look tasks up by code).
func indexTasks(tasks []models.WBSTask) map[string]models.WBSTask {
	m := make(map[string]models.WBSTask, len(tasks))
	for _, t := range tasks {
		m[t.Code] = t
	}
	return m
}

func hasChangeRule(changes []ScopeChange, rule string) bool {
	for _, c := range changes {
		if c.RuleApplied == rule {
			return true
		}
	}
	return false
}

func depExists(deps []models.WBSTemplateDep, pred, succ string) bool {
	for _, d := range deps {
		if d.PredecessorCode == pred && d.SuccessorCode == succ {
			return true
		}
	}
	return false
}

func TestApplyScope_FoundationSlab(t *testing.T) {
	// Case-insensitive ("Slab") removal of waterproofing/drain tile.
	tasks, _, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{FoundationType: "Slab"})
	idx := indexTasks(tasks)
	if _, ok := idx["8.7"]; ok {
		t.Error("8.7 should be removed for slab foundation")
	}
	if _, ok := idx["8.8"]; ok {
		t.Error("8.8 should be removed for slab foundation")
	}
	if !hasChangeRule(changes, "foundation=slab: remove waterproofing/drain tasks") {
		t.Error("missing slab scope-change record")
	}
}

func TestApplyScope_SlabFiltersDanglingDeps(t *testing.T) {
	// A dependency into a removed task must be dropped from the output.
	deps := []models.WBSTemplateDep{
		{PredecessorCode: "8.6", SuccessorCode: "8.7", Type: types.DependencyTypeFS},
	}
	_, outDeps, _ := ApplyScope(baseTemplate(), deps, ProjectScopeContext{FoundationType: "slab"})
	if depExists(outDeps, "8.6", "8.7") {
		t.Error("dependency into removed task 8.7 should be filtered out")
	}
}

func TestApplyScope_FoundationBasement(t *testing.T) {
	tasks, deps, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{FoundationType: "basement"})
	idx := indexTasks(tasks)
	for _, code := range []string{"8.12", "8.13", "8.14"} {
		if _, ok := idx[code]; !ok {
			t.Errorf("basement task %s should be added", code)
		}
	}
	// Added tasks bring their FS chain: 8.6->8.12->8.13->8.14.
	if !depExists(deps, "8.6", "8.12") || !depExists(deps, "8.12", "8.13") || !depExists(deps, "8.13", "8.14") {
		t.Errorf("basement dependency chain not wired: %+v", deps)
	}
	if !hasChangeRule(changes, "foundation=basement: add drain tile, damp proofing, egress tasks") {
		t.Error("missing basement scope-change record")
	}
}

func TestApplyScope_NoFoundationRuleForUnknownType(t *testing.T) {
	orig := baseTemplate()
	tasks, _, changes := ApplyScope(orig, nil, ProjectScopeContext{FoundationType: "crawlspace"})
	if len(tasks) != len(orig) {
		t.Errorf("unknown foundation type should not change task count: got %d want %d", len(tasks), len(orig))
	}
	if hasChangeRule(changes, "foundation=slab: remove waterproofing/drain tasks") {
		t.Error("no foundation change expected for crawlspace")
	}
}

func TestApplyScope_StoriesOne(t *testing.T) {
	tasks, _, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{Stories: 1})
	idx := indexTasks(tasks)
	if _, ok := idx["9.2"]; ok {
		t.Error("second floor framing 9.2 should be removed for single story")
	}
	if got := idx["9.1"].BaseDurationDays; !almostEqual(got, 7.0, 1e-9) {
		t.Errorf("9.1 duration = %v, want 7.0 (10 * 0.7)", got)
	}
	if !hasChangeRule(changes, "stories=1: remove second floor framing, reduce first floor framing 30%") {
		t.Error("missing single-story scope-change record")
	}
}

func TestApplyScope_StoriesThree(t *testing.T) {
	tasks, _, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{Stories: 3})
	idx := indexTasks(tasks)
	if _, ok := idx["9.8"]; !ok {
		t.Error("engineered floor system 9.8 should be added for 3+ stories")
	}
	for _, code := range []string{"9.1", "9.2", "9.3"} {
		if got := idx[code].BaseDurationDays; !almostEqual(got, 13.0, 1e-9) {
			t.Errorf("%s duration = %v, want 13.0 (10 * 1.3)", code, got)
		}
	}
	if !hasChangeRule(changes, "stories>=3: add engineered floor system, increase framing durations 30%") {
		t.Error("missing multi-story scope-change record")
	}
}

func TestApplyScope_StoriesTwoIsNeutral(t *testing.T) {
	orig := baseTemplate()
	tasks, _, changes := ApplyScope(orig, nil, ProjectScopeContext{Stories: 2})
	if len(tasks) != len(orig) {
		t.Errorf("two stories should not add/remove tasks: got %d want %d", len(tasks), len(orig))
	}
	idx := indexTasks(tasks)
	if got := idx["9.1"].BaseDurationDays; !almostEqual(got, 10.0, 1e-9) {
		t.Errorf("9.1 duration should be unchanged at 10.0, got %v", got)
	}
	for _, c := range changes {
		if c.RuleApplied == "stories=1: remove second floor framing, reduce first floor framing 30%" ||
			c.RuleApplied == "stories>=3: add engineered floor system, increase framing durations 30%" {
			t.Errorf("unexpected story change for 2 stories: %q", c.RuleApplied)
		}
	}
}

func TestApplyScope_SizeRule(t *testing.T) {
	// Above the 4000 sqft threshold adds extended site prep with a 7.4 pred.
	tasks, deps, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{GSF: 5000})
	idx := indexTasks(tasks)
	if _, ok := idx["7.5"]; !ok {
		t.Error("extended site prep 7.5 should be added for gsf>4000")
	}
	if !depExists(deps, "7.4", "7.5") {
		t.Error("7.5 should depend on 7.4")
	}
	if !hasChangeRule(changes, "gsf>4000: add extended site prep task") {
		t.Error("missing size scope-change record")
	}

	// At the threshold (<=4000) the rule is a no-op.
	tasksAt, _, _ := ApplyScope(baseTemplate(), nil, ProjectScopeContext{GSF: 4000})
	if _, ok := indexTasks(tasksAt)["7.5"]; ok {
		t.Error("7.5 should NOT be added at exactly 4000 sqft")
	}
}

func TestApplyScope_TopographyHillside(t *testing.T) {
	tasks, deps, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{Topography: "Hillside"})
	idx := indexTasks(tasks)
	if _, ok := idx["7.6"]; !ok {
		t.Error("retaining wall 7.6 should be added for hillside topography")
	}
	if !depExists(deps, "7.4", "7.6") {
		t.Error("7.6 should depend on 7.4")
	}
	// Non-inspection 8.* tasks extend 40%; the inspection (8.2) is untouched.
	if got := idx["8.1"].BaseDurationDays; !almostEqual(got, 14.0, 1e-9) {
		t.Errorf("8.1 duration = %v, want 14.0 (10 * 1.4)", got)
	}
	if got := idx["8.2"].BaseDurationDays; !almostEqual(got, 1.0, 1e-9) {
		t.Errorf("inspection 8.2 duration = %v, want 1.0 (untouched)", got)
	}
	if !hasChangeRule(changes, "topography=hillside: add retaining wall, extend foundation durations 40%") {
		t.Error("missing topography scope-change record")
	}
}

func TestApplyScope_TopographyFlatIsNeutral(t *testing.T) {
	orig := baseTemplate()
	tasks, _, _ := ApplyScope(orig, nil, ProjectScopeContext{Topography: "flat"})
	if _, ok := indexTasks(tasks)["7.6"]; ok {
		t.Error("retaining wall should not be added for flat topography")
	}
	if len(tasks) != len(orig) {
		t.Errorf("flat topography should not change task count: got %d want %d", len(tasks), len(orig))
	}
}

func TestApplyScope_InProgressExactCode(t *testing.T) {
	tasks, _, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{CompletedWBSCodes: []string{"8.1"}})
	idx := indexTasks(tasks)
	if got := idx["8.1"].BaseDurationDays; !almostEqual(got, 0.5, 1e-9) {
		t.Errorf("completed 8.1 duration = %v, want 0.5", got)
	}
	if got := idx["8.6"].BaseDurationDays; !almostEqual(got, 5.0, 1e-9) {
		t.Errorf("non-completed 8.6 duration = %v, want 5.0 (unchanged)", got)
	}
	if !hasChangeRule(changes, "in-progress: mark completed tasks") {
		t.Error("missing in-progress scope-change record")
	}
}

func TestApplyScope_InProgressWildcard(t *testing.T) {
	// "8.x" collapses every 8.* task to the 0.5-day completed marker.
	tasks, _, _ := ApplyScope(baseTemplate(), nil, ProjectScopeContext{CompletedWBSCodes: []string{"8.x"}})
	idx := indexTasks(tasks)
	for _, code := range []string{"8.1", "8.2", "8.6", "8.7", "8.8"} {
		if got := idx[code].BaseDurationDays; !almostEqual(got, 0.5, 1e-9) {
			t.Errorf("wildcard-completed %s duration = %v, want 0.5", code, got)
		}
	}
	// 9.* untouched.
	if got := idx["9.1"].BaseDurationDays; !almostEqual(got, 10.0, 1e-9) {
		t.Errorf("9.1 should be unchanged at 10.0, got %v", got)
	}
}

func TestApplyScope_InProgressNoCompleted(t *testing.T) {
	// A completed code that matches nothing produces no change.
	_, _, changes := ApplyScope(baseTemplate(), nil, ProjectScopeContext{CompletedWBSCodes: []string{"99.9"}})
	if hasChangeRule(changes, "in-progress: mark completed tasks") {
		t.Error("no in-progress change expected when no codes match")
	}
}

func TestCompletedTaskCodes(t *testing.T) {
	all := baseTemplate()
	got := CompletedTaskCodes([]string{"8.x", "9.1", "bogus"}, all)
	set := make(map[string]bool, len(got))
	for _, c := range got {
		set[c] = true
	}
	for _, code := range []string{"8.1", "8.2", "8.6", "8.7", "8.8", "9.1"} {
		if !set[code] {
			t.Errorf("expected %s in expanded completed set, got %v", code, got)
		}
	}
	if set["bogus"] {
		t.Error("unknown code 'bogus' should not appear in expansion")
	}
	if set["9.2"] {
		t.Error("9.2 was not in the completed input and should not appear")
	}
}

func TestIsTaskCompleted(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		completed []string
		want      bool
	}{
		{"exact match", "8.6", []string{"8.6"}, true},
		{"no match", "8.6", []string{"9.1"}, false},
		{"wildcard prefix match", "8.7", []string{"8.x"}, true},
		{"wildcard non-match", "9.1", []string{"8.x"}, false},
		{"empty completed", "8.6", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTaskCompleted(tt.code, tt.completed); got != tt.want {
				t.Errorf("IsTaskCompleted(%q, %v) = %v, want %v", tt.code, tt.completed, got, tt.want)
			}
		})
	}
}

func TestNewDep(t *testing.T) {
	d := newDep("8.6", "8.12")
	if d.PredecessorCode != "8.6" || d.SuccessorCode != "8.12" {
		t.Errorf("newDep codes = %q->%q, want 8.6->8.12", d.PredecessorCode, d.SuccessorCode)
	}
	if d.Type != types.DependencyTypeFS {
		t.Errorf("newDep type = %q, want FS", d.Type)
	}
	if d.LagDays != 0 {
		t.Errorf("newDep lag = %d, want 0", d.LagDays)
	}
}
