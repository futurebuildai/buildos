package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/futurebuildai/buildos/internal/brain"
)

// fakeBilling lets tests script Brain responses without spinning up
// an HTTP server. Either field set drives the corresponding return.
type fakeBilling struct {
	summary    *brain.UsageSummary
	summaryErr error
	daily      *brain.DailyUsageResponse
	dailyErr   error

	lastRange brain.UsageRange
}

func (f *fakeBilling) GetUsageSummary(_ context.Context, r brain.UsageRange) (*brain.UsageSummary, error) {
	f.lastRange = r
	return f.summary, f.summaryErr
}

func (f *fakeBilling) GetDailyUsage(_ context.Context, r brain.UsageRange) (*brain.DailyUsageResponse, error) {
	f.lastRange = r
	return f.daily, f.dailyErr
}

func TestBillingHandler_Usage_OK(t *testing.T) {
	want := &brain.UsageSummary{
		OrgID:        "org-1",
		TotalTokens:  1000,
		InputTokens:  600,
		OutputTokens: 400,
		CostCents:    250,
		CurrencyCode: "USD",
	}
	h := NewBillingHandler(&fakeBilling{summary: want})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	h.Usage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Usage *brain.UsageSummary `json:"usage"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Usage == nil || body.Data.Usage.TotalTokens != want.TotalTokens {
		t.Errorf("body.usage = %+v, want %+v", body.Data.Usage, want)
	}
}

func TestBillingHandler_Usage_ParsesRange(t *testing.T) {
	fake := &fakeBilling{summary: &brain.UsageSummary{}}
	h := NewBillingHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/billing/usage?start=2026-04-01T00:00:00Z&end=2026-05-01T00:00:00Z", nil)
	h.Usage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	wantStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if !fake.lastRange.Start.Equal(wantStart) {
		t.Errorf("start = %s, want %s", fake.lastRange.Start, wantStart)
	}
}

func TestBillingHandler_Usage_RejectsMalformedStart(t *testing.T) {
	h := NewBillingHandler(&fakeBilling{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage?start=not-a-date", nil)
	h.Usage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestBillingHandler_Usage_RejectsEndBeforeStart(t *testing.T) {
	h := NewBillingHandler(&fakeBilling{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/billing/usage?start=2026-05-01T00:00:00Z&end=2026-04-01T00:00:00Z", nil)
	h.Usage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestBillingHandler_Usage_BrainUnauthSurfaces401(t *testing.T) {
	h := NewBillingHandler(&fakeBilling{
		summaryErr: &brain.HTTPError{StatusCode: http.StatusUnauthorized},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	h.Usage(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestBillingHandler_Usage_Brain5xxSurfaces502(t *testing.T) {
	h := NewBillingHandler(&fakeBilling{
		summaryErr: &brain.HTTPError{StatusCode: http.StatusServiceUnavailable},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	h.Usage(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "UPSTREAM_ERROR") {
		t.Errorf("body should mention UPSTREAM_ERROR; got %s", rec.Body.String())
	}
}

func TestBillingHandler_Usage_BrainTransientSurfaces502(t *testing.T) {
	h := NewBillingHandler(&fakeBilling{summaryErr: brain.ErrTransient})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage", nil)
	h.Usage(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestBillingHandler_DailyUsage_OK(t *testing.T) {
	want := &brain.DailyUsageResponse{
		OrgID: "org-1",
		Days:  []brain.DailyUsage{{Date: "2026-04-29", InputTokens: 100, OutputTokens: 50}},
	}
	h := NewBillingHandler(&fakeBilling{daily: want})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage/daily", nil)
	h.DailyUsage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Daily *brain.DailyUsageResponse `json:"daily"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Daily == nil || len(body.Data.Daily.Days) != 1 {
		t.Errorf("body.daily = %+v", body.Data.Daily)
	}
}

func TestBillingHandler_DailyUsage_ErrorMappingMatchesUsage(t *testing.T) {
	// Same writeBrainError path; confirm one of the mappings end-to-end.
	h := NewBillingHandler(&fakeBilling{
		dailyErr: errors.New("unexpected raw error"),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/usage/daily", nil)
	h.DailyUsage(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (untyped error fallback)", rec.Code)
	}
}
