//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/ai"
)

// recordingMetrics is an ai.MetricsObserver that records every observed
// AI call so the test can assert the kind/model/err of the round trip.
type recordingMetrics struct {
	mu    sync.Mutex
	calls []recordedCall
}

type recordedCall struct {
	kind  string
	model string
	err   error
}

func (m *recordingMetrics) ObserveAICall(kind, model string, _ time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, recordedCall{kind: kind, model: model, err: err})
}

func (m *recordingMetrics) snapshot() []recordedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]recordedCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// fakeAnthropic stands in for the Anthropic /v1/messages endpoint. It
// records the inbound x-api-key (proving the vault-decrypted key reaches
// the wire) and the call count, then returns a minimal valid text body.
type fakeAnthropic struct {
	mu      sync.Mutex
	calls   int
	lastKey string
}

func (f *fakeAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls++
	f.lastKey = r.Header.Get("x-api-key")
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":          "msg_test",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-sonnet-4-5",
		"stop_reason": "end_turn",
		"content": []map[string]any{
			{"type": "text", "text": "Framing inspection first; then pour footings."},
		},
	})
}

func (f *fakeAnthropic) snapshot() (int, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.lastKey
}

// TestAIVault_EndToEnd exercises the full standalone AI path end to end:
// the REAL encrypted VaultService resolves the per-org Anthropic key, and
// the native ai.Client decrypts it via the KeyResolver and sends it
// upstream. No real Anthropic key or network is required — a fake
// upstream stands in, so the test is fully deterministic.
//
// Proves, in order:
//   - no key set → ai.ErrUnconfigured, upstream NEVER called, no metric;
//   - key set    → 200 + the *decrypted* key on the x-api-key header
//     (the complete vault decrypt → AI wire path), plus a metric recorded
//     (kind=daily_briefing, fast model, nil err);
//   - other org  → ai.ErrUnconfigured and no extra upstream hit (org
//     isolation: one org's key never resolves for another).
func TestAIVault_EndToEnd(t *testing.T) {
	svc, orgID := newVaultService(t)

	fake := &fakeAnthropic{}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	metrics := &recordingMetrics{}
	client, err := ai.NewClient(ai.Config{
		KeyResolver: svc, // the REAL encrypted vault, not a stub
		BaseURL:     srv.URL,
		FastModel:   "claude-sonnet-4-5",
		Retry:       ai.RetryConfig{MaxAttempts: 3, BaseDelayMs: 1, Multiplier: 2.0},
		Metrics:     metrics,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := ai.ContextWithOrgID(context.Background(), orgID.String())
	briefReq := ai.DailyBriefingRequest{
		Tasks:  []string{"Frame second floor"},
		Alerts: []string{"Rain expected"},
	}

	// 1) No key configured → ErrUnconfigured, upstream untouched.
	if _, err := client.DailyBriefing(ctx, briefReq); !errors.Is(err, ai.ErrUnconfigured) {
		t.Fatalf("DailyBriefing(no key) err = %v, want ErrUnconfigured", err)
	}
	if calls, _ := fake.snapshot(); calls != 0 {
		t.Fatalf("upstream called %d times before a key was set; want 0", calls)
	}
	if got := len(metrics.snapshot()); got != 0 {
		t.Fatalf("metrics recorded %d calls before a key was set; want 0", got)
	}

	// 2) Set the key in the encrypted vault, then call again.
	const wantKey = "sk-ant-vault-e2e-7777"
	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID:    orgID,
		Provider: ProviderAnthropic,
		Label:    "E2E Anthropic",
		Key:      wantKey,
		UserSub:  "owner-e2e",
	}); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}

	resp, err := client.DailyBriefing(ctx, briefReq)
	if err != nil {
		t.Fatalf("DailyBriefing(with key): %v", err)
	}
	if resp.Reply == "" {
		t.Error("empty reply on the happy path")
	}
	calls, lastKey := fake.snapshot()
	if calls != 1 {
		t.Fatalf("upstream called %d times after key set; want 1", calls)
	}
	if lastKey != wantKey {
		t.Errorf("x-api-key on the wire = %q, want the vault-decrypted %q", lastKey, wantKey)
	}
	recorded := metrics.snapshot()
	if len(recorded) != 1 {
		t.Fatalf("metrics recorded %d calls; want 1", len(recorded))
	}
	if recorded[0].kind != "daily_briefing" {
		t.Errorf("metric kind = %q, want daily_briefing", recorded[0].kind)
	}
	if recorded[0].model != "claude-sonnet-4-5" {
		t.Errorf("metric model = %q, want the fast model", recorded[0].model)
	}
	if recorded[0].err != nil {
		t.Errorf("metric err = %v, want nil on the happy path", recorded[0].err)
	}

	// 3) Org isolation: a different org has no credential → unconfigured,
	//    and the configured org's key never resolves across the boundary
	//    (the call must not reach the wire).
	otherCtx := ai.ContextWithOrgID(context.Background(), uuid.New().String())
	if _, err := client.DailyBriefing(otherCtx, briefReq); !errors.Is(err, ai.ErrUnconfigured) {
		t.Fatalf("DailyBriefing(other org) err = %v, want ErrUnconfigured", err)
	}
	if calls, _ := fake.snapshot(); calls != 1 {
		t.Fatalf("upstream called %d times total; the other-org call must not reach the wire", calls)
	}
}
