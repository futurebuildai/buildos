package models

import (
	"time"

	"github.com/google/uuid"
)

// DailyReport is a DERIVED read model — NOT a table. It is assembled on read per
// (project, date) from daily_logs + crew_checkins + task_progress (Chunk C of
// DAILY_REPORTS_CLIENT_UPDATES). It is the operator/office view: SafetyIncidents
// IS present here (internal surface). The client-safe homeowner narrative is a
// separate, redacted projection built from an allowlist in the service layer.
//
// CrewCount is derived from the crew_checkins JSONB array length only — the
// opaque crew_members shape is never surfaced (crew identities are Restricted
// PII). Photos resolve daily_logs.photo_asset_ids → short-lived signed GET URLs
// via the Chunk A AssetService; when storage is unconfigured Photos is empty but
// PhotoCount still reflects the raw id count (text works with zero photos).
type DailyReport struct {
	ProjectID         uuid.UUID          `json:"project_id"`
	ProjectName       string             `json:"project_name"`
	LogDate           time.Time          `json:"log_date"`
	WeatherConditions string             `json:"weather_conditions,omitempty"`
	WorkSummary       string             `json:"work_summary"`
	SafetyIncidents   string             `json:"safety_incidents,omitempty"` // INTERNAL — never to client
	Photos            []PhotoRef         `json:"photos,omitempty"`           // resolved assets (signed GET); empty when storage off
	PhotoCount        int                `json:"photo_count"`
	ReportedBy        uuid.UUID          `json:"reported_by"` // users.id (no display_name — Restricted)
	CrewCount         int                `json:"crew_count"`  // crew_checkins JSONB length, count only
	TaskProgress      []TaskProgressLine `json:"task_progress,omitempty"`
	ReportedAt        time.Time          `json:"reported_at"`
	HasLog            bool               `json:"has_log"` // false when only crew/progress (no daily_logs row)
}

// PhotoRef is one resolved photo on a daily report: the asset id + a short-lived
// signed GET URL (operator surface, 15 min). The raw storage key never appears.
type PhotoRef struct {
	AssetID   uuid.UUID `json:"asset_id"`
	ThumbURL  string    `json:"thumb_url"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskProgressLine is one per-task progress entry folded into a daily report.
// It carries the WBS/name/percent for the day — NOT GPS or the reporter's
// identity (both Restricted; stripped at the read-model boundary).
type TaskProgressLine struct {
	TaskID          uuid.UUID `json:"task_id"`
	WBSCode         string    `json:"wbs_code"`
	Name            string    `json:"name"`
	PercentComplete int       `json:"percent_complete"`
	Notes           string    `json:"notes,omitempty"`
	ReportedAt      time.Time `json:"reported_at"`
}

// DailyReportSummary is the list-row projection: enough to render a date
// selector / list without resolving photos or task-progress detail.
type DailyReportSummary struct {
	ProjectID         uuid.UUID `json:"project_id"`
	LogDate           time.Time `json:"log_date"`
	WeatherConditions string    `json:"weather_conditions,omitempty"`
	WorkSummary       string    `json:"work_summary"`
	HasSafetyIncident bool      `json:"has_safety_incident"`
	PhotoCount        int       `json:"photo_count"`
	CrewCount         int       `json:"crew_count"`
	TaskProgressCount int       `json:"task_progress_count"`
	ReportedAt        time.Time `json:"reported_at"`
}
