//go:build integration

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

func strPtr(s string) *string { return &s }

// fixedClock returns the same time on every call. Used to make
// IssueBootstrapToken expiry deterministic across test runs.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// newSetupService constructs a SetupService bound to a fresh pool +
// no-op audit + injected clock. Helper for every test in this file.
func newSetupService(t *testing.T, clock func() time.Time) (*SetupService, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	svc := NewSetupService(pool, store.NewSetupStore(), NewNoopAuditRecorder(), clock)
	return svc, orgID
}

func TestSetupService_GetState_FreshOrg(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	got, err := svc.GetState(context.Background(), orgID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.OnboardingComplete {
		t.Error("OnboardingComplete should be false for fresh org")
	}
	if len(got.Trades) != 0 {
		t.Errorf("Trades len = %d, want 0", len(got.Trades))
	}
	if got.DefaultCalendar != nil {
		t.Error("DefaultCalendar should be nil for fresh org")
	}
}

func TestSetupService_UpdateCompanyInfo_HappyPath(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	cp, err := svc.UpdateCompanyInfo(context.Background(), UpdateCompanyInfoInput{
		OrgID:     orgID,
		UserSub:   "user-1",
		LegalName: strPtr("Kelbrook LLC"),
		Region:    strPtr("US-CT"),
	})
	if err != nil {
		t.Fatalf("UpdateCompanyInfo: %v", err)
	}
	if cp.LegalName == nil || *cp.LegalName != "Kelbrook LLC" {
		t.Errorf("LegalName = %v", cp.LegalName)
	}
}

func TestSetupService_UpdateCompanyInfo_RejectsEmptyPatch(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	_, err := svc.UpdateCompanyInfo(context.Background(), UpdateCompanyInfoInput{OrgID: orgID})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestSetupService_UpdateCompanyInfo_RejectsBadRegion(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	_, err := svc.UpdateCompanyInfo(context.Background(), UpdateCompanyInfoInput{
		OrgID:  orgID,
		Region: strPtr("not a region!"),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// TestSetupService_UpdateCompanyInfo_GuardLegs covers the two
// UpdateCompanyInfo branches the happy/empty-patch/bad-region tests
// miss: the pre-tx nil-org short-circuit, and the in-tx
// guardNotComplete leg (a late company-info edit on a finalized org is
// rejected with ErrSetupAlreadyComplete via mapSetupStoreError rather
// than silently mutating a configured org).
func TestSetupService_UpdateCompanyInfo_GuardLegs(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()

	// nil org short-circuits before the tx.
	if _, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{
		OrgID:     uuid.Nil,
		LegalName: strPtr("Kelbrook LLC"),
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil org: err = %v, want ErrInvalidInput", err)
	}

	// Drive the wizard to completion, then a late company-info edit is
	// rejected by the in-tx guardNotComplete leg.
	mustStep := func(name string, fn func() error) {
		t.Helper()
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	mustStep("company info", func() error {
		_, e := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")})
		return e
	})
	mustStep("trade", func() error {
		_, e := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"})
		return e
	})
	mustStep("cost code", func() error {
		_, e := svc.CreateCostCode(ctx, CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "x", Division: "03"})
		return e
	})
	mustStep("calendar", func() error {
		_, e := svc.CreateCalendar(ctx, CreateCalendarInput{OrgID: orgID, Name: "Default", WorkingDaysMask: models.WorkingDaysMonFri, IsDefault: true})
		return e
	})
	mustStep("complete", func() error {
		_, e := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID, UserSub: "owner-1"})
		return e
	})

	_, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Renamed LLC")})
	if !errors.Is(err, ErrSetupAlreadyComplete) {
		t.Fatalf("post-complete UpdateCompanyInfo: err = %v, want ErrSetupAlreadyComplete", err)
	}
}

func TestSetupService_CreateTrade_NormalizesCode(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	got, err := svc.CreateTrade(context.Background(), CreateTradeInput{
		OrgID: orgID,
		Code:  "  elec ", // mixed case + whitespace
		Name:  "Electrical",
	})
	if err != nil {
		t.Fatalf("CreateTrade: %v", err)
	}
	if got.Code != "ELEC" {
		t.Errorf("Code = %q, want ELEC", got.Code)
	}
}

func TestSetupService_CreateTrade_RejectsBadCode(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	_, err := svc.CreateTrade(context.Background(), CreateTradeInput{
		OrgID: orgID,
		Code:  "elec/plumbing", // slash not allowed
		Name:  "x",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestSetupService_CreateTrade_DuplicateCode_MapsToInvalidInput(t *testing.T) {
	// UNIQUE(org_id, code) at the DB layer becomes ErrInvalidInput
	// at the service layer (via mapSetupStoreError) so the HTTP handler
	// returns 409/422, not 500.
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	if _, err := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"}); err != nil {
		t.Fatalf("first trade: %v", err)
	}
	_, err := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Dup"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput on duplicate code", err)
	}
}

func TestSetupService_CreateCostCode_AcceptsCSIFormat(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	got, err := svc.CreateCostCode(context.Background(), CreateCostCodeInput{
		OrgID:    orgID,
		Code:     "03-30-00",
		Name:     "Cast-in-Place Concrete",
		Division: "03 Concrete",
	})
	if err != nil {
		t.Fatalf("CreateCostCode: %v", err)
	}
	if got.Code != "03-30-00" || got.Division != "03 Concrete" {
		t.Errorf("got = %+v", got)
	}
}

func TestSetupService_CreateCostCode_RejectsBadFormat(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	cases := []string{"3-30-00", "03-3-00", "03_30_00", "concrete", "03-30-00-99"}
	for _, code := range cases {
		_, err := svc.CreateCostCode(context.Background(), CreateCostCodeInput{
			OrgID:    orgID,
			Code:     code,
			Name:     "x",
			Division: "03 Concrete",
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("code %q: err = %v, want ErrInvalidInput", code, err)
		}
	}
}

// TestSetupService_CreateCostCode_ValidationGuards covers the three
// pre-tx input guards the AcceptsCSIFormat/RejectsBadFormat tests miss:
// a nil org, a blank (after-trim) name, and a blank division — each
// short-circuits with ErrInvalidInput before the code reaches the
// guardNotComplete tx, so no row is written.
func TestSetupService_CreateCostCode_ValidationGuards(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	cases := []struct {
		name string
		in   CreateCostCodeInput
	}{
		{"nil org", CreateCostCodeInput{OrgID: uuid.Nil, Code: "03-30-00", Name: "x", Division: "03"}},
		{"empty name", CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "   ", Division: "03"}},
		{"empty division", CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "x", Division: "  "}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.CreateCostCode(ctx, c.in); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestSetupService_CreateCalendar_HappyPath(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	got, err := svc.CreateCalendar(context.Background(), CreateCalendarInput{
		OrgID:           orgID,
		Name:            "Default",
		Timezone:        "America/New_York",
		WorkingDaysMask: models.WorkingDaysMonFri,
		IsDefault:       true,
	})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	if !got.IsDefault {
		t.Error("IsDefault = false")
	}
	if got.WorkingDaysMask != models.WorkingDaysMonFri {
		t.Errorf("WorkingDaysMask = %d, want %d", got.WorkingDaysMask, models.WorkingDaysMonFri)
	}
	if got.DailyWorkMinutes != 480 {
		t.Errorf("DailyWorkMinutes = %d, want 480 (default)", got.DailyWorkMinutes)
	}
}

func TestSetupService_CreateCalendar_RejectsBadTimezone(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	_, err := svc.CreateCalendar(context.Background(), CreateCalendarInput{
		OrgID:    orgID,
		Name:     "Default",
		Timezone: "America/Hartford", // not a real TZ
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestSetupService_CreateCalendar_TwoDefaults_SecondFails(t *testing.T) {
	// Partial UNIQUE index idx_working_calendars_org_default_unique
	// rejects a second default calendar for the same org.
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	if _, err := svc.CreateCalendar(ctx, CreateCalendarInput{
		OrgID: orgID, Name: "First", IsDefault: true,
	}); err != nil {
		t.Fatalf("first calendar: %v", err)
	}
	_, err := svc.CreateCalendar(ctx, CreateCalendarInput{
		OrgID: orgID, Name: "Second", IsDefault: true,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput on duplicate default", err)
	}
}

func TestSetupService_AddHoliday_TruncatesToDate(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	cal, err := svc.CreateCalendar(ctx, CreateCalendarInput{OrgID: orgID, Name: "Default", IsDefault: true})
	if err != nil {
		t.Fatalf("create cal: %v", err)
	}
	july4 := time.Date(2026, time.July, 4, 14, 30, 0, 0, time.UTC) // mid-day
	got, err := svc.AddHoliday(ctx, AddHolidayInput{
		OrgID:       orgID,
		CalendarID:  cal.ID,
		HolidayDate: july4,
		Name:        "Independence Day",
	})
	if err != nil {
		t.Fatalf("AddHoliday: %v", err)
	}
	if got.HolidayDate.Hour() != 0 || got.HolidayDate.Minute() != 0 {
		t.Errorf("HolidayDate = %v, want midnight UTC", got.HolidayDate)
	}
	if got.HolidayDate.Year() != 2026 || got.HolidayDate.Month() != time.July || got.HolidayDate.Day() != 4 {
		t.Errorf("HolidayDate = %v, want 2026-07-04", got.HolidayDate)
	}
}

// TestSetupService_AddHoliday_ValidationGuards covers the three input
// guards that short-circuit before the tx opens — the legs the
// TruncatesToDate happy path never reaches: missing org/calendar id,
// a blank (after trim) name, and a zero holiday date.
func TestSetupService_AddHoliday_ValidationGuards(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	calID := uuid.New()
	someDate := time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC)

	cases := map[string]AddHolidayInput{
		"nil org":      {CalendarID: calID, HolidayDate: someDate, Name: "Independence Day"},
		"nil calendar": {OrgID: orgID, HolidayDate: someDate, Name: "Independence Day"},
		"blank name":   {OrgID: orgID, CalendarID: calID, HolidayDate: someDate, Name: "   "},
		"zero date":    {OrgID: orgID, CalendarID: calID, Name: "Independence Day"},
	}
	for name, in := range cases {
		if _, err := svc.AddHoliday(ctx, in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestSetupService_AddJurisdiction_RejectsInvalidJSON(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	_, err := svc.AddJurisdiction(context.Background(), AddJurisdictionInput{
		OrgID:       orgID,
		Name:        "Hartford CT",
		PermitTypes: []byte(`{not valid json`),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// TestSetupService_AddJurisdiction_HappyPath drives the step-6 write tx
// end-to-end: a fully-populated jurisdiction (region, both JSONB blobs,
// notes) round-trips back through the returned model with the name
// trimmed. The leading/trailing whitespace on Name proves the TrimSpace
// guard runs before the insert.
func TestSetupService_AddJurisdiction_HappyPath(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	got, err := svc.AddJurisdiction(context.Background(), AddJurisdictionInput{
		OrgID:               orgID,
		UserSub:             "owner-1",
		Name:                "  Hartford CT  ",
		Region:              strPtr("US-CT"),
		PermitTypes:         []byte(`["building","electrical"]`),
		InspectionChecklist: []byte(`["rough-in","final"]`),
		Notes:               strPtr("County office closes at noon Fridays."),
	})
	if err != nil {
		t.Fatalf("AddJurisdiction: %v", err)
	}
	if got.ID == uuid.Nil {
		t.Error("ID is nil, want a generated uuid")
	}
	if got.OrgID != orgID {
		t.Errorf("OrgID = %s, want %s", got.OrgID, orgID)
	}
	if got.Name != "Hartford CT" {
		t.Errorf("Name = %q, want trimmed %q", got.Name, "Hartford CT")
	}
	if got.Region == nil || *got.Region != "US-CT" {
		t.Errorf("Region = %v, want US-CT", got.Region)
	}
	if !json.Valid(got.PermitTypes) || string(got.PermitTypes) != `["building", "electrical"]` {
		t.Errorf("PermitTypes = %s", got.PermitTypes)
	}
	if got.Notes == nil || *got.Notes == "" {
		t.Errorf("Notes = %v, want the seeded note", got.Notes)
	}
}

// TestSetupService_AddJurisdiction_ValidationGuards covers the input
// guards that short-circuit before the tx opens: nil org, blank (after
// trim) name, and the inspection_checklist JSON validity check (the
// permit_types leg is already covered by RejectsInvalidJSON above).
func TestSetupService_AddJurisdiction_ValidationGuards(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	cases := map[string]AddJurisdictionInput{
		"nil org":             {Name: "Hartford CT"},
		"blank name":          {OrgID: orgID, Name: "   "},
		"bad inspection json": {OrgID: orgID, Name: "Hartford CT", InspectionChecklist: []byte(`{nope`)},
	}
	for name, in := range cases {
		if _, err := svc.AddJurisdiction(ctx, in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

// TestSetupService_AddJurisdiction_RejectsAfterComplete proves the
// guardNotComplete leg inside the tx: once the wizard is finalized, a
// late jurisdiction write is rejected with ErrSetupAlreadyComplete
// rather than silently mutating a configured org.
func TestSetupService_AddJurisdiction_RejectsAfterComplete(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()

	mustStep := func(name string, fn func() error) {
		t.Helper()
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	mustStep("company info", func() error {
		_, e := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")})
		return e
	})
	mustStep("trade", func() error {
		_, e := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"})
		return e
	})
	mustStep("cost code", func() error {
		_, e := svc.CreateCostCode(ctx, CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "x", Division: "03"})
		return e
	})
	mustStep("calendar", func() error {
		_, e := svc.CreateCalendar(ctx, CreateCalendarInput{OrgID: orgID, Name: "Default", WorkingDaysMask: models.WorkingDaysMonFri, IsDefault: true})
		return e
	})
	mustStep("complete", func() error {
		_, e := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID, UserSub: "owner-1"})
		return e
	})

	_, err := svc.AddJurisdiction(ctx, AddJurisdictionInput{OrgID: orgID, Name: "Hartford CT"})
	if !errors.Is(err, ErrSetupAlreadyComplete) {
		t.Fatalf("err = %v, want ErrSetupAlreadyComplete", err)
	}
}

func TestSetupService_Complete_RejectsIncompletePrereqs(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	// Step 1 only — no trades, codes, calendar.
	if _, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")}); err != nil {
		t.Fatalf("update company: %v", err)
	}
	_, err := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput on missing prereqs", err)
	}
}

// TestSetupService_Complete_PrereqLegs walks the four in-tx
// minimum-prerequisite checks one at a time by adding exactly the
// missing piece between calls — each Complete fails with ErrInvalidInput
// on the *next* unmet requirement, exercising the legal-name / trade /
// cost-code / default-calendar legs individually (the happy-path test
// only ever sees them all satisfied). The nil-org guard is checked too.
func TestSetupService_Complete_PrereqLegs(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()

	mustFail := func(stage string) {
		t.Helper()
		if _, err := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: err = %v, want ErrInvalidInput", stage, err)
		}
	}

	// Fresh org: no legal_name yet.
	mustFail("no legal name")

	// +legal_name → next missing is a trade.
	if _, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")}); err != nil {
		t.Fatalf("update company: %v", err)
	}
	mustFail("no trade")

	// +trade → next missing is a cost code.
	if _, err := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"}); err != nil {
		t.Fatalf("trade: %v", err)
	}
	mustFail("no cost code")

	// +cost code → next missing is a default working calendar.
	if _, err := svc.CreateCostCode(ctx, CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "x", Division: "03"}); err != nil {
		t.Fatalf("cost code: %v", err)
	}
	mustFail("no default calendar")

	// nil org short-circuits before the tx.
	if _, err := svc.Complete(ctx, CompleteSetupInput{OrgID: uuid.Nil}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil org: err = %v, want ErrInvalidInput", err)
	}
}

func TestSetupService_Complete_HappyPathAndIdempotent(t *testing.T) {
	now := time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC)
	svc, orgID := newSetupService(t, fixedClock(now))
	ctx := context.Background()

	mustStep := func(name string, fn func() error) {
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	mustStep("company info", func() error {
		_, e := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")})
		return e
	})
	mustStep("trade", func() error {
		_, e := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"})
		return e
	})
	mustStep("cost code", func() error {
		_, e := svc.CreateCostCode(ctx, CreateCostCodeInput{
			OrgID: orgID, Code: "03-30-00", Name: "Cast-in-Place", Division: "03 Concrete",
		})
		return e
	})
	mustStep("calendar", func() error {
		_, e := svc.CreateCalendar(ctx, CreateCalendarInput{
			OrgID: orgID, Name: "Default", WorkingDaysMask: models.WorkingDaysMonFri, IsDefault: true,
		})
		return e
	})

	cp, err := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID, UserSub: "owner-1"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !cp.OnboardingComplete {
		t.Error("OnboardingComplete = false after Complete()")
	}
	if cp.OnboardingCompletedAt == nil || !cp.OnboardingCompletedAt.Equal(now) {
		t.Errorf("OnboardingCompletedAt = %v, want %v", cp.OnboardingCompletedAt, now)
	}

	// Idempotent: re-call preserves the original completion time.
	cp2, err := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID, UserSub: "owner-1"})
	if err != nil {
		t.Fatalf("second Complete: %v", err)
	}
	if !cp2.OnboardingCompletedAt.Equal(now) {
		t.Errorf("second OnboardingCompletedAt = %v, want %v", cp2.OnboardingCompletedAt, now)
	}

	// Post-completion mutations are rejected.
	_, err = svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "PLBG", Name: "Plumbing"})
	if !errors.Is(err, ErrSetupAlreadyComplete) {
		t.Fatalf("post-complete trade: err = %v, want ErrSetupAlreadyComplete", err)
	}
}

func TestSetupService_IssueBootstrapToken_ExpiresAtRespectsClock(t *testing.T) {
	now := time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC)
	svc, orgID := newSetupService(t, fixedClock(now))
	issued, err := svc.IssueBootstrapToken(context.Background(), orgID, "operator", 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if issued.Cleartext == "" {
		t.Fatal("Cleartext is empty")
	}
	if !issued.ExpiresAt.Equal(now.Add(DefaultBootstrapTokenTTL)) {
		t.Errorf("ExpiresAt = %v, want %v", issued.ExpiresAt, now.Add(DefaultBootstrapTokenTTL))
	}
}

func TestSetupService_IssueBootstrapToken_RefusesAfterComplete(t *testing.T) {
	// Driving a wizard to completion + then trying to issue a token
	// returns ErrSetupAlreadyComplete (defense-in-depth: a stale
	// operator script shouldn't be able to mint a fresh claim after
	// the org is configured).
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	if _, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")}); err != nil {
		t.Fatalf("update company: %v", err)
	}
	if _, err := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"}); err != nil {
		t.Fatalf("trade: %v", err)
	}
	if _, err := svc.CreateCostCode(ctx, CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "x", Division: "03"}); err != nil {
		t.Fatalf("cost: %v", err)
	}
	if _, err := svc.CreateCalendar(ctx, CreateCalendarInput{OrgID: orgID, Name: "Default", WorkingDaysMask: models.WorkingDaysMonFri, IsDefault: true}); err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if _, err := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	_, err := svc.IssueBootstrapToken(ctx, orgID, "ops", 0)
	if !errors.Is(err, ErrSetupAlreadyComplete) {
		t.Errorf("err = %v, want ErrSetupAlreadyComplete", err)
	}
}

func TestSetupService_RedeemBootstrapToken_RoundTrip(t *testing.T) {
	now := time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC)
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	userID := uuid.New()
	testdb.SeedUser(t, pool, userID, orgID)
	svc := NewSetupService(pool, store.NewSetupStore(), NewNoopAuditRecorder(), fixedClock(now))
	ctx := context.Background()

	issued, err := svc.IssueBootstrapToken(ctx, orgID, "ops", 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := svc.RedeemBootstrapToken(ctx, issued.Cleartext, userID)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if got.OrgID != orgID {
		t.Errorf("OrgID = %s, want %s", got.OrgID, orgID)
	}

	// Second redemption fails.
	_, err = svc.RedeemBootstrapToken(ctx, issued.Cleartext, userID)
	if !errors.Is(err, ErrInvalidBootstrapToken) {
		t.Errorf("second redeem err = %v, want ErrInvalidBootstrapToken", err)
	}
}

// TestSetupService_RedeemBootstrapToken_Guards covers the two early
// input guards that short-circuit before the redeem tx: an empty
// cleartext maps to the uniform ErrInvalidBootstrapToken (never leaking
// probe info), and a nil redeemer id is an ErrInvalidInput caller fault.
func TestSetupService_RedeemBootstrapToken_Guards(t *testing.T) {
	svc, _ := newSetupService(t, nil)
	ctx := context.Background()

	if _, err := svc.RedeemBootstrapToken(ctx, "", uuid.New()); !errors.Is(err, ErrInvalidBootstrapToken) {
		t.Errorf("empty cleartext: err = %v, want ErrInvalidBootstrapToken", err)
	}
	if _, err := svc.RedeemBootstrapToken(ctx, "some-cleartext", uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil redeemer: err = %v, want ErrInvalidInput", err)
	}
}

func TestSetupService_RedeemBootstrapToken_WrongCleartext(t *testing.T) {
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	userID := uuid.New()
	testdb.SeedUser(t, pool, userID, orgID)
	svc := NewSetupService(pool, store.NewSetupStore(), NewNoopAuditRecorder(), nil)
	ctx := context.Background()

	if _, err := svc.IssueBootstrapToken(ctx, orgID, "ops", 0); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	_, err := svc.RedeemBootstrapToken(ctx, "wrong-cleartext", userID)
	if !errors.Is(err, ErrInvalidBootstrapToken) {
		t.Errorf("err = %v, want ErrInvalidBootstrapToken", err)
	}
}

func TestSetupService_RedeemBootstrapToken_ExpiredToken(t *testing.T) {
	now := time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC)
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	userID := uuid.New()
	testdb.SeedUser(t, pool, userID, orgID)

	clk := now
	svc := NewSetupService(pool, store.NewSetupStore(), NewNoopAuditRecorder(), func() time.Time { return clk })
	ctx := context.Background()

	issued, err := svc.IssueBootstrapToken(ctx, orgID, "ops", time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	clk = issued.ExpiresAt.Add(time.Second)
	_, err = svc.RedeemBootstrapToken(ctx, issued.Cleartext, userID)
	if !errors.Is(err, ErrInvalidBootstrapToken) {
		t.Errorf("expired-token redeem err = %v, want ErrInvalidBootstrapToken", err)
	}
}

func TestSetupService_IsOnboardingComplete(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()
	done, err := svc.IsOnboardingComplete(ctx, orgID)
	if err != nil {
		t.Fatalf("IsOnboardingComplete: %v", err)
	}
	if done {
		t.Error("done = true for fresh org")
	}

	// Drive the wizard to completion.
	if _, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")}); err != nil {
		t.Fatalf("update company: %v", err)
	}
	if _, err := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"}); err != nil {
		t.Fatalf("trade: %v", err)
	}
	if _, err := svc.CreateCostCode(ctx, CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "x", Division: "03"}); err != nil {
		t.Fatalf("cost code: %v", err)
	}
	if _, err := svc.CreateCalendar(ctx, CreateCalendarInput{OrgID: orgID, Name: "Default", WorkingDaysMask: models.WorkingDaysMonFri, IsDefault: true}); err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if _, err := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	done, err = svc.IsOnboardingComplete(ctx, orgID)
	if err != nil {
		t.Fatalf("IsOnboardingComplete: %v", err)
	}
	if !done {
		t.Error("done = false after Complete()")
	}
}

