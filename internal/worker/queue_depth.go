package worker

import (
	"context"
	"log/slog"
	"time"
)

// QueueDepthObserver receives the River backlog gauges. *obs.Metrics satisfies
// it; kept local so the worker package stays decoupled from internal/obs.
type QueueDepthObserver interface {
	SetQueueDepth(n int)
	SetOldestAvailableSeconds(s float64)
}

// ObserveQueueDepth periodically samples the AVAILABLE River job backlog (count
// + the age of the oldest available job) into metrics, so a worker that is
// up-and-scraping but NOT draining is visible — the gap 4b-ii flagged
// (BuildOSWorkerDown only catches a dead/unscraped process; a wedged-but-alive
// worker lets the backlog grow while the job-outcome metrics go stale).
//
// Returns a stop func that cancels the collector; call it on shutdown. Runs one
// sample inline before the ticker so the gauges populate at boot. A query error
// is logged and the gauges keep their last value (never crash the worker).
func (r *Registry) ObserveQueueDepth(ctx context.Context, metrics QueueDepthObserver, interval time.Duration, logger *slog.Logger) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	sample := func() {
		depth, oldest, err := r.queueDepth(ctx)
		if err != nil {
			if ctx.Err() == nil { // don't log the expected error on shutdown
				logger.WarnContext(ctx, "river queue-depth sample failed", "error", err)
			}
			return
		}
		metrics.SetQueueDepth(depth)
		metrics.SetOldestAvailableSeconds(oldest)
	}
	sample()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sample()
			}
		}
	}()
	return cancel
}

// queueDepth returns the count of available (ready-to-run) River jobs and the
// age in seconds of the oldest (0 when the queue is empty). `available` — NOT
// `pending` (which is dependency-gated and would miss the real backlog) — plus
// `scheduled_at <= now()` is the ready-to-run set; index-served by
// river_job_prioritized_fetching_index.
func (r *Registry) queueDepth(ctx context.Context) (depth int, oldestSecs float64, err error) {
	var count int64
	err = r.pool.QueryRow(ctx, `
		SELECT count(*),
		       COALESCE(EXTRACT(EPOCH FROM (now() - min(scheduled_at))), 0)::float8
		FROM river_job
		WHERE state = 'available' AND scheduled_at <= now()`,
	).Scan(&count, &oldestSecs)
	if err != nil {
		return 0, 0, err
	}
	return int(count), oldestSecs, nil
}
