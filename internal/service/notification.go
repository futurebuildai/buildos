package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

// MaxNotificationAttempts is the cap on River retries for the
// field_notification_retry job. After the Nth attempt fails, the
// service records the failure in field_notification_dlq and returns
// the error to River so the job is discarded.
const MaxNotificationAttempts = 6

// NotificationDelivery is the input to a NotificationSender. Payload is
// a raw JSON document whose shape depends on NotificationType:
//   - sms: {"to":"+1...","body":"..."}
//   - push: FCM/APNs shape
//   - email: SES/Resend shape
//
// Storing the payload as json.RawMessage matches the JSONB DLQ column
// and avoids a marshal/parse round trip when the worker re-enqueues.
type NotificationDelivery struct {
	UserID           uuid.UUID
	NotificationType string
	Payload          json.RawMessage
}

// NotificationSender is the transport boundary — service tests use a
// fake that returns scripted errors; production wires Twilio/FCM/etc.
// senders here. One interface, multiple impls per transport.
type NotificationSender interface {
	Send(ctx context.Context, n NotificationDelivery) error
}

// NewLoggingSender returns a sender that logs and always succeeds.
// Use as the default in dev/staging while real transports are pending.
func NewLoggingSender(logger *slog.Logger) NotificationSender {
	return &loggingSender{logger: logger}
}

type loggingSender struct {
	logger *slog.Logger
}

func (s *loggingSender) Send(ctx context.Context, n NotificationDelivery) error {
	s.logger.InfoContext(ctx, "notification delivered (logging-only sender)",
		"user_id", n.UserID,
		"notification_type", n.NotificationType,
	)
	return nil
}

// NotificationDeliveryService wraps a sender + DLQ store so the
// FieldNotificationRetryWorker can stay thin (just call Deliver).
type NotificationDeliveryService struct {
	pool   *pgxpool.Pool
	sender NotificationSender
	store  *store.NotificationsStore
	logger *slog.Logger
}

// NewNotificationDeliveryService creates a new service. Pass a real
// sender (Twilio/FCM) or NewLoggingSender for dev.
func NewNotificationDeliveryService(pool *pgxpool.Pool, sender NotificationSender, dlqStore *store.NotificationsStore, logger *slog.Logger) *NotificationDeliveryService {
	return &NotificationDeliveryService{
		pool:   pool,
		sender: sender,
		store:  dlqStore,
		logger: logger,
	}
}

// Deliver attempts to send a notification.
//
// On success: returns nil; River won't retry.
// On failure with attempt < MaxNotificationAttempts: returns the error
// so River reschedules using the worker's NextRetry policy.
// On failure with attempt == MaxNotificationAttempts: writes to the DLQ
// table THEN returns the error so River marks the job discarded.
//
// Unknown notification_type: returns an error immediately without
// touching the DLQ. The job will burn its retry budget repeating the
// same validation failure (since args are immutable). This is a
// programmer error, not a transport failure — visible in River's job
// table is enough; we don't pollute the DLQ with malformed inputs.
//
// `attempt` is the current 1-based attempt number (River.Job.Attempt).
func (s *NotificationDeliveryService) Deliver(ctx context.Context, attempt int, n NotificationDelivery) error {
	if !models.IsValidNotificationType(n.NotificationType) {
		return fmt.Errorf("notification: unknown notification_type %q (programmer error)", n.NotificationType)
	}

	err := s.sender.Send(ctx, n)
	if err == nil {
		return nil
	}

	s.logger.WarnContext(ctx, "notification delivery failed",
		"user_id", n.UserID,
		"notification_type", n.NotificationType,
		"attempt", attempt,
		"max_attempts", MaxNotificationAttempts,
		"error", err,
	)

	// Final attempt — record in DLQ before bubbling to River.
	if attempt >= MaxNotificationAttempts {
		return s.recordDLQAndError(ctx, attempt, n, err)
	}
	return err
}

// DeliverNotification adapts worker.FieldNotificationRetryArgs into
// the typed Deliver signature, satisfying worker.NotificationDeliverer.
// Service tests can call Deliver directly with a typed
// NotificationDelivery; the worker boundary calls this method instead.
func (s *NotificationDeliveryService) DeliverNotification(ctx context.Context, attempt int, args worker.FieldNotificationRetryArgs) error {
	return s.Deliver(ctx, attempt, NotificationDelivery{
		UserID:           args.UserID,
		NotificationType: args.NotificationType,
		Payload:          args.Payload,
	})
}

// Compile-time check that NotificationDeliveryService satisfies the
// worker package's interface. Catches signature drift at build time
// rather than at the first scheduled tick.
var _ worker.NotificationDeliverer = (*NotificationDeliveryService)(nil)

// recordDLQAndError opens its own short-lived tx to land the DLQ entry
// (so a successful insert isn't rolled back by River's discard handling
// of the returned error). If the DLQ insert itself fails we log and
// fall through — better to keep River's retry signal than swallow.
func (s *NotificationDeliveryService) recordDLQAndError(ctx context.Context, attempt int, n NotificationDelivery, sendErr error) error {
	dlqErr := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.store.InsertDLQEntry(ctx, tx, store.InsertDLQEntryParams{
			UserID:           n.UserID,
			NotificationType: n.NotificationType,
			Payload:          n.Payload,
			RetryCount:       attempt,
			LastError:        sendErr.Error(),
		})
		return err
	})
	if dlqErr != nil {
		s.logger.ErrorContext(ctx, "failed to record DLQ entry; River will still discard the job",
			"user_id", n.UserID, "error", dlqErr)
	}
	return sendErr
}
