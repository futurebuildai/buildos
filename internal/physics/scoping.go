package physics

import (
	"strings"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/models/types"
)

// ProjectScopeContext captures the attributes that drive scope-adaptive WBS generation.
type ProjectScopeContext struct {
	FoundationType string
	Stories        int
	GSF            float64
	Bedrooms       int
	Bathrooms      int
	Topography     string
	SoilConditions string

	CompletedWBSCodes []string
	CurrentPhase      string
}

// ScopeChange records a single modification made by the scoping engine.
type ScopeChange struct {
	RuleApplied         string             `json:"rule_applied"`
	TasksAdded          []string           `json:"tasks_added,omitempty"`
	TasksRemoved        []string           `json:"tasks_removed,omitempty"`
	DurationAdjustments map[string]float64 `json:"duration_adjustments,omitempty"`
}

// ApplyScope takes the master WBS template and adapts it based on project attributes.
func ApplyScope(
	tasks []models.WBSTask,
	deps []models.WBSTemplateDep,
	ctx ProjectScopeContext,
) ([]models.WBSTask, []models.WBSTemplateDep, []ScopeChange) {
	var changes []ScopeChange
	var addedDeps []models.WBSTemplateDep

	taskMap := make(map[string]models.WBSTask, len(tasks))
	for _, t := range tasks {
		taskMap[t.Code] = t
	}

	origCodes := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		origCodes[t.Code] = true
	}

	changes = append(changes, applyFoundationRules(taskMap, ctx)...)
	changes = append(changes, applyStoryRules(taskMap, ctx)...)
	changes = append(changes, applySizeRules(taskMap, ctx)...)
	changes = append(changes, applyTopographyRules(taskMap, ctx)...)
	changes = append(changes, applyInProgressRules(taskMap, ctx)...)

	for code, t := range taskMap {
		if origCodes[code] {
			continue
		}
		for _, predCode := range t.PredecessorCodes {
			addedDeps = append(addedDeps, newDep(predCode, code))
		}
	}
	deps = append(deps, addedDeps...)

	result := make([]models.WBSTask, 0, len(taskMap))
	for _, t := range tasks {
		if _, exists := taskMap[t.Code]; exists {
			result = append(result, taskMap[t.Code])
		}
	}
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

	validCodes := make(map[string]bool, len(result))
	for _, t := range result {
		validCodes[t.Code] = true
	}
	var filteredDeps []models.WBSTemplateDep
	for _, d := range deps {
		if validCodes[d.PredecessorCode] && validCodes[d.SuccessorCode] {
			filteredDeps = append(filteredDeps, d)
		}
	}

	return result, filteredDeps, changes
}

func applyFoundationRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	var changes []ScopeChange
	ft := strings.ToLower(ctx.FoundationType)

	switch ft {
	case "slab":
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

func applyStoryRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	var changes []ScopeChange

	if ctx.Stories <= 0 {
		return nil
	}

	if ctx.Stories == 1 {
		var removed []string
		if _, exists := taskMap["9.2"]; exists {
			delete(taskMap, "9.2")
			removed = append(removed, "9.2")
		}

		adjustments := make(map[string]float64)
		if t, exists := taskMap["9.1"]; exists {
			t.BaseDurationDays *= 0.7
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
				t.BaseDurationDays *= 1.3
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

func applySizeRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	if ctx.GSF <= 4000 {
		return nil
	}

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

func applyTopographyRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	topoStr := strings.ToLower(ctx.Topography)
	if topoStr != "hillside" {
		return nil
	}

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
				t.BaseDurationDays *= 1.4
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

func applyInProgressRules(taskMap map[string]models.WBSTask, ctx ProjectScopeContext) []ScopeChange {
	if len(ctx.CompletedWBSCodes) == 0 {
		return nil
	}

	completedSet := make(map[string]bool)
	for _, code := range ctx.CompletedWBSCodes {
		if strings.HasSuffix(code, ".x") {
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

	var completed []string
	for code := range completedSet {
		if t, exists := taskMap[code]; exists {
			t.BaseDurationDays = 0.5
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

func newDep(pred, succ string) models.WBSTemplateDep {
	return models.WBSTemplateDep{
		PredecessorCode: pred,
		SuccessorCode:   succ,
		Type:            types.DependencyTypeFS,
		LagDays:         0,
	}
}
