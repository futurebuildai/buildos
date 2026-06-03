package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// mockSetupService implements SetupServicer for handler tests. Each
// method records its input on a captured-args field and returns the
// corresponding result/err. The pattern matches mockPipelineService.
type mockSetupService struct {
	stateResult service.SetupState
	stateErr    error

	companyResult models.CompanyProfile
	companyErr    error

	tradeResult models.TradeCategory
	tradeErr    error

	costCodeResult models.CostCode
	costCodeErr    error

	calendarResult models.WorkingCalendar
	calendarErr    error

	holidayResult models.HolidayOverride
	holidayErr    error

	jurisdictionResult models.PermitJurisdiction
	jurisdictionErr    error

	completeResult models.CompanyProfile
	completeErr    error

	// Captured args for assertions.
	lastStateOrgID        uuid.UUID
	lastCompanyInput      service.UpdateCompanyInfoInput
	lastTradeInput        service.CreateTradeInput
	lastCostCodeInput     service.CreateCostCodeInput
	lastCalendarInput     service.CreateCalendarInput
	lastHolidayInput      service.AddHolidayInput
	lastJurisdictionInput service.AddJurisdictionInput
	lastCompleteInput     service.CompleteSetupInput
}

func (m *mockSetupService) GetState(_ context.Context, orgID uuid.UUID) (service.SetupState, error) {
	m.lastStateOrgID = orgID
	return m.stateResult, m.stateErr
}
func (m *mockSetupService) UpdateCompanyInfo(_ context.Context, in service.UpdateCompanyInfoInput) (models.CompanyProfile, error) {
	m.lastCompanyInput = in
	return m.companyResult, m.companyErr
}
func (m *mockSetupService) CreateTrade(_ context.Context, in service.CreateTradeInput) (models.TradeCategory, error) {
	m.lastTradeInput = in
	return m.tradeResult, m.tradeErr
}
func (m *mockSetupService) CreateCostCode(_ context.Context, in service.CreateCostCodeInput) (models.CostCode, error) {
	m.lastCostCodeInput = in
	return m.costCodeResult, m.costCodeErr
}
func (m *mockSetupService) CreateCalendar(_ context.Context, in service.CreateCalendarInput) (models.WorkingCalendar, error) {
	m.lastCalendarInput = in
	return m.calendarResult, m.calendarErr
}
func (m *mockSetupService) AddHoliday(_ context.Context, in service.AddHolidayInput) (models.HolidayOverride, error) {
	m.lastHolidayInput = in
	return m.holidayResult, m.holidayErr
}
func (m *mockSetupService) AddJurisdiction(_ context.Context, in service.AddJurisdictionInput) (models.PermitJurisdiction, error) {
	m.lastJurisdictionInput = in
	return m.jurisdictionResult, m.jurisdictionErr
}
func (m *mockSetupService) Complete(_ context.Context, in service.CompleteSetupInput) (models.CompanyProfile, error) {
	m.lastCompleteInput = in
	return m.completeResult, m.completeErr
}
func (m *mockSetupService) IsOnboardingComplete(_ context.Context, _ uuid.UUID) (bool, error) {
	return true, nil
}

const testCalendarID = "33333333-3333-3333-3333-333333333333"

// ---------- GET /setup/state ----------

func TestSetup_State_OK(t *testing.T) {
	orgUUID := uuid.MustParse(testOrgID)
	legal := "Kelbrook Construction"
	svc := &mockSetupService{
		stateResult: service.SetupState{
			OrgID:              orgUUID,
			OnboardingComplete: false,
			CompanyProfile:     models.CompanyProfile{LegalName: &legal},
			Trades:             []models.TradeCategory{{Code: "ELEC", Name: "Electrical"}},
			CostCodes:          []models.CostCode{{Code: "03-30-00", Name: "Concrete", Division: "03"}},
		},
	}
	h := NewSetupHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/setup/state", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.State(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastStateOrgID != orgUUID {
		t.Errorf("State got org=%s, want %s", svc.lastStateOrgID, orgUUID)
	}
	var env struct {
		Data setupStateResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Data.OrgID != orgUUID || env.Data.OnboardingComplete {
		t.Errorf("state body unexpected: %+v", env.Data)
	}
	if len(env.Data.Trades) != 1 || env.Data.Trades[0].Code != "ELEC" {
		t.Errorf("trades not surfaced: %+v", env.Data.Trades)
	}
	if env.Data.PermitJurisdictions == nil {
		t.Errorf("PermitJurisdictions emitted as null; expected stable []")
	}
}

