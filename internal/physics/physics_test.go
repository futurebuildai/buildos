package physics

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// TestCycleDetection_RejectCircular verifies that the system rejects any dependency update that creates a cycle.
// See PRODUCTION_PLAN.md Step 34.
func TestCycleDetection_RejectCircular(t *testing.T) {
	// 1. Setup Context
	projectID := uuid.New()

	// 2. create Tasks A, B, C
	taskA := models.ProjectTask{
		ID:      uuid.New(),
		WBSCode: "A",
		Name:    "Task A",
	}
	taskB := models.ProjectTask{
		ID:      uuid.New(),
		WBSCode: "B",
		Name:    "Task B",
	}
	taskC := models.ProjectTask{
		ID:      uuid.New(),
		WBSCode: "C",
		Name:    "Task C",
	}

	tasks := []models.ProjectTask{taskA, taskB, taskC}

	// 3. Define Circular Dependencies (A -> B -> C -> A)
	deps := []models.TaskDependency{
		{
			ID:             uuid.New(),
			ProjectID:      projectID,
			PredecessorID:  taskA.ID,
			SuccessorID:    taskB.ID,
			DependencyType: models.DependencyTypeFS,
			LagDays:        0,
		},
		{
			ID:             uuid.New(),
			ProjectID:      projectID,
			PredecessorID:  taskB.ID,
			SuccessorID:    taskC.ID,
			DependencyType: models.DependencyTypeFS,
			LagDays:        0,
		},
		{
			ID:             uuid.New(),
			ProjectID:      projectID,
			PredecessorID:  taskC.ID,
			SuccessorID:    taskA.ID, // The Cycle Creator
			DependencyType: models.DependencyTypeFS,
			LagDays:        0,
		},
	}

	// 4. Build Graph
	g := BuildDependencyGraph(tasks, deps)

	// 5. Execute Cycle Detection
	err := DetectCycle(g)

	// 6. Assertions
	require.Error(t, err, "DetectCycle should return an error for circular dependencies")

	// Verify error message clarity (must contain WBS codes)
	errMsg := err.Error()
	assert.Contains(t, errMsg, "cycle detected", "Error message should explicitly mention 'cycle detected'")
	assert.Contains(t, errMsg, "A", "Error message should contain WBS code 'A'")
	assert.Contains(t, errMsg, "B", "Error message should contain WBS code 'B'")
	assert.Contains(t, errMsg, "C", "Error message should contain WBS code 'C'")
}
