package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/a2asigner"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

// A2AIssuer is the iss claim BuildOS stamps on outbound webhooks.
// Brain validates this matches its expected peer identity. The
// inbound counterpart is "fb-brain"; both wire-protocol values are
// frozen for cross-repo compatibility — see CLAUDE.md.
const A2AIssuer = "fb-buildos"

// outboundEnvelope mirrors service.WebhookEnvelope plus an iss field
// — the same shape Brain accepts on its receiver. Defined separately
// from WebhookEnvelope so we can omit Brain-only fields and add
// outbound-only ones without coupling the two directions.
type outboundEnvelope struct {
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	TraceID        string          `json:"trace_id,omitempty"`
	IdempotencyKey uuid.UUID       `json:"idempotency_key"`
	Timestamp      time.Time       `json:"timestamp"`
	Issuer         string          `json:"iss"`
	OrgID          uuid.UUID       `json:"org_id"`
}

// A2AOutboundService dispatches signed webhook events to The Brain's
// receiver. Each Deliver call is a self-contained HTTP POST: build
// envelope, sign, send. River retries are driven from the worker
// boundary — non-final failures bubble back to River; final-attempt
// failures land in a2a_outbound_dlq before discarding the job.
type A2AOutboundService struct {
	pool       *pgxpool.Pool
	dlqStore   *store.A2AOutboundStore
	signer     *a2asigner.Signer
	httpClient *http.Client
	targetURL  string
	logger     *slog.Logger
}

// NewA2AOutboundService creates the service. targetURL is the full
// receiver URL on Brain (e.g. "https://brain.example/api/v1/a2a/webhook").
// signer must be non-nil; cmd/server / cmd/worker should use the
// no-op worker fallback when no signing key is provisioned rather
// than constructing this service with nil dependencies.
func NewA2AOutboundService(pool *pgxpool.Pool, dlq *store.A2AOutboundStore, signer *a2asigner.Signer, targetURL string, httpClient *http.Client, logger *slog.Logger) *A2AOutboundService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &A2AOutboundService{
		pool:       pool,
		dlqStore:   dlq,
		signer:     signer,
		httpClient: httpClient,
		targetURL:  targetURL,
		logger:     logger,
	}
}

// DeliverA2AWebhook builds the outbound envelope, signs it, and POSTs
// to Brain. On non-final-attempt failures returns the error so River
// reschedules. On final-attempt failures writes to a2a_outbound_dlq
// then bubbles the error so River discards.
//
// 4xx (other than 429) responses are non-retryable — these are
// permanent semantic rejections from Brain (bad signature, malformed
// envelope) and retrying won't help. We DLQ immediately on 4xx
// regardless of attempt number.
func (s *A2AOutboundService) DeliverA2AWebhook(ctx context.Context, attempt int, args worker.A2AWebhookDispatchArgs) error {
	if args.OrgID == uuid.Nil {
		return fmt.Errorf("a2a outbound: missing org_id on dispatch args")
	}
	if args.EventType == "" {
		return fmt.Errorf("a2a outbound: missing event_type")
	}
	if args.IdempotencyKey == uuid.Nil {
		return fmt.Errorf("a2a outbound: missing idempotency_key")
	}

	env := outboundEnvelope{
		EventType:      args.EventType,
		Payload:        args.Payload,
		TraceID:        args.TraceID,
		IdempotencyKey: args.IdempotencyKey,
		Timestamp:      time.Now().UTC(),
		Issuer:         A2AIssuer,
		OrgID:          args.OrgID,
	}
	body, err := json.Marshal(env)
	if err != nil {
		// Marshal failure is a programmer error — DLQ would just
		// rewrite the same data. Bubble immediately so River
		// discards.
		return fmt.Errorf("a2a outbound: marshal envelope: %w", err)
	}

	signature, err := s.signer.SignDetached(body)
	if err != nil {
		// Signing failure means key material is bad — same kind of
		// non-retryable problem as marshal failure.
		return fmt.Errorf("a2a outbound: sign envelope: %w", err)
	}

	deliveryErr := s.post(ctx, body, signature)
	if deliveryErr == nil {
		s.logger.InfoContext(ctx, "a2a outbound delivered",
			"event_type", args.EventType,
			"org_id", args.OrgID,
			"trace_id", args.TraceID,
			"key_id", s.signer.KeyID(),
			"attempt", attempt,
		)
		return nil
	}

	s.logger.WarnContext(ctx, "a2a outbound delivery failed",
		"event_type", args.EventType,
		"org_id", args.OrgID,
		"attempt", attempt,
		"max_attempts", worker.MaxA2AOutboundAttempts,
		"error", deliveryErr,
	)

	// Non-retryable (4xx) → DLQ now, don't burn the retry budget.
	// Retryable + final attempt → DLQ then bubble.
	// Retryable + non-final → bubble for River to reschedule.
	var permErr *permanentError
	isPermanent := errors.As(deliveryErr, &permErr)
	if isPermanent || attempt >= worker.MaxA2AOutboundAttempts {
		return s.recordDLQAndError(ctx, args, attempt, deliveryErr)
	}
	return deliveryErr
}

