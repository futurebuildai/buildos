package api

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-jose/go-jose/v4"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
)

// A2AHandler handles the POST /api/v1/a2a/webhook endpoint.
// This endpoint uses JWS detached signature verification (NOT JWT Bearer auth).
type A2AHandler struct {
	jwks   *mw.JWKSProvider
	logger *slog.Logger
}

// NewA2AHandler creates a handler for A2A webhook events from The Brain.
func NewA2AHandler(jwks *mw.JWKSProvider, logger *slog.Logger) *A2AHandler {
	return &A2AHandler{
		jwks:   jwks,
		logger: logger,
	}
}

// a2aWebhookPayload represents the incoming webhook body from The Brain.
type a2aWebhookPayload struct {
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	TraceID        string          `json:"trace_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Timestamp      string          `json:"timestamp"`
	Issuer         string          `json:"iss"`
}

// ReceiveWebhook processes A2A webhook events from The Brain.
// POST /api/v1/a2a/webhook
//
// Auth: JWS detached signature via X-JWS-Signature header.
// This route BYPASSES the standard JWT middleware.
func (h *A2AHandler) ReceiveWebhook(w http.ResponseWriter, r *http.Request) {
	// Read the raw body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Extract JWS signature header
	jwsSig := r.Header.Get("X-JWS-Signature")
	if jwsSig == "" {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-JWS-Signature header")
		return
	}

	// Extract idempotency key
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "missing X-Idempotency-Key header")
		return
	}

	// Verify JWS detached signature
	if err := h.verifyJWSSignature(r.Context(), body, jwsSig); err != nil {
		h.logger.Warn("JWS verification failed", "error", err, "trace_id", r.Header.Get("X-Trace-ID"))
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid JWS signature")
		return
	}

	// Parse the webhook payload
	var payload a2aWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	// TODO: Check idempotency_key for deduplication (Sprint 5)
	// TODO: Route to event-specific handler based on event_type (Sprint 5)

	h.logger.Info("a2a webhook received",
		"event_type", payload.EventType,
		"trace_id", payload.TraceID,
		"idempotency_key", payload.IdempotencyKey,
	)

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted"})
}

// verifyJWSSignature verifies the detached JWS compact signature against the
// request body using The Brain's public key from JWKS.
//
// JWS Detached Compact Serialization: header..signature (empty payload segment).
// The actual payload is the raw HTTP body, re-attached for verification.
func (h *A2AHandler) verifyJWSSignature(ctx context.Context, body []byte, jwsSig string) error {
	keySet, err := h.jwks.GetKeySet(ctx)
	if err != nil {
		return fmt.Errorf("fetching JWKS for JWS verification: %w", err)
	}

	// Parse the detached JWS — the signature has format "header..signature"
	// We need to re-attach the payload to verify.
	jws, err := jose.ParseDetached(jwsSig, body, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return fmt.Errorf("parsing JWS detached signature: %w", err)
	}

	// Try each key in the JWKS
	for _, key := range keySet.Keys {
		if key.Use != "sig" && key.Use != "" {
			continue
		}
		pubKey, ok := key.Key.(*rsa.PublicKey)
		if !ok {
			continue
		}
		if _, err := jws.Verify(pubKey); err == nil {
			return nil // Verification succeeded
		}
	}

	return fmt.Errorf("no matching key verified the JWS signature")
}
