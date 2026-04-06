package physics

import (
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// loadTestWBS creates a minimal set of WBS tasks and dependencies for testing.
// The reference vault used data.LoadMasterWBS() which reads an embedded JSON file.
// Since that embedded data is not available in the new codebase, we construct
// a representative test fixture inline.
func loadTestWBS(t *testing.T) ([]models.WBSTask, []models.WBSDependency) {
	t.Helper()

	tasks := []models.WBSTask{
		{Code: "7.0", Name: "Site Clearing", BaseDurationDays: 2},
		{Code: "7.1", Name: "Rough Grading", BaseDurationDays: 3},
		{Code: "7.2", Name: "Utility Trenching", BaseDurationDays: 2},
		{Code: "7.3", Name: "Soil Compaction", BaseDurationDays: 1},
		{Code: "7.4", Name: "Fine Grading", BaseDurationDays: 2},
		{Code: "8.0", Name: "Footings Layout", BaseDurationDays: 1},
		{Code: "8.1", Name: "Footing Excavation", BaseDurationDays: 2},
		{Code: "8.2", Name: "Footing Pour", BaseDurationDays: 1},
		{Code: "8.3", Name: "Foundation Wall Forms", BaseDurationDays: 3},
		{Code: "8.4", Name: "Foundation Wall Pour", BaseDurationDays: 1},
		{Code: "8.5", Name: "Foundation Cure", BaseDurationDays: 5},
		{Code: "8.6", Name: "Strip Forms & Backfill", BaseDurationDays: 2},
		{Code: "8.7", Name: "Waterproofing", BaseDurationDays: 2},
		{Code: "8.8", Name: "Drain Inspection", BaseDurationDays: 1, IsInspection: true},
		{Code: "9.0", Name: "Sill Plate & Anchor Bolts", BaseDurationDays: 1},
		{Code: "9.1", Name: "First Floor Framing", BaseDurationDays: 5},
		{Code: "9.2", Name: "Second Floor Framing", BaseDurationDays: 5},
		{Code: "9.3", Name: "Roof Framing", BaseDurationDays: 4},
		{Code: "10.0", Name: "Rough Plumbing", BaseDurationDays: 3},
		{Code: "10.1", Name: "Rough Electrical", BaseDurationDays: 3},
	}

	deps := []models.WBSDependency{
		{PredecessorCode: "7.0", SuccessorCode: "7.1", Type: models.DependencyTypeFS},
		{PredecessorCode: "7.1", SuccessorCode: "7.2", Type: models.DependencyTypeFS},
		{PredecessorCode: "7.2", SuccessorCode: "7.3", Type: models.DependencyTypeFS},
		{PredecessorCode: "7.3", SuccessorCode: "7.4", Type: models.DependencyTypeFS},
		{PredecessorCode: "7.4", SuccessorCode: "8.0", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.0", SuccessorCode: "8.1", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.1", SuccessorCode: "8.2", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.2", SuccessorCode: "8.3", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.3", SuccessorCode: "8.4", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.4", SuccessorCode: "8.5", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.5", SuccessorCode: "8.6", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.6", SuccessorCode: "8.7", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.7", SuccessorCode: "8.8", Type: models.DependencyTypeFS},
		{PredecessorCode: "8.8", SuccessorCode: "9.0", Type: models.DependencyTypeFS},
		{PredecessorCode: "9.0", SuccessorCode: "9.1", Type: models.DependencyTypeFS},
		{PredecessorCode: "9.1", SuccessorCode: "9.2", Type: models.DependencyTypeFS},
		{PredecessorCode: "9.2", SuccessorCode: "9.3", Type: models.DependencyTypeFS},
		{PredecessorCode: "9.3", SuccessorCode: "10.0", Type: models.DependencyTypeFS},
		{PredecessorCode: "10.0", SuccessorCode: "10.1", Type: models.DependencyTypeFS},
	}

	return tasks, deps
}

func taskExists(tasks []models.WBSTask, code string) bool {
	for _, t := range tasks {
		if t.Code == code {
			return true
		}
	}
	return false
}

func getTask(tasks []models.WBSTask, code string) *models.WBSTask {
	for _, t := range tasks {
		if t.Code == code {
			return &t
		}
	}
	return nil
}

func depExists(deps []models.WBSDependency, pred, succ string) bool {
	for _, d := range deps {
		if d.PredecessorCode == pred && d.SuccessorCode == succ {
			return true
		}
	}
	return false
}

func TestScoping_SlabFoundation(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	ctx := ProjectScopeContext{
		FoundationType: "slab",
		Stories:        2,
	}

	result, resultDeps, changes := ApplyScope(tasks, deps, ctx)

	// 8.7 (waterproofing) and 8.8 (drain inspection) should be removed
	if taskExists(result, "8.7") {
		t.Error("expected 8.7 to be removed for slab foundation")
	}
	if taskExists(result, "8.8") {
		t.Error("expected 8.8 to be removed for slab foundation")
	}

	// No deps should reference removed tasks
	for _, d := range resultDeps {
		if d.PredecessorCode == "8.7" || d.SuccessorCode == "8.7" {
			t.Errorf("orphaned dep referencing removed task 8.7: %+v", d)
		}
		if d.PredecessorCode == "8.8" || d.SuccessorCode == "8.8" {
			t.Errorf("orphaned dep referencing removed task 8.8: %+v", d)
		}
	}

	if len(changes) == 0 {
		t.Error("expected scope changes to be recorded")
	}
}

func TestScoping_BasementFoundation(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	ctx := ProjectScopeContext{
		FoundationType: "basement",
		Stories:        2,
	}

	result, _, changes := ApplyScope(tasks, deps, ctx)

	// Basement tasks should be added
	for _, code := range []string{"8.12", "8.13", "8.14"} {
		if !taskExists(result, code) {
			t.Errorf("expected task %s to be added for basement foundation", code)
		}
	}

	if len(changes) == 0 {
		t.Error("expected scope changes to be recorded")
	}
}

func TestScoping_SingleStory(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	originalTask91 := getTask(tasks, "9.1")
	if originalTask91 == nil {
		t.Fatal("expected task 9.1 to exist in master WBS")
	}
	originalDuration := originalTask91.BaseDurationDays

	ctx := ProjectScopeContext{
		FoundationType: "slab",
		Stories:        1,
	}

	result, resultDeps, _ := ApplyScope(tasks, deps, ctx)

	// 9.2 (second floor framing) should be removed
	if taskExists(result, "9.2") {
		t.Error("expected 9.2 to be removed for single-story")
	}

	// 9.1 should have reduced duration
	task91 := getTask(result, "9.1")
	if task91 == nil {
		t.Fatal("expected task 9.1 to still exist")
	}
	expectedDuration := originalDuration * 0.7
	if task91.BaseDurationDays != expectedDuration {
		t.Errorf("expected 9.1 duration %.2f, got %.2f", expectedDuration, task91.BaseDurationDays)
	}

	// No orphaned deps referencing 9.2
	for _, d := range resultDeps {
		if d.PredecessorCode == "9.2" || d.SuccessorCode == "9.2" {
			t.Errorf("orphaned dep referencing removed task 9.2: %+v", d)
		}
	}
}

func TestScoping_ThreeStories(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	ctx := ProjectScopeContext{
		FoundationType: "crawlspace",
		Stories:        3,
	}

	result, _, changes := ApplyScope(tasks, deps, ctx)

	// Engineered floor system should be added
	if !taskExists(result, "9.8") {
		t.Error("expected task 9.8 (engineered floor system) to be added for 3+ stories")
	}

	// Framing durations should be increased
	hasIncrease := false
	for _, c := range changes {
		if c.DurationAdjustments != nil {
			if mult, ok := c.DurationAdjustments["9.1"]; ok && mult > 1.0 {
				hasIncrease = true
			}
		}
	}
	if !hasIncrease {
		t.Error("expected framing durations to be increased for 3+ stories")
	}
}

func TestScoping_LargeHouse(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	ctx := ProjectScopeContext{
		FoundationType: "slab",
		Stories:        2,
		GSF:            5000,
	}

	result, _, _ := ApplyScope(tasks, deps, ctx)

	if !taskExists(result, "7.5") {
		t.Error("expected task 7.5 (extended site prep) to be added for gsf>4000")
	}
}

func TestScoping_Hillside(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	original80 := getTask(tasks, "8.0")
	if original80 == nil {
		t.Fatal("expected 8.0 to exist")
	}
	originalDuration := original80.BaseDurationDays

	ctx := ProjectScopeContext{
		FoundationType: "crawlspace",
		Stories:        2,
		Topography:     "hillside",
	}

	result, _, _ := ApplyScope(tasks, deps, ctx)

	// Retaining wall should be added
	if !taskExists(result, "7.6") {
		t.Error("expected task 7.6 (retaining wall) to be added for hillside")
	}

	// Foundation durations should be increased 40%
	task80 := getTask(result, "8.0")
	if task80 == nil {
		t.Fatal("expected 8.0 to exist after scoping")
	}
	expectedDuration := originalDuration * 1.4
	if task80.BaseDurationDays != expectedDuration {
		t.Errorf("expected 8.0 duration %.2f, got %.2f", expectedDuration, task80.BaseDurationDays)
	}
}

func TestScoping_CompositeRules(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	ctx := ProjectScopeContext{
		FoundationType: "basement",
		Stories:        3,
		GSF:            5500,
		Topography:     "hillside",
	}

	result, resultDeps, changes := ApplyScope(tasks, deps, ctx)

	// All additions should be present
	expectedAdded := []string{"8.12", "8.13", "8.14", "9.8", "7.5", "7.6"}
	for _, code := range expectedAdded {
		if !taskExists(result, code) {
			t.Errorf("expected task %s to exist after composite scoping", code)
		}
	}

	// Deps should be clean (no orphans)
	validCodes := make(map[string]bool)
	for _, t := range result {
		validCodes[t.Code] = true
	}
	for _, d := range resultDeps {
		if !validCodes[d.PredecessorCode] {
			t.Errorf("orphaned dep: predecessor %s not in task set", d.PredecessorCode)
		}
		if !validCodes[d.SuccessorCode] {
			t.Errorf("orphaned dep: successor %s not in task set", d.SuccessorCode)
		}
	}

	if len(changes) < 4 {
		t.Errorf("expected at least 4 scope changes, got %d", len(changes))
	}
}

func TestScoping_InProgressProject(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	ctx := ProjectScopeContext{
		FoundationType:    "slab",
		Stories:           2,
		CompletedWBSCodes: []string{"7.x", "8.x"},
	}

	result, _, changes := ApplyScope(tasks, deps, ctx)

	// All 7.x and 8.x tasks should still exist but with adjusted durations
	for _, task := range result {
		if task.Code == "7.0" || task.Code == "8.0" {
			// Completed tasks should have minimal duration
			if task.BaseDurationDays > 0.5 {
				t.Errorf("expected completed task %s to have minimal duration, got %.2f", task.Code, task.BaseDurationDays)
			}
		}
	}

	// 9.x and later should be untouched
	task90 := getTask(result, "9.0")
	if task90 == nil {
		t.Fatal("expected 9.0 to exist")
	}
	origTask90 := getTask(tasks, "9.0")
	if task90.BaseDurationDays != origTask90.BaseDurationDays {
		t.Errorf("expected pending task 9.0 duration unchanged, got %.2f", task90.BaseDurationDays)
	}

	// Should have a scope change for in-progress
	hasIPChange := false
	for _, c := range changes {
		if c.RuleApplied == "in-progress: mark completed tasks" {
			hasIPChange = true
		}
	}
	if !hasIPChange {
		t.Error("expected in-progress scope change to be recorded")
	}
}

func TestScoping_DependencyIntegrity(t *testing.T) {
	tasks, deps := loadTestWBS(t)
	ctx := ProjectScopeContext{
		FoundationType: "slab",
		Stories:        1,
	}

	result, resultDeps, _ := ApplyScope(tasks, deps, ctx)

	validCodes := make(map[string]bool)
	for _, task := range result {
		validCodes[task.Code] = true
	}

	for _, d := range resultDeps {
		if !validCodes[d.PredecessorCode] {
			t.Errorf("orphaned dep: predecessor %q does not exist in task set", d.PredecessorCode)
		}
		if !validCodes[d.SuccessorCode] {
			t.Errorf("orphaned dep: successor %q does not exist in task set", d.SuccessorCode)
		}
	}
}

func TestCompletedTaskCodes_PhaseExpansion(t *testing.T) {
	tasks, _ := loadTestWBS(t)

	expanded := CompletedTaskCodes([]string{"7.x"}, tasks)
	if len(expanded) == 0 {
		t.Error("expected phase wildcard to expand to individual tasks")
	}

	// All expanded codes should start with "7."
	for _, code := range expanded {
		if len(code) < 2 || code[:2] != "7." {
			t.Errorf("expected all expanded codes to start with '7.', got %q", code)
		}
	}
}

func TestIsTaskCompleted(t *testing.T) {
	completed := []string{"7.x", "8.0", "8.1"}

	tests := []struct {
		code     string
		expected bool
	}{
		{"7.0", true},
		{"7.4", true},
		{"8.0", true},
		{"8.1", true},
		{"8.2", false},
		{"9.0", false},
	}

	for _, tt := range tests {
		if got := IsTaskCompleted(tt.code, completed); got != tt.expected {
			t.Errorf("IsTaskCompleted(%q) = %v, want %v", tt.code, got, tt.expected)
		}
	}
}
