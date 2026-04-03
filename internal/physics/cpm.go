// Package physics implements the deterministic scheduling algorithms.
// See BACKEND_SCOPE.md Section 3.4 (Layer 3: Physics Engine)
package physics

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/models/types"
)

// ErrInvalidTaskDuration indicates a task has no valid duration for CPM calculation.
// See PRODUCTION_PLAN.md: Fail-loudly approach prevents silent schedule corruption.
var ErrInvalidTaskDuration = fmt.Errorf("invalid task duration")

// DependencyGraph encapsulates the topology and edge metadata for CPM.
// See BACKEND_SCOPE.md Section 6.3
type DependencyGraph struct {
	Graph *simple.DirectedGraph

	// UUID <-> int64 mapping for gonum compatibility
	NodeMap map[uuid.UUID]int64
	TaskMap map[int64]uuid.UUID

	// Task data lookup (duration, WBS code, etc.)
	Tasks map[uuid.UUID]models.ProjectTask

	// Edge metadata: Deps[from][to] -> TaskDependency
	// Stores lag_days, dependency_type per DATA_SPINE_SPEC Section 3.4
	Deps map[int64]map[int64]models.TaskDependency
}

// TaskSchedule holds the calculated CPM results for a single task.
// See BACKEND_SCOPE.md Section 6.3
type TaskSchedule struct {
	TaskID      uuid.UUID `json:"task_id"`
	WBSCode     string    `json:"wbs_code"`
	EarlyStart  time.Time `json:"early_start"`
	EarlyFinish time.Time `json:"early_finish"`
	LateStart   time.Time `json:"late_start"`
	LateFinish  time.Time `json:"late_finish"`
	TotalFloat  float64   `json:"total_float"`
	IsCritical  bool      `json:"is_critical"`
}

// Calendar defines the interface for date calculations that respect working days.
// See BACKEND_SCOPE.md Section 6.3
type Calendar interface {
	// AddWorkingDays adds the specified number of working days to a date.
	// DEPRECATED: Use AddWorkDuration for deterministic integer math.
	AddWorkingDays(date time.Time, days float64) time.Time

	// AddWorkDuration adds work duration using integer math for determinism.
	// P1 Correctness Fix: Eliminates IEEE 754 floating-point drift.
	AddWorkDuration(date time.Time, duration time.Duration) time.Time
}

// WorkDay is the standard duration of a working day (8 hours).
const WorkDay = 8 * time.Hour

// StandardCalendar implements a configurable work week calendar.
type StandardCalendar struct {
	WorkDays []time.Weekday
	Holidays []time.Time
}

// SchedulingConfig holds configurable parameters for CPM scheduling.
type SchedulingConfig struct {
	CriticalPathThreshold float64
}

// DefaultSchedulingConfig returns the default scheduling configuration.
func DefaultSchedulingConfig() *SchedulingConfig {
	return &SchedulingConfig{
		CriticalPathThreshold: 0.001,
	}
}

// isHoliday checks if a date matches any holiday (comparing month and day only).
func (c *StandardCalendar) isHoliday(date time.Time) bool {
	for _, h := range c.Holidays {
		if date.Month() == h.Month() && date.Day() == h.Day() {
			return true
		}
	}
	return false
}

// isNonWorkingDay returns true if the date is not a working day or is a holiday.
func (c *StandardCalendar) isNonWorkingDay(date time.Time) bool {
	workDays := c.WorkDays
	if len(workDays) == 0 {
		workDays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday,
			time.Thursday, time.Friday}
	}

	isWorkDay := false
	for _, wd := range workDays {
		if date.Weekday() == wd {
			isWorkDay = true
			break
		}
	}

	if !isWorkDay {
		return true
	}
	return c.isHoliday(date)
}

// AddWorkDuration adds work duration using integer nanosecond math.
// P1 Correctness Fix: Eliminates IEEE 754 floating-point drift.
func (c *StandardCalendar) AddWorkDuration(date time.Time, duration time.Duration) time.Time {
	if duration == 0 {
		return date
	}

	wholeDays := duration / (24 * time.Hour)
	remainder := duration % (24 * time.Hour)

	result := date

	if wholeDays > 0 {
		for i := time.Duration(0); i < wholeDays; i++ {
			result = result.Add(24 * time.Hour)
			for c.isNonWorkingDay(result) {
				result = result.Add(24 * time.Hour)
			}
		}
	} else if wholeDays < 0 {
		for i := time.Duration(0); i > wholeDays; i-- {
			result = result.Add(-24 * time.Hour)
			for c.isNonWorkingDay(result) {
				result = result.Add(-24 * time.Hour)
			}
		}
	}

	if remainder != 0 {
		result = result.Add(remainder)
	}

	return result.Truncate(time.Minute)
}

