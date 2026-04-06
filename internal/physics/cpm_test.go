package physics

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// Helper to create a ProjectTask with minimal fields
func makeTask(wbsCode string) models.ProjectTask {
	return models.ProjectTask{
		ID:      uuid.New(),
		WBSCode: wbsCode,
		Name:    "Task " + wbsCode,
	}
}

// Helper to create a TaskDependency
func makeDep(projectID, predID, succID uuid.UUID, depType models.DependencyType, lag int) models.TaskDependency {
	return models.TaskDependency{
		ID:             uuid.New(),
		ProjectID:      projectID,
		PredecessorID:  predID,
		SuccessorID:    succID,
		DependencyType: depType,
		LagDays:        lag,
	}
}

// TestTopologicalSort_LinearDAG verifies processing order for A->B->C chain.
func TestTopologicalSort_LinearDAG(t *testing.T) {
	projectID := uuid.New()

	// Create tasks: A -> B -> C
	taskA := makeTask("1.1")
	taskB := makeTask("1.2")
	taskC := makeTask("1.3")

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskC.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	sorted, err := TopologicalSort(g)

	require.NoError(t, err)
	require.Len(t, sorted, 3)

	// A must come before B, B must come before C
	indexA := indexOf(sorted, taskA.ID)
	indexB := indexOf(sorted, taskB.ID)
	indexC := indexOf(sorted, taskC.ID)

	assert.Less(t, indexA, indexB, "A should come before B")
	assert.Less(t, indexB, indexC, "B should come before C")
}

// TestTopologicalSort_BranchingDAG verifies parallel paths: A->B, A->C, B->D, C->D.
func TestTopologicalSort_BranchingDAG(t *testing.T) {
	projectID := uuid.New()

	taskA := makeTask("2.1")
	taskB := makeTask("2.2")
	taskC := makeTask("2.3")
	taskD := makeTask("2.4")

	tasks := []models.ProjectTask{taskA, taskB, taskC, taskD}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskA.ID, taskC.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskD.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskC.ID, taskD.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	sorted, err := TopologicalSort(g)

	require.NoError(t, err)
	require.Len(t, sorted, 4)

	indexA := indexOf(sorted, taskA.ID)
	indexB := indexOf(sorted, taskB.ID)
	indexC := indexOf(sorted, taskC.ID)
	indexD := indexOf(sorted, taskD.ID)

	assert.Less(t, indexA, indexB, "A should come before B")
	assert.Less(t, indexA, indexC, "A should come before C")
	assert.Less(t, indexB, indexD, "B should come before D")
	assert.Less(t, indexC, indexD, "C should come before D")
}

// TestDetectCycle_RejectsCircular verifies cycle detection per Step 34.
func TestDetectCycle_RejectsCircular(t *testing.T) {
	projectID := uuid.New()

	// Create circular dependency: A -> B -> C -> A
	taskA := makeTask("3.1")
	taskB := makeTask("3.2")
	taskC := makeTask("3.3")

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskC.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskC.ID, taskA.ID, models.DependencyTypeFS, 0), // Creates cycle
	}

	g := BuildDependencyGraph(tasks, deps)
	err := DetectCycle(g)

	require.Error(t, err, "Cycle should be detected")
	assert.Contains(t, err.Error(), "cycle detected", "Error should mention cycle")
	// Per Step 34: error should contain WBS codes
	assert.Contains(t, err.Error(), "3.1", "Error should contain WBS code 3.1")
}

// TestDependencyGraph_MetadataIntegrity verifies lag_days is stored correctly.
func TestDependencyGraph_MetadataIntegrity(t *testing.T) {
	projectID := uuid.New()

	taskA := makeTask("4.1")
	taskB := makeTask("4.2")

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 5), // 5-day lag
	}

	g := BuildDependencyGraph(tasks, deps)

	// Verify edge metadata via Deps map
	dep, exists := g.GetDependency(taskA.ID, taskB.ID)

	require.True(t, exists, "Dependency should exist in graph")
	assert.Equal(t, 5, dep.LagDays, "Lag should be 5 days")
	assert.Equal(t, models.DependencyTypeFS, dep.DependencyType, "Type should be FS")
}

// TestBuildDependencyGraph_EmptyTasks handles edge case of empty input.
func TestBuildDependencyGraph_EmptyTasks(t *testing.T) {
	g := BuildDependencyGraph(nil, nil)

	assert.NotNil(t, g, "Graph should not be nil")
	assert.Equal(t, 0, g.Graph.Nodes().Len(), "Graph should have no nodes")

	sorted, err := TopologicalSort(g)
	require.NoError(t, err)
	assert.Len(t, sorted, 0, "Sorted list should be empty")
}

// indexOf returns the index of id in slice, or -1 if not found.
func indexOf(slice []uuid.UUID, id uuid.UUID) int {
	for i, v := range slice {
		if v == id {
			return i
		}
	}
	return -1
}

