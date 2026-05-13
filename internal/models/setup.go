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
	LegalName               *string
	Address                 *string
	EIN                     *string
	CompanyType             *string
	Region                  *string
	OnboardingComplete      bool
	OnboardingCompletedAt   *time.Time
}

// SetupBootstrapToken records a one-shot owner-claim token. CleartextToken
// is never persisted — store.SetupStore hashes it before insert. Stored
// fields mirror the setup_bootstrap_tokens table.
type SetupBootstrapToken struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	TokenHash   string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	RedeemedAt  *time.Time
	RedeemedBy  *uuid.UUID
}

// IsActive returns true when the token has not yet been redeemed AND
// has not expired. The setup service is the source of truth for clock
// resolution; this is a convenience used in tests + the service.
func (t SetupBootstrapToken) IsActive(now time.Time) bool {
	return t.RedeemedAt == nil && now.Before(t.ExpiresAt)
}

// TradeCategory mirrors the trade_categories table row.
type TradeCategory struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	Code        string
	Name        string
	Description *string
	IsDefault   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CostCode mirrors the cost_codes table row. Codes follow CSI
// MasterFormat (e.g. "03-30-00" for Cast-in-Place Concrete); Division
// is the first 2-digit segment (e.g. "03 Concrete") denormalized for
// fast filters.
type CostCode struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	Code       string
	Name       string
	Division   string
	ParentCode *string
	IsDefault  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	ID                uuid.UUID
	OrgID             uuid.UUID
	Name              string
	Timezone          string
	WorkingDaysMask   int16
	DailyWorkMinutes  int
	IsDefault         bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	ID          uuid.UUID
	CalendarID  uuid.UUID
	OrgID       uuid.UUID
	HolidayDate time.Time
	Name        string
	CreatedAt   time.Time
}

// PermitJurisdiction mirrors the permit_jurisdictions table row.
// PermitTypes and InspectionChecklist are stored as JSONB; the store
// passes them through as raw bytes so callers can choose to leave them
// as opaque blobs or marshal into typed shapes per-feature.
type PermitJurisdiction struct {
	ID                  uuid.UUID
	OrgID               uuid.UUID
	Name                string
	Region              *string
	PermitTypes         []byte // raw JSONB
	InspectionChecklist []byte // raw JSONB
	Notes               *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
