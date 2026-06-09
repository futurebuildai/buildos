package worker

import (
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// JobMetricsObserver records a River job's terminal outcome. *obs.Metrics
// satisfies it via ObserveJob(kind, outcome) — kept as a local interface so the
// worker package stays decoupled from internal/obs.
type JobMetricsObserver interface {
	ObserveJob(kind, outcome string)
}

// jobOutcome maps a River subscription event's TERMINAL job state to the obs
// outcome vocabulary ("success" / "error" / "discarded"). record is false for
// events that are not a terminal run outcome (e.g. a snooze) or are malformed,
// so the caller skips them.
//
//   - completed → success
//   - retryable → error      (a failed ATTEMPT that River will retry — so the
//     error counter is attempt-level, matching how the AI metric counts)
//   - discarded → discarded  (retries exhausted; the job's effect won't happen)
//   - cancelled → discarded  (terminal, never completed)
func jobOutcome(ev *river.Event) (outcome string, record bool) {
	if ev == nil || ev.Job == nil {
		return "", false
	}
	switch ev.Job.State {
	case rivertype.JobStateCompleted:
		return "success", true
	case rivertype.JobStateRetryable:
		return "error", true
	case rivertype.JobStateDiscarded, rivertype.JobStateCancelled:
		return "discarded", true
	default:
		return "", false
	}
}

// ObserveJobMetrics subscribes to job terminal events and records each via
// metrics.ObserveJob on a background goroutine, so buildos_river_job_runs_total
// reflects real job outcomes (success / error / discarded by kind). Returns a
// stop func that cancels the subscription; call it on shutdown. The goroutine
// exits when the subscription channel closes — on stop() OR when the River
// client stops — so it can never leak past the client's lifetime. Safe to call
// before or after Start.
//
// River's subscription channel is buffered + non-blocking (events can drop under
// extreme sustained load); ObserveJob is a cheap counter increment done inline,
// so drops are not a practical concern at a single-tenant fork's job volume.
func (r *Registry) ObserveJobMetrics(metrics JobMetricsObserver, logger *slog.Logger) (stop func()) {
	events, cancel := r.Client.Subscribe(
		river.EventKindJobCompleted,
		river.EventKindJobFailed,
		river.EventKindJobCancelled,
	)
	go drainJobEvents(events, metrics, logger)
	return cancel
}

// drainJobEvents records terminal job outcomes from a River subscription channel
// until it closes. Extracted from ObserveJobMetrics so the drain loop is unit-
// testable with a synthetic channel (no River client / DB needed).
func drainJobEvents(events <-chan *river.Event, metrics JobMetricsObserver, logger *slog.Logger) {
	for ev := range events {
		if outcome, ok := jobOutcome(ev); ok {
			metrics.ObserveJob(ev.Job.Kind, outcome)
		}
	}
	logger.Debug("river job-metrics subscription closed")
}