// TestSetupService_IsOnboardingComplete_Guards covers the two error
// legs the happy-path test above never reaches: the nil-org input guard
// and the unknown-org → store.ErrNotFound translation (the pgx.ErrNoRows
// → ErrNotFound mapping the SetupGate middleware relies on to 403 a
// request whose org has no organizations row).
func TestSetupService_IsOnboardingComplete_Guards(t *testing.T) {
	svc, _ := newSetupService(t, nil)
	ctx := context.Background()

	if _, err := svc.IsOnboardingComplete(ctx, uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.IsOnboardingComplete(ctx, uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown org: err = %v, want store.ErrNotFound", err)
	}
}

// TestSetupService_GetState_PopulatedSnapshot drives the read-only-tx
// snapshot with every wizard section filled — exercising the legs the
// FreshOrg test skips: the default-calendar-present branch (calendar +
// its holidays loaded) plus non-empty trades / cost-codes / permit
// jurisdictions. The nil-org input guard is checked too.
func TestSetupService_GetState_PopulatedSnapshot(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()

	mustStep := func(name string, fn func() error) {
		t.Helper()
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	mustStep("company info", func() error {
		_, e := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")})
		return e
	})
	mustStep("trade", func() error {
		_, e := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"})
		return e
	})
	mustStep("cost code", func() error {
		_, e := svc.CreateCostCode(ctx, CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "Cast-in-Place", Division: "03 Concrete"})
		return e
	})
	var calID uuid.UUID
	mustStep("calendar", func() error {
		cal, e := svc.CreateCalendar(ctx, CreateCalendarInput{OrgID: orgID, Name: "Default", WorkingDaysMask: models.WorkingDaysMonFri, IsDefault: true})
		calID = cal.ID
		return e
	})
	mustStep("holiday", func() error {
		_, e := svc.AddHoliday(ctx, AddHolidayInput{
			OrgID:       orgID,
			CalendarID:  calID,
			HolidayDate: time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC),
			Name:        "Independence Day",
		})
		return e
	})
	mustStep("jurisdiction", func() error {
		_, e := svc.AddJurisdiction(ctx, AddJurisdictionInput{OrgID: orgID, Name: "Hartford CT"})
		return e
	})

	got, err := svc.GetState(ctx, orgID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.OnboardingComplete {
		t.Error("OnboardingComplete = true, want false (Complete not called)")
	}
	if len(got.Trades) != 1 {
		t.Errorf("Trades len = %d, want 1", len(got.Trades))
	}
	if len(got.CostCodes) != 1 {
		t.Errorf("CostCodes len = %d, want 1", len(got.CostCodes))
	}
	if got.DefaultCalendar == nil || got.DefaultCalendar.ID != calID {
		t.Fatalf("DefaultCalendar = %v, want the seeded default", got.DefaultCalendar)
	}
	if len(got.DefaultHolidays) != 1 {
		t.Errorf("DefaultHolidays len = %d, want 1", len(got.DefaultHolidays))
	}
	if len(got.PermitJurisdictions) != 1 {
		t.Errorf("PermitJurisdictions len = %d, want 1", len(got.PermitJurisdictions))
	}

	t.Run("nil org is rejected", func(t *testing.T) {
		if _, err := svc.GetState(ctx, uuid.Nil); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})
}

func TestSetupService_AuditTrail(t *testing.T) {
	// Spot-check that audit rows DO land for wizard steps. A separate
	// recorder captures entries instead of writing to audit_log.
	rec := &capturingAuditRecorder{}
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	svc := NewSetupService(pool, store.NewSetupStore(), rec, nil)

	ctx := context.Background()
	if _, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{
		OrgID: orgID, UserSub: "operator", LegalName: strPtr("Kelbrook LLC"),
	}); err != nil {
		t.Fatalf("update company: %v", err)
	}

	if len(rec.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(rec.entries))
	}
	e := rec.entries[0]
	if e.Action != AuditActionSetupCompanyInfo {
		t.Errorf("Action = %q, want %q", e.Action, AuditActionSetupCompanyInfo)
	}
	if e.UserSub != "operator" {
		t.Errorf("UserSub = %q, want operator", e.UserSub)
	}
	// After-blob is the patch JSON.
	var after map[string]any
	if err := json.Unmarshal(e.After, &after); err != nil {
		t.Fatalf("unmarshal After: %v", err)
	}
	if after["legal_name"] != "Kelbrook LLC" {
		t.Errorf("After.legal_name = %v, want Kelbrook LLC", after["legal_name"])
	}
}

