package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// SetupServicer is the subset of *service.SetupService consumed by
// SetupHandler. Mirrors the methods W2 ships; the handler depends on
// the interface so unit tests can substitute a fake (matches the
// PipelineServicer / BudgetServicer pattern elsewhere in this package).
type SetupServicer interface {
	GetState(ctx context.Context, orgID uuid.UUID) (service.SetupState, error)
	UpdateCompanyInfo(ctx context.Context, in service.UpdateCompanyInfoInput) (models.CompanyProfile, error)
	CreateTrade(ctx context.Context, in service.CreateTradeInput) (models.TradeCategory, error)
	CreateCostCode(ctx context.Context, in service.CreateCostCodeInput) (models.CostCode, error)
	CreateCalendar(ctx context.Context, in service.CreateCalendarInput) (models.WorkingCalendar, error)
	AddHoliday(ctx context.Context, in service.AddHolidayInput) (models.HolidayOverride, error)
	AddJurisdiction(ctx context.Context, in service.AddJurisdictionInput) (models.PermitJurisdiction, error)
	Complete(ctx context.Context, in service.CompleteSetupInput) (models.CompanyProfile, error)
	RedeemBootstrapTokenForSubject(ctx context.Context, cleartext, subject string, callerOrgID uuid.UUID) (service.RedeemedBootstrapToken, error)
	// IsOnboardingComplete is consumed by the SetupGate middleware
	// (matches mw.OnboardingChecker). Kept on this interface so a
	// single concrete service satisfies both the wizard handlers
	// and the gate — RouterConfig only needs one SetupService field.
	IsOnboardingComplete(ctx context.Context, orgID uuid.UUID) (bool, error)
}

// SetupHandler exposes the /api/v1/setup/* surface — the embedded
// onboarding wizard (MB-7). Routes are mounted exempt from SetupGate
// (the gate itself relies on these to be reachable while
// onboarding_complete=false) and gated by RoleAdmin minimum,
// except the bootstrap-redeem path which any authenticated user may
// call (the token IS the privilege grant).
type SetupHandler struct {
	svc SetupServicer
}

// NewSetupHandler binds the handler to a SetupServicer.
func NewSetupHandler(svc SetupServicer) *SetupHandler { return &SetupHandler{svc: svc} }

