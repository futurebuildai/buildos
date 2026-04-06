package a2a

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

// =============================================================================
// Helpers
// =============================================================================

func mustTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test RSA key: %v", err)
	}
	return key
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// writeTempPEM writes an RSA private key to a temporary PEM file and returns
// the file path. The caller should defer os.Remove on the returned path.
func writeTempPEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()

	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS#8: %v", err)
	}

	f, err := os.CreateTemp("", "a2a-test-key-*.pem")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	if err := pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: der}); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatalf("encode PEM: %v", err)
	}
	f.Close()
	return f.Name()
}

// =============================================================================
// NewEmitter Tests
// =============================================================================

func TestNewEmitter_RequiresTargetURL(t *testing.T) {
	_, err := NewEmitter(EmitterConfig{
		TargetURL: "",
		DevMode:   true,
	})
	if err == nil {
		t.Fatal("expected error for empty target URL")
	}
	if !strings.Contains(err.Error(), "target URL is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewEmitter_DevMode_GeneratesKey(t *testing.T) {
	em, err := NewEmitter(EmitterConfig{
		TargetURL: "https://brain.test/api/v1/a2a/webhook",
		DevMode:   true,
		Logger:    discardLogger(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if em == nil {
		t.Fatal("emitter should not be nil")
	}
	if em.signingKey == nil {
		t.Fatal("signing key should be populated in dev mode")
	}
	if em.keyID != "os-dev-key" {
		t.Errorf("keyID = %q, want %q", em.keyID, "os-dev-key")
	}
	if em.targetURL != "https://brain.test/api/v1/a2a/webhook" {
		t.Errorf("targetURL mismatch")
	}
}

func TestNewEmitter_ProductionMode_RequiresKeyPath(t *testing.T) {
	_, err := NewEmitter(EmitterConfig{
		TargetURL: "https://brain.test/api/v1/a2a/webhook",
		DevMode:   false,
	})
	if err == nil {
		t.Fatal("expected error when no signing key in production mode")
	}
	if !strings.Contains(err.Error(), "signing key path required in production mode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewEmitter_WithPEMKeyPath(t *testing.T) {
	key := mustTestKey(t)
	path := writeTempPEM(t, key)
	defer os.Remove(path)

	em, err := NewEmitter(EmitterConfig{
		TargetURL:      "https://brain.test/api/v1/a2a/webhook",
		SigningKeyPath: path,
		Logger:         discardLogger(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if em.keyID != "os-signing-key-1" {
		t.Errorf("keyID = %q, want %q", em.keyID, "os-signing-key-1")
	}
	if em.signingKey == nil {
		t.Fatal("signing key should be loaded from PEM")
	}
}

func TestNewEmitter_InvalidKeyPath(t *testing.T) {
	_, err := NewEmitter(EmitterConfig{
		TargetURL:      "https://brain.test/api/v1/a2a/webhook",
		SigningKeyPath: "/nonexistent/path/to/key.pem",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent key file")
	}
}

func TestNewEmitter_DefaultLogger(t *testing.T) {
	em, err := NewEmitter(EmitterConfig{
		TargetURL: "https://brain.test/api/v1/a2a/webhook",
		DevMode:   true,
		// Logger intentionally nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if em.logger == nil {
		t.Fatal("logger should default to slog.Default(), not nil")
	}
}

// =============================================================================
// Event Type Constants
// =============================================================================

func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ScheduleUpdated", EventScheduleUpdated, "os.schedule_updated"},
		{"ProcurementStatus", EventProcurementStatus, "os.procurement_status"},
		{"TaskCompleted", EventTaskCompleted, "os.task_completed"},
		{"InspectionResult", EventInspectionResult, "os.inspection_result"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// =============================================================================
// JWS RS256 Signing and Verification Round-Trip
// =============================================================================

func TestSignDetached_ProducesDetachedFormat(t *testing.T) {
	em := &Emitter{
		signingKey: mustTestKey(t),
		keyID:      "test-key",
		logger:     discardLogger(),
	}

	payload := []byte(`{"event_type":"os.schedule_updated","payload":{}}`)
	sig, err := em.signDetached(payload)
	if err != nil {
		t.Fatalf("signDetached: %v", err)
	}

	// Detached JWS format: "header..signature" (empty middle part)
	parts := strings.Split(sig, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	if parts[0] == "" {
		t.Error("header part should not be empty")
	}
	if parts[1] != "" {
		t.Errorf("payload part should be empty in detached format, got %q", parts[1])
	}
	if parts[2] == "" {
		t.Error("signature part should not be empty")
	}
}

func TestSignDetached_VerifyRoundTrip(t *testing.T) {
	key := mustTestKey(t)
	em := &Emitter{
		signingKey: key,
		keyID:      "round-trip-key",
		logger:     discardLogger(),
	}

	payload := []byte(`{"project_id":"abc-123","changes":{"delta":3}}`)
	sig, err := em.signDetached(payload)
	if err != nil {
		t.Fatalf("signDetached: %v", err)
	}

	// Reconstruct the full compact JWS by reinserting the base64url-encoded payload
	parts := strings.Split(sig, ".")
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	fullJWS := parts[0] + "." + encodedPayload + "." + parts[2]

	// Parse and verify with the corresponding public key
	jws, err := jose.ParseSigned(fullJWS, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatalf("ParseSigned: %v", err)
	}

	verified, err := jws.Verify(&key.PublicKey)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if string(verified) != string(payload) {
		t.Errorf("verified payload mismatch:\n  got:  %s\n  want: %s", verified, payload)
	}
}

func TestSignDetached_IncludesKIDAndAlg(t *testing.T) {
	em := &Emitter{
		signingKey: mustTestKey(t),
		keyID:      "os-signing-key-42",
		logger:     discardLogger(),
	}

	sig, err := em.signDetached([]byte(`{}`))
	if err != nil {
		t.Fatalf("signDetached: %v", err)
	}

	headerRaw, err := base64.RawURLEncoding.DecodeString(strings.Split(sig, ".")[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}

	var header map[string]any
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}

	if header["kid"] != "os-signing-key-42" {
		t.Errorf("kid = %v, want %q", header["kid"], "os-signing-key-42")
	}
	if header["alg"] != "RS256" {
		t.Errorf("alg = %v, want %q", header["alg"], "RS256")
	}
}

func TestSignDetached_WrongKeyFailsVerification(t *testing.T) {
	signingKey := mustTestKey(t)
	wrongKey := mustTestKey(t)

	em := &Emitter{
		signingKey: signingKey,
		keyID:      "correct-key",
		logger:     discardLogger(),
	}

	payload := []byte(`{"test":"data"}`)
	sig, err := em.signDetached(payload)
	if err != nil {
		t.Fatalf("signDetached: %v", err)
	}

	parts := strings.Split(sig, ".")
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	fullJWS := parts[0] + "." + encodedPayload + "." + parts[2]

	jws, err := jose.ParseSigned(fullJWS, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatalf("ParseSigned: %v", err)
	}

	// Verification with wrong key should fail
	_, err = jws.Verify(&wrongKey.PublicKey)
	if err == nil {
		t.Fatal("verification with wrong key should have failed")
	}
}

// =============================================================================
// splitCompact Tests
// =============================================================================

func TestSplitCompact(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"three parts", "header.payload.signature", []string{"header", "payload", "signature"}},
		{"detached", "header..signature", []string{"header", "", "signature"}},
		{"no dots", "nodots", []string{"nodots"}},
		{"empty", "", []string{""}},
		{"trailing dot", "a.", []string{"a", ""}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCompact(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// =============================================================================
// Webhook Payload Construction
// =============================================================================

func TestEmit_PayloadEnvelope(t *testing.T) {
	var receivedBody []byte
	var receivedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em, err := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	inner := map[string]string{"project_id": "proj-99"}
	if err := em.Emit(context.Background(), EventScheduleUpdated, inner); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// Decode envelope
	var env webhookPayload
	if err := json.Unmarshal(receivedBody, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if env.EventType != EventScheduleUpdated {
		t.Errorf("EventType = %q, want %q", env.EventType, EventScheduleUpdated)
	}
	if env.Issuer != "futurebuild-os" {
		t.Errorf("Issuer = %q, want %q", env.Issuer, "futurebuild-os")
	}

	// TraceID must be a valid UUID
	if _, err := uuid.Parse(env.TraceID); err != nil {
		t.Errorf("TraceID is not valid UUID: %v", err)
	}

	// IdempotencyKey must be a valid UUID
	if _, err := uuid.Parse(env.IdempotencyKey); err != nil {
		t.Errorf("IdempotencyKey is not valid UUID: %v", err)
	}

	// Timestamp must be RFC3339
	if _, err := time.Parse(time.RFC3339, env.Timestamp); err != nil {
		t.Errorf("Timestamp is not RFC3339: %v", err)
	}

	// Inner payload must survive
	var innerDecoded map[string]string
	if err := json.Unmarshal(env.Payload, &innerDecoded); err != nil {
		t.Fatalf("unmarshal inner: %v", err)
	}
	if innerDecoded["project_id"] != "proj-99" {
		t.Errorf("inner project_id = %q, want %q", innerDecoded["project_id"], "proj-99")
	}

	// Headers
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("X-JWS-Signature") == "" {
		t.Error("X-JWS-Signature should not be empty")
	}
	if receivedHeaders.Get("X-Idempotency-Key") == "" {
		t.Error("X-Idempotency-Key should not be empty")
	}
	if receivedHeaders.Get("X-Trace-ID") == "" {
		t.Error("X-Trace-ID should not be empty")
	}

	// JWS header must be detached format
	jwsSig := receivedHeaders.Get("X-JWS-Signature")
	jwsParts := strings.Split(jwsSig, ".")
	if len(jwsParts) != 3 || jwsParts[1] != "" {
		t.Error("X-JWS-Signature should be in detached format (header..signature)")
	}
}

func TestEmit_AllEventTypes(t *testing.T) {
	events := []string{
		EventScheduleUpdated,
		EventProcurementStatus,
		EventTaskCompleted,
		EventInspectionResult,
	}

	for _, et := range events {
		t.Run(et, func(t *testing.T) {
			var gotType string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var env webhookPayload
				json.Unmarshal(body, &env)
				gotType = env.EventType
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})
			if err := em.Emit(context.Background(), et, map[string]string{"k": "v"}); err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if gotType != et {
				t.Errorf("event type = %q, want %q", gotType, et)
			}
		})
	}
}

// =============================================================================
// Idempotency Key Uniqueness
// =============================================================================

func TestEmit_UniqueIdempotencyKeys(t *testing.T) {
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("X-Idempotency-Key"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})

	for i := 0; i < 5; i++ {
		if err := em.Emit(context.Background(), EventTaskCompleted, i); err != nil {
			t.Fatalf("Emit %d: %v", i, err)
		}
	}

	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate idempotency key: %q", k)
		}
		seen[k] = true
	}
	if len(keys) != 5 {
		t.Errorf("expected 5 keys, got %d", len(keys))
	}
}

// =============================================================================
// Convenience Methods
// =============================================================================

func TestEmitScheduleUpdate(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})

	pid := uuid.New()
	changes := map[string]string{"task": "foundation", "delta_days": "3"}
	if err := em.EmitScheduleUpdate(context.Background(), pid, changes); err != nil {
		t.Fatalf("EmitScheduleUpdate: %v", err)
	}

	var env webhookPayload
	json.Unmarshal(body, &env)

	if env.EventType != EventScheduleUpdated {
		t.Errorf("EventType = %q, want %q", env.EventType, EventScheduleUpdated)
	}

	var inner map[string]any
	json.Unmarshal(env.Payload, &inner)
	if inner["project_id"] != pid.String() {
		t.Errorf("project_id = %v, want %v", inner["project_id"], pid.String())
	}
	if inner["updated_at"] == nil {
		t.Error("updated_at should be present")
	}
}

func TestEmitProcurementStatus(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})

	itemID := uuid.New()
	if err := em.EmitProcurementStatus(context.Background(), itemID, "DELIVERED"); err != nil {
		t.Fatalf("EmitProcurementStatus: %v", err)
	}

	var env webhookPayload
	json.Unmarshal(body, &env)

	if env.EventType != EventProcurementStatus {
		t.Errorf("EventType = %q, want %q", env.EventType, EventProcurementStatus)
	}

	var inner map[string]any
	json.Unmarshal(env.Payload, &inner)
	if inner["item_id"] != itemID.String() {
		t.Errorf("item_id = %v, want %v", inner["item_id"], itemID.String())
	}
	if inner["status"] != "DELIVERED" {
		t.Errorf("status = %v, want DELIVERED", inner["status"])
	}
}

// =============================================================================
// Error Handling: retries, status codes, connection refused
// =============================================================================

func TestEmit_500_RetriesThreeTimes(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})

	err := em.Emit(context.Background(), EventTaskCompleted, "test")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Errorf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestEmit_409_TreatedAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})

	err := em.Emit(context.Background(), EventTaskCompleted, "dup")
	if err != nil {
		t.Fatalf("409 should not produce error: %v", err)
	}
}

func TestEmit_ConnectionRefused(t *testing.T) {
	em, _ := NewEmitter(EmitterConfig{
		TargetURL: "http://127.0.0.1:1", // port 1 should refuse
		DevMode:   true,
		Logger:    discardLogger(),
	})

	err := em.Emit(context.Background(), EventTaskCompleted, "test")
	if err == nil {
		t.Fatal("expected error on connection refused")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEmit_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := em.Emit(ctx, EventTaskCompleted, "timeout")
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

func TestEmit_RetrySucceedsOnSecondAttempt(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})

	err := em.Emit(context.Background(), EventTaskCompleted, "retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestEmit_2xxStatusesSucceed(t *testing.T) {
	codes := []int{200, 201, 202, 204}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()

			em, _ := NewEmitter(EmitterConfig{TargetURL: srv.URL, DevMode: true, Logger: discardLogger()})
			if err := em.Emit(context.Background(), EventTaskCompleted, nil); err != nil {
				t.Fatalf("status %d should succeed: %v", code, err)
			}
		})
	}
}

