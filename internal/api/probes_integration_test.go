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
	"time"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// scriptedPinger is the test substitute for *brain.Client. nil err =
// healthy; non-nil = unhealthy.
type scriptedPinger struct {
	err error
}

func (p *scriptedPinger) Ping(_ context.Context) error { return p.err }

// scriptedJWKS lets us script CacheStatus + CacheTTL outputs.
type scriptedJWKS struct {
	keyCount int
	age      time.Duration
	ttl      time.Duration
}

func (s *scriptedJWKS) CacheStatus() (int, time.Duration) { return s.keyCount, s.age }
func (s *scriptedJWKS) CacheTTL() time.Duration           { return s.ttl }

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

func TestReadinessHandler_AllHealthy(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	readinessHandler(
		pool,
		&scriptedPinger{},
		&scriptedJWKS{keyCount: 3, age: 30 * time.Second, ttl: 5 * time.Minute},
		probesQuietLogger(),
	).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	status, comps := decodeReady(t, rec)
	if status != "ok" {
		t.Errorf("status = %q", status)
	}
	for k, want := range map[string]string{"database": "ok", "brain": "ok", "jwks": "ok"} {
		if comps[k] != want {
			t.Errorf("%s = %q, want %q", k, comps[k], want)
		}
	}
}

func TestReadinessHandler_OptionalDepsAllSkipped(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	readinessHandler(pool, nil, nil, probesQuietLogger()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	_, comps := decodeReady(t, rec)
	if comps["brain"] != "skipped" || comps["jwks"] != "skipped" {
		t.Errorf("optional deps not marked skipped: %v", comps)
	}
}

func TestReadinessHandler_BrainUnhealthyFlipsReady(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	readinessHandler(
		pool,
		&scriptedPinger{err: errors.New("dial: connection refused")},
		&scriptedJWKS{keyCount: 3, age: 30 * time.Second, ttl: 5 * time.Minute},
		probesQuietLogger(),
	).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	status, comps := decodeReady(t, rec)
	if status != "unhealthy" || comps["brain"] != "unhealthy" {
		t.Errorf("expected brain unhealthy: %s %v", status, comps)
	}
	if comps["database"] != "ok" {
		t.Errorf("database should still be ok: %v", comps)
	}
}

func TestReadinessHandler_DBUnhealthyFlipsReady(t *testing.T) {
	pool := testdb.NewPool(t)
	pool.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(pool, &scriptedPinger{}, &scriptedJWKS{keyCount: 3, ttl: 5 * time.Minute}, probesQuietLogger()).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	_, comps := decodeReady(t, rec)
	if comps["database"] != "unhealthy" {
		t.Errorf("database = %q, want unhealthy", comps["database"])
	}
}

func TestReadinessHandler_JWKSColdStartFlipsReady(t *testing.T) {
	// keyCount == 0 means we've never successfully fetched. Treat as
	// unhealthy: we can't validate any JWT until the JWKS arrives,
	// so the LB shouldn't send us live traffic yet.
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(
		pool,
		&scriptedPinger{},
		&scriptedJWKS{keyCount: 0, ttl: 5 * time.Minute},
		probesQuietLogger(),
	).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	_, comps := decodeReady(t, rec)
	if comps["jwks"] != "unhealthy" {
		t.Errorf("jwks = %q, want unhealthy", comps["jwks"])
	}
}

func TestReadinessHandler_JWKSStaleFlipsReady(t *testing.T) {
	// age > 2*ttl → stale. The provider's GetKeySet falls back to
	// stale cache on refresh failures, so this state means we've
	// been unable to refresh long enough that Brain may have
	// rotated. Fail closed.
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(
		pool,
		&scriptedPinger{},
		&scriptedJWKS{keyCount: 3, age: 11 * time.Minute, ttl: 5 * time.Minute},
		probesQuietLogger(),
	).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	_, comps := decodeReady(t, rec)
	if comps["jwks"] != "unhealthy" {
		t.Errorf("jwks = %q, want unhealthy", comps["jwks"])
	}
}

func TestReadinessHandler_JWKSWithinTTLAcceptable(t *testing.T) {
	// age < 2*ttl is fine — the TTL itself triggers a refresh on
	// the next GetKeySet call, but the cache is not yet so old that
	// it's likely past Brain's rotation horizon.
	pool := testdb.NewPool(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readinessHandler(
		pool,
		&scriptedPinger{},
		&scriptedJWKS{keyCount: 3, age: 8 * time.Minute, ttl: 5 * time.Minute},
		probesQuietLogger(),
	).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	_, comps := decodeReady(t, rec)
	if comps["jwks"] != "ok" {
		t.Errorf("jwks = %q, want ok", comps["jwks"])
	}
}