// ---------- Wire-shape DTOs ----------
//
// PermitJurisdictionDTO re-emits the model's []byte JSONB fields as
// json.RawMessage so the wire body shows raw JSON rather than base64.
// All other wizard models have native JSON tags and emit directly.
type permitJurisdictionDTO struct {
	ID                  uuid.UUID       `json:"id"`
	OrgID               uuid.UUID       `json:"org_id"`
	Name                string          `json:"name"`
	Region              *string         `json:"region,omitempty"`
	PermitTypes         json.RawMessage `json:"permit_types,omitempty"`
	InspectionChecklist json.RawMessage `json:"inspection_checklist,omitempty"`
	Notes               *string         `json:"notes,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

func newPermitJurisdictionDTO(j models.PermitJurisdiction) permitJurisdictionDTO {
	return permitJurisdictionDTO{
		ID:                  j.ID,
		OrgID:               j.OrgID,
		Name:                j.Name,
		Region:              j.Region,
		PermitTypes:         json.RawMessage(j.PermitTypes),
		InspectionChecklist: json.RawMessage(j.InspectionChecklist),
		Notes:               j.Notes,
		CreatedAt:           j.CreatedAt,
		UpdatedAt:           j.UpdatedAt,
	}
}

// setupStateResponse is the GET /setup/state response body. Mirrors
// service.SetupState but lifts the jurisdiction list through the DTO
// for proper JSON-bytes emission.
type setupStateResponse struct {
	OrgID               uuid.UUID                `json:"org_id"`
	OnboardingComplete  bool                     `json:"onboarding_complete"`
	CompanyProfile      models.CompanyProfile    `json:"company_profile"`
	Trades              []models.TradeCategory   `json:"trades"`
	CostCodes           []models.CostCode        `json:"cost_codes"`
	DefaultCalendar     *models.WorkingCalendar  `json:"default_calendar,omitempty"`
	DefaultHolidays     []models.HolidayOverride `json:"default_holidays,omitempty"`
	PermitJurisdictions []permitJurisdictionDTO  `json:"permit_jurisdictions"`
}

func newSetupStateResponse(s service.SetupState) setupStateResponse {
	resp := setupStateResponse{
		OrgID:              s.OrgID,
		OnboardingComplete: s.OnboardingComplete,
		CompanyProfile:     s.CompanyProfile,
		Trades:             nonNilTrades(s.Trades),
		CostCodes:          nonNilCostCodes(s.CostCodes),
		DefaultCalendar:    s.DefaultCalendar,
		DefaultHolidays:    nonNilHolidays(s.DefaultHolidays),
	}
	if len(s.PermitJurisdictions) > 0 {
		resp.PermitJurisdictions = make([]permitJurisdictionDTO, 0, len(s.PermitJurisdictions))
		for _, j := range s.PermitJurisdictions {
			resp.PermitJurisdictions = append(resp.PermitJurisdictions, newPermitJurisdictionDTO(j))
		}
	} else {
		resp.PermitJurisdictions = []permitJurisdictionDTO{}
	}
	return resp
}

// nonNilTrades / nonNilCostCodes / nonNilHolidays return an empty
// slice (not nil) when the input is nil — so wire shape is a stable
// "[]" rather than a null. Wizard UI prefers stable list shapes.
func nonNilTrades(s []models.TradeCategory) []models.TradeCategory {
	if s == nil {
		return []models.TradeCategory{}
	}
	return s
}
func nonNilCostCodes(s []models.CostCode) []models.CostCode {
	if s == nil {
		return []models.CostCode{}
	}
	return s
}
func nonNilHolidays(s []models.HolidayOverride) []models.HolidayOverride {
	if s == nil {
		return []models.HolidayOverride{}
	}
	return s
}

// ---------- GET /setup/state ----------

// State returns the wizard's current progress for the caller's org.
// GET /api/v1/setup/state — admin RBAC enforced by router.
func (h *SetupHandler) State(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	state, err := h.svc.GetState(r.Context(), orgID)
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, newSetupStateResponse(state))
}

// ---------- POST /setup/company-info ----------

type companyInfoRequest struct {
	LegalName   *string `json:"legal_name,omitempty"`
	Address     *string `json:"address,omitempty"`
	EIN         *string `json:"ein,omitempty"`
	CompanyType *string `json:"company_type,omitempty"`
	Region      *string `json:"region,omitempty"`
}

// CompanyInfo persists wizard step-1 fields.
// POST /api/v1/setup/company-info
func (h *SetupHandler) CompanyInfo(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body companyInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	cp, err := h.svc.UpdateCompanyInfo(r.Context(), service.UpdateCompanyInfoInput{
		OrgID:       orgID,
		UserSub:     claims.Sub,
		LegalName:   body.LegalName,
		Address:     body.Address,
		EIN:         body.EIN,
		CompanyType: body.CompanyType,
		Region:      body.Region,
	})
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"company_profile": cp})
}

// ---------- POST /setup/trades ----------

type tradeRequest struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	IsDefault   bool    `json:"is_default"`
}

// CreateTrade adds one trade category.
// POST /api/v1/setup/trades
func (h *SetupHandler) CreateTrade(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body tradeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	t, err := h.svc.CreateTrade(r.Context(), service.CreateTradeInput{
		OrgID:       orgID,
		UserSub:     claims.Sub,
		Code:        body.Code,
		Name:        body.Name,
		Description: body.Description,
		IsDefault:   body.IsDefault,
	})
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"trade": t})
}

// ---------- POST /setup/cost-codes ----------

type costCodeRequest struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Division   string  `json:"division"`
	ParentCode *string `json:"parent_code,omitempty"`
	IsDefault  bool    `json:"is_default"`
}

// CreateCostCode adds one CSI MasterFormat cost code.
// POST /api/v1/setup/cost-codes
func (h *SetupHandler) CreateCostCode(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body costCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	c, err := h.svc.CreateCostCode(r.Context(), service.CreateCostCodeInput{
		OrgID:      orgID,
		UserSub:    claims.Sub,
		Code:       body.Code,
		Name:       body.Name,
		Division:   body.Division,
		ParentCode: body.ParentCode,
		IsDefault:  body.IsDefault,
	})
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"cost_code": c})
}

// ---------- POST /setup/calendars ----------

type calendarRequest struct {
	Name             string `json:"name"`
	Timezone         string `json:"timezone"`
	WorkingDaysMask  int16  `json:"working_days_mask"`
	DailyWorkMinutes int    `json:"daily_work_minutes"`
	IsDefault        bool   `json:"is_default"`
}

// CreateCalendar adds a working calendar.
// POST /api/v1/setup/calendars
func (h *SetupHandler) CreateCalendar(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body calendarRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	cal, err := h.svc.CreateCalendar(r.Context(), service.CreateCalendarInput{
		OrgID:            orgID,
		UserSub:          claims.Sub,
		Name:             body.Name,
		Timezone:         body.Timezone,
		WorkingDaysMask:  body.WorkingDaysMask,
		DailyWorkMinutes: body.DailyWorkMinutes,
		IsDefault:        body.IsDefault,
	})
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"calendar": cal})
}

// ---------- POST /setup/calendars/{calendarID}/holidays ----------

type holidayRequest struct {
	HolidayDate string `json:"holiday_date"` // YYYY-MM-DD or RFC3339
	Name        string `json:"name"`
}

// AddHoliday inserts a holiday override on an existing working calendar.
// POST /api/v1/setup/calendars/{calendarID}/holidays
func (h *SetupHandler) AddHoliday(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	calendarID, ok := parseUUIDFromURL(w, r, "calendarID")
	if !ok {
		return
	}
	var body holidayRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	date, err := parseHolidayDate(body.HolidayDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "holiday_date must be YYYY-MM-DD or RFC3339")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	hol, err := h.svc.AddHoliday(r.Context(), service.AddHolidayInput{
		OrgID:       orgID,
		UserSub:     claims.Sub,
		CalendarID:  calendarID,
		HolidayDate: date,
		Name:        body.Name,
	})
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"holiday": hol})
}

// parseHolidayDate accepts YYYY-MM-DD (the canonical wire shape for
// DATE columns) and falls back to RFC3339 so callers that always
// send timestamps don't have to special-case the wizard.
func parseHolidayDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty date")
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// ---------- POST /setup/jurisdictions ----------

type jurisdictionRequest struct {
	Name                string          `json:"name"`
	Region              *string         `json:"region,omitempty"`
	PermitTypes         json.RawMessage `json:"permit_types,omitempty"`
	InspectionChecklist json.RawMessage `json:"inspection_checklist,omitempty"`
	Notes               *string         `json:"notes,omitempty"`
}

// AddJurisdiction inserts a permit jurisdiction.
// POST /api/v1/setup/jurisdictions
func (h *SetupHandler) AddJurisdiction(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body jurisdictionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	j, err := h.svc.AddJurisdiction(r.Context(), service.AddJurisdictionInput{
		OrgID:               orgID,
		UserSub:             claims.Sub,
		Name:                body.Name,
		Region:              body.Region,
		PermitTypes:         []byte(body.PermitTypes),
		InspectionChecklist: []byte(body.InspectionChecklist),
		Notes:               body.Notes,
	})
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"jurisdiction": newPermitJurisdictionDTO(j)})
}

// ---------- POST /setup/complete ----------

// Complete flips onboarding_complete=true after verifying the wizard's
// required prereqs (company info, ≥1 trade, ≥1 cost code, a default
// calendar) are present. Idempotent — already-complete returns 200
// with the existing snapshot.
// POST /api/v1/setup/complete
func (h *SetupHandler) Complete(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	cp, err := h.svc.Complete(r.Context(), service.CompleteSetupInput{
		OrgID:   orgID,
		UserSub: claims.Sub,
	})
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"company_profile": cp})
}

// ---------- POST /setup/bootstrap ----------

type bootstrapRequest struct {
	Token string `json:"token"`
}

type bootstrapResponse struct {
	Redeemed bool      `json:"redeemed"`
	OrgID    uuid.UUID `json:"org_id"`
	TokenID  uuid.UUID `json:"token_id"`
}

// Bootstrap redeems the one-shot owner-claim token shipped via
// fork-init. JWT-authenticated but NOT admin-gated — the cleartext
// token IS the admin-grant proof. Cross-org safety is enforced
// inside the service.
//
// POST /api/v1/setup/bootstrap
func (h *SetupHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	var body bootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if body.Token == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "token is required")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	if claims.Sub == "" {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "subject claim required")
		return
	}
	callerOrgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	out, err := h.svc.RedeemBootstrapTokenForSubject(r.Context(), body.Token, claims.Sub, callerOrgID)
	if err != nil {
		writeSetupError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, bootstrapResponse{
		Redeemed: true,
		OrgID:    out.OrgID,
		TokenID:  out.ID,
	})
}

// ---------- error mapping ----------

// writeSetupError maps service sentinel errors to HTTP responses.
//
//	ErrInvalidBootstrapToken       → 401 INVALID_BOOTSTRAP_TOKEN (intentionally
//	                                  uniform across all token failure modes —
//	                                  per security analysis in service/setup.go)
//	ErrBootstrapUserNotProvisioned → 412 PRECONDITION_FAILED  (Brain JIT user
//	                                  provisioning hasn't run for this subject)
//	ErrSetupAlreadyComplete        → 409 SETUP_ALREADY_COMPLETE
//	ErrNotFound                    → 404 NOT_FOUND
//	ErrInvalidInput                → 400 VALIDATION_ERROR
//	default                        → 500 INTERNAL_ERROR (don't leak DB)
func writeSetupError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidBootstrapToken):
		writeErrorResponse(w, r, http.StatusUnauthorized, "INVALID_BOOTSTRAP_TOKEN", "bootstrap token invalid or expired")
	case errors.Is(err, service.ErrBootstrapUserNotProvisioned):
		writeErrorResponse(w, r, http.StatusPreconditionFailed, "BOOTSTRAP_USER_NOT_PROVISIONED", err.Error())
	case errors.Is(err, service.ErrSetupAlreadyComplete):
		writeErrorResponse(w, r, http.StatusConflict, "SETUP_ALREADY_COMPLETE", err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountSetupRoutes wires the setup wizard under /api/v1/setup. Kept
// in this file (rather than router.go) so adding/renaming routes in
// the wizard touches a single file. Caller is responsible for placing
// this group INSIDE the auth middleware group and OUTSIDE the
// SetupGate middleware (DefaultSetupGateExemptPrefixes already lists
// /api/v1/setup).
func MountSetupRoutes(r chi.Router, h *SetupHandler) {
	r.Route("/api/v1/setup", func(r chi.Router) {
		// Bootstrap requires Auth (so we know the caller's JWT sub
		// and org_id) but NOT admin RBAC. The cleartext token is
		// the privilege grant; gating on RoleAdmin here would
		// require Brain to pre-assign admin before bootstrap,
		// defeating the point.
		r.Post("/bootstrap", h.Bootstrap)

		// Every other wizard step requires admin minimum.
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireMinRole(mw.RoleAdmin))
			r.Get("/state", h.State)
			r.Post("/company-info", h.CompanyInfo)
			r.Post("/trades", h.CreateTrade)
			r.Post("/cost-codes", h.CreateCostCode)
			r.Post("/calendars", h.CreateCalendar)
			r.Post("/calendars/{calendarID}/holidays", h.AddHoliday)
			r.Post("/jurisdictions", h.AddJurisdiction)
			r.Post("/complete", h.Complete)
		})
	})
}
