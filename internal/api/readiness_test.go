package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// deadPool builds a pgxpool aimed at a refused address (127.0.0.1:1) so the
// first Ping fails fast with "connection refused". pgxpool.New connects
// lazily — it returns without dialing — so construction is cheap and offline,
// and the readiness probe's own 2s deadline bounds the failed dial. This lets
// the DB-down leg of readinessHandler be exercised as a unit test (no Docker).
func deadPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestReadinessHandler_DBDown covers the unhealthy leg: when the only hard
// dependency (Postgres) can't be pinged, the probe must return 503 with a
// machine-readable body the load balancer / orchestrator can parse, so a
// degraded instance is pulled from rotation rather than served traffic.
func TestReadinessHandler_DBDown(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := readinessHandler(deadPool(t), logger)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness with DB down = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"unhealthy"`) {
		t.Errorf("body = %q, want status unhealthy", body)
	}
	if !strings.Contains(body, `"database":"unhealthy"`) {
		t.Errorf("body = %q, want database component unhealthy", body)
	}
}

// TestReadinessHandler_DBDown_NilLogger covers the nil-logger branch: the probe
// is wired before observability in some boot orders, so it must not panic when
// no logger is supplied — it just skips the warn log and still emits the 503.
func TestReadinessHandler_DBDown_NilLogger(t *testing.T) {
	h := readinessHandler(deadPool(t), nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness (nil logger) = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), `"database":"unhealthy"`) {
		t.Errorf("body = %q, want database component unhealthy", rec.Body.String())
	}
}
