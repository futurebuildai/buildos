package physics

import (
	"strings"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ProjectScopeContext captures the attributes that drive scope-adaptive WBS generation.
// These fields are known at onboarding before a project is created.
type ProjectScopeContext struct {
	FoundationType string  // slab, crawlspace, basement
	Stories        int     // 1, 2, 3+
	GSF            float64 // gross square footage
	Bedrooms       int
	Bathrooms      int
	Topography     string // flat, sloped, hillside
	SoilConditions string // normal, rocky, clay, sandy

	// In-progress project support
	CompletedWBSCodes []string // WBS codes already completed
	CurrentPhase      string   // e.g. "10.x" if currently in rough-ins
}

// ScopeChange records a single modification made by the scoping engine.
type ScopeChange struct {
	RuleApplied         string             `json:"rule_applied"`
	TasksAdded          []string           `json:"tasks_added,omitempty"`
	TasksRemoved        []string           `json:"tasks_removed,omitempty"`
	DurationAdjustments map[string]float64 `json:"duration_adjustments,omitempty"` // code -> multiplier
}

// ApplyScope takes the master WBS template and adapts it based on project attributes.
// Returns the modified task list, dependency list, and a log of changes made.
// This is deterministic -- no AI, no randomness.
func ApplyScope(
	tasks []models.WBSTask,
	deps []models.WBSDependency,
	ctx ProjectScopeContext,
) ([]models.WBSTask, []models.WBSDependency, []ScopeChange) {
	var changes []ScopeChange
	var addedDeps []models.WBSDependency

	// Build working copy
	taskMap := make(map[string]models.WBSTask, len(tasks))
	for _, t := range tasks {
		taskMap[t.Code] = t
	}

	// Snapshot original codes to detect newly-added tasks
	origCodes := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		origCodes[t.Code] = true
	}

	// Apply rules in order
	changes = append(changes, applyFoundationRules(taskMap, ctx)...)
	changes = append(changes, applyStoryRules(taskMap, ctx)...)
	changes = append(changes, applySizeRules(taskMap, ctx)...)
	changes = append(changes, applyTopographyRules(taskMap, ctx)...)
	changes = append(changes, applyInProgressRules(taskMap, ctx)...)

	// Generate dependency edges for newly-added tasks from their PredecessorCodes
	for code, t := range taskMap {
		if origCodes[code] {
			continue // Original task -- deps already in master
		}
		for _, predCode := range t.PredecessorCodes {
			addedDeps = append(addedDeps, newDep(predCode, code))
		}
	}
	deps = append(deps, addedDeps...)

	// Rebuild flat task list from map
	result := make([]models.WBSTask, 0, len(taskMap))
	for _, t := range tasks {
		if _, exists := taskMap[t.Code]; exists {
			result = append(result, taskMap[t.Code])
		}
	}
	// Append any tasks that were added (not in original list)
	for code, t := range taskMap {
		found := false
		for _, orig := range tasks {
			if orig.Code == code {
				found = true
				break
			}
		}
		if !found {
			result = append(result, t)
		}
	}

	// Filter deps to only include edges where both tasks exist
	validCodes := make(map[string]bool, len(result))
	for _, t := range result {
		validCodes[t.Code] = true
	}
	var filteredDeps []models.WBSDependency
	for _, d := range deps {
		if validCodes[d.PredecessorCode] && validCodes[d.SuccessorCode] {
			filteredDeps = append(filteredDeps, d)
		}
	}

	return result, filteredDeps, changes
}

