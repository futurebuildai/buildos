package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Reported-via channel constants for task_progress.reported_via.
const (
	ReportedViaWeb    = "web"
	ReportedViaMobile = "mobile"
)

// IsValidReportedVia reports whether s is one of the allowed channels.
func IsValidReportedVia(s string) bool {
	return s == ReportedViaWeb || s == ReportedViaMobile
}

// TaskProgress mirrors a task_progress row. idempotency_key is the
// dedup token from the mobile client's offline outbox; the column is
// UNIQUE so a replayed insert lands as a 409.
type TaskProgress struct {
	ID              uuid.UUID  `json:"id"`
	TaskID          uuid.UUID  `json:"task_id"`
	ReportedBy      uuid.UUID  `json:"reported_by"`
	PercentComplete int        `json:"percent_complete"`
	Notes           *string    `json:"notes,omitempty"`
	PhotoAssetID    *uuid.UUID `json:"photo_asset_id,omitempty"`
	GPSLat          *float64   `json:"gps_lat,omitempty"`
	GPSLng          *float64   `json:"gps_lng,omitempty"`
	ReportedVia     string     `json:"reported_via"`
	IdempotencyKey  uuid.UUID  `json:"idempotency_key"`
	ReportedAt      time.Time  `json:"reported_at"`
}

// CrewCheckin mirrors a crew_checkins row. crew_members is JSONB
// holding an array of {worker_id, gps_lat, gps_lng}; we store it
// opaque so the mobile shape can evolve without a schema migration.
type CrewCheckin struct {
	ID             uuid.UUID       `json:"id"`
	OrgID          uuid.UUID       `json:"org_id"`
	ProjectID      uuid.UUID       `json:"project_id"`
	ReportedBy     uuid.UUID       `json:"reported_by"`
	CrewMembers    json.RawMessage `json:"crew_members"`
	GPSLat         *float64        `json:"gps_lat,omitempty"`
	GPSLng         *float64        `json:"gps_lng,omitempty"`
	Notes          *string         `json:"notes,omitempty"`
	IdempotencyKey uuid.UUID       `json:"idempotency_key"`
	ReportedAt     time.Time       `json:"reported_at"`
}

// DailyLog mirrors a daily_logs row.
type DailyLog struct {
	ID                 uuid.UUID   `json:"id"`
	OrgID              uuid.UUID   `json:"org_id"`
	ProjectID          uuid.UUID   `json:"project_id"`
	ReportedBy         uuid.UUID   `json:"reported_by"`
	LogDate            time.Time   `json:"log_date"`
	WeatherConditions  *string     `json:"weather_conditions,omitempty"`
	WorkSummary        string      `json:"work_summary"`
	SafetyIncidents    *string     `json:"safety_incidents,omitempty"`
	PhotoAssetIDs      []uuid.UUID `json:"photo_asset_ids,omitempty"`
	IdempotencyKey     uuid.UUID   `json:"idempotency_key"`
	ReportedAt         time.Time   `json:"reported_at"`
}

// FieldSyncResponse bundles everything a mobile client needs to refresh
// its local state. ServerTime is what the client should pass back as
// `?since=` on the next sync — the API contract uses a server-authoritative
// timestamp so clock skew doesn't drop or double-deliver rows.
type FieldSyncResponse struct {
	Tasks      []ProjectTask `json:"tasks"`
	FeedCards  []FeedCard    `json:"feed_cards"`
	ServerTime time.Time     `json:"server_time"`
}