// =============================================================================
// doPost Request Construction
// =============================================================================

func TestDoPost_Headers(t *testing.T) {
	var gotReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r.Clone(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em := &Emitter{
		targetURL:  srv.URL,
		signingKey: mustTestKey(t),
		keyID:      "test",
		httpClient: &http.Client{Timeout: 5 * time.Second},
		logger:     discardLogger(),
	}

	err := em.doPost(context.Background(), []byte(`{}`), "hdr..sig", "idem-42", "trace-99")
	if err != nil {
		t.Fatalf("doPost: %v", err)
	}

	if gotReq.Method != http.MethodPost {
		t.Errorf("Method = %q, want POST", gotReq.Method)
	}
	if gotReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", gotReq.Header.Get("Content-Type"))
	}
	if gotReq.Header.Get("X-JWS-Signature") != "hdr..sig" {
		t.Errorf("X-JWS-Signature = %q", gotReq.Header.Get("X-JWS-Signature"))
	}
	if gotReq.Header.Get("X-Idempotency-Key") != "idem-42" {
		t.Errorf("X-Idempotency-Key = %q", gotReq.Header.Get("X-Idempotency-Key"))
	}
	if gotReq.Header.Get("X-Trace-ID") != "trace-99" {
		t.Errorf("X-Trace-ID = %q", gotReq.Header.Get("X-Trace-ID"))
	}
}

