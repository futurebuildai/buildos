package brain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := NewClient(Config{
		BaseURL: srv.URL,
		Retry: RetryConfig{
			MaxAttempts: 3,
			BaseDelayMs: 1, // keep tests fast; backoff still exercised
			Multiplier:  2.0,
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv.Close
}

func ctxWithToken(t *testing.T) context.Context {
	t.Helper()
	return ContextWithToken(context.Background(), "test-token-abc123")
}

// writeEnvelope marshals data into Brain's standard {data,meta} envelope.
func writeEnvelope(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{
		"data": data,
		"meta": map[string]string{"request_id": "test-req-123"},
	}
	_ = json.NewEncoder(w).Encode(body)
}

func TestMaestroChat_HappyPath(t *testing.T) {
	wantSession := uuid.New()

	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Method + path
		if r.Method != "POST" || r.URL.Path != "/api/maestro/chat" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		// Auth header
		if got := r.Header.Get("Authorization"); got != "Bearer test-token-abc123" {
			t.Errorf("Authorization = %q", got)
		}
		// Body decodes as ChatRequest
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Message != "morning briefing for proj-foo" {
			t.Errorf("message = %q", req.Message)
		}
		writeEnvelope(w, http.StatusOK, ChatResponse{
			SessionID: wantSession,
			Reply:     "Here's your briefing...",
		})
	})
	defer cleanup()

	resp, err := c.Maestro.Chat(ctxWithToken(t), ChatRequest{Message: "morning briefing for proj-foo"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.SessionID != wantSession {
		t.Errorf("session_id = %s, want %s", resp.SessionID, wantSession)
	}
	if resp.Reply != "Here's your briefing..." {
		t.Errorf("reply = %q", resp.Reply)
	}
}

func TestMaestroChat_RejectsEmptyMessage(t *testing.T) {
	calls := int32(0)
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	})
	defer cleanup()

	_, err := c.Maestro.Chat(ctxWithToken(t), ChatRequest{Message: ""})
	if err == nil {
		t.Fatal("expected error for empty message")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("server received %d calls; client should have validated locally", calls)
	}
}

func TestClient_NoTokenInContext_ReturnsUnauthenticated(t *testing.T) {
	calls := int32(0)
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	})
	defer cleanup()

	// Context without token — request should never leave the client.
	_, err := c.Maestro.Chat(context.Background(), ChatRequest{Message: "hi"})
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("server received %d calls; client should have short-circuited", calls)
	}
}

