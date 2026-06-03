//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
	"github.com/futurebuildai/buildos/internal/worker"
)

// workerRetryArgs builds the River job payload the worker boundary hands
// to DeliverNotification — a final-attempt failure through this entrypoint
// must persist a DLQ row just like a direct Deliver call.
func workerRetryArgs(userID uuid.UUID) worker.FieldNotificationRetryArgs {
	return worker.FieldNotificationRetryArgs{
		UserID:           userID,
		NotificationType: "push",
		Payload:          json.RawMessage(`{"title":"adapter"}`),
	}
}

// newNotificationService wires a NotificationDeliveryService to a fresh
// migrated pool with a REAL NotificationsStore so the final-attempt DLQ
// write actually lands in field_notification_dlq (the path the nil-pool
// unit tests deliberately skip). sender is supplied per-test so each can
// script success/failure. Returns the service + the seeded org id; users
// are seeded per-test (the DLQ user_id FK requires an existing user).
func newNotificationService(t *testing.T, sender NotificationSender) (*NotificationDeliveryService, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Switchback Builders")
	svc := NewNotificationDeliveryService(pool, sender, store.NewNotificationsStore(), quietLogger())
	return svc, orgID
}

// dlqCount returns how many field_notification_dlq rows exist for a user.
func dlqCount(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM field_notification_dlq WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count field_notification_dlq: %v", err)
	}
	return n
}

// TestNotification_FinalAttempt_WritesDLQAndReturnsError is the canonical
// dead-letter round-trip: a sender that fails on the FINAL attempt drives
// Deliver into recordDLQAndError, which opens its own tx and persists the
// discard to field_notification_dlq — then still returns the transport
// error so River marks the job discarded. The persisted row carries the
// attempt count + the stringified error.
func TestNotification_FinalAttempt_WritesDLQAndReturnsError(t *testing.T) {
	wantErr := errors.New("transport: twilio 500")
	svc, orgID := newNotificationService(t, &scriptedSender{errs: []error{wantErr}})
	ctx := context.Background()

	userID := uuid.New()
	testdb.SeedUser(t, svc.pool, userID, orgID)

	err := svc.Deliver(ctx, MaxNotificationAttempts, NotificationDelivery{
		UserID:           userID,
		NotificationType: "sms",
		Payload:          json.RawMessage(`{"to":"+15551234567","body":"final attempt"}`),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Deliver(final) = %v, want wrapped %v", err, wantErr)
	}

	if got := dlqCount(t, svc.pool, userID); got != 1 {
		t.Fatalf("DLQ rows = %d, want 1", got)
	}

	var retryCount int
	var lastError string
	var notifType string
	if err := svc.pool.QueryRow(ctx,
		`SELECT retry_count, last_error, notification_type FROM field_notification_dlq WHERE user_id = $1`,
		userID).Scan(&retryCount, &lastError, &notifType); err != nil {
		t.Fatalf("read DLQ row: %v", err)
	}
	if retryCount != MaxNotificationAttempts {
		t.Errorf("DLQ retry_count = %d, want %d", retryCount, MaxNotificationAttempts)
	}
	if lastError != wantErr.Error() {
		t.Errorf("DLQ last_error = %q, want %q", lastError, wantErr.Error())
	}
	if notifType != "sms" {
		t.Errorf("DLQ notification_type = %q, want sms", notifType)
	}
}

// TestNotification_NonFinalFailure_NoDLQ proves that a transport failure
// BEFORE the final attempt returns the error (so River reschedules) but
// does NOT write to the DLQ — even with a real pool wired. Complements the
// nil-pool unit test by exercising the same branch against a live DB.
func TestNotification_NonFinalFailure_NoDLQ(t *testing.T) {
	wantErr := errors.New("transient: twilio 503")
	svc, orgID := newNotificationService(t, &scriptedSender{errs: []error{wantErr}})
	ctx := context.Background()

	userID := uuid.New()
	testdb.SeedUser(t, svc.pool, userID, orgID)

	err := svc.Deliver(ctx, 1, NotificationDelivery{
		UserID:           userID,
		NotificationType: "push",
		Payload:          json.RawMessage(`{"title":"hi"}`),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Deliver(attempt 1) = %v, want %v", err, wantErr)
	}
	if got := dlqCount(t, svc.pool, userID); got != 0 {
		t.Errorf("non-final failure wrote %d DLQ rows, want 0", got)
	}
}

// TestNotification_Success_NoDLQ proves a successful send (even on the
// final attempt) returns nil and never touches the DLQ.
func TestNotification_Success_NoDLQ(t *testing.T) {
	svc, orgID := newNotificationService(t, &scriptedSender{}) // always succeeds
	ctx := context.Background()

	userID := uuid.New()
	testdb.SeedUser(t, svc.pool, userID, orgID)

	if err := svc.Deliver(ctx, MaxNotificationAttempts, NotificationDelivery{
		UserID:           userID,
		NotificationType: "email",
		Payload:          json.RawMessage(`{"subject":"ok"}`),
	}); err != nil {
		t.Fatalf("Deliver(success) = %v, want nil", err)
	}
	if got := dlqCount(t, svc.pool, userID); got != 0 {
		t.Errorf("successful send wrote %d DLQ rows, want 0", got)
	}
}

// TestNotification_DeliverNotification_AdapterWritesDLQ exercises the
// worker-boundary adapter (the worker.NotificationDeliverer impl): it maps
// FieldNotificationRetryArgs into the typed Deliver call, so a final-attempt
// failure through this entrypoint also persists a DLQ row.
func TestNotification_DeliverNotification_AdapterWritesDLQ(t *testing.T) {
	wantErr := errors.New("transport: fcm unavailable")
	svc, orgID := newNotificationService(t, &scriptedSender{errs: []error{wantErr}})
	ctx := context.Background()

	userID := uuid.New()
	testdb.SeedUser(t, svc.pool, userID, orgID)

	err := svc.DeliverNotification(ctx, MaxNotificationAttempts, workerRetryArgs(userID))
	if !errors.Is(err, wantErr) {
		t.Fatalf("DeliverNotification(final) = %v, want wrapped %v", err, wantErr)
	}
	if got := dlqCount(t, svc.pool, userID); got != 1 {
		t.Errorf("adapter DLQ rows = %d, want 1", got)
	}
}
