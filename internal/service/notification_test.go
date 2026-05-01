package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// scriptedSender returns the next error from `errs` on each Send.
// Empty errs slice = always succeed. Used to script "fail twice then
// succeed" or "fail every time" scenarios.
type scriptedSender struct {
	errs  []error
	calls int
}

func (s *scriptedSender) Send(_ context.Context, _ NotificationDelivery) error {
	defer func() { s.calls++ }()
	if s.calls < len(s.errs) {
		return s.errs[s.calls]
	}
	return nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// nilDLQStore is a NotificationsStore handle the service can hold for
// non-final-attempt tests where the DLQ insert path is never reached.
// We pass nil pool/store into NewNotificationDeliveryService — the
// non-final paths don't touch them.
//
// For final-attempt tests we'd need a real DB; those live in the
// integration suite.

func TestDeliver_SuccessReturnsNil_NoRetry(t *testing.T) {
	sender := &scriptedSender{} // always succeeds
	svc := NewNotificationDeliveryService(nil, sender, nil, quietLogger())

	err := svc.Deliver(context.Background(), 1, NotificationDelivery{
		UserID:           uuid.New(),
		NotificationType: "sms",
		Payload:          json.RawMessage(`{"body":"hi"}`),
	})
	if err != nil {
		t.Errorf("Deliver: unexpected error %v", err)
	}
	if sender.calls != 1 {
		t.Errorf("sender called %d times; want 1", sender.calls)
	}
}

func TestDeliver_NonFinalFailure_ReturnsErrorWithoutDLQTouch(t *testing.T) {
	wantErr := errors.New("transient: twilio 503")
	sender := &scriptedSender{errs: []error{wantErr}}
	// nil pool/store — Deliver should NOT touch them on non-final attempts.
	svc := NewNotificationDeliveryService(nil, sender, nil, quietLogger())

	err := svc.Deliver(context.Background(), 1, NotificationDelivery{
		UserID:           uuid.New(),
		NotificationType: "sms",
		Payload:          json.RawMessage(`{"body":"hi"}`),
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestDeliver_RejectsUnknownType_DoesNotTouchSenderOrDLQ(t *testing.T) {
	// Unknown type is a programmer error — we error out before touching
	// the sender (no transport call) AND before touching the pool (no
	// DLQ insert). Passing nil pool/store proves the path is pure.
	sender := &scriptedSender{}
	svc := NewNotificationDeliveryService(nil, sender, nil, quietLogger())

	err := svc.Deliver(context.Background(), 1, NotificationDelivery{
		UserID:           uuid.New(),
		NotificationType: "bogus",
		Payload:          json.RawMessage(`{"x":1}`),
	})
	if err == nil {
		t.Fatal("expected error for unknown notification type")
	}
	if sender.calls != 0 {
		t.Errorf("sender called %d times for unknown type; want 0", sender.calls)
	}
}

func TestMaxNotificationAttemptsConstantStable(t *testing.T) {
	// MaxNotificationAttempts is duplicated in InsertOpts on the worker
	// side; if either drifts, the retry budget is wrong. This test is a
	// canary — if it fails because someone changed the constant, also
	// update worker.FieldNotificationRetryArgs.InsertOpts and the
	// NextRetry delay table to match.
	if MaxNotificationAttempts != 6 {
		t.Errorf("MaxNotificationAttempts = %d; SPRINT_PLAN target is 6", MaxNotificationAttempts)
	}
}