// capturingAuditRecorder is an in-memory AuditRecorder used to verify
// audit trail content without touching the audit_log table. Not safe
// for concurrent use — the wizard service runs sequentially per request.
type capturingAuditRecorder struct {
	entries []AuditEntry
}

func (c *capturingAuditRecorder) Record(_ context.Context, _ pgx.Tx, e AuditEntry) {
	c.entries = append(c.entries, e)
}

// validBootstrapCleartext returns a well-formed 43-char base64url
// cleartext (RawURLEncoding of bootstrapTokenByteLen bytes) — the exact
// shape SeedBootstrapTokenIfNeeded validates before hashing.
func validBootstrapCleartext() string {
	b := make([]byte, bootstrapTokenByteLen)
	for i := range b {
		b[i] = byte(i + 1)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func TestSetupService_SeedBootstrapTokenIfNeeded_ValidationShortCircuits(t *testing.T) {
	// The three format guards return before any DB work, so a bad env
	// var fails fast at boot rather than at first redeem.
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()

	// Empty cleartext: not configured → silent no-op, never touches DB.
	if seeded, err := svc.SeedBootstrapTokenIfNeeded(ctx, "", orgID, 0); err != nil || seeded {
		t.Fatalf("empty cleartext = (%v, %v), want (false, nil)", seeded, err)
	}
	// Wrong length: rejected before hashing.
	if _, err := svc.SeedBootstrapTokenIfNeeded(ctx, "too-short", orgID, 0); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("short cleartext err = %v, want ErrInvalidInput", err)
	}
	// Right length (43) but not base64url ('!' is outside the alphabet).
	bad := "!" + validBootstrapCleartext()[1:]
	if _, err := svc.SeedBootstrapTokenIfNeeded(ctx, bad, orgID, 0); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("non-base64url cleartext err = %v, want ErrInvalidInput", err)
	}
}