// Helper to create a ProjectTask with duration
func makeTaskWithDuration(wbsCode string, calculatedDuration float64) models.ProjectTask {
	return models.ProjectTask{
		ID:                 uuid.New(),
		WBSCode:            wbsCode,
		Name:               "Task " + wbsCode,
		CalculatedDuration: calculatedDuration,
	}
}

// TestForwardPass_LinearDAG verifies ES/EF calculation for A->B->C chain.
func TestForwardPass_LinearDAG(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Create tasks: A (2 days) -> B (3 days) -> C (1 day)
	taskA := makeTaskWithDuration("7.1", 2.0)
	taskB := makeTaskWithDuration("7.2", 3.0)
	taskC := makeTaskWithDuration("7.3", 1.0)

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskC.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)
	require.Len(t, schedule, 3)

	cal := &StandardCalendar{}

	// Task A: ES = project start, EF = ES + 2 working days
	schedA := schedule[taskA.ID]
	assert.Equal(t, projectStart, schedA.EarlyStart, "A should start at project start")
	assert.Equal(t, cal.AddWorkingDays(projectStart, 2), schedA.EarlyFinish, "A should finish 2 working days later")

	// Task B: ES = A.EF, EF = ES + 3 working days
	schedB := schedule[taskB.ID]
	assert.Equal(t, schedA.EarlyFinish, schedB.EarlyStart, "B should start when A finishes")
	assert.Equal(t, cal.AddWorkingDays(projectStart, 5), schedB.EarlyFinish, "B should finish at working day 5")

	// Task C: ES = B.EF, EF = ES + 1 working day
	schedC := schedule[taskC.ID]
	assert.Equal(t, schedB.EarlyFinish, schedC.EarlyStart, "C should start when B finishes")
	assert.Equal(t, cal.AddWorkingDays(projectStart, 6), schedC.EarlyFinish, "C should finish at working day 6")
}

// TestForwardPass_BranchingDAG verifies max() logic with multiple predecessors.
func TestForwardPass_BranchingDAG(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (2 days) -> D
	// B (5 days) -> D (takes max)
	// C (3 days) -> D
	taskA := makeTaskWithDuration("8.1", 2.0)
	taskB := makeTaskWithDuration("8.2", 5.0) // Longest path
	taskC := makeTaskWithDuration("8.3", 3.0)
	taskD := makeTaskWithDuration("8.4", 1.0)

	tasks := []models.ProjectTask{taskA, taskB, taskC, taskD}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskD.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskD.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskC.ID, taskD.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)

	cal := &StandardCalendar{}

	// D's ES should be max(A.EF, B.EF, C.EF) = B.EF (working day 5)
	schedB := schedule[taskB.ID]
	schedD := schedule[taskD.ID]

	assert.Equal(t, schedB.EarlyFinish, schedD.EarlyStart, "D should start at max predecessor finish (B)")
	expectedDFinish := cal.AddWorkingDays(projectStart, 6) // Working day 5 + 1 = Working day 6
	assert.Equal(t, expectedDFinish, schedD.EarlyFinish, "D should finish at working day 6")
}

// TestForwardPass_LagDays verifies lag is added to constraint date.
func TestForwardPass_LagDays(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (2 days) --[5 day lag]--> B (1 day)
	taskA := makeTaskWithDuration("9.1", 2.0)
	taskB := makeTaskWithDuration("9.2", 1.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 5), // 5-day lag
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)

	cal := &StandardCalendar{}
	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// B.ES = A.EF + 5 working days lag
	expectedBStart := cal.AddWorkingDays(schedA.EarlyFinish, 5)
	assert.Equal(t, expectedBStart, schedB.EarlyStart, "B should start 5 working days after A finishes")
}

// TestForwardPass_SSType verifies Start-to-Start dependency.
func TestForwardPass_SSType(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (5 days) --[SS+2]--> B (3 days)
	// B starts 2 days after A starts
	taskA := makeTaskWithDuration("10.1", 5.0)
	taskB := makeTaskWithDuration("10.2", 3.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeSS, 2), // SS with 2-day lag
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// B.ES = A.ES + 2 days
	expectedBStart := schedA.EarlyStart.AddDate(0, 0, 2)
	assert.Equal(t, expectedBStart, schedB.EarlyStart, "B should start 2 days after A starts (SS)")
}

// TestForwardPass_FFType verifies Finish-to-Finish dependency.
func TestForwardPass_FFType(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (5 days) --[FF+0]--> B (3 days)
	// B finishes when A finishes, so B.ES = A.EF - B.duration
	taskA := makeTaskWithDuration("11.1", 5.0)
	taskB := makeTaskWithDuration("11.2", 3.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFF, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)

	cal := &StandardCalendar{}
	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// B.EF = A.EF, so B.ES = A.EF - 3 working days
	expectedBStart := cal.AddWorkingDays(schedA.EarlyFinish, -3)
	assert.Equal(t, expectedBStart, schedB.EarlyStart, "B should start 3 days before A finishes (FF)")
	assert.Equal(t, schedA.EarlyFinish, schedB.EarlyFinish, "B should finish when A finishes (FF)")
}

