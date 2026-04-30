package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/a2asigner"
	"github.com/futurebuildai/buildos/internal/worker"
)

// testSigner builds a Signer from a fresh in-memory RSA-2048 key.
// 2048-bit is fast enough on every CI runner.
func testSigner(t *testing.T) *a2asigner.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	signer, err := a2asigner.NewSignerFromPEM(pemBytes, "test-1")
	if err != nil {
		t.Fatalf("NewSignerFromPEM: %v", err)
	}
	return signer
}

func quietOutboundLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestA2AOutbound_DeliverHappyPath(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("X-JWS-Signature") == "" {
			t.Errorf("missing X-JWS-Signature header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		var env outboundEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		if env.Issuer != A2AIssuer {
			t.Errorf("iss = %q, want %q", env.Issuer, A2AIssuer)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := NewA2AOutboundService(nil, nil, testSigner(t), srv.URL, srv.Client(), quietOutboundLogger())

	err := svc.DeliverA2AWebhook(context.Background(), 1, worker.A2AWebhookDispatchArgs{
		OrgID:          uuid.New(),
		EventType:      "test.event",
		Payload:        json.RawMessage(`{"hello":"world"}`),
		IdempotencyKey: uuid.New(),
	})
	if err != nil {
		t.Errorf("Deliver: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("server called %d times; want 1", calls.Load())
	}
}

func TestA2AOutbound_RejectsMissingFields(t *testing.T) {
	svc := NewA2AOutboundService(nil, nil, testSigner(t), "http://x", &http.Client{}, quietOutboundLogger())

	cases := []struct {
		name string
		args worker.A2AWebhookDispatchArgs
	}{
		{"nil org", worker.A2AWebhookDispatchArgs{EventType: "x", IdempotencyKey: uuid.New()}},
		{"empty event_type", worker.A2AWebhookDispatchArgs{OrgID: uuid.New(), IdempotencyKey: uuid.New()}},
		{"nil idempotency", worker.A2AWebhookDispatchArgs{OrgID: uuid.New(), EventType: "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := svc.DeliverA2AWebhook(context.Background(), 1, c.args); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestA2AOutbound_4xxIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad envelope"}`))
	}))
	defer srv.Close()

	// nil pool is fine — recordDLQAndError will fail to begin a tx
	// and log; the upstream error still propagates which is what we
	// check.
	svc := NewA2AOutboundService(nil, nil, testSigner(t), srv.URL, srv.Client(), quietOutboundLogger())

	err := svc.DeliverA2AWebhook(context.Background(), 1, worker.A2AWebhookDispatchArgs{
		OrgID:          uuid.New(),
		EventType:      "test.event",
		Payload:        json.RawMessage(`{}`),
		IdempotencyKey: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error on 4xx")
	}
	var perm *permanentError
	if !errors.As(err, &perm) {
		t.Errorf("err = %v, want permanentError", err)
	}
	if perm != nil && perm.StatusCode != 400 {
		t.Errorf("perm.StatusCode = %d, want 400", perm.StatusCode)
	}
}

func TestA2AOutbound_429IsRetryable(t *testing.T) {
	// 429 must NOT be treated as permanent — Brain rate-limited us
	// and we should reschedule.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	svc := NewA2AOutboundService(nil, nil, testSigner(t), srv.URL, srv.Client(), quietOutboundLogger())

	err := svc.DeliverA2AWebhook(context.Background(), 1, worker.A2AWebhookDispatchArgs{
		OrgID:          uuid.New(),
		EventType:      "test.event",
		Payload:        json.RawMessage(`{}`),
		IdempotencyKey: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error on 429")
	}
	var perm *permanentError
	if errors.As(err, &perm) {
		t.Errorf("429 should be retryable, got permanentError: %v", err)
	}
}

func TestA2AOutbound_5xxIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	svc := NewA2AOutboundService(nil, nil, testSigner(t), srv.URL, srv.Client(), quietOutboundLogger())

	err := svc.DeliverA2AWebhook(context.Background(), 1, worker.A2AWebhookDispatchArgs{
		OrgID:          uuid.New(),
		EventType:      "test.event",
		Payload:        json.RawMessage(`{}`),
		IdempotencyKey: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error on 5xx")
	}
	var perm *permanentError
	if errors.As(err, &perm) {
		t.Errorf("5xx should be retryable, got permanentError: %v", err)
	}
}
