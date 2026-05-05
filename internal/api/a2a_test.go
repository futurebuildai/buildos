package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/service"
)

// alwaysOKVerifier passes any JWS — used by handler tests where the
// JWS path isn't what's under test. The production JWKSVerifier is
// exercised end-to-end at integration time.
type alwaysOKVerifier struct{}

func (alwaysOKVerifier) Verify(context.Context, []byte, string) error { return nil }

// alwaysFailVerifier returns an error — used to test the 401 path.
type alwaysFailVerifier struct{}

func (alwaysFailVerifier) Verify(context.Context, []byte, string) error {
	return errors.New("test: bad signature")
}

// mockA2AService implements A2AServicer for handler tests.
type mockA2AService struct {
	result service.ProcessResult
	err    error

	lastEnvelope service.WebhookEnvelope
}

func (m *mockA2AService) ProcessWebhook(_ context.Context, env service.WebhookEnvelope) (service.ProcessResult, error) {
	m.lastEnvelope = env
	return m.result, m.err
}

// quietLogger silences slog output during tests.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newWebhookRequest(t *testing.T, body any, headers map[string]string) *http.Request {
	t.Helper()
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	r := httptest.NewRequest("POST", "/api/v1/a2a/webhook", strings.NewReader(string(bodyBytes)))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestA2AHandler_HappyPath(t *testing.T) {
	cardID := uuid.New()
	svc := &mockA2AService{
		result: service.ProcessResult{
			AlreadyProcessed: false,
			FeedCardID:       &cardID,
			EventType:        "review_material_quote",
		},
	}
	h := NewA2AHandler(alwaysOKVerifier{}, svc, quietLogger())

	envelope := service.WebhookEnvelope{
		EventType:      service.EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{"vendor":"Acme","total_cents":1000,"currency_code":"USD"}`),
		OrgID:          uuid.New(),
		Issuer:         "fb-brain",
	}
	r := newWebhookRequest(t, envelope, map[string]string{"X-JWS-Signature": "test-sig"})
	w := httptest.NewRecorder()
	h.ReceiveWebhook(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastEnvelope.EventType != service.EventReviewMaterialQuote {
		t.Errorf("envelope event_type = %q", svc.lastEnvelope.EventType)
	}
}

func TestA2AHandler_MissingSignatureReturns401(t *testing.T) {
	h := NewA2AHandler(alwaysOKVerifier{}, &mockA2AService{}, quietLogger())
	r := newWebhookRequest(t, service.WebhookEnvelope{
		EventType:      service.EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{}`),
		OrgID:          uuid.New(),
	}, nil) // no X-JWS-Signature
	w := httptest.NewRecorder()
	h.ReceiveWebhook(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", w.Code)
	}
}

