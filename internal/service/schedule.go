package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/futurebuildai/buildos/internal/physics"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

// ScheduleService orchestrates CPM scheduling with transactional guarantees.
// Implements the pattern from ARCHITECTURE.md Section 4:
// CPM calculation + schedule persistence + River job insertion in a single transaction.
type ScheduleService struct {
	pool          *pgxpool.Pool
	scheduleStore *store.ScheduleStore
	riverClient   *river.Client[pgx.Tx]
}

// NewScheduleService creates a new ScheduleService.
func NewScheduleService(pool *pgxpool.Pool, scheduleStore *store.ScheduleStore, riverClient *river.Client[pgx.Tx]) *ScheduleService {
	return &ScheduleService{
		pool:          pool,
		scheduleStore: scheduleStore,
		riverClient:   riverClient,
	}
}

// RecalculateSchedule runs the CPM physics engine and persists results.
// All operations execute within a single pgx.BeginTxFunc transaction to prevent
// "phantom jobs" that reference data not yet committed.
func (s *ScheduleService) RecalculateSchedule(ctx context.Context, projectID uuid.UUID) (*physics.CPMResult, time.Duration, error) {
	var cpmResult *physics.CPMResult
	var computeTime time.Duration

	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// 1. Load tasks and dependencies from DB
		tasks, err := s.scheduleStore.GetProjectTasks(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load tasks: %w", err)
		}
		if len(tasks) == 0 {
			return fmt.Errorf("no tasks found for project %s", projectID)
		}

		deps, err := s.scheduleStore.GetProjectDependencies(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load dependencies: %w", err)
		}

		projectStart, err := s.scheduleStore.GetProjectStartDate(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load project start: %w", err)
		}

		// 2. Run CPM ForwardPass + BackwardPass
		computeStart := time.Now()

		graph := physics.BuildDependencyGraph(tasks, deps)
		cal := &physics.StandardCalendar{}

		schedule, err := physics.ForwardPass(graph, projectStart, cal, nil)
		if err != nil {
			return fmt.Errorf("forward pass: %w", err)
		}

		criticalPath, err := physics.BackwardPass(graph, schedule, cal, nil)
		if err != nil {
			return fmt.Errorf("backward pass: %w", err)
		}

		computeTime = time.Since(computeStart)

		// Find project end date
		var projectEnd time.Time
		first := true
		for _, sched := range schedule {
			if first || sched.EarlyFinish.After(projectEnd) {
				projectEnd = sched.EarlyFinish
				first = false
			}
		}

		// Build result
		taskSchedules := make([]physics.TaskSchedule, 0, len(schedule))
		for _, ts := range schedule {
			taskSchedules = append(taskSchedules, ts)
		}

		cpmResult = &physics.CPMResult{
			Tasks:        taskSchedules,
			CriticalPath: criticalPath,
			ProjectEnd:   projectEnd,
		}

		// 3. Persist schedule results
		scheduleResults := make(map[uuid.UUID]store.ScheduleResult, len(schedule))
		for taskID, ts := range schedule {
			scheduleResults[taskID] = store.ScheduleResult{
				EarlyStart:  ts.EarlyStart,
				EarlyFinish: ts.EarlyFinish,
				LateStart:   ts.LateStart,
				LateFinish:  ts.LateFinish,
				TotalFloat:  int(math.Round(ts.TotalFloat)),
				IsCritical:  ts.IsCritical,
			}
		}

		if err := s.scheduleStore.UpdateSchedule(ctx, tx, tasks, scheduleResults); err != nil {
			return fmt.Errorf("persist schedule: %w", err)
		}

		// 4. Enqueue delay cascade if critical path changed (SAME TRANSACTION)
		if len(criticalPath) > 0 {
			cpmResult.CriticalPathChanged = true
			_, err := s.riverClient.InsertTx(ctx, tx, &worker.DelayCascadeArgs{
				ProjectID: projectID,
			}, nil)
			if err != nil {
				return fmt.Errorf("enqueue delay cascade: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, 0, err
	}

	return cpmResult, computeTime, nil
}