// TestForwardPass_SFType verifies Start-to-Finish dependency.
func TestForwardPass_SFType(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (5 days) --[SF+0]--> B (3 days)
	// SF: Successor finishes after predecessor starts
	// EF(B) = ES(A) + lag, so ES(B) = ES(A) + lag - duration(B)
	taskA := makeTaskWithDuration("21.1", 5.0)
	taskB := makeTaskWithDuration("21.2", 3.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeSF, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)

	cal := &StandardCalendar{}
	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// B.EF = A.ES + lag = projectStart
	// B.ES = B.EF - 3 working days
	expectedBStart := cal.AddWorkingDays(schedA.EarlyStart, -3)
	assert.Equal(t, expectedBStart, schedB.EarlyStart, "B.ES should be A.ES minus B duration (SF)")
}

// TestForwardPass_RootTask verifies root tasks start at project start.
func TestForwardPass_RootTask(t *testing.T) {
	projectStart := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// Single task with no predecessors
	taskA := makeTaskWithDuration("5.3", 3.0)

	tasks := []models.ProjectTask{taskA}
	deps := []models.TaskDependency{} // No dependencies

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	assert.Equal(t, projectStart, schedA.EarlyStart, "Root task should start at project start")
	expectedFinish := projectStart.AddDate(0, 0, 3)
	assert.Equal(t, expectedFinish, schedA.EarlyFinish, "Root task should finish after duration")
}

// TestForwardPass_DurationPrecedence verifies ManualOverride > WeatherAdjusted > Calculated.
func TestForwardPass_DurationPrecedence(t *testing.T) {
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Task with all three durations set
	override := 10.0
	taskA := models.ProjectTask{
		ID:                      uuid.New(),
		WBSCode:                 "12.1",
		Name:                    "Override Test",
		CalculatedDuration:      2.0,
		WeatherAdjustedDuration: 5.0,
		ManualOverrideDays:      &override,
	}

	tasks := []models.ProjectTask{taskA}
	deps := []models.TaskDependency{}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)

	cal := &StandardCalendar{}
	schedA := schedule[taskA.ID]
	expectedFinish := cal.AddWorkingDays(projectStart, 10) // Should use override (10 working days)
	assert.Equal(t, expectedFinish, schedA.EarlyFinish, "Should use ManualOverrideDays (10) over others")
}

// TestForwardPass_EmptyGraph handles edge case of empty input.
func TestForwardPass_EmptyGraph(t *testing.T) {
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	g := BuildDependencyGraph(nil, nil)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)
	assert.Len(t, schedule, 0, "Empty graph should produce empty schedule")
}

// TestForwardPass_MaterialConstraint verifies material constraints push task start dates.
// See PRODUCTION_PLAN.md Step 46 (MRP Feedback Loop)
func TestForwardPass_MaterialConstraint(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Create tasks: A (2 days) -> B (3 days)
	// B normally starts at Day 2 (after A finishes)
	// Material constraint: B's material arrives Day 10
	// Expected: B should start Day 10, not Day 2
	taskA := makeTaskWithDuration("MAT.1", 2.0)
	taskB := makeTaskWithDuration("MAT.2", 3.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
	}

	// Material arrives on Day 10 for Task B
	materialDeliveryDate := time.Date(2026, 1, 22, 0, 0, 0, 0, time.UTC) // Day 10 from project start
	materialConstraints := map[uuid.UUID]time.Time{
		taskB.ID: materialDeliveryDate,
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, materialConstraints)

	require.NoError(t, err)
	require.Len(t, schedule, 2)

	cal := &StandardCalendar{}

	// Task A: ES = project start, EF = ES + 2 working days
	schedA := schedule[taskA.ID]
	assert.Equal(t, projectStart, schedA.EarlyStart, "A should start at project start")
	assert.Equal(t, cal.AddWorkingDays(projectStart, 2), schedA.EarlyFinish, "A should finish 2 working days later")

	// Task B: ES should be pushed to material delivery date (Day 10), not A.EF (Day 2)
	// Material constraint is HARD - task cannot start before material arrives
	schedB := schedule[taskB.ID]
	assert.Equal(t, materialDeliveryDate, schedB.EarlyStart, "B should start when material arrives (Day 10)")
	assert.Equal(t, cal.AddWorkingDays(materialDeliveryDate, 3), schedB.EarlyFinish, "B should finish 3 working days after Day 10")
}

// TestForwardPass_MaterialConstraint_NoEffect verifies material constraint is ignored when predecessor is later.
func TestForwardPass_MaterialConstraint_NoEffect(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Create tasks: A (10 days) -> B (2 days)
	// B normally starts at Day 10 (after A finishes)
	// Material arrives Day 5 - before predecessor finishes
	// Expected: B should start Day 10, NOT Day 5 (predecessor constraint is tighter)
	taskA := makeTaskWithDuration("MAT2.1", 10.0)
	taskB := makeTaskWithDuration("MAT2.2", 2.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
	}

	// Material arrives early on Day 5 - should not affect schedule
	materialDeliveryDate := time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC) // Working day 5
	materialConstraints := map[uuid.UUID]time.Time{
		taskB.ID: materialDeliveryDate,
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, materialConstraints)

	require.NoError(t, err)

	cal := &StandardCalendar{}
	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// B.ES should be A.EF (working day 10), not material date (Day 5)
	expectedBStart := schedA.EarlyFinish
	assert.Equal(t, expectedBStart, schedB.EarlyStart, "B should start when A finishes (Day 10), not material arrival (Day 5)")
	assert.Equal(t, cal.AddWorkingDays(expectedBStart, 2), schedB.EarlyFinish, "B should finish 2 working days after A")
}

