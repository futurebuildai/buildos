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

	// Captured args for assertions.
	lastListInput           service.ListProspectsInput
	lastCreateInput         service.CreateProspectInput
	lastUpdateInput         service.UpdateProspectInput
	lastAdvanceInput        service.AdvanceProspectInput
	lastLoseInput           service.LoseProspectInput
	lastCreateEstimateInput service.CreateEstimateInput
	lastUpdateEstimateInput service.UpdateEstimateStatusInput
	lastCreatePermitInput   service.CreatePermitInput
	lastUpdatePermitInput   service.UpdatePermitInput
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
func (m *mockPipelineService) AdvanceProspect(_ context.Context, in service.AdvanceProspectInput) (models.Prospect, error) {
	m.lastAdvanceInput = in
	return m.advanceResult, m.advanceErr
}
func (m *mockPipelineService) LoseProspect(_ context.Context, in service.LoseProspectInput) (models.Prospect, error) {
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

func TestAdvanceProspect_PermitIssuedReturns501(t *testing.T) {
	// Service returns ErrNotImplemented for PERMIT_ISSUED until Sprint 3 PR 3.
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
