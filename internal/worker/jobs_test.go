package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// TestJobArgsKind pins the River job-kind strings. The kind is persisted in
// the river_job table and matched at dispatch, so renaming one would orphan
// every already-queued job of that kind — this is a wire-protocol contract.
func TestJobArgsKind(t *testing.T) {
	tests := []struct {
		args river.JobArgs
		want string
	}{
		{DailyBriefingArgs{}, "daily_briefing"},
		{ProcurementCheckArgs{}, "procurement_check"},
		{HydrateProjectArgs{}, "hydrate_project"},
		{CorporateRollupArgs{}, "corporate_rollup"},
		{CertificationAlertsArgs{}, "certification_alerts"},
		{MaintenanceRemindersArgs{}, "maintenance_reminders"},
		{FieldNotificationRetryArgs{}, "field_notification_retry"},
		{DelayCascadeArgs{}, "delay_cascade"},
		{PipelineAnalyticsArgs{}, "pipeline_analytics"},
		{PermitIssuedTransitionArgs{}, "permit_issued_transition"},
	}
	seen := make(map[string]bool, len(tests))
	for _, tt := range tests {
		if got := tt.args.Kind(); got != tt.want {
			t.Errorf("%T.Kind() = %q, want %q", tt.args, got, tt.want)
		}
		if seen[tt.want] {
			t.Errorf("duplicate job kind %q", tt.want)
		}
		seen[tt.want] = true
	}
}

func TestFieldNotificationRetryArgs_InsertOpts(t *testing.T) {
	if got := (FieldNotificationRetryArgs{}).InsertOpts().MaxAttempts; got != 6 {
		t.Errorf("MaxAttempts = %d, want 6", got)
	}
}

func TestFieldNotificationRetryWorker_NextRetry(t *testing.T) {
	w := &FieldNotificationRetryWorker{}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 30 * time.Second},
		{1, 60 * time.Second},
		{2, 2 * time.Minute},
		{3, 5 * time.Minute},
		{4, 30 * time.Minute},
		{5, 60 * time.Minute},
		{6, 60 * time.Minute},   // clamps to the last delay
		{100, 60 * time.Minute}, // far past the schedule still clamps
	}
	for _, tt := range tests {
		job := &river.Job[FieldNotificationRetryArgs]{JobRow: &rivertype.JobRow{Attempt: tt.attempt}}
		before := time.Now()
		got := w.NextRetry(job)
		delay := got.Sub(before)
		// got == time.Now()+want where the inner Now() is >= before, so the
		// observed delay is in [want, want+slack].
		if delay < tt.want || delay > tt.want+time.Second {
			t.Errorf("attempt %d: delay = %v, want ~%v", tt.attempt, delay, tt.want)
		}
	}
}

func TestWorkerConstructors_PanicOnNilDependency(t *testing.T) {
	t.Run("ProcurementCheckWorker", func(t *testing.T) {
		assertPanics(t, func() { NewProcurementCheckWorker(nil) })
	})
	t.Run("CorporateRollupWorker", func(t *testing.T) {
		assertPanics(t, func() { NewCorporateRollupWorker(nil) })
	})
	t.Run("FieldNotificationRetryWorker", func(t *testing.T) {
		assertPanics(t, func() { NewFieldNotificationRetryWorker(nil) })
	})
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected panic, got none")
		}
	}()
	fn()
}

// fakeChecker / fakeRunner / fakeDeliverer are the service-layer dependency
// doubles. recording whether they were called lets the worker tests prove the
// delegation happened.
type fakeChecker struct {
	rows   int64
	err    error
	called bool
}

func (f *fakeChecker) RecomputeStatuses(context.Context) (int64, error) {
	f.called = true
	return f.rows, f.err
}

type fakeRunner struct {
	rows   int64
	err    error
	called bool
}

func (f *fakeRunner) RunCorporateRollup(context.Context) (int64, error) {
	f.called = true
	return f.rows, f.err
}

type fakeDeliverer struct {
	err        error
	gotAttempt int
	gotArgs    FieldNotificationRetryArgs
	called     bool
}

func (f *fakeDeliverer) DeliverNotification(_ context.Context, attempt int, args FieldNotificationRetryArgs) error {
	f.called = true
	f.gotAttempt = attempt
	f.gotArgs = args
	return f.err
}

