package physics

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/models/types"
)

// Helper to create a ProjectTask with minimal fields
func makeTask(wbsCode string, durationDays int) models.ProjectTask {
	return models.ProjectTask{
		ID:           uuid.New(),
		WBSCode:      wbsCode,
		Name:         "Task " + wbsCode,
		DurationDays: durationDays,
	}
}

// Helper to create a TaskDependency
func makeDep(projectID, predID, succID uuid.UUID, depType types.DependencyType, lag int) models.TaskDependency {
	return models.TaskDependency{
		ID:             uuid.New(),
		ProjectID:      projectID,
		PredecessorID:  predID,
		SuccessorID:    succID,
		DependencyType: depType,
		LagDays:        lag,
	}
}

func indexOf(ids []uuid.UUID, target uuid.UUID) int {
	for i, id := range ids {
		if id == target {
			return i
		}
	}
	return -1
}

// TestTopologicalSort_LinearDAG verifies processing order for A->B->C chain.
func TestTopologicalSort_LinearDAG(t *testing.T) {
	projectID := uuid.New()

	taskA := makeTask("1.1", 3)
	taskB := makeTask("1.2", 2)
	taskC := makeTask("1.3", 1)

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskC.ID, types.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	sorted, err := TopologicalSort(g)

	require.NoError(t, err)
	require.Len(t, sorted, 3)

	indexA := indexOf(sorted, taskA.ID)
	indexB := indexOf(sorted, taskB.ID)
	indexC := indexOf(sorted, taskC.ID)

	assert.Less(t, indexA, indexB, "A should come before B")
	assert.Less(t, indexB, indexC, "B should come before C")
}

// TestTopologicalSort_BranchingDAG verifies parallel paths.
func TestTopologicalSort_BranchingDAG(t *testing.T) {
	projectID := uuid.New()

	taskA := makeTask("2.1", 3)
	taskB := makeTask("2.2", 2)
	taskC := makeTask("2.3", 2)
	taskD := makeTask("2.4", 1)

	tasks := []models.ProjectTask{taskA, taskB, taskC, taskD}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskA.ID, taskC.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskD.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskC.ID, taskD.ID, types.DependencyTypeFS, 0),
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

// TestDetectCycle verifies circular dependency detection.
func TestDetectCycle(t *testing.T) {
	projectID := uuid.New()

	taskA := makeTask("3.1", 2)
	taskB := makeTask("3.2", 2)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskA.ID, types.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	err := DetectCycle(g)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")
}

// TestForwardPass_FS verifies basic Finish-to-Start scheduling.
func TestForwardPass_FS(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC) // Monday

	taskA := makeTask("6.1", 3)
	taskB := makeTask("6.2", 2)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, types.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	cal := &StandardCalendar{}

	schedule, err := ForwardPass(g, projectStart, cal, nil)

	require.NoError(t, err)
	require.Len(t, schedule, 2)

	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	assert.Equal(t, projectStart, schedA.EarlyStart, "A starts at project start")
	assert.True(t, schedB.EarlyStart.Equal(schedA.EarlyFinish) || schedB.EarlyStart.After(schedA.EarlyFinish),
		"B starts at or after A finishes")
}

// TestForwardPass_SS verifies Start-to-Start scheduling.
func TestForwardPass_SS(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	taskA := makeTask("7.1", 5)
	taskB := makeTask("7.2", 3)

	tasks := []models.ProjectTask{taskA, taskB}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, types.DependencyTypeSS, 2),
	}

	g := BuildDependencyGraph(tasks, deps)
	cal := &StandardCalendar{}

	schedule, err := ForwardPass(g, projectStart, cal, nil)

	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	schedB := schedule[taskB.ID]

	// B starts 2 days after A starts (SS+2 lag)
	expectedBStart := cal.AddWorkDuration(schedA.EarlyStart, 2*24*time.Hour)
	assert.Equal(t, expectedBStart, schedB.EarlyStart, "B starts 2 working days after A starts (SS+2)")
}

// TestForwardPass_MaterialConstraint verifies material delivery constraints.
func TestForwardPass_MaterialConstraint(t *testing.T) {
	projectStart := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	taskA := makeTask("8.1", 3)

	tasks := []models.ProjectTask{taskA}

	g := BuildDependencyGraph(tasks, nil)
	cal := &StandardCalendar{}

	// Material arrives 5 days after project start
	matDate := cal.AddWorkDuration(projectStart, 5*24*time.Hour)
	materialConstraints := map[uuid.UUID]time.Time{
		taskA.ID: matDate,
	}

	schedule, err := ForwardPass(g, projectStart, cal, materialConstraints)

	require.NoError(t, err)

	schedA := schedule[taskA.ID]
	assert.Equal(t, matDate, schedA.EarlyStart, "Task cannot start before material arrives")
}