// AddWorkingDays adds fractional working days to a date, skipping weekends and holidays.
// DEPRECATED: Use AddWorkDuration for guaranteed deterministic integer math.
func (c *StandardCalendar) AddWorkingDays(date time.Time, days float64) time.Time {
	if days == 0 {
		return date
	}

	wholeDays := int(days)
	fraction := days - float64(wholeDays)

	result := date

	if wholeDays > 0 {
		for i := 0; i < wholeDays; i++ {
			result = result.Add(24 * time.Hour)
			for c.isNonWorkingDay(result) {
				result = result.Add(24 * time.Hour)
			}
		}
	} else if wholeDays < 0 {
		for i := 0; i > wholeDays; i-- {
			result = result.Add(-24 * time.Hour)
			for c.isNonWorkingDay(result) {
				result = result.Add(-24 * time.Hour)
			}
		}
	}

	if fraction != 0 {
		result = result.Add(time.Duration(fraction * 24 * float64(time.Hour)))
	}

	return result.Truncate(time.Minute)
}

// CPMResult represents the output of CPM scheduling.
type CPMResult struct {
	Tasks            []TaskSchedule `json:"tasks"`
	CriticalPath     []string       `json:"critical_path"`
	ProjectEnd       time.Time      `json:"project_end"`
	CriticalPathChanged bool        `json:"critical_path_changed"`
}

// BuildDependencyGraph constructs a DAG from ProjectTask and TaskDependency models.
func BuildDependencyGraph(tasks []models.ProjectTask, deps []models.TaskDependency) *DependencyGraph {
	g := &DependencyGraph{
		Graph:   simple.NewDirectedGraph(),
		NodeMap: make(map[uuid.UUID]int64),
		TaskMap: make(map[int64]uuid.UUID),
		Tasks:   make(map[uuid.UUID]models.ProjectTask),
		Deps:    make(map[int64]map[int64]models.TaskDependency),
	}

	var nodeID int64 = 1
	for _, task := range tasks {
		node := simple.Node(nodeID)
		g.Graph.AddNode(node)
		g.NodeMap[task.ID] = nodeID
		g.TaskMap[nodeID] = task.ID
		g.Tasks[task.ID] = task
		nodeID++
	}

	for _, dep := range deps {
		fromID, fromExists := g.NodeMap[dep.PredecessorID]
		toID, toExists := g.NodeMap[dep.SuccessorID]

		if !fromExists || !toExists {
			continue
		}

		edge := simple.Edge{F: simple.Node(fromID), T: simple.Node(toID)}
		g.Graph.SetEdge(edge)

		if g.Deps[fromID] == nil {
			g.Deps[fromID] = make(map[int64]models.TaskDependency)
		}
		g.Deps[fromID][toID] = dep
	}

	return g
}

// TopologicalSort returns task IDs in processing order for CPM forward/backward passes.
func TopologicalSort(g *DependencyGraph) ([]uuid.UUID, error) {
	sorted, err := topo.Sort(g.Graph)
	if err != nil {
		return nil, fmt.Errorf("topological sort failed: %w", err)
	}

	result := make([]uuid.UUID, len(sorted))
	for i, node := range sorted {
		result[i] = g.TaskMap[node.ID()]
	}

	return result, nil
}

// DetectCycle checks if the dependency graph contains circular dependencies.
func DetectCycle(g *DependencyGraph) error {
	_, err := topo.Sort(g.Graph)
	if err == nil {
		return nil
	}

	unorderable, ok := err.(topo.Unorderable)
	if !ok {
		return fmt.Errorf("cycle detected: %w", err)
	}

	var cyclicTasks []string
	for _, component := range unorderable {
		for _, node := range component {
			taskID := g.TaskMap[node.ID()]
			if task, exists := g.Tasks[taskID]; exists {
				cyclicTasks = append(cyclicTasks, task.WBSCode)
			}
		}
	}

	return fmt.Errorf("cycle detected involving tasks: %v", cyclicTasks)
}

// GetDependency retrieves edge metadata for a specific predecessor->successor relationship.
func (g *DependencyGraph) GetDependency(predecessorID, successorID uuid.UUID) (models.TaskDependency, bool) {
	fromID, fromExists := g.NodeMap[predecessorID]
	toID, toExists := g.NodeMap[successorID]

	if !fromExists || !toExists {
		return models.TaskDependency{}, false
	}

	if g.Deps[fromID] == nil {
		return models.TaskDependency{}, false
	}

	dep, exists := g.Deps[fromID][toID]
	return dep, exists
}

