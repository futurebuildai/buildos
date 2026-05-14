package models

import (
	"time"

	"github.com/google/uuid"
)

// Setup wizard data types. Each step of the onboarding wizard
// (MB-7 / W1-W3) reads and writes a subset of these. Wire shapes are
// driven by what the wizard UI needs to render and submit; persistence
// shapes match the schema added in migration 010.

// CompanyProfile captures the wizard step 1 fields stored as columns on
// the organizations table.
type CompanyProfile struct {
	LegalName             *string    `json:"legal_name,omitempty"`
	Address               *string    `json:"address,omitempty"`
	EIN                   *string    `json:"ein,omitempty"`
	CompanyType           *string    `json:"company_type,omitempty"`
	Region                *string    `json:"region,omitempty"`
	OnboardingComplete    bool       `json:"onboarding_complete"`
	OnboardingCompletedAt *time.Time `json:"onboarding_completed_at,omitempty"`
}

// SetupBootstrapToken records a one-shot owner-claim token. CleartextToken
// is never persisted — store.SetupStore hashes it before insert. Stored
// fields mirror the setup_bootstrap_tokens table.
//
// Wire note: TokenHash is intentionally NOT JSON-tagged for emission; it
// is internal-only. Wire shapes built from this type should be carved
// out as DTOs that drop hash + redeemed_by.
type SetupBootstrapToken struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	TokenHash  string     `json:"-"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RedeemedAt *time.Time `json:"redeemed_at,omitempty"`
	RedeemedBy *uuid.UUID `json:"redeemed_by,omitempty"`
}

// IsActive returns true when the token has not yet been redeemed AND
// has not expired. The setup service is the source of truth for clock
// resolution; this is a convenience used in tests + the service.
func (t SetupBootstrapToken) IsActive(now time.Time) bool {
	return t.RedeemedAt == nil && now.Before(t.ExpiresAt)
}

// TradeCategory mirrors the trade_categories table row.
type TradeCategory struct {
	ID          uuid.UUID `json:"id"`
	OrgID       uuid.UUID `json:"org_id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CostCode mirrors the cost_codes table row. Codes follow CSI
// MasterFormat (e.g. "03-30-00" for Cast-in-Place Concrete); Division
// is the first 2-digit segment (e.g. "03 Concrete") denormalized for
// fast filters.
type CostCode struct {
	ID         uuid.UUID `json:"id"`
	OrgID      uuid.UUID `json:"org_id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Division   string    `json:"division"`
	ParentCode *string   `json:"parent_code,omitempty"`
	IsDefault  bool      `json:"is_default"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// WorkingDayBit is a single-day flag in WorkingCalendar.WorkingDaysMask.
// Mon..Sun map to bits 0..6 so the bitmap is human-readable in dumps
// (0b0011111 = 31 = Mon-Fri).
type WorkingDayBit uint8

const (
	WorkingDayMon WorkingDayBit = 1 << 0
	WorkingDayTue WorkingDayBit = 1 << 1
	WorkingDayWed WorkingDayBit = 1 << 2
	WorkingDayThu WorkingDayBit = 1 << 3
	WorkingDayFri WorkingDayBit = 1 << 4
	WorkingDaySat WorkingDayBit = 1 << 5
	WorkingDaySun WorkingDayBit = 1 << 6

	// WorkingDaysMonFri is the standard 5-day work week (Mon-Fri).
	WorkingDaysMonFri = int16(WorkingDayMon | WorkingDayTue | WorkingDayWed | WorkingDayThu | WorkingDayFri)
)

// WorkingCalendar mirrors the working_calendars table row.
type WorkingCalendar struct {
	ID               uuid.UUID `json:"id"`
	OrgID            uuid.UUID `json:"org_id"`
	Name             string    `json:"name"`
	Timezone         string    `json:"timezone"`
	WorkingDaysMask  int16     `json:"working_days_mask"`
	DailyWorkMinutes int       `json:"daily_work_minutes"`
	IsDefault        bool      `json:"is_default"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// IsWorkingDay returns true when the given Go time.Weekday is set in
// the calendar's working_days_mask. Go's time.Weekday has Sunday=0;
// the schema bitmap uses Mon=0, so we translate.
func (c WorkingCalendar) IsWorkingDay(d time.Weekday) bool {
	// time.Weekday: Sun=0, Mon=1, ..., Sat=6
	// schema bit:   Mon=0, Tue=1, ..., Sun=6
	var bit int
	switch d {
	case time.Monday:
		bit = 0
	case time.Tuesday:
		bit = 1
	case time.Wednesday:
		bit = 2
	case time.Thursday:
		bit = 3
	case time.Friday:
		bit = 4
	case time.Saturday:
		bit = 5
	case time.Sunday:
		bit = 6
	}
	return c.WorkingDaysMask&(1<<bit) != 0
}

// HolidayOverride mirrors the holiday_overrides table row.
type HolidayOverride struct {
	ID          uuid.UUID `json:"id"`
	CalendarID  uuid.UUID `json:"calendar_id"`
	OrgID       uuid.UUID `json:"org_id"`
	HolidayDate time.Time `json:"holiday_date"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
}

// PermitJurisdiction mirrors the permit_jurisdictions table row.
// PermitTypes and InspectionChecklist are stored as JSONB; the store
// passes them through as raw bytes so callers can choose to leave them
// as opaque blobs or marshal into typed shapes per-feature.
//
// json:"-" on the JSONB byte slices is deliberate: a default []byte
// marshal would emit base64 over the wire, which is wrong for stored
// JSON. The api-layer DTO re-emits these as json.RawMessage so the
// raw JSON lands on the wire verbatim. Keeping the model as []byte
// avoids pgx Scan-type mismatches that json.RawMessage would create.
type PermitJurisdiction struct {
	ID                  uuid.UUID `json:"id"`
	OrgID               uuid.UUID `json:"org_id"`
	Name                string    `json:"name"`
	Region              *string   `json:"region,omitempty"`
	PermitTypes         []byte    `json:"-"` // raw JSONB; emitted via api DTO
	InspectionChecklist []byte    `json:"-"` // raw JSONB; emitted via api DTO
	Notes               *string   `json:"notes,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}
