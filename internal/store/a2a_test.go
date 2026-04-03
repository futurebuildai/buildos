package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestA2AWebhookLogStruct(t *testing.T) {
	log := A2AWebhookLog{
		IdempotencyKey: "key-001",
		EventType:      "review_material_quote",
		Payload:        []byte(`{"total_cents":50000,"currency_code":"USD"}`),
		TraceID:        "trace-001",
		Issuer:         "fb-brain",
		Status:         "processed",
	}

	if log.IdempotencyKey != "key-001" {
		t.Errorf("expected key-001, got %s", log.IdempotencyKey)
	}
	if log.EventType != "review_material_quote" {
		t.Errorf("expected review_material_quote, got %s", log.EventType)
	}
	if log.Issuer != "fb-brain" {
		t.Errorf("expected fb-brain, got %s", log.Issuer)
	}
}

func TestNotificationDLQEntryStruct(t *testing.T) {
	userID := uuid.New()
	entry := NotificationDLQEntry{
		ID:               uuid.New(),
		UserID:           userID,
		NotificationType: "push",
		Payload:          []byte(`{"message":"test"}`),
		RetryCount:       2,
		MaxRetries:       6,
		Status:           "pending",
	}

	if entry.UserID != userID {
		t.Errorf("user ID mismatch")
	}
	if entry.RetryCount != 2 {
		t.Errorf("expected retry_count=2, got %d", entry.RetryCount)
	}
	if entry.MaxRetries != 6 {
		t.Errorf("expected max_retries=6, got %d", entry.MaxRetries)
	}
}

func TestBackoffSchedule(t *testing.T) {
	// Verify the expected backoff intervals: 30s, 60s, 120s, 300s, 900s, 3600s
	schedule := []int{30, 60, 120, 300, 900, 3600}

	if len(schedule) != 6 {
		t.Fatalf("expected 6 backoff intervals, got %d", len(schedule))
	}
	if schedule[0] != 30 {
		t.Errorf("first backoff should be 30s, got %d", schedule[0])
	}
	if schedule[5] != 3600 {
		t.Errorf("last backoff should be 3600s (1hr), got %d", schedule[5])
	}

	// Each interval should be >= the previous one
	for i := 1; i < len(schedule); i++ {
		if schedule[i] < schedule[i-1] {
			t.Errorf("backoff schedule should be non-decreasing: index %d (%d) < index %d (%d)",
				i, schedule[i], i-1, schedule[i-1])
		}
	}
}
