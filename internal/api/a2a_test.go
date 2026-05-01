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
