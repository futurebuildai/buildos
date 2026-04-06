// Package a2a provides Agent-to-Agent webhook communication from OS to Brain.
// Uses JWS RS256 detached compact serialization to sign outbound payloads,
// matching the pattern Brain uses for its outbound webhooks to OS.
package a2a

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

// OS-originated event types sent back to Brain.
const (
	EventScheduleUpdated    = "os.schedule_updated"
	EventProcurementStatus  = "os.procurement_status"
	EventTaskCompleted      = "os.task_completed"
	EventInspectionResult   = "os.inspection_result"
)

// Emitter sends JWS-signed A2A webhooks from OS to Brain.
type Emitter struct {
	targetURL  string
	signingKey *rsa.PrivateKey
	keyID      string
	httpClient *http.Client
	logger     *slog.Logger
}

// EmitterConfig holds configuration for creating an Emitter.
type EmitterConfig struct {
	// TargetURL is Brain's A2A webhook endpoint (e.g., "https://brain.futurebuild.ai/api/v1/a2a/webhook").
	TargetURL string

	// SigningKeyPath is the path to the RS256 private key PEM file.
	// If empty and DevMode is true, a test key is generated.
	SigningKeyPath string

	// DevMode generates a test signing key if no key path is provided.
	DevMode bool

	// Logger for structured logging.
	Logger *slog.Logger
}