func TestProcurementCheckWorker_Work(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fc := &fakeChecker{rows: 4}
		w := NewProcurementCheckWorker(fc)
		if err := w.Work(context.Background(), &river.Job[ProcurementCheckArgs]{}); err != nil {
			t.Fatalf("Work() = %v, want nil", err)
		}
		if !fc.called {
			t.Error("RecomputeStatuses was not called")
		}
	})
	t.Run("wraps checker error", func(t *testing.T) {
		sentinel := errors.New("db down")
		w := NewProcurementCheckWorker(&fakeChecker{err: sentinel})
		err := w.Work(context.Background(), &river.Job[ProcurementCheckArgs]{})
		if !errors.Is(err, sentinel) {
			t.Errorf("Work() = %v, want wrapped %v", err, sentinel)
		}
	})
}

func TestCorporateRollupWorker_Work(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fr := &fakeRunner{rows: 2}
		w := NewCorporateRollupWorker(fr)
		if err := w.Work(context.Background(), &river.Job[CorporateRollupArgs]{}); err != nil {
			t.Fatalf("Work() = %v, want nil", err)
		}
		if !fr.called {
			t.Error("RunCorporateRollup was not called")
		}
	})
	t.Run("wraps runner error", func(t *testing.T) {
		sentinel := errors.New("rollup failed")
		w := NewCorporateRollupWorker(&fakeRunner{err: sentinel})
		err := w.Work(context.Background(), &river.Job[CorporateRollupArgs]{})
		if !errors.Is(err, sentinel) {
			t.Errorf("Work() = %v, want wrapped %v", err, sentinel)
		}
	})
}

func TestFieldNotificationRetryWorker_Work(t *testing.T) {
	t.Run("delegates with attempt and args", func(t *testing.T) {
		fd := &fakeDeliverer{}
		w := NewFieldNotificationRetryWorker(fd)
		args := FieldNotificationRetryArgs{
			UserID:           uuid.New(),
			NotificationType: "sms",
			Payload:          json.RawMessage(`{"to":"+15551234567","body":"hi"}`),
		}
		job := &river.Job[FieldNotificationRetryArgs]{JobRow: &rivertype.JobRow{Attempt: 3}, Args: args}
		if err := w.Work(context.Background(), job); err != nil {
			t.Fatalf("Work() = %v, want nil", err)
		}
		if !fd.called || fd.gotAttempt != 3 || fd.gotArgs.NotificationType != "sms" {
			t.Errorf("deliverer got called=%v attempt=%d type=%q, want true/3/sms", fd.called, fd.gotAttempt, fd.gotArgs.NotificationType)
		}
	})
	t.Run("propagates deliverer error", func(t *testing.T) {
		sentinel := errors.New("send failed")
		w := NewFieldNotificationRetryWorker(&fakeDeliverer{err: sentinel})
		job := &river.Job[FieldNotificationRetryArgs]{JobRow: &rivertype.JobRow{Attempt: 0}}
		if err := w.Work(context.Background(), job); !errors.Is(err, sentinel) {
			t.Errorf("Work() = %v, want %v", err, sentinel)
		}
	})
}

// TestPlaceholderWorkers_Work covers the not-yet-implemented workers that log
// and return nil — they must never error, since River would otherwise retry a
// no-op forever.
func TestPlaceholderWorkers_Work(t *testing.T) {
	ctx := context.Background()
	checks := []struct {
		name string
		run  func() error
	}{
		{"daily_briefing", func() error {
			return (&DailyBriefingWorker{}).Work(ctx, &river.Job[DailyBriefingArgs]{})
		}},
		{"hydrate_project", func() error {
			return (&HydrateProjectWorker{}).Work(ctx, &river.Job[HydrateProjectArgs]{Args: HydrateProjectArgs{ProjectID: uuid.New()}})
		}},
		{"certification_alerts", func() error {
			return (&CertificationAlertsWorker{}).Work(ctx, &river.Job[CertificationAlertsArgs]{})
		}},
		{"maintenance_reminders", func() error {
			return (&MaintenanceRemindersWorker{}).Work(ctx, &river.Job[MaintenanceRemindersArgs]{})
		}},
		{"delay_cascade", func() error {
			return (&DelayCascadeWorker{}).Work(ctx, &river.Job[DelayCascadeArgs]{Args: DelayCascadeArgs{ProjectID: uuid.New()}})
		}},
		{"pipeline_analytics", func() error {
			return (&PipelineAnalyticsWorker{}).Work(ctx, &river.Job[PipelineAnalyticsArgs]{})
		}},
		{"permit_issued_transition", func() error {
			return (&PermitIssuedTransitionWorker{}).Work(ctx, &river.Job[PermitIssuedTransitionArgs]{Args: PermitIssuedTransitionArgs{ProspectID: uuid.New()}})
		}},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(); err != nil {
				t.Errorf("%s placeholder Work() = %v, want nil", c.name, err)
			}
		})
	}
}