// applyFoundationRules adapts WBS for foundation type.
func applyFoundationRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	var changes []ScopeChange
	ft := strings.ToLower(ctx.FoundationType)

	switch ft {
	case "slab":
		// Slab-on-grade: remove waterproofing/drain tasks
		var removed []string
		for _, code := range []string{"8.7", "8.8"} {
			if _, exists := taskMap[code]; exists {
				delete(taskMap, code)
				removed = append(removed, code)
			}
		}
		if len(removed) > 0 {
			changes = append(changes, ScopeChange{
				RuleApplied:  "foundation=slab: remove waterproofing/drain tasks",
				TasksRemoved: removed,
			})
		}

	case "basement":
		// Basement: add drain tile, damp proofing, egress tasks
		addedTasks := []models.WBSTask{
			{
				Code:             "8.12",
				Name:             "Drain Tile Installation",
				BaseDurationDays: 2,
				ResponsibleParty: "Trade Partner",
				Deliverable:      "Work Completion",
				PredecessorCodes: []string{"8.6"},
			},
			{
				Code:             "8.13",
				Name:             "Damp Proofing / Waterproofing",
				BaseDurationDays: 2,
				ResponsibleParty: "Trade Partner",
				Deliverable:      "Work Completion",
				PredecessorCodes: []string{"8.12"},
			},
			{
				Code:             "8.14",
				Name:             "Basement Egress Window Installation",
				BaseDurationDays: 1,
				ResponsibleParty: "Trade Partner",
				Deliverable:      "Work Completion",
				PredecessorCodes: []string{"8.13"},
			},
		}

		var added []string
		for _, t := range addedTasks {
			taskMap[t.Code] = t
			added = append(added, t.Code)
		}
		changes = append(changes, ScopeChange{
			RuleApplied: "foundation=basement: add drain tile, damp proofing, egress tasks",
			TasksAdded:  added,
		})
	}

	return changes
}

// applyStoryRules adapts WBS for building height.
func applyStoryRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	var changes []ScopeChange

	if ctx.Stories <= 0 {
		return nil
	}

	if ctx.Stories == 1 {
		// Single story: remove second floor framing, reduce first floor framing duration
		var removed []string
		if _, exists := taskMap["9.2"]; exists {
			delete(taskMap, "9.2")
			removed = append(removed, "9.2")
		}

		adjustments := make(map[string]float64)
		if t, exists := taskMap["9.1"]; exists {
			t.BaseDurationDays *= 0.7 // Reduce by 30%
			taskMap["9.1"] = t
			adjustments["9.1"] = 0.7
		}

		if len(removed) > 0 || len(adjustments) > 0 {
			changes = append(changes, ScopeChange{
				RuleApplied:         "stories=1: remove second floor framing, reduce first floor framing 30%",
				TasksRemoved:        removed,
				DurationAdjustments: adjustments,
			})
		}
	}

	if ctx.Stories >= 3 {
		// 3+ stories: add engineered floor system, increase framing durations
		engTask := models.WBSTask{
			Code:             "9.8",
			Name:             "Engineered Floor System Installation",
			BaseDurationDays: 3,
			ResponsibleParty: "Trade Partner",
			Deliverable:      "Work Completion",
			PredecessorCodes: []string{"9.1"},
		}
		taskMap[engTask.Code] = engTask

		adjustments := make(map[string]float64)
		for _, code := range []string{"9.1", "9.2", "9.3"} {
			if t, exists := taskMap[code]; exists {
				t.BaseDurationDays *= 1.3 // Increase by 30%
				taskMap[code] = t
				adjustments[code] = 1.3
			}
		}

		changes = append(changes, ScopeChange{
			RuleApplied:         "stories>=3: add engineered floor system, increase framing durations 30%",
			TasksAdded:          []string{"9.8"},
			DurationAdjustments: adjustments,
		})
	}

	return changes
}

// applySizeRules adapts WBS for building size.
func applySizeRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	if ctx.GSF <= 4000 {
		return nil
	}

	// Large houses: add extended site prep
	extTask := models.WBSTask{
		Code:             "7.5",
		Name:             "Extended Site Preparation (Large Footprint)",
		BaseDurationDays: 3,
		ResponsibleParty: "Trade Partner",
		Deliverable:      "Work Completion",
		PredecessorCodes: []string{"7.4"},
	}
	taskMap[extTask.Code] = extTask

	return []ScopeChange{{
		RuleApplied: "gsf>4000: add extended site prep task",
		TasksAdded:  []string{"7.5"},
	}}
}

