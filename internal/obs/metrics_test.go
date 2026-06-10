package obs

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestObserveAICall_LabelsAndOutcome(t *testing.T) {
	m := NewMetrics()

	// A success and an error call on the same (kind, model) — outcome is
	// derived from err (nil → "success", non-nil → "error").
	m.ObserveAICall("InvoiceExtract", "claude-opus-4", 120*time.Millisecond, nil)
	m.ObserveAICall("InvoiceExtract", "claude-opus-4", 80*time.Millisecond, errors.New("rate limited"))

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	// Counter lines carry the outcome derived from err; labels are
	// alphabetised by Prometheus (kind, model, outcome).
	wantSuccess := `buildos_ai_requests_total{kind="InvoiceExtract",model="claude-opus-4",outcome="success"} 1`
	wantError := `buildos_ai_requests_total{kind="InvoiceExtract",model="claude-opus-4",outcome="error"} 1`
	if !strings.Contains(body, wantSuccess) {
		t.Errorf("missing success counter line %q\ngot:\n%s", wantSuccess, body)
	}
	if !strings.Contains(body, wantError) {
		t.Errorf("missing error counter line %q\ngot:\n%s", wantError, body)
	}
	// The duration histogram recorded both observations (count == 2).
	wantDurCount := `buildos_ai_request_duration_seconds_count{kind="InvoiceExtract",model="claude-opus-4"} 2`
	if !strings.Contains(body, wantDurCount) {
		t.Errorf("missing duration count line %q\ngot:\n%s", wantDurCount, body)
	}
}

func TestStatusClass(t *testing.T) {
	cases := map[int]string{
		0:   "0xx",
		100: "1xx",
		200: "2xx",
		301: "3xx",
		404: "4xx",
		500: "5xx",
		599: "5xx",
	}
	for code, want := range cases {
		if got := statusClass(code); got != want {
			t.Errorf("statusClass(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestNewMetrics_HandlerEmpty200(t *testing.T) {
	// Promhttp serves an empty body until at least one observation
	// lands on each metric (Prometheus emits HELP/TYPE only with at
	// least one sample). The other tests below verify each metric
	// surfaces correctly after an Inc / Observe — this one just
	// confirms the handler responds 200 OK with no observations.
	m := NewMetrics()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHTTPMiddleware_CountsAndLabels(t *testing.T) {
	m := NewMetrics()

	// chi gives us a route pattern via its routing context — wire a
	// trivial chi router so the middleware sees a real pattern.
	r := chi.NewRouter()
	r.Use(m.HTTPMiddleware)
	r.Get("/api/projects/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/projects/abc", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}

	// Scrape /metrics and verify a counter line for the route pattern
	// (not the raw URL — that's the cardinality safety property).
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	wantLine := `buildos_http_requests_total{method="GET",route="/api/projects/{id}",status="2xx"} 3`
	if !strings.Contains(body, wantLine) {
		t.Errorf("expected counter line %q\ngot:\n%s", wantLine, body)
	}
}

func TestHTTPMiddleware_5xxStatusClassed(t *testing.T) {
	m := NewMetrics()
	r := chi.NewRouter()
	r.Use(m.HTTPMiddleware)
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), `status="5xx"`) {
		t.Errorf("expected status=\"5xx\" label; got %s", rec.Body.String())
	}
}

func TestObserveJob_RecordsCounter(t *testing.T) {
	m := NewMetrics()
	m.ObserveJob("daily_briefing", "success")
	m.ObserveJob("daily_briefing", "error")
	m.ObserveJob("pipeline_analytics", "discarded")

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, w := range []string{
		`buildos_river_job_runs_total{kind="daily_briefing",outcome="success"} 1`,
		`buildos_river_job_runs_total{kind="daily_briefing",outcome="error"} 1`,
		`buildos_river_job_runs_total{kind="pipeline_analytics",outcome="discarded"} 1`,
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing line %q in:\n%s", w, body)
		}
	}
}

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

func TestObserveErrorResponse_RecordsByCodeAndStatus(t *testing.T) {
	m := NewMetrics()
	m.ObserveErrorResponse("SETUP_INCOMPLETE", "4xx")
	m.ObserveErrorResponse("SETUP_INCOMPLETE", "4xx")
	m.ObserveErrorResponse("AI_CIRCUIT_OPEN", "5xx")

	body := scrape(t, m)
	for _, w := range []string{
		`buildos_http_error_responses_total{code="SETUP_INCOMPLETE",status="4xx"} 2`,
		`buildos_http_error_responses_total{code="AI_CIRCUIT_OPEN",status="5xx"} 1`,
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing line %q in:\n%s", w, body)
		}
	}
}

func TestRegisterPoolGauges_ScrapesStat(t *testing.T) {
	m := NewMetrics()
	m.RegisterPoolGauges(func() (a, i, t, mx int32) { return 3, 7, 10, 25 })

	body := scrape(t, m)
	for _, w := range []string{
		"buildos_db_pool_acquired_conns 3",
		"buildos_db_pool_idle_conns 7",
		"buildos_db_pool_total_conns 10",
		"buildos_db_pool_max_conns 25",
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing line %q in:\n%s", w, body)
		}
	}
}

func TestRiverQueueGauges(t *testing.T) {
	m := NewMetrics()
	m.SetQueueDepth(4)
	m.SetOldestAvailableSeconds(42)

	body := scrape(t, m)
	for _, w := range []string{
		"buildos_river_queue_depth 4",
		"buildos_river_oldest_available_seconds 42",
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing line %q in:\n%s", w, body)
		}
	}
}
