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
	// Positive days move forward, negative days move backward.
	// Non-working days (weekends) are skipped.
	// DEPRECATED: Use AddWorkDuration for deterministic integer math.
	AddWorkingDays(date time.Time, days float64) time.Time

	// AddWorkDuration adds work duration using integer math for determinism.
	// Duration is expressed in nanoseconds (use WorkDay constant for convenience).
	// P1 Correctness Fix: Eliminates IEEE 754 floating-point drift.
	AddWorkDuration(date time.Time, duration time.Duration) time.Time
}

// WorkDay is the standard duration of a working day (8 hours).
// All scheduling calculations should use this as the atomic unit.
// P1 Correctness Fix: Integer math prevents floating-point drift.
//
// DESIGN DECISION (Code Review Issue 2A - Work Day Semantics):
// The current AddWorkDuration implementation uses 24-hour CALENDAR days
// (skipping weekends), not 8-hour work days. This is standard practice
// for construction scheduling where:
// - Tasks are estimated in workdays (Mon-Fri)
// - Duration represents calendar days, not labor hours
// - A "5-day task" spans Mon-Fri (5 calendar days, not 40 hours)
//
// Work week length is configurable per project via PhysicsConfig.WorkDaysPerWeek.
// Use NewStandardCalendar(workDaysPerWeek) to construct a calendar with
// the desired work week. Default is 5 (Mon-Fri).
const WorkDay = 8 * time.Hour

// StandardCalendar implements a configurable work week calendar.
// Weekends and specified Holidays are skipped when calculating working days.
// See PRODUCTION_PLAN.md Task 5 (Enhance Physics Engine).
type StandardCalendar struct {
	// WorkDays defines which weekdays are working days.
	// Defaults to Mon-Fri if nil or empty.
	WorkDays []time.Weekday
	// Holidays is a list of non-working dates. Comparison is by month and day only,
	// ignoring year (e.g., Dec 25 matches any year's Christmas).
	Holidays []time.Time
}

// NewStandardCalendar creates a StandardCalendar with the given number of
// working days per week. Days are assigned starting from Monday.
//
//   - workDaysPerWeek=5 -> Mon-Fri (default)
//   - workDaysPerWeek=6 -> Mon-Sat
//   - workDaysPerWeek=7 -> Mon-Sun (no weekends)
//
// If workDaysPerWeek is <= 0 or > 7, it defaults to 5 (Mon-Fri).
// This value is typically sourced from config.PhysicsConfig.WorkDaysPerWeek.
func NewStandardCalendar(workDaysPerWeek int) *StandardCalendar {
	if workDaysPerWeek <= 0 || workDaysPerWeek > 7 {
		workDaysPerWeek = 5
	}

	// Map count to weekdays starting from Monday.
	allDays := []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday, time.Sunday,
	}

	return &StandardCalendar{
		WorkDays: allDays[:workDaysPerWeek],
	}
}

// SchedulingConfig holds configurable parameters for CPM scheduling.
// See PRODUCTION_PLAN.md Task 5.
type SchedulingConfig struct {
	// CriticalPathThreshold is the float precision for critical path detection.
	// Tasks with TotalFloat <= this value are considered critical.
	// Default: 0.001 (per PRODUCTION_PLAN Step 33)
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
		// Default to Mon-Fri work week
		workDays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday,
			time.Thursday, time.Friday}
	}

	// Check if weekday is in working days
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
// This is the deterministic alternative to AddWorkingDays.
// Duration should be a multiple of WorkDay for best results.
// P1 Correctness Fix: Eliminates IEEE 754 floating-point drift.
func (c *StandardCalendar) AddWorkDuration(date time.Time, duration time.Duration) time.Time {
	if duration == 0 {
		return date
	}

	// Integer math only - no float64!
	wholeDays := duration / (24 * time.Hour)
	remainder := duration % (24 * time.Hour)

	result := date

	if wholeDays > 0 {
		// Forward: add working days
		for i := time.Duration(0); i < wholeDays; i++ {
			result = result.Add(24 * time.Hour)
			// Skip non-working days (weekends and holidays)
			for c.isNonWorkingDay(result) {
				result = result.Add(24 * time.Hour)
			}
		}
	} else if wholeDays < 0 {
		// Backward: subtract working days
		for i := time.Duration(0); i > wholeDays; i-- {
			result = result.Add(-24 * time.Hour)
			// Skip non-working days (weekends and holidays)
			for c.isNonWorkingDay(result) {
				result = result.Add(-24 * time.Hour)
			}
		}
	}

	// Add remainder within the work day (already in nanoseconds - integer math)
	if remainder != 0 {
		result = result.Add(remainder)
	}

	// P1 Fix: Truncate to minute precision for cross-architecture determinism
	return result.Truncate(time.Minute)
}