// ============================================================================
// Backward Pass Tests
// ============================================================================

// TestBackwardPass_LinearDAG verifies LS/LF calculation for A->B->C chain.
func TestBackwardPass_LinearDAG(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Create tasks: A (2 days) -> B (3 days) -> C (1 day)
	taskA := makeTaskWithDuration("13.1", 2.0)
	taskB := makeTaskWithDuration("13.2", 3.0)
	taskC := makeTaskWithDuration("13.3", 1.0)

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskC.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	criticalPath, err := BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	// All tasks should be critical in a linear chain
	assert.Len(t, criticalPath, 3, "All tasks should be on critical path")

	cal := &StandardCalendar{}

	// Task C (terminal): LF = project end (working day 6), LS = working day 5
	schedC := schedule[taskC.ID]
	expectedCLF := cal.AddWorkingDays(projectStart, 6) // Project end in working days
	assert.Equal(t, expectedCLF, schedC.LateFinish, "C.LF should be project end")
	assert.Equal(t, schedC.EarlyStart, schedC.LateStart, "C should have zero float")
	assert.True(t, schedC.IsCritical, "C should be critical")

	// Task B: LF = C.LS, LS = LF - 3
	schedB := schedule[taskB.ID]
	assert.Equal(t, schedC.LateStart, schedB.LateFinish, "B.LF should equal C.LS")
	assert.True(t, schedB.IsCritical, "B should be critical")

	// Task A: LF = B.LS, LS = LF - 2
	schedA := schedule[taskA.ID]
	assert.Equal(t, schedB.LateStart, schedA.LateFinish, "A.LF should equal B.LS")
	assert.True(t, schedA.IsCritical, "A should be critical")
}

// TestBackwardPass_BranchingDAG verifies min() logic with multiple successors.
func TestBackwardPass_BranchingDAG(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (2 days) -> B (3 days),  A -> C (1 day)
	// B and C are parallel, but B is longer - determines project end
	taskA := makeTaskWithDuration("14.1", 2.0)
	taskB := makeTaskWithDuration("14.2", 3.0) // Longer path
	taskC := makeTaskWithDuration("14.3", 1.0) // Shorter path

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskA.ID, taskC.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	criticalPath, err := BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	// A and B should be critical; C has float
	assert.Contains(t, criticalPath, "14.1", "A should be on critical path")
	assert.Contains(t, criticalPath, "14.2", "B should be on critical path")
	assert.NotContains(t, criticalPath, "14.3", "C should NOT be on critical path")

	// A's LF is constrained by min(B.LS, C.LS) = B.LS (tighter constraint)
	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]
	assert.Equal(t, schedB.LateStart, schedA.LateFinish, "A.LF should be constrained by B.LS")

	// C should have positive float
	schedC := schedule[taskC.ID]
	assert.Greater(t, schedC.TotalFloat, 0.0, "C should have positive float")
}

// TestBackwardPass_LagDays verifies lag subtraction in backward pass.
func TestBackwardPass_LagDays(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (2 days) --[5 day lag]--> B (1 day)
	taskA := makeTaskWithDuration("15.1", 2.0)
	taskB := makeTaskWithDuration("15.2", 1.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 5),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	_, err = BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// A.LF = B.LS - 5 working day lag
	cal := &StandardCalendar{}
	expectedALF := cal.AddWorkingDays(schedB.LateStart, -5)
	assert.Equal(t, expectedALF, schedA.LateFinish, "A.LF should account for 5-day lag")
}

// TestBackwardPass_CriticalPath verifies tasks with Float=0 are correctly identified.
func TestBackwardPass_CriticalPath(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Create diamond pattern: A -> B, A -> C, B -> D, C -> D
	// With B longer than C, A-B-D is critical
	taskA := makeTaskWithDuration("16.1", 1.0)
	taskB := makeTaskWithDuration("16.2", 5.0) // Longer
	taskC := makeTaskWithDuration("16.3", 2.0) // Shorter
	taskD := makeTaskWithDuration("16.4", 1.0)

	tasks := []models.ProjectTask{taskA, taskB, taskC, taskD}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskA.ID, taskC.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskD.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskC.ID, taskD.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	criticalPath, err := BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	// Critical path should be A -> B -> D
	assert.Len(t, criticalPath, 3, "Critical path should have 3 tasks")
	assert.Contains(t, criticalPath, "16.1", "A is critical")
	assert.Contains(t, criticalPath, "16.2", "B is critical")
	assert.Contains(t, criticalPath, "16.4", "D is critical")
	assert.NotContains(t, criticalPath, "16.3", "C is NOT critical")

	// Verify C has positive float
	schedC := schedule[taskC.ID]
	assert.Equal(t, 3.0, schedC.TotalFloat, "C should have 3 days float (5-2)")
}