// GetPredecessors returns all predecessor task IDs for the given task.
func (g *DependencyGraph) GetPredecessors(taskID uuid.UUID) []uuid.UUID {
	nodeID, exists := g.NodeMap[taskID]
	if !exists {
		return nil
	}

	var predecessors []uuid.UUID
	nodes := g.Graph.To(nodeID)
	for nodes.Next() {
		predNodeID := nodes.Node().ID()
		if predTaskID, ok := g.TaskMap[predNodeID]; ok {
			predecessors = append(predecessors, predTaskID)
		}
	}
	return predecessors
}

// GetSuccessors returns all successor task IDs for the given task.
func (g *DependencyGraph) GetSuccessors(taskID uuid.UUID) []uuid.UUID {
	nodeID, exists := g.NodeMap[taskID]
	if !exists {
		return nil
	}

	var successors []uuid.UUID
	nodes := g.Graph.From(nodeID)
	for nodes.Next() {
		succNodeID := nodes.Node().ID()
		if succTaskID, ok := g.TaskMap[succNodeID]; ok {
			successors = append(successors, succTaskID)
		}
	}
	return successors
}

// getTaskDuration resolves the effective duration for a task as time.Duration.
// Precedence: ManualOverrideDays > WeatherAdjustedDuration > CalculatedDuration > DurationDays
// Returns ErrInvalidTaskDuration if no valid duration exists (fail-loudly approach).
func getTaskDuration(task models.ProjectTask) (time.Duration, error) {
	var durationDays float64
	if task.ManualOverrideDays != nil && *task.ManualOverrideDays > 0 {
		durationDays = *task.ManualOverrideDays
	} else if task.WeatherAdjustedDuration > 0 {
		durationDays = task.WeatherAdjustedDuration
	} else if task.CalculatedDuration > 0 {
		durationDays = task.CalculatedDuration
	} else if task.DurationDays > 0 {
		durationDays = float64(task.DurationDays)
	} else {
		return 0, fmt.Errorf("%w: task %q (ID: %s) has no valid duration (ManualOverride=nil, WeatherAdjusted=0, Calculated=0, DurationDays=0)",
			ErrInvalidTaskDuration, task.WBSCode, task.ID)
	}
	return time.Duration(durationDays * float64(24*time.Hour)), nil
}

// ForwardPass calculates Early Start (ES) and Early Finish (EF) for all tasks.
func ForwardPass(g *DependencyGraph, projectStart time.Time, cal Calendar, materialConstraints map[uuid.UUID]time.Time) (map[uuid.UUID]TaskSchedule, error) {
	sorted, err := TopologicalSort(g)
	if err != nil {
		return nil, fmt.Errorf("forward pass failed: %w", err)
	}

	schedule := make(map[uuid.UUID]TaskSchedule)

	for _, taskID := range sorted {
		task, exists := g.Tasks[taskID]
		if !exists {
			continue
		}

		duration, err := getTaskDuration(task)
		if err != nil {
			return nil, fmt.Errorf("forward pass failed: %w", err)
		}
		predecessors := g.GetPredecessors(taskID)

		var earlyStart time.Time

		if len(predecessors) == 0 {
			earlyStart = projectStart
		} else {
			var maxConstraintDate time.Time
			firstPredecessor := true

			for _, predID := range predecessors {
				predSchedule, predExists := schedule[predID]
				if !predExists {
					continue
				}

				dep, depExists := g.GetDependency(predID, taskID)
				if !depExists {
					dep = models.TaskDependency{
						DependencyType: types.DependencyTypeFS,
						LagDays:        0,
					}
				}

				constraintDate := calculateConstraintDate(
					cal,
					predSchedule,
					duration,
					dep.DependencyType,
					dep.LagDays,
				)

				if firstPredecessor || constraintDate.After(maxConstraintDate) {
					maxConstraintDate = constraintDate
					firstPredecessor = false
				}
			}

			earlyStart = maxConstraintDate
		}

		// Material Constraint Check (MRP Feedback Loop)
		if materialConstraints != nil {
			if matDate, ok := materialConstraints[taskID]; ok {
				if matDate.After(earlyStart) {
					earlyStart = matDate
				}
			}
		}

		earlyFinish := cal.AddWorkDuration(earlyStart, duration)

		schedule[taskID] = TaskSchedule{
			TaskID:      taskID,
			WBSCode:     task.WBSCode,
			EarlyStart:  earlyStart,
			EarlyFinish: earlyFinish,
		}
	}

	return schedule, nil
}

