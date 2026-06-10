package obs

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the BuildOS Prometheus surface. One instance per process
// — held by the registry that mounts /metrics. Counters and histograms
// are public fields so callers can observe directly without going
// through getter wrappers.
type Metrics struct {
	registry *prometheus.Registry

	// HTTPRequests counts each HTTP request by route, method, and
	// status class ("2xx" / "4xx" / "5xx"). Cardinality is bounded by
	// the chi route pattern (not the raw URL), so a /projects/{id}
	// path produces one label set rather than one per project id.
	HTTPRequests *prometheus.CounterVec

	// HTTPDuration tracks request latency in seconds. Buckets favor
	// the typical web shape (1ms-2s); slow tails land in the +Inf
	// bucket where they should be visible in dashboards.
	HTTPDuration *prometheus.HistogramVec

	// AIRequests counts every native Anthropic call by task kind,
	// model, and outcome ("success" / "error").
	AIRequests *prometheus.CounterVec

	// AIDuration tracks native AI call latency. Buckets shifted
	// upward since LLM calls are typically 5-30s.
	AIDuration *prometheus.HistogramVec

	// JobRuns counts every River job execution by kind and
	// outcome ("success" / "error" / "discarded").
	JobRuns *prometheus.CounterVec

	// ErrorResponses counts JSON error responses by {code, status-class}.
	// `code` is the bounded application error code (RATE_LIMITED,
	// SETUP_INCOMPLETE, AI_CIRCUIT_OPEN, …) — finer than the HTTP status the
	// request middleware sees. (Phase 4b-iii.)
	ErrorResponses *prometheus.CounterVec

	// RiverQueueDepth is the count of AVAILABLE (ready-to-run) River jobs, and
	// RiverOldestAvailableSecs the age of the oldest available job. Together
	// they detect a worker that is up-and-scraping but not DRAINING (the gap
	// 4b-ii flagged). Set periodically by the worker. (Phase 4b-iii.)
	RiverQueueDepth          prometheus.Gauge
	RiverOldestAvailableSecs prometheus.Gauge
}

// NewMetrics constructs a Metrics with all collectors registered. The
// returned registry is what /metrics serves; pass it to
// MetricsHandler when wiring the HTTP route.
//
// Each constructor call creates a fresh registry — do NOT call this
// twice in production (you'd register the same metric names twice and
// panic on the second). cmd/server and cmd/worker each create their
// own; that's fine because they're separate processes.
func NewMetrics() *Metrics {
	r := prometheus.NewRegistry()

	httpReqs := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "buildos",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Count of HTTP requests by route, method, and status class.",
		},
		[]string{"route", "method", "status"},
	)
	httpDur := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "buildos",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds, by route + method.",
			// Default buckets cover sub-millisecond to ~10s; matches
			// Prometheus's recommended exponential set.
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route", "method"},
	)
	aiReqs := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "buildos",
			Subsystem: "ai",
			Name:      "requests_total",
			Help:      "Count of native Anthropic calls by task kind, model, and outcome.",
		},
		[]string{"kind", "model", "outcome"},
	)
	aiDur := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "buildos",
			Subsystem: "ai",
			Name:      "request_duration_seconds",
			Help:      "Native Anthropic call duration in seconds, by task kind + model.",
			// LLM calls dominate; widen the bucket set toward 60s.
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60},
		},
		[]string{"kind", "model"},
	)
	jobRuns := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "buildos",
			Subsystem: "river",
			Name:      "job_runs_total",
			Help:      "River job executions by kind and outcome.",
		},
		[]string{"kind", "outcome"},
	)

	errResps := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "buildos",
			Subsystem: "http",
			Name:      "error_responses_total",
			Help:      "JSON error responses by application error code and status class.",
		},
		[]string{"code", "status"},
	)
	queueDepth := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "buildos",
		Subsystem: "river",
		Name:      "queue_depth",
		Help:      "Count of available (ready-to-run) River jobs.",
	})
	oldestAvail := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "buildos",
		Subsystem: "river",
		Name:      "oldest_available_seconds",
		Help:      "Age in seconds of the oldest available River job (0 when the queue is empty).",
	})

	r.MustRegister(httpReqs, httpDur, aiReqs, aiDur, jobRuns, errResps, queueDepth, oldestAvail)

	return &Metrics{
		registry:                 r,
		HTTPRequests:             httpReqs,
		HTTPDuration:             httpDur,
		AIRequests:               aiReqs,
		AIDuration:               aiDur,
		JobRuns:                  jobRuns,
		ErrorResponses:           errResps,
		RiverQueueDepth:          queueDepth,
		RiverOldestAvailableSecs: oldestAvail,
	}
}