// TestBackwardPass_CriticalPath verifies critical path identification.
func TestBackwardPass_CriticalPath(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	// Linear chain: A(3d) -> B(2d) -> C(1d) — all critical
	taskA := makeTask("9.1", 3)
	taskB := makeTask("9.2", 2)
	taskC := makeTask("9.3", 1)

	tasks := []models.ProjectTask{taskA, taskB, taskC}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskC.ID, types.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	cal := &StandardCalendar{}

	schedule, err := ForwardPass(g, projectStart, cal, nil)
	require.NoError(t, err)

	criticalPath, err := BackwardPass(g, schedule, cal, nil)
	require.NoError(t, err)

	// All tasks on a linear chain are critical
	assert.Len(t, criticalPath, 3, "All 3 tasks should be on critical path")
	assert.Contains(t, criticalPath, "9.1")
	assert.Contains(t, criticalPath, "9.2")
	assert.Contains(t, criticalPath, "9.3")

	// Verify zero float on critical tasks
	for _, taskID := range []uuid.UUID{taskA.ID, taskB.ID, taskC.ID} {
		sched := schedule[taskID]
		assert.True(t, sched.IsCritical, "Task %s should be critical", sched.WBSCode)
	}
}

// TestBackwardPass_FloatCalculation verifies float on parallel paths.
func TestBackwardPass_FloatCalculation(t *testing.T) {
	projectID := uuid.New()
	projectStart := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

	// A(5d) -> C(1d)  — critical path (6 days)
	// A(5d) -> B(2d) -> C(1d) — non-critical (B has float)
	// Wait, this doesn't give float. Let me make a diamond:
	// A(3d) -> B(5d) -> D(1d) — critical (9d)
	// A(3d) -> C(2d) -> D(1d) — non-critical (C has 3d float)
	taskA := makeTask("10.1", 3)
	taskB := makeTask("10.2", 5)
	taskC := makeTask("10.3", 2)
	taskD := makeTask("10.4", 1)

	tasks := []models.ProjectTask{taskA, taskB, taskC, taskD}
	deps := []models.TaskDependency{
		makeDep(projectID, taskA.ID, taskB.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskA.ID, taskC.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskB.ID, taskD.ID, types.DependencyTypeFS, 0),
		makeDep(projectID, taskC.ID, taskD.ID, types.DependencyTypeFS, 0),
	}

	g := BuildDependencyGraph(tasks, deps)
	cal := &StandardCalendar{}

	schedule, err := ForwardPass(g, projectStart, cal, nil)
	require.NoError(t, err)

	criticalPath, err := BackwardPass(g, schedule, cal, nil)
	require.NoError(t, err)

	// A, B, D are critical; C is not
	assert.Contains(t, criticalPath, "10.1", "A is critical")
	assert.Contains(t, criticalPath, "10.2", "B is critical")
	assert.Contains(t, criticalPath, "10.4", "D is critical")
	assert.NotContains(t, criticalPath, "10.3", "C has float and is not critical")

	// C should have positive total float
	schedC := schedule[taskC.ID]
	assert.Greater(t, schedC.TotalFloat, 0.0, "Task C should have positive float")
}

// TestCalendar_WeekendSkip verifies that weekend days are skipped.
func TestCalendar_WeekendSkip(t *testing.T) {
	cal := &StandardCalendar{}
	friday := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC) // Friday

	// Adding 1 working day should skip to Monday
	result := cal.AddWorkDuration(friday, 24*time.Hour)
	assert.Equal(t, time.Monday, result.Weekday(), "Should skip weekend to Monday")
}

// TestCalendar_HolidaySkip verifies that holidays are skipped.
func TestCalendar_HolidaySkip(t *testing.T) {
	cal := &StandardCalendar{
		Holidays: []time.Time{
			time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), // Wednesday
		},
	}
	tuesday := time.Date(2026, 1, 6, 8, 0, 0, 0, time.UTC) // Tuesday

	// Adding 1 working day should skip Wednesday (holiday) to Thursday
	result := cal.AddWorkDuration(tuesday, 24*time.Hour)
	assert.Equal(t, time.Thursday, result.Weekday(), "Should skip holiday to Thursday")
}

// TestGetTaskDuration_FailLoudly verifies zero-duration tasks are rejected.
func TestGetTaskDuration_FailLoudly(t *testing.T) {
	task := models.ProjectTask{
		ID:      uuid.New(),
		WBSCode: "99.1",
	}

	_, err := getTaskDuration(task)
	assert.ErrorIs(t, err, ErrInvalidTaskDuration)
	assert.Contains(t, err.Error(), "99.1")
}