// post builds and executes the signed HTTP POST. Returns nil on 2xx,
// a *permanentError for 4xx (other than 429), an error for 5xx and
// network failures.
func (s *A2AOutboundService) post(ctx context.Context, body []byte, signature string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-JWS-Signature", signature)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("transport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 2xx — success.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Drain so the underlying connection is reusable.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	// Read up to 4 KiB of the body for diagnostics.
	preview, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// 4xx (other than 429 Too Many Requests) is non-retryable.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		return &permanentError{
			StatusCode: resp.StatusCode,
			Body:       string(preview),
		}
	}
	// 5xx + 429 — retryable.
	return fmt.Errorf("a2a outbound: HTTP %d: %s", resp.StatusCode, string(preview))
}

// permanentError marks a Brain response we won't retry. 4xx (other
// than 429) means BuildOS sent something Brain semantically rejected;
// retrying with the same payload is futile.
type permanentError struct {
	StatusCode int
	Body       string
}

func (e *permanentError) Error() string {
	return fmt.Sprintf("a2a outbound: permanent HTTP %d: %s", e.StatusCode, e.Body)
}

// recordDLQAndError writes the row in its own short-lived tx so a
// successful insert isn't rolled back by River's discard handling of
// the returned error. If the DLQ insert itself fails we log and fall
// through — better to keep River's retry signal than swallow the
// upstream error.
//
// When pool or dlqStore are nil (test rig with focused behavior
// assertions, or a deployment that has explicitly skipped DLQ
// persistence) we just log and return — no panic on nil dependencies.
func (s *A2AOutboundService) recordDLQAndError(ctx context.Context, args worker.A2AWebhookDispatchArgs, attempt int, deliveryErr error) error {
	if s.pool == nil || s.dlqStore == nil {
		s.logger.WarnContext(ctx, "a2a outbound: DLQ persistence skipped (no pool/store wired)",
			"event_type", args.EventType, "org_id", args.OrgID, "error", deliveryErr)
		return deliveryErr
	}
	idemPtr := args.IdempotencyKey
	dlqErr := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.dlqStore.InsertOutboundDLQ(ctx, tx, store.InsertOutboundDLQParams{
			OrgID:          args.OrgID,
			EventType:      args.EventType,
			TargetURL:      s.targetURL,
			Payload:        args.Payload,
			TraceID:        args.TraceID,
			IdempotencyKey: &idemPtr,
			RetryCount:     attempt,
			LastError:      deliveryErr.Error(),
		})
		return err
	})
	if dlqErr != nil {
		s.logger.ErrorContext(ctx, "a2a outbound: failed to record DLQ entry; River will still discard the job",
			"event_type", args.EventType, "org_id", args.OrgID, "error", dlqErr)
	}
	return deliveryErr
}

// Compile-time check that A2AOutboundService satisfies the worker
// package's interface. Catches signature drift at build time rather
// than at the first scheduled tick.
var _ worker.A2AWebhookDeliverer = (*A2AOutboundService)(nil)