// AddWorkingDays adds fractional working days to a date, skipping weekends and holidays.
// For positive days (forward pass), if the result lands on a non-working day, it advances.
// For negative days (backward pass), if the result lands on a non-working day, it retreats.
// P1 Correctness Fix: Results are truncated to minute precision for determinism.
// DEPRECATED: Use AddWorkDuration for guaranteed deterministic integer math.
func (c *StandardCalendar) AddWorkingDays(date time.Time, days float64) time.Time {
	if days == 0 {
		return date
	}

	// Work with whole days and fractional remainder
	wholeDays := int(days)
	fraction := days - float64(wholeDays)

	result := date

	if wholeDays > 0 {
		// Forward: add working days
		for i := 0; i < wholeDays; i++ {
			result = result.Add(24 * time.Hour)
			// Skip non-working days (weekends and holidays)
			for c.isNonWorkingDay(result) {
				result = result.Add(24 * time.Hour)
			}
		}
	} else if wholeDays < 0 {
		// Backward: subtract working days
		for i := 0; i > wholeDays; i-- {
			result = result.Add(-24 * time.Hour)
			// Skip non-working days (weekends and holidays)
			for c.isNonWorkingDay(result) {
				result = result.Add(-24 * time.Hour)
			}
		}
	}

	// Handle fractional days (as hours within the current day)
	if fraction != 0 {
		result = result.Add(time.Duration(fraction * 24 * float64(time.Hour)))
	}

	// P1 Correctness Fix: Truncate to minute precision to eliminate nanosecond drift.
	// This ensures 1 + 1 always equals 2, regardless of x86 vs ARM architecture.
	return result.Truncate(time.Minute)
}

// CPMResult represents the output of CPM scheduling.
// See BACKEND_SCOPE.md Section 6.3
type CPMResult struct {
	Tasks        []TaskSchedule `json:"tasks"`
	CriticalPath []string       `json:"critical_path"` // WBS codes
	ProjectEnd   time.Time      `json:"project_end"`
}

// BuildDependencyGraph constructs a DAG from ProjectTask and TaskDependency models.
// See BACKEND_SCOPE.md Section 6.3
func BuildDependencyGraph(tasks []models.ProjectTask, deps []models.TaskDependency) *DependencyGraph {
	g := &DependencyGraph{
		Graph:   simple.NewDirectedGraph(),
		NodeMap: make(map[uuid.UUID]int64),
		TaskMap: make(map[int64]uuid.UUID),
		Tasks:   make(map[uuid.UUID]models.ProjectTask),
		Deps:    make(map[int64]map[int64]models.TaskDependency),
	}

	// Add nodes for each task
	var nodeID int64 = 1
	for _, task := range tasks {
		node := simple.Node(nodeID)
		g.Graph.AddNode(node)
		g.NodeMap[task.ID] = nodeID
		g.TaskMap[nodeID] = task.ID
		g.Tasks[task.ID] = task
		nodeID++
	}

	// Add edges for each dependency
	for _, dep := range deps {
		fromID, fromExists := g.NodeMap[dep.PredecessorID]
		toID, toExists := g.NodeMap[dep.SuccessorID]

		if !fromExists || !toExists {
			// Skip invalid dependencies (predecessor or successor not in task set)
			continue
		}

		// Add edge to graph
		edge := simple.Edge{F: simple.Node(fromID), T: simple.Node(toID)}
		g.Graph.SetEdge(edge)

		// Store edge metadata for O(1) lookup during CPM passes
		if g.Deps[fromID] == nil {
			g.Deps[fromID] = make(map[int64]models.TaskDependency)
		}
		g.Deps[fromID][toID] = dep
	}

	return g
}