// TestGetTaskDuration_Precedence verifies ManualOverride > WeatherAdjusted > Calculated > DurationDays.
func TestGetTaskDuration_Precedence(t *testing.T) {
	override := 10.0
	task := models.ProjectTask{
		ID:                      uuid.New(),
		WBSCode:                 "9.1",
		DurationDays:            3,
		CalculatedDuration:      5.0,
		WeatherAdjustedDuration: 7.0,
		ManualOverrideDays:      &override,
	}

	dur, err := getTaskDuration(task)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(10*float64(24*time.Hour)), dur, "ManualOverride wins")
}

// --- Benchmarks for CI gate (make audit) ---

// generateLinearGraph creates a linear task chain of N tasks for benchmarking.
func generateLinearGraph(n int) (*DependencyGraph, time.Time) {
	projectID := uuid.New()
	tasks := make([]models.ProjectTask, n)
	deps := make([]models.TaskDependency, 0, n-1)

	for i := 0; i < n; i++ {
		tasks[i] = models.ProjectTask{
			ID:           uuid.New(),
			ProjectID:    projectID,
			WBSCode:      fmt.Sprintf("%d.1", i+1),
			Name:         fmt.Sprintf("Task %d", i+1),
			DurationDays: 2 + (i % 5), // 2-6 day durations
		}
	}

	for i := 0; i < n-1; i++ {
		deps = append(deps, models.TaskDependency{
			ID:             uuid.New(),
			ProjectID:      projectID,
			PredecessorID:  tasks[i].ID,
			SuccessorID:    tasks[i+1].ID,
			DependencyType: types.DependencyTypeFS,
			LagDays:        0,
		})
	}

	g := BuildDependencyGraph(tasks, deps)
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	return g, start
}

// generateDiamondGraph creates a more realistic graph with parallel paths.
func generateDiamondGraph(n int) (*DependencyGraph, time.Time) {
	projectID := uuid.New()
	tasks := make([]models.ProjectTask, n)
	var deps []models.TaskDependency

	for i := 0; i < n; i++ {
		tasks[i] = models.ProjectTask{
			ID:           uuid.New(),
			ProjectID:    projectID,
			WBSCode:      fmt.Sprintf("%d.%d", (i/10)+7, (i%10)+1),
			Name:         fmt.Sprintf("Task %d", i+1),
			DurationDays: 1 + (i % 7),
		}
	}

	// Create a realistic dependency pattern:
	// - Every task depends on the task before it (linear spine)
	// - Every 5th task also depends on the task 3 positions back (diamond paths)
	for i := 1; i < n; i++ {
		deps = append(deps, models.TaskDependency{
			ID:             uuid.New(),
			ProjectID:      projectID,
			PredecessorID:  tasks[i-1].ID,
			SuccessorID:    tasks[i].ID,
			DependencyType: types.DependencyTypeFS,
			LagDays:        0,
		})

		if i >= 3 && i%5 == 0 {
			deps = append(deps, models.TaskDependency{
				ID:             uuid.New(),
				ProjectID:      projectID,
				PredecessorID:  tasks[i-3].ID,
				SuccessorID:    tasks[i].ID,
				DependencyType: types.DependencyTypeSS,
				LagDays:        1,
			})
		}
	}

	g := BuildDependencyGraph(tasks, deps)
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	return g, start
}

// BenchmarkCPM80Tasks benchmarks ForwardPass + BackwardPass on an 80-task graph.
// CI gate: must complete in <200ms
func BenchmarkCPM80Tasks(b *testing.B) {
	g, start := generateDiamondGraph(80)
	cal := &StandardCalendar{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		schedule, err := ForwardPass(g, start, cal, nil)
		if err != nil {
			b.Fatal(err)
		}
		_, err = BackwardPass(g, schedule, cal, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCPM200Tasks benchmarks ForwardPass + BackwardPass on a 200-task graph.
// CI gate: must complete in <500ms
func BenchmarkCPM200Tasks(b *testing.B) {
	g, start := generateDiamondGraph(200)
	cal := &StandardCalendar{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		schedule, err := ForwardPass(g, start, cal, nil)
		if err != nil {
			b.Fatal(err)
		}
		_, err = BackwardPass(g, schedule, cal, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBuildGraph80 benchmarks graph construction for 80 tasks.
func BenchmarkBuildGraph80(b *testing.B) {
	projectID := uuid.New()
	tasks := make([]models.ProjectTask, 80)
	deps := make([]models.TaskDependency, 79)

	for i := 0; i < 80; i++ {
		tasks[i] = models.ProjectTask{
			ID:           uuid.New(),
			ProjectID:    projectID,
			WBSCode:      fmt.Sprintf("%d.1", i+1),
			DurationDays: 3,
		}
	}
	for i := 0; i < 79; i++ {
		deps[i] = models.TaskDependency{
			ID:            uuid.New(),
			PredecessorID: tasks[i].ID,
			SuccessorID:   tasks[i+1].ID,
			DependencyType: types.DependencyTypeFS,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildDependencyGraph(tasks, deps)
	}
}
