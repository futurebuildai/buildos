package models

import (
	"time"

	"github.com/google/uuid"
)

// FieldProgress represents a progress report from a field worker.
type FieldProgress struct {
	ID             uuid.UUID `json:"id"`
	ProjectID      uuid.UUID `json:"project_id"`
	TaskID         uuid.UUID `json:"task_id"`
	UserID         uuid.UUID `json:"user_id"`
	PercentComplete int      `json:"percent_complete"`
	Notes          string    `json:"notes,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// FieldCheckin represents a field worker location check-in.
type FieldCheckin struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	ProjectID      uuid.UUID `json:"project_id"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// DailyLog represents a field worker's daily summary.
type DailyLog struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	ProjectID      uuid.UUID `json:"project_id"`
	LogDate        time.Time `json:"log_date"`
	Summary        string    `json:"summary"`
	HoursWorked    float64   `json:"hours_worked"`
	WeatherNotes   string    `json:"weather_notes,omitempty"`
	SafetyNotes    string    `json:"safety_notes,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// SyncPayload is returned by GET /api/v1/field/sync.
type SyncPayload struct {
	FeedCards []FeedCard `json:"feed_cards"`
	Tasks     []SyncTask `json:"tasks"`
	SyncedAt  string     `json:"synced_at"`
}

// SyncTask is a minimal task representation for field sync.
type SyncTask struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	PercentComplete int        `json:"percent_complete"`
	DueDate         *time.Time `json:"due_date,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