// NewEmitter creates an A2A webhook emitter with JWS RS256 signing.
// In dev mode, generates a test RSA key if no key path is provided.
func NewEmitter(cfg EmitterConfig) (*Emitter, error) {
	if cfg.TargetURL == "" {
		return nil, fmt.Errorf("a2a emitter: target URL is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	var signingKey *rsa.PrivateKey
	var keyID string

	if cfg.SigningKeyPath != "" {
		key, err := loadPrivateKey(cfg.SigningKeyPath)
		if err != nil {
			return nil, fmt.Errorf("a2a emitter: loading signing key: %w", err)
		}
		signingKey = key
		keyID = "os-signing-key-1"
	} else if cfg.DevMode {
		key, err := generateDevKey()
		if err != nil {
			return nil, fmt.Errorf("a2a emitter: generating dev key: %w", err)
		}
		signingKey = key
		keyID = "os-dev-key"
		cfg.Logger.Warn("a2a emitter: using generated dev signing key — NOT for production")
	} else {
		return nil, fmt.Errorf("a2a emitter: signing key path required in production mode")
	}

	return &Emitter{
		targetURL:  cfg.TargetURL,
		signingKey: signingKey,
		keyID:      keyID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     cfg.Logger,
	}, nil
}

// webhookPayload is the standard envelope for OS -> Brain webhooks.
type webhookPayload struct {
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	TraceID        string          `json:"trace_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Timestamp      string          `json:"timestamp"`
	Issuer         string          `json:"iss"`
}

// Emit sends a JWS-signed webhook to Brain with retry.
// The payload is JSON-encoded, wrapped in the standard envelope, JWS-signed,
// and POSTed to Brain's A2A webhook endpoint.
func (e *Emitter) Emit(ctx context.Context, eventType string, payload any) error {
	// Encode the inner payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	// Build the webhook envelope
	envelope := webhookPayload{
		EventType:      eventType,
		Payload:        payloadBytes,
		TraceID:        uuid.New().String(),
		IdempotencyKey: uuid.New().String(),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Issuer:         "futurebuild-os",
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshaling webhook envelope: %w", err)
	}

	// JWS detached compact signature
	jwsSig, err := e.signDetached(body)
	if err != nil {
		return fmt.Errorf("JWS signing: %w", err)
	}

	// Retry with simple exponential backoff: 3 attempts (0s, 1s, 4s)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			e.logger.Info("a2a webhook retry",
				"attempt", attempt+1,
				"backoff", backoff,
				"event_type", eventType,
				"trace_id", envelope.TraceID,
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}
		}

		lastErr = e.doPost(ctx, body, jwsSig, envelope.IdempotencyKey, envelope.TraceID)
		if lastErr == nil {
			e.logger.Info("a2a webhook sent",
				"event_type", eventType,
				"trace_id", envelope.TraceID,
				"attempt", attempt+1,
			)
			return nil
		}

		e.logger.Warn("a2a webhook attempt failed",
			"event_type", eventType,
			"trace_id", envelope.TraceID,
			"attempt", attempt+1,
			"error", lastErr,
		)
	}

	return fmt.Errorf("a2a webhook failed after 3 attempts: %w", lastErr)
}

// doPost sends the HTTP POST request to Brain.
func (e *Emitter) doPost(ctx context.Context, body []byte, jwsSig, idempotencyKey, traceID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-JWS-Signature", jwsSig)
	req.Header.Set("X-Idempotency-Key", idempotencyKey)
	req.Header.Set("X-Trace-ID", traceID)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (cap at 1KB for error diagnostics)
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// 409 Conflict = idempotency duplicate — treat as success
	if resp.StatusCode == http.StatusConflict {
		e.logger.Info("a2a webhook duplicate (idempotency)", "idempotency_key", idempotencyKey)
		return nil
	}

	return fmt.Errorf("brain returned status %d: %s", resp.StatusCode, string(respBody))
}

// signDetached creates a JWS RS256 detached compact serialization signature.
// Detached means the payload is NOT included in the JWS — only the header and signature.
// The receiver reconstructs the full JWS by inserting the raw body as the payload.
// Format: "header..signature" (payload section is empty).
func (e *Emitter) signDetached(payload []byte) (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: e.signingKey},
		(&jose.SignerOptions{}).WithHeader("kid", e.keyID),
	)
	if err != nil {
		return "", fmt.Errorf("creating signer: %w", err)
	}

	jws, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("signing payload: %w", err)
	}

	// Serialize to compact form: header.payload.signature
	compact, err := jws.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("compact serialize: %w", err)
	}

	// Convert to detached: remove the payload portion (between the two dots)
	// Compact format: BASE64URL(header).BASE64URL(payload).BASE64URL(signature)
	// Detached format: BASE64URL(header)..BASE64URL(signature)
	parts := splitCompact(compact)
	if len(parts) != 3 {
		return "", fmt.Errorf("unexpected compact JWS format: expected 3 parts, got %d", len(parts))
	}

	return parts[0] + ".." + parts[2], nil
}

// splitCompact splits a compact JWS into its three dot-separated parts.
func splitCompact(compact string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(compact); i++ {
		if compact[i] == '.' {
			parts = append(parts, compact[start:i])
			start = i + 1
		}
	}
	parts = append(parts, compact[start:])
	return parts
}

// EmitScheduleUpdate sends a schedule update event to Brain.
func (e *Emitter) EmitScheduleUpdate(ctx context.Context, projectID uuid.UUID, changes any) error {
	payload := map[string]any{
		"project_id": projectID.String(),
		"changes":    changes,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	return e.Emit(ctx, EventScheduleUpdated, payload)
}

// EmitProcurementStatus sends a procurement status change event to Brain.
func (e *Emitter) EmitProcurementStatus(ctx context.Context, itemID uuid.UUID, status string) error {
	payload := map[string]any{
		"item_id":    itemID.String(),
		"status":     status,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	return e.Emit(ctx, EventProcurementStatus, payload)
}

// loadPrivateKey reads an RSA private key from a PEM file.
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}

	// Use go-jose to parse the PEM
	// go-jose doesn't have a direct PEM parser, so use crypto/x509
	return parseRSAPrivateKeyPEM(pemData)
}

// parseRSAPrivateKeyPEM parses an RSA private key from PEM bytes.
func parseRSAPrivateKeyPEM(pemData []byte) (*rsa.PrivateKey, error) {
	// Import crypto/x509 parsing
	// We use encoding/pem from stdlib
	block, _ := decodePEM(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in key file")
	}

	key, err := parsePrivateKeyDER(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}

	return rsaKey, nil
}

// generateDevKey creates a 2048-bit RSA key for development use.
func generateDevKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}