// Handler returns the /metrics HTTP handler bound to this Metrics's
// registry. Mount it under no auth — Prometheus scrape conventions.
// Tighten via network policy / IP allowlist at the LB layer if you
// don't want it open.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// statusClass groups numeric status codes into the buckets ops cares
// about: "2xx", "3xx", "4xx", "5xx", or "0xx" (a transport error
// before any response was received).
func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	case code > 0:
		return "1xx"
	default:
		return "0xx"
	}
}

// HTTPMiddleware records request count + duration metrics. Mount it
// near the top of the middleware stack — after RequestID and Recoverer
// so panics still get counted, but before route handlers so the chi
// route pattern is available.
//
// Route label uses chi's matched route pattern (e.g.
// "/api/v1/projects/{projectID}") rather than the raw URL — keeps
// label cardinality bounded.
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)

		// Pull the chi route pattern AFTER the handler runs — that's
		// when chi has populated the routing context. Unmatched paths
		// (chi NotFound — API typos, scanner probes, and every SPA
		// deep link served by the catch-all) collapse into one sentinel
		// label: the raw URL would make label cardinality unbounded.
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "(unmatched)"
		}
		method := r.Method
		status := statusClass(ww.status)

		m.HTTPRequests.WithLabelValues(route, method, status).Inc()
		m.HTTPDuration.WithLabelValues(route, method).
			Observe(time.Since(start).Seconds())
	})
}

// statusCapturingWriter records the response status so the middleware
// can label metrics. Default WriteHeader-to-200 if the handler never
// calls it (matching net/http semantics).
type statusCapturingWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

// ObserveAICall records one native Anthropic call. It satisfies
// ai.MetricsObserver. outcome is derived from err: "success" when nil,
// "error" otherwise. Wired into the AI client via ai.Config.Metrics.
func (m *Metrics) ObserveAICall(kind, model string, dur time.Duration, err error) {
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	m.AIRequests.WithLabelValues(kind, model, outcome).Inc()
	m.AIDuration.WithLabelValues(kind, model).Observe(dur.Seconds())
}

// ObserveJob records one River job execution. Call from a worker
// wrapper or River middleware. outcome should be one of
// "success", "error", "discarded".
func (m *Metrics) ObserveJob(kind, outcome string) {
	m.JobRuns.WithLabelValues(kind, outcome).Inc()
}

// ObserveErrorResponse records one JSON error response. Wired into the API error
// writers via the router. (Phase 4b-iii.)
func (m *Metrics) ObserveErrorResponse(code, status string) {
	m.ErrorResponses.WithLabelValues(code, status).Inc()
}

// SetQueueDepth / SetOldestAvailableSeconds update the River backlog gauges.
// Called by the worker's queue-depth collector. (Phase 4b-iii.)
func (m *Metrics) SetQueueDepth(n int) { m.RiverQueueDepth.Set(float64(n)) }

func (m *Metrics) SetOldestAvailableSeconds(s float64) { m.RiverOldestAvailableSecs.Set(s) }

// RegisterPoolGauges registers buildos_db_pool_* GaugeFuncs backed by stat,
// which returns a pgxpool.Stat snapshot's (acquired, idle, total, max) conn
// counts. GaugeFunc is scrape-time + concurrency-safe — no goroutine/ticker
// needed. Pass a closure (e.g. over pool.Stat()) so internal/obs stays free of a
// pgxpool import. Call once at startup. (Phase 4b-iii.)
func (m *Metrics) RegisterPoolGauges(stat func() (acquired, idle, total, maxConns int32)) {
	gauge := func(name, help string, pick func(a, i, t, mx int32) int32) prometheus.GaugeFunc {
		return prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "buildos", Subsystem: "db_pool", Name: name, Help: help,
		}, func() float64 {
			a, i, t, mx := stat()
			return float64(pick(a, i, t, mx))
		})
	}
	m.registry.MustRegister(
		gauge("acquired_conns", "Currently in-use pgx connections.", func(a, _, _, _ int32) int32 { return a }),
		gauge("idle_conns", "Idle pgx connections in the pool.", func(_, i, _, _ int32) int32 { return i }),
		gauge("total_conns", "Total (idle+in-use) pgx connections.", func(_, _, t, _ int32) int32 { return t }),
		gauge("max_conns", "Configured max pool size.", func(_, _, _, mx int32) int32 { return mx }),
	)
}