// calculateConstraintDate determines the earliest start date for a successor.
func calculateConstraintDate(
	cal Calendar,
	predSchedule TaskSchedule,
	successorDuration time.Duration,
	depType types.DependencyType,
	lagDays int,
) time.Time {
	lag := time.Duration(lagDays) * 24 * time.Hour

	switch depType {
	case types.DependencyTypeFS:
		return cal.AddWorkDuration(predSchedule.EarlyFinish, lag)
	case types.DependencyTypeSS:
		return cal.AddWorkDuration(predSchedule.EarlyStart, lag)
	case types.DependencyTypeFF:
		return cal.AddWorkDuration(predSchedule.EarlyFinish, lag-successorDuration)
	case types.DependencyTypeSF:
		return cal.AddWorkDuration(predSchedule.EarlyStart, lag-successorDuration)
	default:
		return cal.AddWorkDuration(predSchedule.EarlyFinish, lag)
	}
}

// BackwardPass calculates Late Start (LS), Late Finish (LF), Total Float,
// and identifies the critical path for all tasks.
func BackwardPass(g *DependencyGraph, schedule map[uuid.UUID]TaskSchedule, cal Calendar, config *SchedulingConfig) ([]string, error) {
	if config == nil {
		config = DefaultSchedulingConfig()
	}

	if len(schedule) == 0 {
		return nil, nil
	}

	var projectEnd time.Time
	first := true
	for _, sched := range schedule {
		if first || sched.EarlyFinish.After(projectEnd) {
			projectEnd = sched.EarlyFinish
			first = false
		}
	}

	sorted, err := TopologicalSort(g)
	if err != nil {
		return nil, fmt.Errorf("backward pass failed: %w", err)
	}

	reversed := make([]uuid.UUID, len(sorted))
	for i, id := range sorted {
		reversed[len(sorted)-1-i] = id
	}

	for _, taskID := range reversed {
		task, exists := g.Tasks[taskID]
		if !exists {
			continue
		}

		sched, schedExists := schedule[taskID]
		if !schedExists {
			continue
		}

		duration, err := getTaskDuration(task)
		if err != nil {
			return nil, fmt.Errorf("backward pass failed: %w", err)
		}
		successors := g.GetSuccessors(taskID)

		var lateFinish time.Time

		if len(successors) == 0 {
			lateFinish = projectEnd
		} else {
			firstSuccessor := true

			for _, succID := range successors {
				succSchedule, succExists := schedule[succID]
				if !succExists {
					continue
				}

				dep, depExists := g.GetDependency(taskID, succID)
				if !depExists {
					dep = models.TaskDependency{
						DependencyType: types.DependencyTypeFS,
						LagDays:        0,
					}
				}

				constraintDate := calculateBackwardConstraintDate(
					cal,
					succSchedule,
					duration,
					dep.DependencyType,
					dep.LagDays,
				)

				if firstSuccessor || constraintDate.Before(lateFinish) {
					lateFinish = constraintDate
					firstSuccessor = false
				}
			}
		}

		lateStart := cal.AddWorkDuration(lateFinish, -duration)

		floatDays := lateStart.Sub(sched.EarlyStart).Hours() / 24

		sched.LateStart = lateStart
		sched.LateFinish = lateFinish
		sched.TotalFloat = floatDays
		sched.IsCritical = math.Abs(floatDays) < config.CriticalPathThreshold

		schedule[taskID] = sched
	}

	var criticalPath []string
	for _, taskID := range sorted {
		if sched, exists := schedule[taskID]; exists && sched.IsCritical {
			criticalPath = append(criticalPath, sched.WBSCode)
		}
	}

	return criticalPath, nil
}

// calculateBackwardConstraintDate determines the latest finish date for a predecessor.
func calculateBackwardConstraintDate(
	cal Calendar,
	succSchedule TaskSchedule,
	predecessorDuration time.Duration,
	depType types.DependencyType,
	lagDays int,
) time.Time {
	lag := time.Duration(lagDays) * 24 * time.Hour

	switch depType {
	case types.DependencyTypeFS:
		return cal.AddWorkDuration(succSchedule.LateStart, -lag)
	case types.DependencyTypeSS:
		return cal.AddWorkDuration(succSchedule.LateStart, -lag+predecessorDuration)
	case types.DependencyTypeFF:
		return cal.AddWorkDuration(succSchedule.LateFinish, -lag)
	case types.DependencyTypeSF:
		return cal.AddWorkDuration(succSchedule.LateFinish, -lag+predecessorDuration)
	default:
		return cal.AddWorkDuration(succSchedule.LateStart, -lag)
	}
}
