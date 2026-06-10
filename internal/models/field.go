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
	ID                uuid.UUID   `json:"id"`
	OrgID             uuid.UUID   `json:"org_id"`
	ProjectID         uuid.UUID   `json:"project_id"`
	ReportedBy        uuid.UUID   `json:"reported_by"`
	LogDate           time.Time   `json:"log_date"`
	WeatherConditions *string     `json:"weather_conditions,omitempty"`
	WorkSummary       string      `json:"work_summary"`
	SafetyIncidents   *string     `json:"safety_incidents,omitempty"`
	PhotoAssetIDs     []uuid.UUID `json:"photo_asset_ids,omitempty"`
	IdempotencyKey    uuid.UUID   `json:"idempotency_key"`
	ReportedAt        time.Time   `json:"reported_at"`
}

// FieldEquipment is the FIELD-SAFE projection of a fleet asset currently
// allocated to one of the caller's projects (Phase 4a-ii, read-only). It is
// deliberately NOT models.FleetAsset: the field response must not re-serve the
// operator model — that would leak org_id and any column later added to
// fleet_assets (e.g. a cost/value column) to field roles. Carries only what a
// field worker needs to know which equipment is on their site, plus the
// allocation window. There are no monetary columns on fleet_assets or
// equipment_allocations today, so there is no financial field to strip here.
type FieldEquipment struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	AssetType    string    `json:"asset_type"`
	SerialNumber *string   `json:"serial_number,omitempty"`
	Status       string    `json:"status"`
	// Allocation window the asset is on the caller's project for. DATE columns
	// → midnight-UTC time.Time (same as DailyLog.LogDate). end_date is exclusive
	// (the allocations daterange is [start, end)).
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

// FieldSyncResponse bundles everything a mobile client needs to refresh
// its local state. ServerTime is what the client should pass back as
// `?since=` on the next sync — the API contract uses a server-authoritative
// timestamp so clock skew doesn't drop or double-deliver rows.
//
// Equipment is a FULL-SET collection (not a delta): it ignores `?since` and
// always returns the caller's currently-allocated equipment, because relevance
// pivots on the allocation window (and a status flip) — not on a row's
// created_at — and neither fleet table has an updated_at to delta on. The
// mobile client must therefore REPLACE (delete-then-fill) its equipment cache
// each sync, not upsert.
type FieldSyncResponse struct {
	Tasks      []ProjectTask    `json:"tasks"`
	FeedCards  []FeedCard       `json:"feed_cards"`
	Equipment  []FieldEquipment `json:"equipment"`
	ServerTime time.Time        `json:"server_time"`
}
