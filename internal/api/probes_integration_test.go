//go:build integration

package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futurebuildai/buildos/internal/testdb"
)

func probesQuietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func decodeReady(t *testing.T, rec *httptest.ResponseRecorder) (status string, components map[string]string) {
	t.Helper()
	var body struct {
		Status     string            `json:"status"`
		Components map[string]string `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return body.Status, body.Components
}

func TestReadinessHandler_DBHealthy(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	readinessHandler(pool, probesQuietLogger()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	status, comps := decodeReady(t, rec)
	if status != "ok" {
		t.Errorf("status = %q, want ok", status)
	}
	if comps["database"] != "ok" {
		t.Errorf("database = %q, want ok", comps["database"])
	}
}

func TestReadinessHandler_DBUnhealthyFlipsReady(t *testing.T) {
	pool := testdb.NewPool(t)
	pool.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(pool, probesQuietLogger()).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	status, comps := decodeReady(t, rec)
	if status != "unhealthy" {
		t.Errorf("status = %q, want unhealthy", status)
	}
	if comps["database"] != "unhealthy" {
		t.Errorf("database = %q, want unhealthy", comps["database"])
	}
}
