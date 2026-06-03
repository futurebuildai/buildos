//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// strPtr returns a pointer to the given string. Used by table-driven
// tests where the optional-field shape needs distinct values per row.
func strPtr(s string) *string { return &s }

// ---------- company profile ----------

func TestSetupStore_UpdateAndGetCompanyProfile(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.UpdateCompanyProfile(ctx, tx, UpdateCompanyProfileParams{
			OrgID:       orgID,
			LegalName:   strPtr("Kelbrook Construction LLC"),
			Address:     strPtr("123 Main St, Hartford, CT 06103"),
			EIN:         strPtr("12-3456789"),
			CompanyType: strPtr("general_contractor"),
			Region:      strPtr("US-CT"),
		}); err != nil {
			return err
		}
		got, err := s.GetCompanyProfile(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if got.LegalName == nil || *got.LegalName != "Kelbrook Construction LLC" {
			t.Errorf("legal_name = %v", got.LegalName)
		}
		if got.Region == nil || *got.Region != "US-CT" {
			t.Errorf("region = %v", got.Region)
		}
		if got.OnboardingComplete {
			t.Error("onboarding_complete should default to false")
		}
		if got.OnboardingCompletedAt != nil {
			t.Errorf("onboarding_completed_at = %v, want nil", got.OnboardingCompletedAt)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestSetupStore_UpdateCompanyProfile_PartialPatch(t *testing.T) {
	// Wizard-resume behavior: a second submit with a subset of fields
	// must not clobber values from the first submit. COALESCE in the
	// SQL is the contract; this pins it.
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.UpdateCompanyProfile(ctx, tx, UpdateCompanyProfileParams{
			OrgID:     orgID,
			LegalName: strPtr("Kelbrook Construction LLC"),
			Region:    strPtr("US-CT"),
		}); err != nil {
			return err
		}
		// Second submit only sets Address; legal_name + region must survive.
		if err := s.UpdateCompanyProfile(ctx, tx, UpdateCompanyProfileParams{
			OrgID:   orgID,
			Address: strPtr("123 Main St, Hartford, CT 06103"),
		}); err != nil {
			return err
		}
		got, err := s.GetCompanyProfile(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if got.LegalName == nil || *got.LegalName != "Kelbrook Construction LLC" {
			t.Errorf("legal_name lost on partial patch: %v", got.LegalName)
		}
		if got.Region == nil || *got.Region != "US-CT" {
			t.Errorf("region lost on partial patch: %v", got.Region)
		}
		if got.Address == nil || *got.Address != "123 Main St, Hartford, CT 06103" {
			t.Errorf("address = %v", got.Address)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestSetupStore_UpdateCompanyProfile_UnknownOrg_ReturnsNotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return s.UpdateCompanyProfile(ctx, tx, UpdateCompanyProfileParams{
			OrgID:     uuid.New(),
			LegalName: strPtr("Nope LLC"),
		})
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetupStore_CompleteOnboarding_FlipsFlagAndStampsTime(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.CompleteOnboarding(ctx, tx, orgID, now); err != nil {
			return err
		}
		got, err := s.GetCompanyProfile(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if !got.OnboardingComplete {
			t.Error("onboarding_complete should be true")
		}
		if got.OnboardingCompletedAt == nil || !got.OnboardingCompletedAt.Equal(now) {
			t.Errorf("onboarding_completed_at = %v, want %v", got.OnboardingCompletedAt, now)
		}

		// Second call must not advance completed_at (idempotent contract).
		later := now.Add(24 * time.Hour)
		if err := s.CompleteOnboarding(ctx, tx, orgID, later); err != nil {
			return err
		}
		got2, err := s.GetCompanyProfile(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if !got2.OnboardingCompletedAt.Equal(now) {
			t.Errorf("completed_at advanced on second call: %v, want %v", got2.OnboardingCompletedAt, now)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

// TestSetupStore_NotFoundPaths covers the ErrNotFound (no-rows) leg of the
// three setup readers/mutators whose happy path is covered above but whose
// not-found short-circuit was unreached: GetCompanyProfile + CompleteOnboarding
// against an unknown org (QueryRow → pgx.ErrNoRows / Exec → RowsAffected()==0),
// and GetDefaultWorkingCalendar against an org with no default calendar set
// (wizard step 5 incomplete). These are deterministic legs — querying a
// non-existent org/calendar always returns no rows. (UpdateCompanyProfile's
// not-found leg is pinned separately above.)
func TestSetupStore_NotFoundPaths(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	// Seed a real org with NO default working calendar so the
	// GetDefaultWorkingCalendar miss is "exists-but-no-default", not
	// "org missing" — the more representative wizard-in-progress case.
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Halfway House")
	missing := uuid.New() // org never inserted

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, e := s.GetCompanyProfile(ctx, tx, missing); !errors.Is(e, ErrNotFound) {
			t.Errorf("GetCompanyProfile(missing org) = %v, want ErrNotFound", e)
		}
		if e := s.CompleteOnboarding(ctx, tx, missing, now); !errors.Is(e, ErrNotFound) {
			t.Errorf("CompleteOnboarding(missing org) = %v, want ErrNotFound", e)
		}
		if _, e := s.GetDefaultWorkingCalendar(ctx, tx, orgID); !errors.Is(e, ErrNotFound) {
			t.Errorf("GetDefaultWorkingCalendar(no default) = %v, want ErrNotFound", e)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("not-found tx: %v", err)
	}
}

// ---------- bootstrap tokens ----------

func TestSetupStore_BootstrapToken_LookupActive(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	now := time.Now().UTC()
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		id, err := s.CreateBootstrapToken(ctx, tx, CreateBootstrapTokenParams{
			OrgID:     orgID,
			TokenHash: "hash-abc",
			ExpiresAt: now.Add(24 * time.Hour),
		})
		if err != nil {
			return err
		}
		got, err := s.GetActiveBootstrapTokenByHash(ctx, tx, "hash-abc", now)
		if err != nil {
			return err
		}
		if got.ID != id {
			t.Errorf("lookup id = %s, want %s", got.ID, id)
		}
		if got.OrgID != orgID {
			t.Errorf("lookup org = %s, want %s", got.OrgID, orgID)
		}
		if !got.IsActive(now) {
			t.Error("freshly-issued token should be active")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestSetupStore_BootstrapToken_LookupExpired_ReturnsNotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	now := time.Now().UTC()
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := s.CreateBootstrapToken(ctx, tx, CreateBootstrapTokenParams{
			OrgID:     orgID,
			TokenHash: "hash-expired",
			ExpiresAt: now.Add(-1 * time.Hour),
		}); err != nil {
			return err
		}
		_, err := s.GetActiveBootstrapTokenByHash(ctx, tx, "hash-expired", now)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestSetupStore_BootstrapToken_Redeem_OneShot(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")
	testdb.SeedUser(t, pool, userID, orgID)

	now := time.Now().UTC()
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		id, err := s.CreateBootstrapToken(ctx, tx, CreateBootstrapTokenParams{
			OrgID:     orgID,
			TokenHash: "hash-one-shot",
			ExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			return err
		}
		// First redemption succeeds.
		if err := s.RedeemBootstrapToken(ctx, tx, id, userID, now); err != nil {
			return err
		}
		// Lookup must now miss — redeemed_at is set.
		if _, err := s.GetActiveBootstrapTokenByHash(ctx, tx, "hash-one-shot", now); !errors.Is(err, ErrNotFound) {
			t.Errorf("post-redemption lookup err = %v, want ErrNotFound", err)
		}
		// Second redemption must miss (one-shot contract).
		if err := s.RedeemBootstrapToken(ctx, tx, id, userID, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("second redemption err = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

// ---------- trade categories ----------

func TestSetupStore_TradeCategories_CreateAndListOrdered(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Insert in a non-sorted order to verify the list orders by
		// (is_default DESC, name ASC).
		if _, err := s.CreateTradeCategory(ctx, tx, CreateTradeCategoryParams{
			OrgID: orgID, Code: "CUST", Name: "Custom Trade", IsDefault: false,
		}); err != nil {
			return err
		}
		if _, err := s.CreateTradeCategory(ctx, tx, CreateTradeCategoryParams{
			OrgID: orgID, Code: "ELEC", Name: "Electrical", IsDefault: true,
		}); err != nil {
			return err
		}
		if _, err := s.CreateTradeCategory(ctx, tx, CreateTradeCategoryParams{
			OrgID: orgID, Code: "PLBG", Name: "Plumbing", IsDefault: true,
		}); err != nil {
			return err
		}

		got, err := s.ListTradeCategories(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		// is_default DESC first → defaults at top, sorted by name.
		if got[0].Name != "Electrical" || got[1].Name != "Plumbing" || got[2].Name != "Custom Trade" {
			t.Errorf("ordering wrong: %s, %s, %s", got[0].Name, got[1].Name, got[2].Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestSetupStore_TradeCategories_DuplicateCode_FailsUniqueConstraint(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	// Use a separate tx for each insert so the second's failure doesn't
	// poison the first.
	if err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.CreateTradeCategory(ctx, tx, CreateTradeCategoryParams{
			OrgID: orgID, Code: "ELEC", Name: "Electrical",
		})
		return err
	}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.CreateTradeCategory(ctx, tx, CreateTradeCategoryParams{
			OrgID: orgID, Code: "ELEC", Name: "Electrical (dup)",
		})
		return err
	})
	if err == nil {
		t.Fatal("duplicate code must violate UNIQUE(org_id, code)")
	}
}

// ---------- cost codes ----------

func TestSetupStore_CostCodes_OrderedByCodeAscending(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Insert MasterFormat codes out of natural order.
		for _, c := range []struct {
			Code, Name, Division string
		}{
			{"26-05-00", "Common Work Results for Electrical", "26 Electrical"},
			{"03-30-00", "Cast-in-Place Concrete", "03 Concrete"},
			{"01-50-00", "Temporary Facilities and Controls", "01 General Requirements"},
		} {
			if _, err := s.CreateCostCode(ctx, tx, CreateCostCodeParams{
				OrgID: orgID, Code: c.Code, Name: c.Name, Division: c.Division, IsDefault: true,
			}); err != nil {
				return err
			}
		}

		got, err := s.ListCostCodes(ctx, tx, orgID)
		if err != nil {
			return err
		}
		want := []string{"01-50-00", "03-30-00", "26-05-00"}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i, c := range got {
			if c.Code != want[i] {
				t.Errorf("got[%d] = %q, want %q", i, c.Code, want[i])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

// ---------- working calendars ----------

func TestSetupStore_WorkingCalendars_DefaultAndIsWorkingDay(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		cal, err := s.CreateWorkingCalendar(ctx, tx, CreateWorkingCalendarParams{
			OrgID:            orgID,
			Name:             "Hartford Standard",
			Timezone:         "America/New_York",
			WorkingDaysMask:  models.WorkingDaysMonFri,
			DailyWorkMinutes: 480,
			IsDefault:        true,
		})
		if err != nil {
			return err
		}
		got, err := s.GetDefaultWorkingCalendar(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if got.ID != cal.ID {
			t.Errorf("default id = %s, want %s", got.ID, cal.ID)
		}
		// Mon-Fri working, Sat-Sun off.
		for _, d := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday} {
			if !got.IsWorkingDay(d) {
				t.Errorf("%s should be a working day", d)
			}
		}
		for _, d := range []time.Weekday{time.Saturday, time.Sunday} {
			if got.IsWorkingDay(d) {
				t.Errorf("%s should NOT be a working day", d)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestSetupStore_WorkingCalendars_TwoDefaultsRejected(t *testing.T) {
	// The partial unique index enforces at-most-one default per org.
	// Service must clear the prior default before promoting another;
	// this test pins the SQL guard.
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	if err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.CreateWorkingCalendar(ctx, tx, CreateWorkingCalendarParams{
			OrgID:            orgID,
			Name:             "First",
			Timezone:         "America/New_York",
			WorkingDaysMask:  models.WorkingDaysMonFri,
			DailyWorkMinutes: 480,
			IsDefault:        true,
		})
		return err
	}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.CreateWorkingCalendar(ctx, tx, CreateWorkingCalendarParams{
			OrgID:            orgID,
			Name:             "Second",
			Timezone:         "America/New_York",
			WorkingDaysMask:  models.WorkingDaysMonFri,
			DailyWorkMinutes: 480,
			IsDefault:        true,
		})
		return err
	})
	if err == nil {
		t.Fatal("second default in same org must violate unique index")
	}
}

func TestSetupStore_HolidayOverrides_ListedAscending(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		cal, err := s.CreateWorkingCalendar(ctx, tx, CreateWorkingCalendarParams{
			OrgID:            orgID,
			Name:             "Hartford Standard",
			Timezone:         "America/New_York",
			WorkingDaysMask:  models.WorkingDaysMonFri,
			DailyWorkMinutes: 480,
			IsDefault:        true,
		})
		if err != nil {
			return err
		}

		// Insert 2026 federal holidays out of order.
		dates := []struct {
			Date time.Time
			Name string
		}{
			{time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), "Independence Day"},
			{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "New Year's Day"},
			{time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC), "Christmas Day"},
		}
		for _, d := range dates {
			if _, err := s.CreateHolidayOverride(ctx, tx, CreateHolidayOverrideParams{
				CalendarID: cal.ID, OrgID: orgID, HolidayDate: d.Date, Name: d.Name,
			}); err != nil {
				return err
			}
		}

		got, err := s.ListHolidaysForCalendar(ctx, tx, cal.ID)
		if err != nil {
			return err
		}
		want := []string{"New Year's Day", "Independence Day", "Christmas Day"}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i, h := range got {
			if h.Name != want[i] {
				t.Errorf("got[%d] = %q, want %q", i, h.Name, want[i])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestSetupStore_HolidayOverrides_DuplicateDateRejected(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	var calID uuid.UUID
	if err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		cal, err := s.CreateWorkingCalendar(ctx, tx, CreateWorkingCalendarParams{
			OrgID: orgID, Name: "Std", Timezone: "UTC",
			WorkingDaysMask: models.WorkingDaysMonFri, DailyWorkMinutes: 480, IsDefault: true,
		})
		if err != nil {
			return err
		}
		calID = cal.ID
		_, err = s.CreateHolidayOverride(ctx, tx, CreateHolidayOverrideParams{
			CalendarID:  cal.ID,
			OrgID:       orgID,
			HolidayDate: time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
			Name:        "Independence Day",
		})
		return err
	}); err != nil {
		t.Fatalf("first inserts: %v", err)
	}

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.CreateHolidayOverride(ctx, tx, CreateHolidayOverrideParams{
			CalendarID:  calID,
			OrgID:       orgID,
			HolidayDate: time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
			Name:        "Independence Day (dup)",
		})
		return err
	})
	if err == nil {
		t.Fatal("duplicate (calendar_id, holiday_date) must violate UNIQUE")
	}
}

// ---------- permit jurisdictions ----------

func TestSetupStore_PermitJurisdictions_StoresAndReadsBackJSONB(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewSetupStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Insert one with explicit JSONB blobs and one with nil
		// (schema default '[]').
		j1, err := s.CreatePermitJurisdiction(ctx, tx, CreatePermitJurisdictionParams{
			OrgID:               orgID,
			Name:                "Hartford CT Building Dept",
			Region:              strPtr("US-CT"),
			PermitTypes:         []byte(`["building","electrical","plumbing"]`),
			InspectionChecklist: []byte(`[{"step":"footing","required":true}]`),
			Notes:               strPtr("Online portal at hartford.gov/permits"),
		})
		if err != nil {
			return err
		}
		j2, err := s.CreatePermitJurisdiction(ctx, tx, CreatePermitJurisdictionParams{
			OrgID: orgID,
			Name:  "West Hartford CT",
		})
		if err != nil {
			return err
		}

		got, err := s.ListPermitJurisdictions(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		// ORDER BY name ASC: "Hartford CT…" before "West Hartford CT".
		if got[0].ID != j1.ID || got[1].ID != j2.ID {
			t.Errorf("ordering wrong: %v, %v", got[0].Name, got[1].Name)
		}
		if string(got[0].PermitTypes) != `["building", "electrical", "plumbing"]` &&
			string(got[0].PermitTypes) != `["building","electrical","plumbing"]` {
			// Postgres jsonb may reformat with single-space separators.
			t.Errorf("permit_types round-trip = %q", string(got[0].PermitTypes))
		}
		if string(got[1].PermitTypes) != "[]" {
			t.Errorf("default permit_types = %q, want []", string(got[1].PermitTypes))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
