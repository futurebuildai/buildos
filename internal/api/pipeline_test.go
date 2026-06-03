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
	"github.com/futurebuildai/buildos/internal/store"
)

// mockPipelineService implements PipelineServicer for handler tests.
type mockPipelineService struct {
	listResult    store.ProspectsPage
	listErr       error
	getResult     models.ProspectWithDetails
	getErr        error
	createResult  models.Prospect
	createErr     error
	updateResult  models.Prospect
	updateErr     error
	advanceResult models.Prospect
	advanceErr    error
	loseResult    models.Prospect
	loseErr       error

	createEstimateResult models.PipelineEstimate
	createEstimateErr    error
	updateEstimateResult models.PipelineEstimate
	updateEstimateErr    error
	createPermitResult   models.Permit
	createPermitErr      error
	updatePermitResult   models.Permit
	updatePermitErr      error
	analyticsResult      []models.PipelineAnalyticsRow
	analyticsErr         error

	// Captured args for assertions.
	lastListInput           service.ListProspectsInput
	lastCreateInput         service.CreateProspectInput
	lastUpdateInput         service.UpdateProspectInput
	lastAdvanceInput        service.AdvanceProspectInput
	lastAdvanceUserSub      string
	lastLoseInput           service.LoseProspectInput
	lastLoseUserSub         string
	lastCreateEstimateInput service.CreateEstimateInput
	lastUpdateEstimateInput service.UpdateEstimateStatusInput
	lastCreatePermitInput   service.CreatePermitInput
	lastUpdatePermitInput   service.UpdatePermitInput
	lastAnalyticsOrgID      uuid.UUID
	lastGetProspectID       uuid.UUID
	lastGetCallerOrg        uuid.UUID
}

func (m *mockPipelineService) ListProspects(_ context.Context, in service.ListProspectsInput) (store.ProspectsPage, error) {
	m.lastListInput = in
	return m.listResult, m.listErr
}
func (m *mockPipelineService) GetProspectWithDetails(_ context.Context, id, orgID uuid.UUID) (models.ProspectWithDetails, error) {
	m.lastGetProspectID = id
	m.lastGetCallerOrg = orgID
	return m.getResult, m.getErr
}
func (m *mockPipelineService) CreateProspect(_ context.Context, in service.CreateProspectInput) (models.Prospect, error) {
	m.lastCreateInput = in
	return m.createResult, m.createErr
}
func (m *mockPipelineService) UpdateProspect(_ context.Context, in service.UpdateProspectInput) (models.Prospect, error) {
	m.lastUpdateInput = in
	return m.updateResult, m.updateErr
}
func (m *mockPipelineService) AdvanceProspect(_ context.Context, callerUserSub string, in service.AdvanceProspectInput) (models.Prospect, error) {
	m.lastAdvanceUserSub = callerUserSub
	m.lastAdvanceInput = in
	return m.advanceResult, m.advanceErr
}
func (m *mockPipelineService) LoseProspect(_ context.Context, callerUserSub string, in service.LoseProspectInput) (models.Prospect, error) {
	m.lastLoseUserSub = callerUserSub
	m.lastLoseInput = in
	return m.loseResult, m.loseErr
}

// Estimate / permit hooks added in Sprint 3 PR 2b.
func (m *mockPipelineService) CreateEstimate(_ context.Context, in service.CreateEstimateInput) (models.PipelineEstimate, error) {
	m.lastCreateEstimateInput = in
	return m.createEstimateResult, m.createEstimateErr
}
func (m *mockPipelineService) UpdateEstimateStatus(_ context.Context, in service.UpdateEstimateStatusInput) (models.PipelineEstimate, error) {
	m.lastUpdateEstimateInput = in
	return m.updateEstimateResult, m.updateEstimateErr
}
func (m *mockPipelineService) CreatePermit(_ context.Context, in service.CreatePermitInput) (models.Permit, error) {
	m.lastCreatePermitInput = in
	return m.createPermitResult, m.createPermitErr
}
func (m *mockPipelineService) UpdatePermit(_ context.Context, in service.UpdatePermitInput) (models.Permit, error) {
	m.lastUpdatePermitInput = in
	return m.updatePermitResult, m.updatePermitErr
}
func (m *mockPipelineService) GetPipelineAnalytics(_ context.Context, orgID uuid.UUID) ([]models.PipelineAnalyticsRow, error) {
	m.lastAnalyticsOrgID = orgID
	return m.analyticsResult, m.analyticsErr
}