// applyTopographyRules adapts WBS for site topography.
func applyTopographyRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	topo := strings.ToLower(ctx.Topography)
	if topo != "hillside" {
		return nil
	}

	// Hillside: add retaining wall tasks, extend foundation 40%
	retainingTask := models.WBSTask{
		Code:             "7.6",
		Name:             "Retaining Wall Construction",
		BaseDurationDays: 5,
		ResponsibleParty: "Trade Partner",
		Deliverable:      "Work Completion",
		PredecessorCodes: []string{"7.4"},
	}
	taskMap[retainingTask.Code] = retainingTask

	adjustments := make(map[string]float64)
	for code := range taskMap {
		if strings.HasPrefix(code, "8.") {
			t := taskMap[code]
			if !t.IsInspection {
				t.BaseDurationDays *= 1.4 // Extend foundation 40%
				taskMap[code] = t
				adjustments[code] = 1.4
			}
		}
	}

	return []ScopeChange{{
		RuleApplied:         "topography=hillside: add retaining wall, extend foundation durations 40%",
		TasksAdded:          []string{"7.6"},
		DurationAdjustments: adjustments,
	}}
}

// applyInProgressRules marks completed tasks and adjusts for in-progress projects.
func applyInProgressRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	if len(ctx.CompletedWBSCodes) == 0 {
		return nil
	}

	// Build a set of completed codes, expanding phase wildcards (e.g., "8.x")
	completedSet := make(map[string]bool)
	for _, code := range ctx.CompletedWBSCodes {
		if strings.HasSuffix(code, ".x") {
			// Phase wildcard: "8.x" marks all tasks starting with "8."
			prefix := strings.TrimSuffix(code, "x")
			for taskCode := range taskMap {
				if strings.HasPrefix(taskCode, prefix) {
					completedSet[taskCode] = true
				}
			}
		} else {
			completedSet[code] = true
		}
	}

	// Mark completed tasks with zero duration (they'll get actual dates in the CPM pass)
	var completed []string
	for code := range completedSet {
		if t, exists := taskMap[code]; exists {
			// Set duration to a minimal value - actual dates will override in CPM
			t.BaseDurationDays = 0.5 // Minimum quantum (will be overridden by actual dates)
			taskMap[code] = t
			completed = append(completed, code)
		}
	}

	if len(completed) == 0 {
		return nil
	}

	return []ScopeChange{{
		RuleApplied:  "in-progress: mark completed tasks",
		TasksRemoved: nil,
		TasksAdded:   nil,
		DurationAdjustments: map[string]float64{
			"_completed_count": float64(len(completed)),
		},
	}}
}

// CompletedTaskCodes expands phase wildcards and returns individual task codes.
// Used by the schedule preview to set up materialConstraints for completed tasks.
func CompletedTaskCodes(completedInput []string, allTasks []models.WBSTask) []string {
	expandedSet := make(map[string]bool)
	allCodes := make(map[string]bool, len(allTasks))
	for _, t := range allTasks {
		allCodes[t.Code] = true
	}

	for _, code := range completedInput {
		if strings.HasSuffix(code, ".x") {
			prefix := strings.TrimSuffix(code, "x")
			for taskCode := range allCodes {
				if strings.HasPrefix(taskCode, prefix) {
					expandedSet[taskCode] = true
				}
			}
		} else if allCodes[code] {
			expandedSet[code] = true
		}
	}

	result := make([]string, 0, len(expandedSet))
	for code := range expandedSet {
		result = append(result, code)
	}
	return result
}

// IsTaskCompleted checks if a WBS code is in the completed set.
func IsTaskCompleted(code string, completedCodes []string) bool {
	for _, cc := range completedCodes {
		if cc == code {
			return true
		}
		if strings.HasSuffix(cc, ".x") {
			prefix := strings.TrimSuffix(cc, "x")
			if strings.HasPrefix(code, prefix) {
				return true
			}
		}
	}
	return false
}

// newDep is a convenience constructor for WBSDependency.
func newDep(pred, succ string) models.WBSDependency {
	return models.WBSDependency{
		PredecessorCode: pred,
		SuccessorCode:   succ,
		Type:            models.DependencyTypeFS,
		LagDays:         0,
	}
}
