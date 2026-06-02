package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
)

// These tests exercise ScheduleService's early-return validation paths
// and its pure helpers. Both ListProjectTasks and UpdateTask validate
// status / percent_complete BEFORE pgx.BeginTxFunc touches the pool, so
// a service built with a nil pool/store is safe here; the CPM
// round-trip (recalc + persistence + River enqueue) needs a real
// Postgres and lives in the integration suite. ganttFromTasks and
// isValidTaskStatus are pure and need no service at all.

func newScheduleValidationService() *ScheduleService {
	return NewScheduleService(nil, nil, nil, nil)
}

func TestListProjectTasks_RejectsUnknownStatus(t *testing.T) {
	svc := newScheduleValidationService()
	_, err := svc.ListProjectTasks(context.Background(), ListProjectTasksInput{
		ProjectID: uuid.New(),
		OrgID:     uuid.New(),
		Status:    "bogus",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateTask_Validation(t *testing.T) {
	svc := newScheduleValidationService()
	below, above := -1, 101
	bad := "bogus"
	cases := []struct {
		name string
		in   UpdateTaskInput
	}{
		{"percent below floor", UpdateTaskInput{TaskID: uuid.New(), ProjectID: uuid.New(), OrgID: uuid.New(), PercentComplete: &below}},
		{"percent above ceiling", UpdateTaskInput{TaskID: uuid.New(), ProjectID: uuid.New(), OrgID: uuid.New(), PercentComplete: &above}},
		{"unknown status", UpdateTaskInput{TaskID: uuid.New(), ProjectID: uuid.New(), OrgID: uuid.New(), Status: &bad}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.UpdateTask(context.Background(), c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestIsValidTaskStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"pending", true},
		{"in_progress", true},
		{"completed", true},
		{"", false},
		{"PENDING", false}, // case-sensitive: matches the schema CHECK exactly
		{"done", false},
		{"in progress", false}, // space, not underscore
	}
	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			if got := isValidTaskStatus(c.status); got != c.want {
				t.Errorf("isValidTaskStatus(%q) = %v, want %v", c.status, got, c.want)
			}
		})
	}
}

func TestGanttFromTasks_NeverComputed(t *testing.T) {
	// A project that has never been recalculated: no CPM results, so
	// zero ProjectEnd, empty (non-nil) critical path. The frontend
	// detects this and prompts the user to run /recalculate.
	tasks := []models.ProjectTask{
		{ID: uuid.New(), WBSCode: "1.0", IsCritical: false},
		{ID: uuid.New(), WBSCode: "2.0", IsCritical: false},
	}
	view := ganttFromTasks(tasks)
	if view.CriticalPath == nil {
		t.Fatal("CriticalPath must be non-nil (stable [] for JSON), got nil")
	}
	if len(view.CriticalPath) != 0 {
		t.Errorf("CriticalPath = %v, want empty", view.CriticalPath)
	}
	if !view.ProjectEnd.IsZero() {
		t.Errorf("ProjectEnd = %v, want zero", view.ProjectEnd)
	}
	if len(view.Tasks) != 2 {
		t.Errorf("Tasks len = %d, want 2", len(view.Tasks))
	}
}

func TestGanttFromTasks_CriticalPathInInputOrder(t *testing.T) {
	t1 := uuid.New()
	t2 := uuid.New()
	t3 := uuid.New()
	// Input is pre-sorted by WBS; critical tasks must come out in that
	// same order (the helper appends as it walks, no re-sort).
	tasks := []models.ProjectTask{
		{ID: t1, WBSCode: "1.0", IsCritical: true},
		{ID: t2, WBSCode: "2.0", IsCritical: false},
		{ID: t3, WBSCode: "3.0", IsCritical: true},
	}
	view := ganttFromTasks(tasks)
	want := []uuid.UUID{t1, t3}
	if len(view.CriticalPath) != len(want) {
		t.Fatalf("CriticalPath = %v, want %v", view.CriticalPath, want)
	}
	for i := range want {
		if view.CriticalPath[i] != want[i] {
			t.Errorf("CriticalPath[%d] = %v, want %v", i, view.CriticalPath[i], want[i])
		}
	}
}

func TestGanttFromTasks_ProjectEndIsMaxEarlyFinish(t *testing.T) {
	early := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	// Out-of-order finishes + one task with a nil EarlyFinish (never
	// computed for that row) — must be skipped for the max, not panic.
	tasks := []models.ProjectTask{
		{ID: uuid.New(), WBSCode: "1.0", EarlyFinish: &mid},
		{ID: uuid.New(), WBSCode: "2.0", EarlyFinish: &late},
		{ID: uuid.New(), WBSCode: "3.0", EarlyFinish: &early},
		{ID: uuid.New(), WBSCode: "4.0", EarlyFinish: nil},
	}
	view := ganttFromTasks(tasks)
	if !view.ProjectEnd.Equal(late) {
		t.Errorf("ProjectEnd = %v, want %v (max early_finish)", view.ProjectEnd, late)
	}
}