const (
	testProspectID = "55555555-5555-5555-5555-555555555555"
)

func TestListProspects_OK(t *testing.T) {
	svc := &mockPipelineService{
		listResult: store.ProspectsPage{
			Prospects:  []models.Prospect{{Name: "Smith Residence", PipelineStage: models.StageLead}},
			Total:      1,
			Page:       1,
			PerPage:    50,
			TotalPages: 1,
		},
	}
	h := NewPipelineHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/pipeline/prospects?stage=LEAD&page=1&per_page=50",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.ListProspects(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastListInput.Stage != "LEAD" {
		t.Errorf("stage filter = %q, want LEAD", svc.lastListInput.Stage)
	}
	if svc.lastListInput.Page != 1 || svc.lastListInput.PerPage != 50 {
		t.Errorf("pagination = (%d,%d), want (1,50)", svc.lastListInput.Page, svc.lastListInput.PerPage)
	}

	var resp struct {
		Pagination paginationMeta `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Pagination.Total != 1 || resp.Pagination.TotalPages != 1 {
		t.Errorf("pagination meta = %+v", resp.Pagination)
	}
}

func TestListProspects_OrgMismatchReturns403(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	r := buildRequest(t, "GET", "/api/v1/org/"+otherOrgID+"/pipeline/prospects",
		testOrgID, map[string]string{"orgID": otherOrgID}, nil)
	w := httptest.NewRecorder()
	h.ListProspects(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status=%d, want 403", w.Code)
	}
}

func TestCreateProspect_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		createResult: models.Prospect{Name: "Test", PipelineStage: models.StageLead, ProbabilityPct: 10},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{"name":"Test","client_name":"Bob"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects",
		testOrgID, map[string]string{"orgID": testOrgID}, body)
	w := httptest.NewRecorder()
	h.CreateProspect(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCreateInput.OrgID.String() != testOrgID {
		t.Errorf("create got org_id=%s, want %s", svc.lastCreateInput.OrgID, testOrgID)
	}
}

func TestCreateProspect_InvalidInputReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		createErr: service.ErrInvalidInput,
	})
	body := strings.NewReader(`{"name":""}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects",
		testOrgID, map[string]string{"orgID": testOrgID}, body)
	w := httptest.NewRecorder()
	h.CreateProspect(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestGetProspect_NotFoundReturns404(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		getErr: service.ErrNotFound,
	})
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID,
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, nil)
	w := httptest.NewRecorder()
	h.GetProspect(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestAdvanceProspect_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		advanceResult: models.Prospect{PipelineStage: models.StageQualified, ProbabilityPct: 25},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{"target_stage":"QUALIFIED"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/advance",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.AdvanceProspect(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastAdvanceInput.Target != models.StageQualified {
		t.Errorf("advance target = %s, want QUALIFIED", svc.lastAdvanceInput.Target)
	}
	if svc.lastAdvanceUserSub != "test-sub" {
		t.Errorf("service got user_sub=%q, want test-sub", svc.lastAdvanceUserSub)
	}
}

func TestAdvanceProspect_InvalidTransitionReturns409(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		advanceErr: service.ErrInvalidTransition,
	})
	body := strings.NewReader(`{"target_stage":"PERMIT_APPLIED"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/advance",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.AdvanceProspect(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "INVALID_TRANSITION") {
		t.Errorf("body should contain INVALID_TRANSITION: %s", w.Body.String())
	}
}

func TestAdvanceProspect_TerminalSourceReturns409(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		advanceErr: service.ErrTerminalStage,
	})
	body := strings.NewReader(`{"target_stage":"QUALIFIED"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/advance",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.AdvanceProspect(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409", w.Code)
	}
}