func TestDoPost_BodySent(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	em := &Emitter{
		targetURL:  srv.URL,
		signingKey: mustTestKey(t),
		keyID:      "test",
		httpClient: &http.Client{Timeout: 5 * time.Second},
		logger:     discardLogger(),
	}

	payload := []byte(`{"event":"test","payload":{}}`)
	if err := em.doPost(context.Background(), payload, "sig", "key", "trace"); err != nil {
		t.Fatalf("doPost: %v", err)
	}
	if string(gotBody) != string(payload) {
		t.Errorf("body mismatch:\n  got:  %s\n  want: %s", gotBody, payload)
	}
}

// =============================================================================
// PEM Parsing (pem.go)
// =============================================================================

func TestParseRSAPrivateKeyPEM_PKCS8(t *testing.T) {
	key := mustTestKey(t)
	path := writeTempPEM(t, key)
	defer os.Remove(path)

	pemData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PEM: %v", err)
	}

	parsed, err := parseRSAPrivateKeyPEM(pemData)
	if err != nil {
		t.Fatalf("parseRSAPrivateKeyPEM: %v", err)
	}

	if parsed.N.Cmp(key.N) != 0 {
		t.Error("parsed key modulus does not match original")
	}
}

func TestParseRSAPrivateKeyPEM_PKCS1(t *testing.T) {
	key := mustTestKey(t)

	// Encode as PKCS#1 (RSA PRIVATE KEY)
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})

	parsed, err := parseRSAPrivateKeyPEM(pemBlock)
	if err != nil {
		t.Fatalf("parseRSAPrivateKeyPEM (PKCS#1): %v", err)
	}

	if parsed.N.Cmp(key.N) != 0 {
		t.Error("parsed key modulus does not match original")
	}
}

func TestParseRSAPrivateKeyPEM_InvalidPEM(t *testing.T) {
	_, err := parseRSAPrivateKeyPEM([]byte("not-a-pem"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	if !strings.Contains(err.Error(), "no PEM block found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRSAPrivateKeyPEM_GarbageDER(t *testing.T) {
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage")})
	_, err := parseRSAPrivateKeyPEM(pemBlock)
	if err == nil {
		t.Fatal("expected error for garbage DER")
	}
}

func TestDecodePEM_ReturnsNilForEmptyInput(t *testing.T) {
	block, rest := decodePEM([]byte{})
	if block != nil {
		t.Error("expected nil block for empty input")
	}
	if len(rest) != 0 {
		t.Error("expected empty rest for empty input")
	}
}

// =============================================================================
// generateDevKey
// =============================================================================

func TestGenerateDevKey_Produces2048BitKey(t *testing.T) {
	key, err := generateDevKey()
	if err != nil {
		t.Fatalf("generateDevKey: %v", err)
	}
	if key == nil {
		t.Fatal("key should not be nil")
	}
	if key.N.BitLen() < 2048 {
		t.Errorf("bit length = %d, want >= 2048", key.N.BitLen())
	}
}