// TestBackwardPass_SSType verifies Start-to-Start backward constraint.
func TestBackwardPass_SSType(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (5 days) --[SS+2]--> B (3 days)
	taskA := makeTaskWithDuration("17.1", 5.0)
	taskB := makeTaskWithDuration("17.2", 3.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeSS, 2),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	_, err = BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// With SS: LS(A) = LS(B) - lag = LS(B) - 2
	expectedALS := schedB.LateStart.AddDate(0, 0, -2)
	assert.Equal(t, expectedALS, schedA.LateStart, "A.LS should be B.LS minus 2 days (SS)")
}

// TestBackwardPass_FFType verifies Finish-to-Finish backward constraint.
func TestBackwardPass_FFType(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (5 days) --[FF+0]--> B (3 days)
	taskA := makeTaskWithDuration("18.1", 5.0)
	taskB := makeTaskWithDuration("18.2", 3.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFF, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	_, err = BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// With FF: LF(A) = LF(B) - lag = LF(B)
	assert.Equal(t, schedB.LateFinish, schedA.LateFinish, "A.LF should equal B.LF (FF)")
}

// TestBackwardPass_SFType verifies Start-to-Finish backward constraint.
func TestBackwardPass_SFType(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (5 days) --[SF+0]--> B (3 days)
	// SF backward: LS(pred) = LF(succ) - lag
	taskA := makeTaskWithDuration("22.1", 5.0)
	taskB := makeTaskWithDuration("22.2", 3.0)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeSF, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	_, err = BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// With SF: LS(A) = LF(B) - lag
	expectedALS := schedB.LateFinish.AddDate(0, 0, 0) // lag = 0
	assert.Equal(t, expectedALS, schedA.LateStart, "A.LS should equal B.LF (SF)")
}

// TestBackwardPass_TerminalTask verifies terminal tasks have LF = project end.
func TestBackwardPass_TerminalTask(t *testing.T) {
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Single task with no successors
	taskA := makeTaskWithDuration("19.1", 5.0)

	tasks := []models.ProjectTask{taskA}
	deps := []models.TaskDependency{}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	_, err = BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	schedA := schedule[taskA.ID]

	// Terminal task: LF = EF (project end)
	assert.Equal(t, schedA.EarlyFinish, schedA.LateFinish, "Terminal task LF should equal EF")
	assert.Equal(t, schedA.EarlyStart, schedA.LateStart, "Terminal task LS should equal ES")
	assert.Equal(t, 0.0, schedA.TotalFloat, "Terminal task should have zero float")
	assert.True(t, schedA.IsCritical, "Terminal task should be critical")
}

// TestBackwardPass_FloatCalculation verifies non-critical tasks show positive float.
func TestBackwardPass_FloatCalculation(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// A (1 day) has two successors:
	// A -> B (10 days) - determines project end
	// A -> C (2 days)  - has 8 days of float
	taskA := makeTaskWithDuration("20.1", 1.0)
	taskB := makeTaskWithDuration("20.2", 10.0)
	taskC := makeTaskWithDuration("20.3", 2.0)

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
		makeDep(projectID, taskA.ID, taskC.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)
	require.NoError(t, err)

	_, err = BackwardPass(g, schedule, &StandardCalendar{}, nil)
	require.NoError(t, err)

	// C should have float equal to the difference between B's duration and C's duration
	// in working days. B takes 10 working days, C takes 2 working days.
	// Float = LS(C) - ES(C), calculated in terms of time difference.
	schedC := schedule[taskC.ID]
	assert.Greater(t, schedC.TotalFloat, 0.0, "C should have positive float")
	assert.False(t, schedC.IsCritical, "C should NOT be critical")

	// B should have zero float
	schedB := schedule[taskB.ID]
	assert.InDelta(t, 0.0, schedB.TotalFloat, 0.001, "B should have zero float")
	assert.True(t, schedB.IsCritical, "B should be critical")
}

// TestBackwardPass_EmptySchedule handles edge case of empty schedule.
func TestBackwardPass_EmptySchedule(t *testing.T) {
	g := BuildDependencyGraph(nil, nil)
	schedule := make(map[uuid.UUID]TaskSchedule)

	criticalPath, err := BackwardPass(g, schedule, &StandardCalendar{}, nil)

	require.NoError(t, err)
	assert.Nil(t, criticalPath, "Empty schedule should produce nil critical path")
}

// TestCalendar_SkipsWeekends verifies that AddWorkingDays correctly skips weekends.
// Task A (3 days) starts on Friday -> should finish on Tuesday (not Sunday).
func TestCalendar_SkipsWeekends(t *testing.T) {
	// Friday, January 16, 2026
	projectStart := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	require.Equal(t, time.Friday, projectStart.Weekday(), "Test setup: should start on Friday")

	// Create a single task: A (3 working days)
	taskA := makeTaskWithDuration("23.1", 3.0)

	tasks := []models.ProjectTask{taskA}
	deps := []models.TaskDependency{}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.NoError(t, err)
	require.Len(t, schedule, 1)

	schedA := schedule[taskA.ID]

	// Task A starts Friday (Jan 16)
	// Day 1: Friday -> Saturday (skip)
	// Day 1: Saturday -> Sunday (skip)
	// Day 1: Sunday -> Monday (Jan 19)
	// Day 2: Monday -> Tuesday (Jan 20)
	// Day 3: Tuesday -> Wednesday (Jan 21)
	// EarlyFinish should be Wednesday, January 21, 2026
	expectedFinish := time.Date(2026, 1, 21, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, projectStart, schedA.EarlyStart, "Task A should start on Friday")
	assert.Equal(t, expectedFinish, schedA.EarlyFinish, "Task A should finish on Wednesday (skipping Sat/Sun)")
	assert.Equal(t, time.Wednesday, schedA.EarlyFinish.Weekday(), "Finish day should be Wednesday")
}

// ============================================================================
// Floating-Point Precision Tests (Phase 2 Remediation)
// ============================================================================

// SevenDayCalendar implements a 7-day work week for precision testing.
// This isolates float drift from calendar/weekend logic.
type SevenDayCalendar struct{}

// AddWorkingDays adds days without skipping weekends (7-day work week).
func (c *SevenDayCalendar) AddWorkingDays(date time.Time, days float64) time.Time {
	// No weekend skipping - pure fractional day arithmetic
	wholeDays := int(days)
	fractionalDays := days - float64(wholeDays)

	// Add whole days
	result := date.AddDate(0, 0, wholeDays)

	// Add fractional days as hours
	hoursToAdd := fractionalDays * 24
	result = result.Add(time.Duration(hoursToAdd * float64(time.Hour)))

	return result.Truncate(time.Minute) // Match StandardCalendar truncation
}

// AddWorkDuration adds work duration using integer math (7-day work week).
func (c *SevenDayCalendar) AddWorkDuration(date time.Time, duration time.Duration) time.Time {
	return date.Add(duration).Truncate(time.Minute)
}

// TestFloatPrecision_LongChain proves the CPM engine doesn't suffer from
// floating-point drift over long dependency chains.
// See Phase 2 Remediation: Task 2 - Physics Engine Precision Verification.
//
// Scenario: 100 tasks in a linear chain (A1->A2->A3->...->A100)
// Each task has duration 0.1 days.
// Expected project duration: 10.0 days exactly.
//
// This test catches the infamous 0.1 + 0.2 != 0.30000000004 problem.
func TestFloatPrecision_LongChain(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC) // Monday

	const numTasks = 100
	const taskDuration = 0.1 // Each task is 0.1 days

	// Build 100 tasks in sequence
	tasks := make([]models.ProjectTask, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = makeTaskWithDuration(
			fmt.Sprintf("FP.%d", i+1),
			taskDuration,
		)
	}

	// Create linear dependency chain: Task[0] -> Task[1] -> ... -> Task[99]
	deps := make([]models.TaskDependency, numTasks-1)
	for i := 0; i < numTasks-1; i++ {
		deps[i] = makeDep(projectID, tasks[i].ID, tasks[i+1].ID, models.DependencyTypeFS, 0)
	}

	// Build graph and run forward pass with 7-day calendar (no weekend logic)
	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &SevenDayCalendar{}, nil)

	require.NoError(t, err)
	require.Len(t, schedule, numTasks)

	// Get the final task's early finish
	lastTaskSchedule := schedule[tasks[numTasks-1].ID]

	// Calculate actual project duration in days
	projectDuration := lastTaskSchedule.EarlyFinish.Sub(projectStart).Hours() / 24.0

	// Expected duration: 100 tasks x 0.1 days = 10.0 days exactly
	expectedDuration := 10.0

	// STRICT CHECK: Use very tight epsilon to catch any float drift
	// If this fails with something like 10.0000000000002, we have a problem
	assert.InDelta(t, expectedDuration, projectDuration, 0.0000001,
		"Project duration should be exactly 10.0 days (100 tasks x 0.1 days). "+
			"Got %.15f, which indicates floating-point drift.", projectDuration)

	// Additional verification: first task should start at project start
	firstTaskSchedule := schedule[tasks[0].ID]
	assert.Equal(t, projectStart, firstTaskSchedule.EarlyStart,
		"First task should start at project start")

	// Log actual values for audit trail
	t.Logf("Tasks: %d, Duration per task: %.1f days", numTasks, taskDuration)
	t.Logf("Expected project duration: %.1f days", expectedDuration)
	t.Logf("Actual project duration: %.15f days", projectDuration)
	t.Logf("Difference from expected: %.15e days", projectDuration-expectedDuration)
}

