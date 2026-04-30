//go:build integration

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// scriptedPinger is the test substitute for *brain.Client. nil err =
// healthy; non-nil = unhealthy.
type scriptedPinger struct {
	err error
}

func (p *scriptedPinger) Ping(_ context.Context) error { return p.err }

func probesQuietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestReadinessHandler_DBHealthyBrainHealthy(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(pool, &scriptedPinger{}, probesQuietLogger()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Status     string            `json:"status"`
		Components map[string]string `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if body.Components["database"] != "ok" || body.Components["brain"] != "ok" {
		t.Errorf("components = %v, want both ok", body.Components)
	}
}

func TestReadinessHandler_DBHealthyNoBrainPinger(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(pool, nil, probesQuietLogger()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Components map[string]string `json:"components"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Components["brain"] != "skipped" {
		t.Errorf("brain status = %q, want skipped (no pinger wired)", body.Components["brain"])
	}
}

func TestReadinessHandler_BrainUnhealthyFlipsReady(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(pool, &scriptedPinger{err: errors.New("dial: connection refused")}, probesQuietLogger()).
		ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	var body struct {
		Status     string            `json:"status"`
		Components map[string]string `json:"components"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Status != "unhealthy" {
		t.Errorf("status = %q, want unhealthy", body.Status)
	}
	if body.Components["brain"] != "unhealthy" {
		t.Errorf("brain status = %q, want unhealthy", body.Components["brain"])
	}
	if body.Components["database"] != "ok" {
		t.Errorf("database status = %q, want ok", body.Components["database"])
	}
}

func TestReadinessHandler_DBUnhealthyFlipsReady(t *testing.T) {
	// Reuse the testdb container, then close the pool to make Ping fail.
	pool := testdb.NewPool(t)
	pool.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(pool, &scriptedPinger{}, probesQuietLogger()).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	var body struct {
		Components map[string]string `json:"components"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Components["database"] != "unhealthy" {
		t.Errorf("database status = %q, want unhealthy", body.Components["database"])
	}
}