// TopologicalSort returns task IDs in processing order for CPM forward/backward passes.
// Returns error if the graph contains a cycle.
// See BACKEND_SCOPE.md Section 6.3
func TopologicalSort(g *DependencyGraph) ([]uuid.UUID, error) {
	sorted, err := topo.Sort(g.Graph)
	if err != nil {
		return nil, fmt.Errorf("topological sort failed: %w", err)
	}

	// Convert node IDs back to UUIDs
	result := make([]uuid.UUID, len(sorted))
	for i, node := range sorted {
		result[i] = g.TaskMap[node.ID()]
	}

	return result, nil
}

// DetectCycle checks if the dependency graph contains circular dependencies.
// Returns a descriptive error with WBS codes of cyclic tasks per PRODUCTION_PLAN Step 34.
func DetectCycle(g *DependencyGraph) error {
	_, err := topo.Sort(g.Graph)
	if err == nil {
		return nil // No cycle
	}

	// Extract cycle information from topo.Unorderable error
	unorderable, ok := err.(topo.Unorderable)
	if !ok {
		return fmt.Errorf("cycle detected: %w", err)
	}

	// Collect WBS codes of cyclic tasks
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
// Returns the TaskDependency and true if found, or empty struct and false if not.
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
// See BACKEND_SCOPE.md Section 6.3
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

// getTaskDuration resolves the effective duration for a task as time.Duration.
// Precedence: ManualOverrideDays > WeatherAdjustedDuration > CalculatedDuration
// Returns ErrInvalidTaskDuration if no valid duration exists (fail-loudly approach).
// P1 Correctness Fix: Returns time.Duration for deterministic integer math.
// See CPM_RES_MODEL_SPEC.md Section 11.2.5
func getTaskDuration(task models.ProjectTask) (time.Duration, error) {
	var durationDays float64
	if task.ManualOverrideDays != nil && *task.ManualOverrideDays > 0 {
		durationDays = *task.ManualOverrideDays
	} else if task.WeatherAdjustedDuration > 0 {
		durationDays = task.WeatherAdjustedDuration
	} else if task.CalculatedDuration > 0 {
		durationDays = task.CalculatedDuration
	} else {
		// Fail-loudly: do NOT silently default to 1.0 day.
		// Invalid durations must halt CPM calculation to prevent invisible schedule corruption.
		return 0, fmt.Errorf("%w: task %q (ID: %s) has no valid duration (ManualOverride=nil, WeatherAdjusted=0, Calculated=0)",
			ErrInvalidTaskDuration, task.WBSCode, task.ID)
	}
	// Convert float64 days to time.Duration using integer multiplication
	// This preserves determinism: 1.5 days = 1.5 * 24 hours = 36 hours
	return time.Duration(durationDays * float64(24*time.Hour)), nil
}

// ForwardPass calculates Early Start (ES) and Early Finish (EF) for all tasks.
// Processes tasks in topological order, handling FS, SS, FF, SF dependencies.
// Uses the provided Calendar for working day calculations.
// materialConstraints enforces "Start No Earlier Than" dates based on material availability.
// See BACKEND_SCOPE.md Section 6.3, CPM_RES_MODEL_SPEC.md Section 11.4, PRODUCTION_PLAN.md Step 46
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
		var earlyFinish time.Time

		if len(predecessors) == 0 {
			// Root task: starts at project start date
			earlyStart = projectStart
		} else {
			// Calculate ES based on all predecessor constraints
			// Start with zero time, then find the maximum constraint date
			var maxConstraintDate time.Time
			firstPredecessor := true

			for _, predID := range predecessors {
				predSchedule, predExists := schedule[predID]
				if !predExists {
					// Predecessor not yet scheduled (shouldn't happen with topo sort)
					continue
				}

				dep, depExists := g.GetDependency(predID, taskID)
				if !depExists {
					// Default to FS with 0 lag if edge metadata missing
					dep = models.TaskDependency{
						DependencyType: models.DependencyTypeFS,
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
		// See PRODUCTION_PLAN.md Step 46: Treat as hard constraint
		// Task cannot start before material arrives on site
		if materialConstraints != nil {
			if matDate, ok := materialConstraints[taskID]; ok {
				if matDate.After(earlyStart) {
					earlyStart = matDate
				}
			}
		}

		earlyFinish = cal.AddWorkDuration(earlyStart, duration)

		schedule[taskID] = TaskSchedule{
			TaskID:      taskID,
			WBSCode:     task.WBSCode,
			EarlyStart:  earlyStart,
			EarlyFinish: earlyFinish,
			// LateStart, LateFinish, TotalFloat, IsCritical set by backward pass
		}
	}

	return schedule, nil
}

// calculateConstraintDate determines the earliest start date for a successor
// based on the predecessor's schedule and the dependency type.
// P1 Correctness Fix: Uses time.Duration and AddWorkDuration for deterministic math.
// See DATA_SPINE_SPEC.md Section 3.4 for dependency types.
func calculateConstraintDate(
	cal Calendar,
	predSchedule TaskSchedule,
	successorDuration time.Duration,
	depType models.DependencyType,
	lagDays int,
) time.Time {
	// Convert lag days to time.Duration for deterministic math
	lag := time.Duration(lagDays) * 24 * time.Hour

	switch depType {
	case models.DependencyTypeFS:
		// Finish-to-Start: Successor starts after predecessor finishes
		// ES(successor) = EF(predecessor) + lag
		return cal.AddWorkDuration(predSchedule.EarlyFinish, lag)

	case models.DependencyTypeSS:
		// Start-to-Start: Successor starts after predecessor starts
		// ES(successor) = ES(predecessor) + lag
		return cal.AddWorkDuration(predSchedule.EarlyStart, lag)

	case models.DependencyTypeFF:
		// Finish-to-Finish: Successor finishes after predecessor finishes
		// EF(successor) = EF(predecessor) + lag
		// ES(successor) = EF(successor) - duration = EF(predecessor) + lag - duration
		return cal.AddWorkDuration(predSchedule.EarlyFinish, lag-successorDuration)

	case models.DependencyTypeSF:
		// Start-to-Finish: Successor finishes after predecessor starts
		// EF(successor) = ES(predecessor) + lag
		// ES(successor) = EF(successor) - duration = ES(predecessor) + lag - duration
		return cal.AddWorkDuration(predSchedule.EarlyStart, lag-successorDuration)

	default:
		// Default to FS behavior
		return cal.AddWorkDuration(predSchedule.EarlyFinish, lag)
	}
}

// GetSuccessors returns all successor task IDs for the given task.
// See BACKEND_SCOPE.md Section 6.3
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

// BackwardPass calculates Late Start (LS), Late Finish (LF), Total Float,
// and identifies the critical path for all tasks.
// Must be called after ForwardPass has populated ES/EF in the schedule.
// Uses the provided Calendar for working day calculations.
// Accepts optional SchedulingConfig; if nil, uses DefaultSchedulingConfig().
// See BACKEND_SCOPE.md Section 6.3
func BackwardPass(g *DependencyGraph, schedule map[uuid.UUID]TaskSchedule, cal Calendar, config *SchedulingConfig) ([]string, error) {
	// Defensive Default: fall back to default config if nil
	if config == nil {
		config = DefaultSchedulingConfig()
	}

	if len(schedule) == 0 {
		return []string{}, nil
	}

	// Find project end date (max EF across all tasks)
	var projectEnd time.Time
	first := true
	for _, sched := range schedule {
		if first || sched.EarlyFinish.After(projectEnd) {
			projectEnd = sched.EarlyFinish
			first = false
		}
	}

	// Get reverse topological order
	sorted, err := TopologicalSort(g)
	if err != nil {
		return nil, fmt.Errorf("backward pass failed: %w", err)
	}

	// Reverse the order for backward pass
	reversed := make([]uuid.UUID, len(sorted))
	for i, id := range sorted {
		reversed[len(sorted)-1-i] = id
	}

	// Process tasks in reverse topological order
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
			// Terminal task: LF = project end date
			lateFinish = projectEnd
		} else {
			// Calculate LF based on minimum constraint from all successors
			firstSuccessor := true

			for _, succID := range successors {
				succSchedule, succExists := schedule[succID]
				if !succExists {
					continue
				}

				dep, depExists := g.GetDependency(taskID, succID)
				if !depExists {
					// Default to FS with 0 lag if edge metadata missing
					dep = models.TaskDependency{
						DependencyType: models.DependencyTypeFS,
						LagDays:        0,
					}
				}

				// Pass predecessor duration for SS/SF constraint calculation
				constraintDate := calculateBackwardConstraintDate(
					cal,
					succSchedule,
					duration, // Predecessor duration for SS/SF
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

		// Calculate total float: LS - ES (or equivalently LF - EF)
		// Float is measured in days
		floatDays := lateStart.Sub(sched.EarlyStart).Hours() / 24

		// Update schedule with backward pass results
		sched.LateStart = lateStart
		sched.LateFinish = lateFinish
		sched.TotalFloat = floatDays
		// Use configurable tolerance-based comparison per PRODUCTION_PLAN Step 33
		sched.IsCritical = math.Abs(floatDays) < config.CriticalPathThreshold

		schedule[taskID] = sched
	}

	// Build critical path (WBS codes of tasks with zero float)
	// Return in topological order (not reversed)
	var criticalPath []string
	for _, taskID := range sorted {
		if sched, exists := schedule[taskID]; exists && sched.IsCritical {
			criticalPath = append(criticalPath, sched.WBSCode)
		}
	}

	return criticalPath, nil
}

// calculateBackwardConstraintDate determines the latest finish date for a predecessor
// based on the successor's schedule and the dependency type.
// For SS and SF types, predecessorDuration is used to convert the LS constraint to LF.
// P1 Correctness Fix: Uses time.Duration and AddWorkDuration for deterministic math.
// See DATA_SPINE_SPEC.md Section 3.4 for dependency types.
func calculateBackwardConstraintDate(
	cal Calendar,
	succSchedule TaskSchedule,
	predecessorDuration time.Duration,
	depType models.DependencyType,
	lagDays int,
) time.Time {
	// Convert lag days to time.Duration for deterministic math
	lag := time.Duration(lagDays) * 24 * time.Hour

	switch depType {
	case models.DependencyTypeFS:
		// Finish-to-Start: Predecessor finishes before successor starts
		// LF(predecessor) = LS(successor) - lag
		return cal.AddWorkDuration(succSchedule.LateStart, -lag)

	case models.DependencyTypeSS:
		// Start-to-Start: Predecessor starts before successor starts
		// Constraint: LS(pred) <= LS(succ) - lag
		// Since caller computes LS = LF - duration, we need:
		// LF - duration = LS(succ) - lag
		// LF = LS(succ) - lag + duration
		return cal.AddWorkDuration(succSchedule.LateStart, -lag+predecessorDuration)

	case models.DependencyTypeFF:
		// Finish-to-Finish: Predecessor finishes before successor finishes
		// LF(predecessor) = LF(successor) - lag
		return cal.AddWorkDuration(succSchedule.LateFinish, -lag)

	case models.DependencyTypeSF:
		// Start-to-Finish: Predecessor starts before successor finishes
		// Constraint: LS(pred) <= LF(succ) - lag
		// Since caller computes LS = LF - duration, we need:
		// LF - duration = LF(succ) - lag
		// LF = LF(succ) - lag + duration
		return cal.AddWorkDuration(succSchedule.LateFinish, -lag+predecessorDuration)

	default:
		// Default to FS behavior
		return cal.AddWorkDuration(succSchedule.LateStart, -lag)
	}
}