// TestFloatPrecision_AccumulatedFractional tests precision with varying fractional durations.
// Uses tasks with 1/3 day duration (known to cause IEEE 754 issues).
func TestFloatPrecision_AccumulatedFractional(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	const numTasks = 30
	const taskDuration = 1.0 / 3.0 // ~0.333... days (repeating decimal)

	// Build 30 tasks in sequence
	tasks := make([]models.ProjectTask, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = makeTaskWithDuration(
			fmt.Sprintf("FR.%d", i+1),
			taskDuration,
		)
	}

	deps := make([]models.TaskDependency, numTasks-1)
	for i := 0; i < numTasks-1; i++ {
		deps[i] = makeDep(projectID, tasks[i].ID, tasks[i+1].ID, models.DependencyTypeFS, 0)
	}

	g := BuildDependencyGraph(tasks, deps)
	schedule, err := ForwardPass(g, projectStart, &SevenDayCalendar{}, nil)

	require.NoError(t, err)

	lastTaskSchedule := schedule[tasks[numTasks-1].ID]
	projectDuration := lastTaskSchedule.EarlyFinish.Sub(projectStart).Hours() / 24.0

	// 30 x (1/3) = 10.0 days exactly
	expectedDuration := 10.0

	// Allow slightly larger epsilon for repeating decimals, but still tight
	assert.InDelta(t, expectedDuration, projectDuration, 0.000001,
		"30 tasks x 1/3 day should equal 10.0 days. Got %.15f", projectDuration)

	t.Logf("30 x (1/3) day tasks = %.15f days (expected 10.0)", projectDuration)
}

