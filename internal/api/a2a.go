package api

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-jose/go-jose/v4"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/service"
)

// A2AServicer is the subset of *service.A2AService consumed by
// A2AHandler. Defined as an interface for testability — handler tests
// inject a mock without spinning up a database.
type A2AServicer interface {
	ProcessWebhook(ctx context.Context, env service.WebhookEnvelope) (service.ProcessResult, error)
}

// JWSVerifier verifies a JWS detached compact signature against a body.
// Defined as an interface so handler tests can inject an always-ok
// implementation without setting up real RSA keys + JWKS plumbing.
type JWSVerifier interface {
	Verify(ctx context.Context, body []byte, signature string) error
}

// jwksVerifier implements JWSVerifier against a JWKSProvider — production
// path. The handler reaches into Brain's published JWKS, picks any key
// flagged for signature use, and tries each until one verifies.
type jwksVerifier struct {
	jwks *mw.JWKSProvider
}

// NewJWKSVerifier constructs a JWS verifier backed by the given JWKS
// provider. Used in production to verify Brain's emitter signature.
func NewJWKSVerifier(jwks *mw.JWKSProvider) JWSVerifier {
	return &jwksVerifier{jwks: jwks}
}

func (v *jwksVerifier) Verify(ctx context.Context, body []byte, signature string) error {
	keySet, err := v.jwks.GetKeySet(ctx)
	if err != nil {
		return fmt.Errorf("fetching JWKS for JWS verification: %w", err)
	}
	jws, err := jose.ParseDetached(signature, body, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return fmt.Errorf("parsing JWS detached signature: %w", err)
	}
	for _, key := range keySet.Keys {
		if key.Use != "sig" && key.Use != "" {
			continue
		}
		pubKey, ok := key.Key.(*rsa.PublicKey)
		if !ok {
			continue
		}
		if _, err := jws.Verify(pubKey); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no matching key verified the JWS signature")
}

// A2AHandler handles POST /api/v1/a2a/webhook — Brain emits JWS-signed
// webhooks here. This route BYPASSES JWT auth (uses JWS detached
// signature instead) and is mounted outside the authenticated route
// group in router.go.
type A2AHandler struct {
	verifier JWSVerifier
	svc      A2AServicer
	logger   *slog.Logger
}

// NewA2AHandler creates a handler with the given JWS verifier + service.
func NewA2AHandler(verifier JWSVerifier, svc A2AServicer, logger *slog.Logger) *A2AHandler {
	return &A2AHandler{verifier: verifier, svc: svc, logger: logger}
}

// ReceiveWebhook processes A2A webhook events from The Brain.
//
// Flow:
//
//  1. Read body (1 MB limit).
//  2. Verify JWS detached signature against Brain's public key.
//  3. Parse the envelope.
//  4. Hand off to the service for idempotency dedup + dispatch.
//
// The service runs the dedup INSERT and the per-event handler in one
// transaction, so duplicate deliveries land cleanly and partial failures
// roll back without orphaned dedup rows.
func (h *A2AHandler) ReceiveWebhook(w http.ResponseWriter, r *http.Request) {
	// Body size cap is enforced by mw.MaxBodySize on the route. Read
	// the (already-capped) body in full; an oversized payload surfaces
	// here as a *http.MaxBytesError and is distinguished from generic
	// transport failures via mw.IsBodyTooLarge.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if mw.IsBodyTooLarge(err) {
			writeErrorResponse(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				"webhook body exceeds 1 MiB")
			return
		}
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "failed to read request body")
		return
	}
	defer r.Body.Close()

	jwsSig := r.Header.Get("X-JWS-Signature")
	if jwsSig == "" {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-JWS-Signature header")
		return
	}

	if err := h.verifier.Verify(r.Context(), body, jwsSig); err != nil {
		h.logger.Warn("a2a JWS verification failed",
			"error", err, "trace_id", r.Header.Get("X-Trace-ID"))
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid JWS signature")
		return
	}

	var env service.WebhookEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	result, err := h.svc.ProcessWebhook(r.Context(), env)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	h.logger.Info("a2a webhook processed",
		"event_type", env.EventType,
		"trace_id", env.TraceID,
		"already_processed", result.AlreadyProcessed,
		"feed_card_id", result.FeedCardID,
	)
	writeJSON(w, r, http.StatusOK, map[string]any{
		"status":            "accepted",
		"already_processed": result.AlreadyProcessed,
		"feed_card_id":      result.FeedCardID,
	})
}

// writeServiceError maps A2A-service sentinel errors to HTTP responses.
// Reuses the budget service's ErrInvalidInput / ErrNotFound and adds
// service.ErrUnknownEvent for unrecognized event_type values.
func (h *A2AHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrUnknownEvent):
		writeErrorResponse(w, r, http.StatusBadRequest, "UNKNOWN_EVENT", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	default:
		h.logger.Error("a2a internal error", "error", err)
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