func TestAdvanceProspect_ErrNotImplementedMaps501(t *testing.T) {
	// Service returns ErrNotImplemented in partial-wiring deployments
	// (e.g., a server with no RiverClient configured can't perform the
	// Kanban→CPM transition). The handler must surface this as 501.
	h := NewPipelineHandler(&mockPipelineService{
		advanceErr: service.ErrNotImplemented,
	})
	body := strings.NewReader(`{"target_stage":"PERMIT_ISSUED","permit_issued_date":"2026-04-01"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/advance",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.AdvanceProspect(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status=%d, want 501", w.Code)
	}
}

func TestAdvanceProspect_MissingTargetReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	body := strings.NewReader(`{}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/advance",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.AdvanceProspect(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestLoseProspect_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		loseResult: models.Prospect{PipelineStage: models.StageLost},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{"reason":"client chose another GC"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/lose",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.LoseProspect(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastLoseInput.Reason != "client chose another GC" {
		t.Errorf("reason captured = %q", svc.lastLoseInput.Reason)
	}
	if svc.lastLoseUserSub != "test-sub" {
		t.Errorf("service got user_sub=%q, want test-sub", svc.lastLoseUserSub)
	}
}

func TestLoseProspect_TerminalReturns409(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		loseErr: service.ErrTerminalStage,
	})
	body := strings.NewReader(`{"reason":"changed our minds"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/lose",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.LoseProspect(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409", w.Code)
	}
}

func TestUpdateProspect_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		updateResult: models.Prospect{Name: "Updated"},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{"name":"Updated"}`)
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID,
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.UpdateProspect(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
}

// ---------- Estimates ----------

const testEstimateID = "66666666-6666-6666-6666-666666666666"

func TestCreateEstimate_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		createEstimateResult: models.PipelineEstimate{
			Version:             1,
			TotalEstimatedCents: 2_500_000,
			CurrencyCode:        "USD",
			Status:              "draft",
		},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{
		"line_items":[{"wbs_code":"6.0","description":"Foundation","estimated_cents":2500000}],
		"margin_pct":15,
		"currency_code":"USD"
	}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/estimates",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.CreateEstimate(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if got := svc.lastCreateEstimateInput; got.CurrencyCode != "USD" || got.MarginPct != 15 || len(got.LineItems) != 1 {
		t.Errorf("input passed to service = %+v", got)
	}
}

func TestCreateEstimate_InvalidCurrencyMaps400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		createEstimateErr: service.ErrInvalidInput,
	})
	body := strings.NewReader(`{"line_items":[{"wbs_code":"x","estimated_cents":1}],"margin_pct":15,"currency_code":"EUR"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/estimates",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.CreateEstimate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestUpdateEstimate_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		updateEstimateResult: models.PipelineEstimate{Status: "sent"},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{"prospect_id":"` + testProspectID + `","status":"sent"}`)
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/estimates/"+testEstimateID,
		testOrgID, map[string]string{"orgID": testOrgID, "estimateID": testEstimateID}, body)
	w := httptest.NewRecorder()
	h.UpdateEstimate(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastUpdateEstimateInput.NewStatus != "sent" {
		t.Errorf("captured status = %q", svc.lastUpdateEstimateInput.NewStatus)
	}
}

func TestUpdateEstimate_MissingProspectIDReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	body := strings.NewReader(`{"status":"sent"}`)
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/estimates/"+testEstimateID,
		testOrgID, map[string]string{"orgID": testOrgID, "estimateID": testEstimateID}, body)
	w := httptest.NewRecorder()
	h.UpdateEstimate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestUpdateEstimate_InvalidTransitionReturns409(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		updateEstimateErr: service.ErrInvalidTransition,
	})
	body := strings.NewReader(`{"prospect_id":"` + testProspectID + `","status":"draft"}`)
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/estimates/"+testEstimateID,
		testOrgID, map[string]string{"orgID": testOrgID, "estimateID": testEstimateID}, body)
	w := httptest.NewRecorder()
	h.UpdateEstimate(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409", w.Code)
	}
}

// ---------- Permits ----------

const testPermitID = "77777777-7777-7777-7777-777777777777"