// ============================================================================
// Holiday-Aware Calendar Tests (Operation Ironclad Task 4)
// ============================================================================

// TestCalendar_SkipsHolidays verifies that StandardCalendar skips configured holidays.
// Adds 1 working day from Dec 24 should skip Dec 25 (Christmas) and land on Dec 26.
func TestCalendar_SkipsHolidays(t *testing.T) {
	// Dec 25, 2026 is a Friday - perfect for testing holiday skipping
	// Dec 24, 2026 is a Thursday
	projectStart := time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC)
	require.Equal(t, time.Thursday, projectStart.Weekday(), "Test setup: should start on Thursday")

	// Configure calendar with Christmas as a holiday
	christmas := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	cal := &StandardCalendar{
		Holidays: []time.Time{christmas},
	}

	// Add 1 working day from Dec 24
	// Without holiday: Dec 24 + 1 = Dec 25
	// With holiday: Dec 24 + 1 = Dec 26 (skips Dec 25)
	result := cal.AddWorkingDays(projectStart, 1)

	// Should skip Dec 25 and land on Dec 26 (Saturday would be skipped by weekend logic)
	// Dec 26, 2026 is a Saturday - so it should actually be Dec 28 (Monday)
	expectedFinish := time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, expectedFinish, result,
		"Adding 1 working day from Dec 24 should skip Christmas (Dec 25) and weekend, landing on Dec 28 (Monday)")
	assert.Equal(t, time.Monday, result.Weekday(), "Result should be Monday")
}

// TestCalendar_HolidayOnWeekday verifies holiday skipping when Dec 25 is a weekday.
// Uses 2025 where Dec 25 is a Thursday.
func TestCalendar_HolidayOnWeekday(t *testing.T) {
	// Dec 25, 2025 is a Thursday
	// Dec 24, 2025 is a Wednesday
	projectStart := time.Date(2025, 12, 24, 0, 0, 0, 0, time.UTC)
	require.Equal(t, time.Wednesday, projectStart.Weekday(), "Test setup: should start on Wednesday")

	// Configure calendar with Christmas (ignores year per implementation)
	christmas := time.Date(2000, 12, 25, 0, 0, 0, 0, time.UTC) // Any year works
	cal := &StandardCalendar{
		Holidays: []time.Time{christmas},
	}

	// Add 1 working day from Dec 24 (Wednesday)
	// Should skip Dec 25 (Thursday holiday) and land on Dec 26 (Friday)
	result := cal.AddWorkingDays(projectStart, 1)

	expectedFinish := time.Date(2025, 12, 26, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, expectedFinish, result,
		"Adding 1 working day from Dec 24 should skip Christmas (Dec 25), landing on Dec 26 (Friday)")
	assert.Equal(t, time.Friday, result.Weekday(), "Result should be Friday")
}

// ============================================================================
// Task Duration Validation Tests (Fail-Loudly Approach)
// See PRODUCTION_PLAN.md: Data Quality Enforcement
// ============================================================================

// TestForwardPass_RejectsZeroDuration verifies CPM halts when task has no valid duration.
func TestForwardPass_RejectsZeroDuration(t *testing.T) {
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Create task with NO valid duration (all zero/nil)
	taskA := models.ProjectTask{
		ID:                      uuid.New(),
		WBSCode:                 "INVALID.1",
		Name:                    "Zero Duration Task",
		CalculatedDuration:      0,   // Zero
		WeatherAdjustedDuration: 0,   // Zero
		ManualOverrideDays:      nil, // Nil
	}

	tasks := []models.ProjectTask{taskA}
	deps := []models.TaskDependency{}

	g := BuildDependencyGraph(tasks, deps)
	_, err := ForwardPass(g, projectStart, &StandardCalendar{}, nil)

	require.Error(t, err, "ForwardPass should error on zero-duration task")
	assert.ErrorIs(t, err, ErrInvalidTaskDuration, "Error should wrap ErrInvalidTaskDuration")
	assert.Contains(t, err.Error(), "INVALID.1", "Error should contain WBS code")
}

