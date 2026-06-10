package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLivenessHandler_AlwaysReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	livenessHandler("staging-abc1234").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	// The deploy pipeline's smoke asserts the rolled binary by this
	// field — it must round-trip the ldflags-stamped version verbatim.
	if body["version"] != "staging-abc1234" {
		t.Errorf("version = %v, want staging-abc1234", body["version"])
	}
}

func TestLivenessHandler_EmptyVersionIsDev(t *testing.T) {
	rec := httptest.NewRecorder()
	livenessHandler("").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["version"] != "dev" {
		t.Errorf("version = %v, want dev fallback", body["version"])
	}
}