func TestCreatePermit_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		createPermitResult: models.Permit{
			PermitType:      "building",
			Jurisdiction:    "Austin TX",
			FeeCents:        50000,
			FeeCurrencyCode: "USD",
			Status:          "not_submitted",
		},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{
		"permit_type":"building","jurisdiction":"Austin TX",
		"fee_cents":50000,"fee_currency_code":"USD"
	}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/permits",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.CreatePermit(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCreatePermitInput.PermitType != "building" {
		t.Errorf("captured permit_type=%q", svc.lastCreatePermitInput.PermitType)
	}
}

func TestCreatePermit_BadDateReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	body := strings.NewReader(`{
		"permit_type":"building","jurisdiction":"Austin TX",
		"fee_cents":1,"fee_currency_code":"USD",
		"submitted_date":"not-a-date"
	}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/permits",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.CreatePermit(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestUpdatePermit_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		updatePermitResult: models.Permit{Status: "submitted"},
	}
	h := NewPipelineHandler(svc)
	body := strings.NewReader(`{"prospect_id":"` + testProspectID + `","status":"submitted","application_number":"BLDG-2026-0042"}`)
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/permits/"+testPermitID,
		testOrgID, map[string]string{"orgID": testOrgID, "permitID": testPermitID}, body)
	w := httptest.NewRecorder()
	h.UpdatePermit(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
}

func TestUpdatePermit_TerminalSourceReturns409(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{
		updatePermitErr: service.ErrInvalidTransition,
	})
	body := strings.NewReader(`{"prospect_id":"` + testProspectID + `","status":"submitted"}`)
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/permits/"+testPermitID,
		testOrgID, map[string]string{"orgID": testOrgID, "permitID": testPermitID}, body)
	w := httptest.NewRecorder()
	h.UpdatePermit(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409", w.Code)
	}
}

// ---------- Shared guard legs (org-mismatch / bad-UUID / malformed body) ----------

// pipelineCase is one handler invocation for the cross-cutting guard-leg
// tables below: the same 403/400 short-circuits live at the top of every
// write/read handler, so a table over the handler set proves each one rather
// than copy-pasting a near-identical test per method.
type pipelineCase struct {
	name   string
	method string
	target string
	vars   map[string]string
	call   func(*PipelineHandler, http.ResponseWriter, *http.Request)
}

// guardCases enumerates the authenticated pipeline handlers that begin with
// requireOrgIDFromURL → parseUUIDFromURL(resource) → callerOrgIDFromClaims.
// The targets here use otherOrgID in the path so the org-mismatch table trips
// the 403; the bad-UUID table overrides the resource path var instead.
func guardCases() []pipelineCase {
	return []pipelineCase{
		{"GetProspect", "GET", "/api/v1/org/" + otherOrgID + "/pipeline/prospects/" + testProspectID,
			map[string]string{"orgID": otherOrgID, "prospectID": testProspectID},
			(*PipelineHandler).GetProspect},
		{"UpdateProspect", "PUT", "/api/v1/org/" + otherOrgID + "/pipeline/prospects/" + testProspectID,
			map[string]string{"orgID": otherOrgID, "prospectID": testProspectID},
			(*PipelineHandler).UpdateProspect},
		{"CreateEstimate", "POST", "/api/v1/org/" + otherOrgID + "/pipeline/prospects/" + testProspectID + "/estimates",
			map[string]string{"orgID": otherOrgID, "prospectID": testProspectID},
			(*PipelineHandler).CreateEstimate},
		{"UpdateEstimate", "PUT", "/api/v1/org/" + otherOrgID + "/pipeline/estimates/" + testEstimateID,
			map[string]string{"orgID": otherOrgID, "estimateID": testEstimateID},
			(*PipelineHandler).UpdateEstimate},
		{"CreatePermit", "POST", "/api/v1/org/" + otherOrgID + "/pipeline/prospects/" + testProspectID + "/permits",
			map[string]string{"orgID": otherOrgID, "prospectID": testProspectID},
			(*PipelineHandler).CreatePermit},
		{"UpdatePermit", "PUT", "/api/v1/org/" + otherOrgID + "/pipeline/permits/" + testPermitID,
			map[string]string{"orgID": otherOrgID, "permitID": testPermitID},
			(*PipelineHandler).UpdatePermit},
	}
}

// TestPipelineHandlers_OrgMismatchReturns403 proves the cross-tenant guard on
// every prospect/estimate/permit handler: a caller whose claim org differs
// from the {orgID} path segment is refused before the service is touched.
func TestPipelineHandlers_OrgMismatchReturns403(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	for _, c := range guardCases() {
		t.Run(c.name, func(t *testing.T) {
			// callerOrgID = testOrgID, path orgID = otherOrgID → mismatch.
			r := buildRequest(t, c.method, c.target, testOrgID, c.vars, nil)
			w := httptest.NewRecorder()
			c.call(h, w, r)
			if w.Code != http.StatusForbidden {
				t.Errorf("status=%d, want 403 (org mismatch)", w.Code)
			}
		})
	}
}

// TestPipelineHandlers_BadResourceUUIDReturns400 proves the resource-id parse
// guard: with the org matching the caller (so requireOrgIDFromURL passes), a
// malformed prospect/estimate/permit path id is rejected as 400.
func TestPipelineHandlers_BadResourceUUIDReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	// Each case names the resource path var to corrupt (the non-orgID key).
	for _, c := range guardCases() {
		t.Run(c.name, func(t *testing.T) {
			vars := map[string]string{"orgID": testOrgID}
			var resourceKey string
			for k := range c.vars {
				if k != "orgID" {
					resourceKey = k
				}
			}
			vars[resourceKey] = "not-a-uuid"
			// Rebuild a same-org target so requireOrgIDFromURL passes first.
			target := strings.Replace(c.target, otherOrgID, testOrgID, 1)
			r := buildRequest(t, c.method, target, testOrgID, vars, nil)
			w := httptest.NewRecorder()
			c.call(h, w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status=%d, want 400 (bad %s)", w.Code, resourceKey)
			}
		})
	}
}

// TestPipelineHandlers_MalformedBodyReturns400 proves the JSON-decode guard on
// the write handlers: a body that isn't valid JSON is rejected as 400 after
// the org + resource-id gates pass. GetProspect is read-only (no body) so it's
// excluded.
func TestPipelineHandlers_MalformedBodyReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	for _, c := range guardCases() {
		if c.name == "GetProspect" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			vars := map[string]string{"orgID": testOrgID}
			for k, v := range c.vars {
				if k != "orgID" {
					vars[k] = v
				}
			}
			target := strings.Replace(c.target, otherOrgID, testOrgID, 1)
			r := buildRequest(t, c.method, target, testOrgID, vars, strings.NewReader("{"))
			w := httptest.NewRecorder()
			c.call(h, w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status=%d, want 400 (malformed body)", w.Code)
			}
		})
	}
}

// TestGetProspect_HappyPath drives the read happy path (the existing GetProspect
// test only exercises the 404 leg, leaving the 200 writeJSON unreached).
func TestGetProspect_HappyPath(t *testing.T) {
	svc := &mockPipelineService{
		getResult: models.ProspectWithDetails{Prospect: models.Prospect{Name: "Hilltop"}},
	}
	h := NewPipelineHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID,
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, nil)
	w := httptest.NewRecorder()
	h.GetProspect(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastGetProspectID.String() != testProspectID || svc.lastGetCallerOrg.String() != testOrgID {
		t.Errorf("service got (%s,%s), want (%s,%s)", svc.lastGetProspectID, svc.lastGetCallerOrg, testProspectID, testOrgID)
	}
}

// TestUpdateProspect_ServiceErrorMaps404 covers the writePipelineError leg of
// UpdateProspect (the existing test only drives the 200 happy path).
func TestUpdateProspect_ServiceErrorMaps404(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{updateErr: service.ErrNotFound})
	body := strings.NewReader(`{"name":"Updated"}`)
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID,
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.UpdateProspect(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

// TestUpdateEstimate_MissingStatusReturns400 covers the status-required guard
// (the existing missing-prospect-id test sends a status, so the nil-status leg
// was unreached).
func TestUpdateEstimate_MissingStatusReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	body := strings.NewReader(`{"prospect_id":"` + testProspectID + `"}`) // no status
	r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/estimates/"+testEstimateID,
		testOrgID, map[string]string{"orgID": testOrgID, "estimateID": testEstimateID}, body)
	w := httptest.NewRecorder()
	h.UpdateEstimate(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400 (status required)", w.Code)
	}
}

// TestCreatePermit_ServiceErrorMaps404 covers CreatePermit's writePipelineError
// leg, and TestCreatePermit_BadExpectedIssueDateReturns400 covers the second
// (expected_issue_date) optional-date parse guard the existing bad-date test
// (which corrupts submitted_date) skips.
func TestCreatePermit_ServiceErrorMaps404(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{createPermitErr: service.ErrNotFound})
	body := strings.NewReader(`{"permit_type":"building","jurisdiction":"Austin TX","fee_cents":1,"fee_currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/permits",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.CreatePermit(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestCreatePermit_BadExpectedIssueDateReturns400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	body := strings.NewReader(`{"permit_type":"building","jurisdiction":"Austin TX","fee_cents":1,"fee_currency_code":"USD","expected_issue_date":"not-a-date"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/pipeline/prospects/"+testProspectID+"/permits",
		testOrgID, map[string]string{"orgID": testOrgID, "prospectID": testProspectID}, body)
	w := httptest.NewRecorder()
	h.CreatePermit(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400 (bad expected_issue_date)", w.Code)
	}
}

// TestUpdatePermit_BadBodyFieldsReturn400 covers the four body-parse guards
// after the path/org gates: a non-UUID prospect_id and each of the three
// optional date fields (submitted / expected_issue / actual_issue).
func TestUpdatePermit_BadBodyFieldsReturn400(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	cases := []struct {
		name string
		body string
	}{
		{"bad prospect_id", `{"prospect_id":"not-a-uuid","status":"submitted"}`},
		{"bad submitted_date", `{"prospect_id":"` + testProspectID + `","submitted_date":"nope"}`},
		{"bad expected_issue_date", `{"prospect_id":"` + testProspectID + `","expected_issue_date":"nope"}`},
		{"bad actual_issue_date", `{"prospect_id":"` + testProspectID + `","actual_issue_date":"nope"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := buildRequest(t, "PUT", "/api/v1/org/"+testOrgID+"/pipeline/permits/"+testPermitID,
				testOrgID, map[string]string{"orgID": testOrgID, "permitID": testPermitID}, strings.NewReader(c.body))
			w := httptest.NewRecorder()
			h.UpdatePermit(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status=%d, want 400 (%s)", w.Code, c.name)
			}
		})
	}
}

// ---------- Analytics ----------

func TestAnalytics_OK(t *testing.T) {
	svc := &mockPipelineService{
		analyticsResult: []models.PipelineAnalyticsRow{
			{CurrencyCode: "USD", TotalEstimatedCents: 5_000_000, WeightedRevenueCents: 1_250_000, ProspectCount: 4},
			{CurrencyCode: "CAD", TotalEstimatedCents: 2_000_000, WeightedRevenueCents: 500_000, ProspectCount: 2},
		},
	}
	h := NewPipelineHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/pipeline/analytics",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.Analytics(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastAnalyticsOrgID.String() != testOrgID {
		t.Errorf("service got org_id=%s, want %s", svc.lastAnalyticsOrgID, testOrgID)
	}
	var resp struct {
		Data struct {
			Analytics []models.PipelineAnalyticsRow `json:"analytics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data.Analytics) != 2 {
		t.Errorf("expected 2 currency rows, got %d", len(resp.Data.Analytics))
	}
}

func TestAnalytics_OrgMismatchReturns403(t *testing.T) {
	h := NewPipelineHandler(&mockPipelineService{})
	r := buildRequest(t, "GET", "/api/v1/org/"+otherOrgID+"/pipeline/analytics",
		testOrgID, map[string]string{"orgID": otherOrgID}, nil)
	w := httptest.NewRecorder()
	h.Analytics(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status=%d, want 403", w.Code)
	}
}
