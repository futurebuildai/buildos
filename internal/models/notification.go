package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Notification types — matches the notification_type column in
// field_notification_dlq and the args of the field_notification_retry
// River job. Senders dispatch on this string to pick the transport
// (Twilio for SMS, FCM for push, SES/etc. for email).
const (
	NotificationTypeSMS   = "sms"
	NotificationTypePush  = "push"
	NotificationTypeEmail = "email"
)

// IsValidNotificationType reports whether s is one of the allowed values.
func IsValidNotificationType(s string) bool {
	switch s {
	case NotificationTypeSMS, NotificationTypePush, NotificationTypeEmail:
		return true
	default:
		return false
	}
}

// FieldNotificationDLQEntry is one record of a notification that
// exhausted River's retry budget. Schema lives in migration 003
// (created in the Sprint 0 walking skeleton); the columns are:
//
//	user_id           UUID NOT NULL REFERENCES users(id)
//	notification_type TEXT NOT NULL
//	payload           JSONB NOT NULL          (transport-specific shape)
//	retry_count       INTEGER NOT NULL
//	last_error        TEXT (nullable)
//	created_at        TIMESTAMPTZ NOT NULL    (when the discard happened)
type FieldNotificationDLQEntry struct {
	ID               uuid.UUID       `json:"id"`
	UserID           uuid.UUID       `json:"user_id"`
	NotificationType string          `json:"notification_type"`
	Payload          json.RawMessage `json:"payload"`
	RetryCount       int             `json:"retry_count"`
	LastError        *string         `json:"last_error,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}