func TestSetupService_SeedBootstrapTokenIfNeeded_SeedsAndIsIdempotent(t *testing.T) {
	rec := &capturingAuditRecorder{}
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	userID := uuid.New()
	testdb.SeedUser(t, pool, userID, orgID)
	svc := NewSetupService(pool, store.NewSetupStore(), rec, nil)
	ctx := context.Background()

	cleartext := validBootstrapCleartext()

	// First call lands a fresh row. ttl=0 exercises the
	// DefaultBootstrapTokenTTL fallback.
	seeded, err := svc.SeedBootstrapTokenIfNeeded(ctx, cleartext, orgID, 0)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if !seeded {
		t.Fatal("first seed = false, want true (fresh row)")
	}
	// Audit captured a system-boot issue entry for the org.
	if len(rec.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(rec.entries))
	}
	if e := rec.entries[0]; e.Action != AuditActionSetupBootstrapIssue || e.UserSub != "system:boot" || e.OrgID != orgID {
		t.Errorf("audit entry = %+v, want issue/system:boot/%s", e, orgID)
	}

	// Second call with the same cleartext is a no-op (UNIQUE(token_hash)
	// swallows the re-insert) and records no new audit row.
	seeded, err = svc.SeedBootstrapTokenIfNeeded(ctx, cleartext, orgID, 0)
	if err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if seeded {
		t.Error("re-seed = true, want false (idempotent)")
	}
	if len(rec.entries) != 1 {
		t.Errorf("audit entries after re-seed = %d, want 1", len(rec.entries))
	}

	// The seeded cleartext is a genuine, redeemable token — proves the
	// hash landed correctly, not just that a row exists.
	got, err := svc.RedeemBootstrapToken(ctx, cleartext, userID)
	if err != nil {
		t.Fatalf("redeem seeded token: %v", err)
	}
	if got.OrgID != orgID {
		t.Errorf("redeemed OrgID = %s, want %s", got.OrgID, orgID)
	}
}

