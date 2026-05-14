package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	redeemResult service.RedeemedBootstrapToken
	redeemErr    error

	// Captured args for assertions.
	lastStateOrgID         uuid.UUID
	lastCompanyInput       service.UpdateCompanyInfoInput
	lastTradeInput         service.CreateTradeInput
	lastCostCodeInput      service.CreateCostCodeInput
	lastCalendarInput      service.CreateCalendarInput
	lastHolidayInput       service.AddHolidayInput
	lastJurisdictionInput  service.AddJurisdictionInput
	lastCompleteInput      service.CompleteSetupInput
	lastRedeemCleartext    string
	lastRedeemSubject      string
	lastRedeemCallerOrgID  uuid.UUID
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
func (m *mockSetupService) RedeemBootstrapTokenForSubject(_ context.Context, cleartext, subject string, callerOrgID uuid.UUID) (service.RedeemedBootstrapToken, error) {
	m.lastRedeemCleartext = cleartext
	m.lastRedeemSubject = subject
	m.lastRedeemCallerOrgID = callerOrgID
	return m.redeemResult, m.redeemErr
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

// ---------- POST /setup/bootstrap ----------

func TestSetup_Bootstrap_HappyPath(t *testing.T) {
	tokenID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	orgUUID := uuid.MustParse(testOrgID)
	svc := &mockSetupService{
		redeemResult: service.RedeemedBootstrapToken{ID: tokenID, OrgID: orgUUID},
	}
	h := NewSetupHandler(svc)
	body := strings.NewReader(`{"token":"abcdef0123456789abcdef0123456789abcdef01234"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/bootstrap", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Bootstrap(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastRedeemCleartext != "abcdef0123456789abcdef0123456789abcdef01234" {
		t.Errorf("cleartext not forwarded: %q", svc.lastRedeemCleartext)
	}
	if svc.lastRedeemSubject != "test-sub" {
		t.Errorf("subject not forwarded: %q", svc.lastRedeemSubject)
	}
	if svc.lastRedeemCallerOrgID != orgUUID {
		t.Errorf("callerOrgID not forwarded: %s", svc.lastRedeemCallerOrgID)
	}
	var env struct {
		Data bootstrapResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.Data.Redeemed || env.Data.OrgID != orgUUID || env.Data.TokenID != tokenID {
		t.Errorf("bootstrap response unexpected: %+v", env.Data)
	}
}

func TestSetup_Bootstrap_MissingToken_400(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{})
	body := strings.NewReader(`{}`)
	r := buildRequest(t, "POST", "/api/v1/setup/bootstrap", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Bootstrap(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestSetup_Bootstrap_InvalidToken_401(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{redeemErr: service.ErrInvalidBootstrapToken})
	body := strings.NewReader(`{"token":"wrong"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/bootstrap", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Bootstrap(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "INVALID_BOOTSTRAP_TOKEN") {
		t.Errorf("error code missing; body=%s", w.Body.String())
	}
}

func TestSetup_Bootstrap_UserNotProvisioned_412(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{redeemErr: service.ErrBootstrapUserNotProvisioned})
	body := strings.NewReader(`{"token":"x"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/bootstrap", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Bootstrap(w, r)
	if w.Code != http.StatusPreconditionFailed {
		t.Errorf("status=%d, want 412", w.Code)
	}
	if !strings.Contains(w.Body.String(), "BOOTSTRAP_USER_NOT_PROVISIONED") {
		t.Errorf("error code missing; body=%s", w.Body.String())
	}
}

func TestSetup_Bootstrap_InvalidOrgIDClaim_401(t *testing.T) {
	h := NewSetupHandler(&mockSetupService{})
	body := strings.NewReader(`{"token":"x"}`)
	r := buildRequest(t, "POST", "/api/v1/setup/bootstrap", "not-a-uuid", nil, body)
	w := httptest.NewRecorder()
	h.Bootstrap(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", w.Code)
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