func TestClient_4xxNotRetried(t *testing.T) {
	calls := int32(0)
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"INVALID_BODY","message":"bad json"}}`))
	})
	defer cleanup()

	_, err := c.Maestro.Chat(ctxWithToken(t), ChatRequest{Message: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err type = %T, want *HTTPError", err)
	}
	if httpErr.StatusCode != 400 || httpErr.Code != "INVALID_BODY" {
		t.Errorf("HTTPError = %+v", httpErr)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("server received %d calls; 4xx should not be retried", calls)
	}
}

func TestClient_5xxRetriedThenTransient(t *testing.T) {
	calls := int32(0)
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer cleanup()

	_, err := c.Maestro.Chat(ctxWithToken(t), ChatRequest{Message: "x"})
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
	if !errors.Is(err, ErrTransient) {
		t.Errorf("err = %v, want wrapped ErrTransient", err)
	}
	// 3 attempts per the test config.
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server received %d calls, want 3", got)
	}
}

func TestClient_5xxThen200_RetrySuccess(t *testing.T) {
	calls := int32(0)
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeEnvelope(w, http.StatusOK, ChatResponse{
			SessionID: uuid.Must(uuid.NewRandom()),
			Reply:     "back online",
		})
	})
	defer cleanup()

	resp, err := c.Maestro.Chat(ctxWithToken(t), ChatRequest{Message: "hi"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Reply != "back online" {
		t.Errorf("reply = %q", resp.Reply)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server received %d calls, want 3", got)
	}
}

func TestClient_ContextCancelDuringBackoff(t *testing.T) {
	calls := int32(0)
	// Build a client with a SLOW backoff so we can cancel mid-wait.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, err := NewClient(Config{
		BaseURL: srv.URL,
		Retry:   RetryConfig{MaxAttempts: 5, BaseDelayMs: 200, Multiplier: 2.0},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Cancel after the first attempt + small slice of the backoff.
	ctx, cancel := context.WithTimeout(ctxWithToken(t), 50*time.Millisecond)
	defer cancel()

	_, err = c.Maestro.Chat(ctx, ChatRequest{Message: "x"})
	if err == nil {
		t.Fatal("expected error from cancelled ctx")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.DeadlineExceeded/Canceled", err)
	}
	// Should NOT have used all 5 attempts.
	if got := atomic.LoadInt32(&calls); got >= 5 {
		t.Errorf("server received %d calls; ctx cancel should have short-circuited retries", got)
	}
}

func TestBilling_GetUsageSummary_QueryParams(t *testing.T) {
	var rawQuery string
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		writeEnvelope(w, http.StatusOK, UsageSummary{
			OrgID:        "demo-org",
			TotalTokens:  10_000,
			CostCents:    150,
			CurrencyCode: "USD",
		})
	})
	defer cleanup()

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 30, 23, 59, 59, 0, time.UTC)
	resp, err := c.Billing.GetUsageSummary(ctxWithToken(t), UsageRange{Start: start, End: end})
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if resp.TotalTokens != 10_000 {
		t.Errorf("total_tokens = %d, want 10000", resp.TotalTokens)
	}
	if !strings.Contains(rawQuery, "start=2026-04-01") || !strings.Contains(rawQuery, "end=2026-04-30") {
		t.Errorf("query = %q (start/end not encoded)", rawQuery)
	}
}

func TestBilling_GetDailyUsage_NoRange_NoQuery(t *testing.T) {
	var rawQuery string
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		writeEnvelope(w, http.StatusOK, DailyUsageResponse{
			OrgID: "demo-org",
			Days:  []DailyUsage{},
		})
	})
	defer cleanup()

	_, err := c.Billing.GetDailyUsage(ctxWithToken(t), UsageRange{})
	if err != nil {
		t.Fatalf("GetDailyUsage: %v", err)
	}
	if rawQuery != "" {
		t.Errorf("zero-value range should not produce ?start/?end; got %q", rawQuery)
	}
}

func TestDoRequest_PropagatesRequestIDHeader(t *testing.T) {
	var seenHeader string
	c, stop := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	defer stop()

	ctx := ctxWithToken(t)
	ctx = ContextWithRequestID(ctx, "req-test-123")

	if _, err := c.doRequest(ctx, "GET", "/api/test", nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if seenHeader != "req-test-123" {
		t.Errorf("X-Request-ID = %q, want req-test-123", seenHeader)
	}
}

func TestDoRequest_OmitsRequestIDHeaderWhenAbsent(t *testing.T) {
	var seenHeader string
	c, stop := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	defer stop()

	if _, err := c.doRequest(ctxWithToken(t), "GET", "/api/test", nil); err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if seenHeader != "" {
		t.Errorf("X-Request-ID should be omitted; got %q", seenHeader)
	}
}

func TestPing_OKReturnsNil(t *testing.T) {
	c, stop := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %q, want /health", r.URL.Path)
		}
		// Auth header MUST NOT be required by Brain's /health.
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Ping should not send Authorization header; got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	defer stop()

	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping returned %v, want nil", err)
	}
}

func TestPing_5xxReturnsHTTPError(t *testing.T) {
	c, stop := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer stop()

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("err = %v, want HTTPError(503)", err)
	}
}

func TestPing_TransportErrorWrapped(t *testing.T) {
	c, err := NewClient(Config{BaseURL: "http://127.0.0.1:1"}) // closed port
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if perr := c.Ping(context.Background()); perr == nil {
		t.Fatal("expected error from closed port")
	}
}

func TestNewClient_BaseURLRequired(t *testing.T) {
	_, err := NewClient(Config{})
	if err == nil {
		t.Error("NewClient with empty BaseURL should error")
	}
}

func TestHTTPError_IsMatchesByStatus(t *testing.T) {
	cases := []struct {
		err       *HTTPError
		want      error
		shouldHit bool
	}{
		{&HTTPError{StatusCode: 401}, ErrUnauthenticated, true},
		{&HTTPError{StatusCode: 404}, ErrNotFound, true},
		{&HTTPError{StatusCode: 500}, ErrTransient, true},
		{&HTTPError{StatusCode: 503}, ErrTransient, true},
		{&HTTPError{StatusCode: 400}, ErrNotFound, false},
		{&HTTPError{StatusCode: 200}, ErrTransient, false},
	}
	for _, c := range cases {
		got := errors.Is(c.err, c.want)
		if got != c.shouldHit {
			t.Errorf("errors.Is(%d, %v) = %v, want %v", c.err.StatusCode, c.want, got, c.shouldHit)
		}
	}
}

func TestTokenFromContext(t *testing.T) {
	if _, ok := TokenFromContext(context.Background()); ok {
		t.Error("empty context should return ok=false")
	}
	ctx := ContextWithToken(context.Background(), "abc")
	if got, ok := TokenFromContext(ctx); !ok || got != "abc" {
		t.Errorf("TokenFromContext = (%q, %v), want (abc, true)", got, ok)
	}
	// Empty token treated as absent (avoid silently sending "Bearer ").
	ctx = ContextWithToken(context.Background(), "")
	if _, ok := TokenFromContext(ctx); ok {
		t.Error("empty token in context should return ok=false")
	}
}