func TestA2AHandler_BadSignatureReturns401(t *testing.T) {
	h := NewA2AHandler(alwaysFailVerifier{}, &mockA2AService{}, quietLogger())
	r := newWebhookRequest(t, service.WebhookEnvelope{
		EventType:      service.EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{}`),
		OrgID:          uuid.New(),
	}, map[string]string{"X-JWS-Signature": "bogus"})
	w := httptest.NewRecorder()
	h.ReceiveWebhook(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid JWS signature") {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestA2AHandler_BadJSONReturns400(t *testing.T) {
	h := NewA2AHandler(alwaysOKVerifier{}, &mockA2AService{}, quietLogger())
	r := httptest.NewRequest("POST", "/api/v1/a2a/webhook", strings.NewReader("not-json"))
	r.Header.Set("X-JWS-Signature", "test-sig")
	w := httptest.NewRecorder()
	h.ReceiveWebhook(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestA2AHandler_ServiceErrorMaps(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    int
		wantStr string
	}{
		{"unknown event", service.ErrUnknownEvent, http.StatusBadRequest, "UNKNOWN_EVENT"},
		{"invalid input", service.ErrInvalidInput, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"unexpected", errors.New("transient db down"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := NewA2AHandler(alwaysOKVerifier{}, &mockA2AService{err: c.err}, quietLogger())
			r := newWebhookRequest(t, service.WebhookEnvelope{
				EventType:      "x",
				IdempotencyKey: uuid.New(),
				Payload:        json.RawMessage(`{}`),
				OrgID:          uuid.New(),
			}, map[string]string{"X-JWS-Signature": "test-sig"})
			w := httptest.NewRecorder()
			h.ReceiveWebhook(w, r)
			if w.Code != c.want {
				t.Errorf("status=%d, want %d; body=%s", w.Code, c.want, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), c.wantStr) {
				t.Errorf("body should contain %q: %s", c.wantStr, w.Body.String())
			}
		})
	}
}

// idempotencyReplayService is a stateful A2AServicer that emulates the
// real ProcessWebhook dedup contract: the first envelope with a given
// IdempotencyKey returns AlreadyProcessed=false with a freshly minted
// FeedCardID, and any subsequent call with the same key returns the
// SAME FeedCardID with AlreadyProcessed=true. Mirrors the production
// behavior of A2AService.ProcessWebhook (single-tx INSERT ... ON
// CONFLICT against the dedup table) without needing a database.
type idempotencyReplayService struct {
	seen  map[uuid.UUID]uuid.UUID // idempotency_key -> feed_card_id
	calls int
}

func (s *idempotencyReplayService) ProcessWebhook(_ context.Context, env service.WebhookEnvelope) (service.ProcessResult, error) {
	s.calls++
	if s.seen == nil {
		s.seen = make(map[uuid.UUID]uuid.UUID)
	}
	if cardID, ok := s.seen[env.IdempotencyKey]; ok {
		return service.ProcessResult{
			AlreadyProcessed: true,
			FeedCardID:       &cardID,
			EventType:        env.EventType,
		}, nil
	}
	cardID := uuid.New()
	s.seen[env.IdempotencyKey] = cardID
	return service.ProcessResult{
		AlreadyProcessed: false,
		FeedCardID:       &cardID,
		EventType:        env.EventType,
	}, nil
}

// TestA2AHandler_IdempotencyReplaySameFeedCardID asserts the documented
// receiver contract: Brain redelivers an envelope with the same
// X-Idempotency-Key (e.g., after a network blip on the first ACK), and
// the receiver returns 200 with AlreadyProcessed=true and the SAME
// feed_card_id from the first call. This is the user-facing guarantee
// that prevents duplicate feed cards on retry.
func TestA2AHandler_IdempotencyReplaySameFeedCardID(t *testing.T) {
	svc := &idempotencyReplayService{}
	h := NewA2AHandler(alwaysOKVerifier{}, svc, quietLogger())

	// One envelope with one idempotency key, posted twice.
	env := service.WebhookEnvelope{
		EventType:      service.EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{"vendor":"Acme","total_cents":1000,"currency_code":"USD"}`),
		OrgID:          uuid.New(),
		Issuer:         "fb-brain",
	}
	headers := map[string]string{"X-JWS-Signature": "test-sig"}

	// First delivery: the dispatch ran, the feed card was created.
	r1 := newWebhookRequest(t, env, headers)
	w1 := httptest.NewRecorder()
	h.ReceiveWebhook(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first call: status=%d, body=%s", w1.Code, w1.Body.String())
	}
	var resp1 struct {
		Data struct {
			Status           string `json:"status"`
			AlreadyProcessed bool   `json:"already_processed"`
			FeedCardID       string `json:"feed_card_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("first call: decode body: %v (body=%s)", err, w1.Body.String())
	}
	if resp1.Data.AlreadyProcessed {
		t.Errorf("first call: already_processed=true, want false")
	}
	if resp1.Data.FeedCardID == "" {
		t.Fatalf("first call: feed_card_id empty, want a UUID")
	}

	// Second delivery: identical idempotency key. The receiver must
	// short-circuit, return AlreadyProcessed=true, and surface the
	// SAME feed_card_id so the caller can correlate the original.
	r2 := newWebhookRequest(t, env, headers)
	w2 := httptest.NewRecorder()
	h.ReceiveWebhook(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second call: status=%d, body=%s", w2.Code, w2.Body.String())
	}
	var resp2 struct {
		Data struct {
			Status           string `json:"status"`
			AlreadyProcessed bool   `json:"already_processed"`
			FeedCardID       string `json:"feed_card_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("second call: decode body: %v (body=%s)", err, w2.Body.String())
	}
	if !resp2.Data.AlreadyProcessed {
		t.Errorf("second call: already_processed=false, want true")
	}
	if resp2.Data.FeedCardID != resp1.Data.FeedCardID {
		t.Errorf("second call: feed_card_id=%q, want %q (must match first delivery)",
			resp2.Data.FeedCardID, resp1.Data.FeedCardID)
	}
	if svc.calls != 2 {
		t.Errorf("service calls = %d, want 2 (handler must invoke the service even on replay; dedup is a service-side concern)", svc.calls)
	}
}

// TestA2AHandler_BodyTooLargeReturns413 asserts the 1 MiB inbound cap
// (mw.A2AInboundMaxBodyBytes) surfaces as 413 PAYLOAD_TOO_LARGE rather
// than the generic 400 the handler emits on JSON decode errors.
//
// In production the cap is mounted via mw.MaxBodySize on the route
// (router.go:176-177) which wraps r.Body with http.MaxBytesReader. The
// handler then distinguishes the resulting *http.MaxBytesError via
// mw.IsBodyTooLarge. To exercise that branch in a handler-only test
// without the chi stack, this test wraps r.Body itself with the same
// MaxBytesReader the middleware would install.
func TestA2AHandler_BodyTooLargeReturns413(t *testing.T) {
	h := NewA2AHandler(alwaysOKVerifier{}, &mockA2AService{}, quietLogger())

	// > 1 MiB of arbitrary bytes; content doesn't matter — the cap
	// trips inside io.ReadAll before any JSON parse happens.
	oversize := strings.Repeat("a", mw.A2AInboundMaxBodyBytes+1)
	r := httptest.NewRequest("POST", "/api/v1/a2a/webhook", strings.NewReader(oversize))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-JWS-Signature", "test-sig")
	w := httptest.NewRecorder()

	// Simulate the router: install the same body-size cap the prod
	// route group mounts (mw.MaxBodySize(mw.A2AInboundMaxBodyBytes)).
	r.Body = http.MaxBytesReader(w, r.Body, mw.A2AInboundMaxBodyBytes)

	h.ReceiveWebhook(w, r)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d, want 413; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Errorf("body should contain PAYLOAD_TOO_LARGE: %s", w.Body.String())
	}
}

func TestA2AHandler_AlreadyProcessedSurfaced(t *testing.T) {
	svc := &mockA2AService{
		result: service.ProcessResult{
			AlreadyProcessed: true,
			EventType:        "review_material_quote",
		},
	}
	h := NewA2AHandler(alwaysOKVerifier{}, svc, quietLogger())
	r := newWebhookRequest(t, service.WebhookEnvelope{
		EventType:      service.EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{}`),
		OrgID:          uuid.New(),
	}, map[string]string{"X-JWS-Signature": "test-sig"})
	w := httptest.NewRecorder()
	h.ReceiveWebhook(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"already_processed":true`) {
		t.Errorf("body should expose already_processed=true: %s", w.Body.String())
	}
}
