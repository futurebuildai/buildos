package worker

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func evWithState(s rivertype.JobState) *river.Event {
	return &river.Event{Job: &rivertype.JobRow{Kind: "daily_briefing", State: s}}
}

func TestJobOutcome(t *testing.T) {
	tests := []struct {
		name    string
		ev      *river.Event
		want    string
		wantRec bool
	}{
		{"completed→success", evWithState(rivertype.JobStateCompleted), "success", true},
		{"retryable→error (a failed attempt that will retry)", evWithState(rivertype.JobStateRetryable), "error", true},
		{"discarded→discarded (retries exhausted)", evWithState(rivertype.JobStateDiscarded), "discarded", true},
		{"cancelled→discarded (terminal, never completed)", evWithState(rivertype.JobStateCancelled), "discarded", true},
		// Non-terminal / uninteresting states are skipped (not recorded).
		{"running→skip", evWithState(rivertype.JobStateRunning), "", false},
		{"scheduled→skip", evWithState(rivertype.JobStateScheduled), "", false},
		{"available→skip", evWithState(rivertype.JobStateAvailable), "", false},
		{"pending→skip", evWithState(rivertype.JobStatePending), "", false},
		{"nil event→skip", nil, "", false},
		{"nil job→skip", &river.Event{Job: nil}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rec := jobOutcome(tt.ev)
			if got != tt.want || rec != tt.wantRec {
				t.Errorf("jobOutcome() = (%q, %v), want (%q, %v)", got, rec, tt.want, tt.wantRec)
			}
		})
	}
}

// recordingObserver is a thread-safe JobMetricsObserver for tests (ObserveJob is
// called from the drain goroutine).
type recordingObserver struct {
	mu    sync.Mutex
	calls [][2]string // {kind, outcome}
}

func (o *recordingObserver) ObserveJob(kind, outcome string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, [2]string{kind, outcome})
}

func ev(kind string, s rivertype.JobState) *river.Event {
	return &river.Event{Job: &rivertype.JobRow{Kind: kind, State: s}}
}

// TestDrainJobEvents proves the drain loop forwards each terminal event to
// ObserveJob with the right kind+outcome, skips non-terminal events, and exits
// when the channel closes.
func TestDrainJobEvents(t *testing.T) {
	ch := make(chan *river.Event, 8)
	ch <- ev("daily_briefing", rivertype.JobStateCompleted)        // → success
	ch <- ev("delay_cascade", rivertype.JobStateRetryable)         // → error
	ch <- ev("field_notification_retry", rivertype.JobStateRunning) // skipped (non-terminal)
	ch <- ev("corporate_rollup", rivertype.JobStateDiscarded)      // → discarded
	close(ch)

	obs := &recordingObserver{}
	done := make(chan struct{})
	go func() {
		drainJobEvents(ch, obs, slog.Default())
		close(done)
	}()
	<-done // drain returns when the channel closes — no leak past the client

	want := [][2]string{
		{"daily_briefing", "success"},
		{"delay_cascade", "error"},
		{"corporate_rollup", "discarded"},
	}
	if len(obs.calls) != len(want) {
		t.Fatalf("ObserveJob called %d times, want %d: %v", len(obs.calls), len(want), obs.calls)
	}
	for i, w := range want {
		if obs.calls[i] != w {
			t.Errorf("call %d = %v, want %v", i, obs.calls[i], w)
		}
	}
}