func TestSetupService_SeedBootstrapTokenIfNeeded_NilOrgPicksIncompleteOrg(t *testing.T) {
	// cmd/server doesn't always know the fork's org_id at boot; uuid.Nil
	// resolves the single onboarding-incomplete org.
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	userID := uuid.New()
	testdb.SeedUser(t, pool, userID, orgID)
	svc := NewSetupService(pool, store.NewSetupStore(), NewNoopAuditRecorder(), nil)
	ctx := context.Background()

	cleartext := validBootstrapCleartext()
	seeded, err := svc.SeedBootstrapTokenIfNeeded(ctx, cleartext, uuid.Nil, time.Hour)
	if err != nil {
		t.Fatalf("seed with nil org: %v", err)
	}
	if !seeded {
		t.Fatal("seeded = false, want true")
	}
	// Redeeming proves the row resolved to the seeded (incomplete) org.
	got, err := svc.RedeemBootstrapToken(ctx, cleartext, userID)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if got.OrgID != orgID {
		t.Errorf("resolved OrgID = %s, want %s", got.OrgID, orgID)
	}
}

func TestSetupService_SeedBootstrapTokenIfNeeded_CompleteOrgIsNoOp(t *testing.T) {
	svc, orgID := newSetupService(t, nil)
	ctx := context.Background()

	// Drive the wizard to completion so onboarding_complete = true.
	if _, err := svc.UpdateCompanyInfo(ctx, UpdateCompanyInfoInput{OrgID: orgID, LegalName: strPtr("Kelbrook LLC")}); err != nil {
		t.Fatalf("update company: %v", err)
	}
	if _, err := svc.CreateTrade(ctx, CreateTradeInput{OrgID: orgID, Code: "ELEC", Name: "Electrical"}); err != nil {
		t.Fatalf("trade: %v", err)
	}
	if _, err := svc.CreateCostCode(ctx, CreateCostCodeInput{OrgID: orgID, Code: "03-30-00", Name: "x", Division: "03"}); err != nil {
		t.Fatalf("cost code: %v", err)
	}
	if _, err := svc.CreateCalendar(ctx, CreateCalendarInput{OrgID: orgID, Name: "Default", WorkingDaysMask: models.WorkingDaysMonFri, IsDefault: true}); err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if _, err := svc.Complete(ctx, CompleteSetupInput{OrgID: orgID}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	cleartext := validBootstrapCleartext()

	// Explicit orgID whose onboarding is done → GetCompanyProfile sees
	// OnboardingComplete and the seed is a no-op.
	if seeded, err := svc.SeedBootstrapTokenIfNeeded(ctx, cleartext, orgID, time.Hour); err != nil || seeded {
		t.Errorf("seed against complete org = (%v, %v), want (false, nil)", seeded, err)
	}
	// uuid.Nil with no incomplete org left → the ORDER BY query returns
	// no rows and the seed is a no-op.
	if seeded, err := svc.SeedBootstrapTokenIfNeeded(ctx, cleartext, uuid.Nil, time.Hour); err != nil || seeded {
		t.Errorf("nil-org seed with all orgs onboarded = (%v, %v), want (false, nil)", seeded, err)
	}
}