func TestSetup_State_InvalidOrgIDClaim_401(t *testing.T) {
	svc := &mockSetupService{}
	h := NewSetupHandler(svc)
	// "not-a-uuid" is an intentionally malformed claim — callerOrgIDFromClaims
	// should 401 before the service is consulted.
	r := buildRequest(t, "GET", "/api/v1/setup/state", "not-a-uuid", nil, nil)
	w := httptest.NewRecorder()
	h.State(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", w.Code)
	}
}

// ---------- POST /setup/company-info ----------

func TestSetup_CompanyInfo_HappyPath(t *testing.T) {
	legal := "Kelbrook Construction"
	svc := &mockSetupService{
		companyResult: models.CompanyProfile{LegalName: &legal},
	}
	h := NewSetupHandler(svc)
	body := strings.NewReader(`{"legal_name":"Kelbrook Construction","region":"US-CT"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/company-info", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CompanyInfo(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCompanyInput.LegalName == nil || *svc.lastCompanyInput.LegalName != "Kelbrook Construction" {
		t.Errorf("LegalName not forwarded: %+v", svc.lastCompanyInput)
	}
	if svc.lastCompanyInput.Region == nil || *svc.lastCompanyInput.Region != "US-CT" {
		t.Errorf("Region not forwarded: %+v", svc.lastCompanyInput)
	}
	if svc.lastCompanyInput.UserSub != "test-sub" {
		t.Errorf("UserSub=%q, want test-sub", svc.lastCompanyInput.UserSub)
	}
}

func TestSetup_CompanyInfo_InvalidJSON_400(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{})
	body := strings.NewReader(`not json`)
	r := buildRequest(t, "POST", "/api/v1/setup/company-info", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CompanyInfo(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestSetup_CompanyInfo_ServiceValidationError_400(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{companyErr: service.ErrInvalidInput})
	body := strings.NewReader(`{}`)
	r := buildRequest(t, "POST", "/api/v1/setup/company-info", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CompanyInfo(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ---------- POST /setup/trades ----------

func TestSetup_CreateTrade_HappyPath(t *testing.T) {
	svc := &mockSetupService{
		tradeResult: models.TradeCategory{Code: "ELEC", Name: "Electrical"},
	}
	h := NewSetupHandler(svc)
	body := strings.NewReader(`{"code":"ELEC","name":"Electrical","is_default":true}`)
	r := buildRequest(t, "POST", "/api/v1/setup/trades", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CreateTrade(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastTradeInput.Code != "ELEC" || svc.lastTradeInput.Name != "Electrical" || !svc.lastTradeInput.IsDefault {
		t.Errorf("trade input not forwarded: %+v", svc.lastTradeInput)
	}
}

func TestSetup_CreateTrade_SetupAlreadyComplete_409(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{tradeErr: service.ErrSetupAlreadyComplete})
	body := strings.NewReader(`{"code":"ELEC","name":"Electrical"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/trades", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CreateTrade(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SETUP_ALREADY_COMPLETE") {
		t.Errorf("error code missing; body=%s", w.Body.String())
	}
}

// ---------- POST /setup/cost-codes ----------

func TestSetup_CreateCostCode_HappyPath(t *testing.T) {
	svc := &mockSetupService{
		costCodeResult: models.CostCode{Code: "03-30-00", Name: "Concrete", Division: "03"},
	}
	h := NewSetupHandler(svc)
	body := strings.NewReader(`{"code":"03-30-00","name":"Concrete","division":"03","is_default":true}`)
	r := buildRequest(t, "POST", "/api/v1/setup/cost-codes", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CreateCostCode(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCostCodeInput.Code != "03-30-00" || svc.lastCostCodeInput.Division != "03" {
		t.Errorf("cost-code input not forwarded: %+v", svc.lastCostCodeInput)
	}
}

func TestSetup_CreateCostCode_InvalidCSI_400(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{costCodeErr: service.ErrInvalidInput})
	body := strings.NewReader(`{"code":"BAD","name":"x","division":"03"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/cost-codes", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CreateCostCode(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ---------- POST /setup/calendars ----------

func TestSetup_CreateCalendar_HappyPath(t *testing.T) {
	svc := &mockSetupService{
		calendarResult: models.WorkingCalendar{
			Name: "Default", Timezone: "America/New_York",
			WorkingDaysMask: models.WorkingDaysMonFri, DailyWorkMinutes: 480, IsDefault: true,
		},
	}
	h := NewSetupHandler(svc)
	body := strings.NewReader(`{"name":"Default","timezone":"America/New_York","working_days_mask":31,"daily_work_minutes":480,"is_default":true}`)
	r := buildRequest(t, "POST", "/api/v1/setup/calendars", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.CreateCalendar(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCalendarInput.Name != "Default" || svc.lastCalendarInput.WorkingDaysMask != 31 || !svc.lastCalendarInput.IsDefault {
		t.Errorf("calendar input not forwarded: %+v", svc.lastCalendarInput)
	}
}

// ---------- POST /setup/calendars/{calendarID}/holidays ----------

func TestSetup_AddHoliday_HappyPath_YMD(t *testing.T) {
	svc := &mockSetupService{
		holidayResult: models.HolidayOverride{Name: "Independence Day"},
	}
	h := NewSetupHandler(svc)
	body := strings.NewReader(`{"holiday_date":"2026-07-04","name":"Independence Day"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/calendars/"+testCalendarID+"/holidays", testOrgID,
		map[string]string{"calendarID": testCalendarID}, body)
	w := httptest.NewRecorder()
	h.AddHoliday(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastHolidayInput.CalendarID.String() != testCalendarID {
		t.Errorf("calendarID not parsed from URL: got %s", svc.lastHolidayInput.CalendarID)
	}
	if svc.lastHolidayInput.HolidayDate.Year() != 2026 ||
		svc.lastHolidayInput.HolidayDate.Month() != 7 ||
		svc.lastHolidayInput.HolidayDate.Day() != 4 {
		t.Errorf("HolidayDate parse wrong: %v", svc.lastHolidayInput.HolidayDate)
	}
}

func TestSetup_AddHoliday_BadDate_400(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{})
	body := strings.NewReader(`{"holiday_date":"yesterday","name":"Bad"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/calendars/"+testCalendarID+"/holidays", testOrgID,
		map[string]string{"calendarID": testCalendarID}, body)
	w := httptest.NewRecorder()
	h.AddHoliday(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestSetup_AddHoliday_BadCalendarID_400(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{})
	body := strings.NewReader(`{"holiday_date":"2026-07-04","name":"x"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/calendars/not-a-uuid/holidays", testOrgID,
		map[string]string{"calendarID": "not-a-uuid"}, body)
	w := httptest.NewRecorder()
	h.AddHoliday(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ---------- POST /setup/jurisdictions ----------

func TestSetup_AddJurisdiction_HappyPath_RawJSONB(t *testing.T) {
	region := "US-CT"
	svc := &mockSetupService{
		jurisdictionResult: models.PermitJurisdiction{
			Name:                "Town of Glastonbury",
			Region:              &region,
			PermitTypes:         []byte(`["building","electrical"]`),
			InspectionChecklist: []byte(`[{"step":"framing"}]`),
		},
	}
	h := NewSetupHandler(svc)
	body := strings.NewReader(`{"name":"Town of Glastonbury","region":"US-CT","permit_types":["building","electrical"],"inspection_checklist":[{"step":"framing"}]}`)
	r := buildRequest(t, "POST", "/api/v1/setup/jurisdictions", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.AddJurisdiction(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	// PermitTypes should land at the service as raw JSON bytes,
	// NOT base64. The DTO conversion happens at the wire boundary.
	if !strings.Contains(string(svc.lastJurisdictionInput.PermitTypes), "building") {
		t.Errorf("permit_types not raw JSON: %s", svc.lastJurisdictionInput.PermitTypes)
	}
	// Response body should re-emit JSONB raw (not base64).
	var env struct {
		Data struct {
			Jurisdiction permitJurisdictionDTO `json:"jurisdiction"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(string(env.Data.Jurisdiction.PermitTypes), "building") {
		t.Errorf("permit_types not emitted as raw JSON: %s", env.Data.Jurisdiction.PermitTypes)
	}
}

// ---------- POST /setup/complete ----------

func TestSetup_Complete_HappyPath(t *testing.T) {
	legal := "Kelbrook Construction"
	svc := &mockSetupService{
		completeResult: models.CompanyProfile{LegalName: &legal, OnboardingComplete: true},
	}
	h := NewSetupHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/setup/complete", testOrgID, nil, strings.NewReader(""))
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCompleteInput.OrgID.String() != testOrgID {
		t.Errorf("Complete got org=%s, want %s", svc.lastCompleteInput.OrgID, testOrgID)
	}
}

func TestSetup_Complete_MissingPrereqs_400(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{completeErr: service.ErrInvalidInput})
	r := buildRequest(t, "POST", "/api/v1/setup/complete", testOrgID, nil, strings.NewReader(""))
	w := httptest.NewRecorder()
	h.Complete(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ---------- error mapping coverage ----------

func TestSetup_ErrorMapping_NotFoundIs404(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{stateErr: service.ErrNotFound})
	r := buildRequest(t, "GET", "/api/v1/setup/state", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.State(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestSetup_ErrorMapping_UnknownErrorIs500(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{stateErr: assertErr("storage exploded")})
	r := buildRequest(t, "GET", "/api/v1/setup/state", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.State(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
	// Body must NOT leak the underlying DB message.
	if strings.Contains(w.Body.String(), "exploded") {
		t.Errorf("internal error leaked to client: %s", w.Body.String())
	}
}

// assertErr is a sentinel-free error used to verify the 500 default
// path doesn't leak the message.
type assertErr string

func (a assertErr) Error() string { return string(a) }

// ---------- newSetupStateResponse: jurisdiction loop + nil-list branches ----------

// TestSetup_State_JurisdictionsAndNilLists drives newSetupStateResponse
// through its remaining legs in one shot: PermitJurisdictions populated
// (the len>0 loop that lifts each row through newPermitJurisdictionDTO),
// Trades/CostCodes nil (the nonNil* "→ stable []" branches), and
// DefaultHolidays non-nil (the nonNilHolidays passthrough). The
// State_OK test covers the inverse, so together they close all three
// nonNil helpers + the response builder.
func TestSetup_State_JurisdictionsAndNilLists(t *testing.T) {
	orgUUID := uuid.MustParse(testOrgID)
	region := "US-CT"
	svc := &mockSetupService{
		stateResult: service.SetupState{
			OrgID:              orgUUID,
			OnboardingComplete: true,
			Trades:             nil, // → nonNilTrades returns []
			CostCodes:          nil, // → nonNilCostCodes returns []
			DefaultHolidays: []models.HolidayOverride{ // → nonNilHolidays passthrough
				{Name: "Independence Day"},
			},
			PermitJurisdictions: []models.PermitJurisdiction{{
				Name:        "Town of Glastonbury",
				Region:      &region,
				PermitTypes: []byte(`["building"]`),
			}},
		},
	}
	h := NewSetupHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/setup/state", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.State(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Data setupStateResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Data.Trades == nil || len(env.Data.Trades) != 0 {
		t.Errorf("Trades should be a stable empty slice, got %+v", env.Data.Trades)
	}
	if env.Data.CostCodes == nil || len(env.Data.CostCodes) != 0 {
		t.Errorf("CostCodes should be a stable empty slice, got %+v", env.Data.CostCodes)
	}
	if len(env.Data.DefaultHolidays) != 1 {
		t.Errorf("DefaultHolidays passthrough failed: %+v", env.Data.DefaultHolidays)
	}
	if len(env.Data.PermitJurisdictions) != 1 ||
		!strings.Contains(string(env.Data.PermitJurisdictions[0].PermitTypes), "building") {
		t.Errorf("jurisdiction loop not surfaced as raw JSON: %+v", env.Data.PermitJurisdictions)
	}
}

// ---------- claim-guard (401) across every wizard handler ----------

// setupHandlerFn is the shared shape of every SetupHandler method, so
// the guard-leg tables below can iterate over method values.
type setupHandlerFn func(*SetupHandler, http.ResponseWriter, *http.Request)

// TestSetup_AllHandlers_InvalidOrgIDClaim_401 proves the
// callerOrgIDFromClaims gate short-circuits with 401 BEFORE any body
// decode or service call on every mutating wizard handler (State has
// its own test). "not-a-uuid" is an intentionally malformed org claim.
func TestSetup_AllHandlers_InvalidOrgIDClaim_401(t *testing.T) {
	cases := []struct {
		name   string
		target string
		params map[string]string
		call   setupHandlerFn
	}{
		{"company-info", "/api/v1/setup/company-info", nil, (*SetupHandler).CompanyInfo},
		{"trades", "/api/v1/setup/trades", nil, (*SetupHandler).CreateTrade},
		{"cost-codes", "/api/v1/setup/cost-codes", nil, (*SetupHandler).CreateCostCode},
		{"calendars", "/api/v1/setup/calendars", nil, (*SetupHandler).CreateCalendar},
		{"holidays", "/api/v1/setup/calendars/" + testCalendarID + "/holidays",
			map[string]string{"calendarID": testCalendarID}, (*SetupHandler).AddHoliday},
		{"jurisdictions", "/api/v1/setup/jurisdictions", nil, (*SetupHandler).AddJurisdiction},
		{"complete", "/api/v1/setup/complete", nil, (*SetupHandler).Complete},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := NewSetupHandler(&mockSetupService{})
			r := buildRequest(t, "POST", c.target, "not-a-uuid", c.params, strings.NewReader(`{}`))
			w := httptest.NewRecorder()
			c.call(h, w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status=%d, want 401", w.Code)
			}
		})
	}
}

// ---------- malformed-JSON (400) across the body handlers ----------

// TestSetup_BodyHandlers_InvalidJSON_400 covers the json.Decode→400
// leg on each body-reading handler (CompanyInfo already has its own).
// AddHoliday gets a valid calendarID so the decode (not the URL parse)
// is the failing step.
func TestSetup_BodyHandlers_InvalidJSON_400(t *testing.T) {
	cases := []struct {
		name   string
		target string
		params map[string]string
		call   setupHandlerFn
	}{
		{"trades", "/api/v1/setup/trades", nil, (*SetupHandler).CreateTrade},
		{"cost-codes", "/api/v1/setup/cost-codes", nil, (*SetupHandler).CreateCostCode},
		{"calendars", "/api/v1/setup/calendars", nil, (*SetupHandler).CreateCalendar},
		{"holidays", "/api/v1/setup/calendars/" + testCalendarID + "/holidays",
			map[string]string{"calendarID": testCalendarID}, (*SetupHandler).AddHoliday},
		{"jurisdictions", "/api/v1/setup/jurisdictions", nil, (*SetupHandler).AddJurisdiction},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := NewSetupHandler(&mockSetupService{})
			r := buildRequest(t, "POST", c.target, testOrgID, c.params, strings.NewReader(`{`))
			w := httptest.NewRecorder()
			c.call(h, w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status=%d, want 400", w.Code)
			}
		})
	}
}

// ---------- service-error → 500 mapping on the remaining handlers ----------

// TestSetup_BodyHandlers_ServiceError_500 exercises the writeSetupError
// default leg (unknown error → 500, message NOT leaked) on the three
// handlers whose error path wasn't otherwise covered: CreateCalendar,
// AddHoliday, AddJurisdiction.
func TestSetup_BodyHandlers_ServiceError_500(t *testing.T) {
	boom := assertErr("storage exploded")
	cases := []struct {
		name   string
		target string
		params map[string]string
		body   string
		mock   *mockSetupService
		call   setupHandlerFn
	}{
		{
			"calendars", "/api/v1/setup/calendars", nil,
			`{"name":"Default","timezone":"America/New_York","working_days_mask":31,"daily_work_minutes":480}`,
			&mockSetupService{calendarErr: boom}, (*SetupHandler).CreateCalendar,
		},
		{
			"holidays", "/api/v1/setup/calendars/" + testCalendarID + "/holidays",
			map[string]string{"calendarID": testCalendarID},
			`{"holiday_date":"2026-07-04","name":"x"}`,
			&mockSetupService{holidayErr: boom}, (*SetupHandler).AddHoliday,
		},
		{
			"jurisdictions", "/api/v1/setup/jurisdictions", nil,
			`{"name":"Town of Glastonbury"}`,
			&mockSetupService{jurisdictionErr: boom}, (*SetupHandler).AddJurisdiction,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := NewSetupHandler(c.mock)
			r := buildRequest(t, "POST", c.target, testOrgID, c.params, strings.NewReader(c.body))
			w := httptest.NewRecorder()
			c.call(h, w, r)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d, want 500; body=%s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "exploded") {
				t.Errorf("internal error leaked to client: %s", w.Body.String())
			}
		})
	}
}

// ---------- parseHolidayDate ----------

// TestParseHolidayDate covers the wizard's date parser directly: the
// empty-string guard, the canonical YYYY-MM-DD shape, the RFC3339
// fallback, and a garbage input that fails both formats.
func TestParseHolidayDate(t *testing.T) {
	if _, err := parseHolidayDate(""); err == nil {
		t.Error("empty date should error")
	}
	ymd, err := parseHolidayDate("2026-07-04")
	if err != nil || ymd.Year() != 2026 || ymd.Month() != time.July || ymd.Day() != 4 {
		t.Errorf("YYYY-MM-DD parse: got %v err=%v", ymd, err)
	}
	rfc, err := parseHolidayDate("2026-12-25T00:00:00Z")
	if err != nil || rfc.Month() != time.December || rfc.Day() != 25 {
		t.Errorf("RFC3339 fallback: got %v err=%v", rfc, err)
	}
	if _, err := parseHolidayDate("not-a-date"); err == nil {
		t.Error("garbage date should error")
	}
}