// TestBackwardPass_RejectsZeroDuration verifies CPM backward pass also validates duration.
func TestBackwardPass_RejectsZeroDuration(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)

	// Create valid task A and invalid task B (zero duration)
	taskA := makeTaskWithDuration("BP.1", 5.0)
	taskB := models.ProjectTask{
		ID:                      uuid.New(),
		WBSCode:                 "BP.2",
		Name:                    "Zero Duration B",
		CalculatedDuration:      0,
		WeatherAdjustedDuration: 0,
		ManualOverrideDays:      nil,
	}

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, models.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)

	// Build schedule manually to bypass ForwardPass validation
	schedule := map[uuid.UUID]TaskSchedule{
		taskA.ID: {
			TaskID:      taskA.ID,
			WBSCode:     taskA.WBSCode,
			EarlyStart:  projectStart,
			EarlyFinish: projectStart.AddDate(0, 0, 5),
		},
		taskB.ID: {
			TaskID:      taskB.ID,
			WBSCode:     taskB.WBSCode,
			EarlyStart:  projectStart.AddDate(0, 0, 5),
			EarlyFinish: projectStart.AddDate(0, 0, 5), // Zero duration EF=ES
		},
	}

	_, err := BackwardPass(g, schedule, &StandardCalendar{}, nil)

	require.Error(t, err, "BackwardPass should error on zero-duration task")
	assert.ErrorIs(t, err, ErrInvalidTaskDuration, "Error should wrap ErrInvalidTaskDuration")
	assert.Contains(t, err.Error(), "BP.2", "Error should contain WBS code")
}

// TestGetTaskDuration_ValidPrecedence verifies duration resolution priority.
// P1 Correctness Fix: Updated to expect time.Duration (deterministic integer math).
func TestGetTaskDuration_ValidPrecedence(t *testing.T) {
	tests := []struct {
		name             string
		task             models.ProjectTask
		expectedDuration time.Duration // Now time.Duration, not float64
		expectError      bool
	}{
		{
			name: "ManualOverride takes precedence",
			task: func() models.ProjectTask {
				override := 10.0
				return models.ProjectTask{
					ID:                      uuid.New(),
					WBSCode:                 "PREC.1",
					CalculatedDuration:      2.0,
					WeatherAdjustedDuration: 5.0,
					ManualOverrideDays:      &override,
				}
			}(),
			expectedDuration: 10 * 24 * time.Hour, // 10 days in duration
			expectError:      false,
		},
		{
			name: "WeatherAdjusted when no override",
			task: models.ProjectTask{
				ID:                      uuid.New(),
				WBSCode:                 "PREC.2",
				CalculatedDuration:      2.0,
				WeatherAdjustedDuration: 5.0,
				ManualOverrideDays:      nil,
			},
			expectedDuration: 5 * 24 * time.Hour, // 5 days in duration
			expectError:      false,
		},
		{
			name: "Calculated when no higher priority",
			task: models.ProjectTask{
				ID:                      uuid.New(),
				WBSCode:                 "PREC.3",
				CalculatedDuration:      3.0,
				WeatherAdjustedDuration: 0,
				ManualOverrideDays:      nil,
			},
			expectedDuration: 3 * 24 * time.Hour, // 3 days in duration
			expectError:      false,
		},
		{
			name: "Error when all zero/nil",
			task: models.ProjectTask{
				ID:                      uuid.New(),
				WBSCode:                 "PREC.4",
				CalculatedDuration:      0,
				WeatherAdjustedDuration: 0,
				ManualOverrideDays:      nil,
			},
			expectedDuration: 0,
			expectError:      true,
		},
		{
			name: "Zero override is ignored",
			task: func() models.ProjectTask {
				zero := 0.0
				return models.ProjectTask{
					ID:                      uuid.New(),
					WBSCode:                 "PREC.5",
					CalculatedDuration:      4.0,
					WeatherAdjustedDuration: 0,
					ManualOverrideDays:      &zero, // Zero override should be ignored
				}
			}(),
			expectedDuration: 4 * 24 * time.Hour, // 4 days in duration
			expectError:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			duration, err := getTaskDuration(tc.task)
			if tc.expectError {
				require.Error(t, err, "Expected error for %s", tc.name)
				assert.ErrorIs(t, err, ErrInvalidTaskDuration)
			} else {
				require.NoError(t, err, "Unexpected error for %s", tc.name)
				assert.Equal(t, tc.expectedDuration, duration, "Duration mismatch for %s", tc.name)
			}
		})
	}
}
