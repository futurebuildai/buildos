package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// ---------- fleet fakes ----------

type fakeFleetService struct {
	listResult     []models.FleetAsset
	listErr        error
	createResult   models.FleetAsset
	createErr      error
	allocateResult models.EquipmentAllocation
	allocateErr    error

	gotStatusFilter []string
	gotCreateInput  service.CreateAssetInput
	gotAllocInput   service.AllocateAssetInput
	gotOrgID        uuid.UUID
	gotSub          string
}

func (f *fakeFleetService) ListAssets(_ context.Context, orgID uuid.UUID, statusFilter []string) ([]models.FleetAsset, error) {
	f.gotOrgID = orgID
	f.gotStatusFilter = statusFilter
	return f.listResult, f.listErr
}

func (f *fakeFleetService) CreateAsset(_ context.Context, orgID uuid.UUID, sub string, in service.CreateAssetInput) (models.FleetAsset, error) {
	f.gotOrgID, f.gotSub, f.gotCreateInput = orgID, sub, in
	return f.createResult, f.createErr
}

func (f *fakeFleetService) AllocateAsset(_ context.Context, orgID uuid.UUID, sub string, in service.AllocateAssetInput) (models.EquipmentAllocation, error) {
	f.gotOrgID, f.gotSub, f.gotAllocInput = orgID, sub, in
	return f.allocateResult, f.allocateErr
}

// fleetReq builds an org-scoped request: claims org == URL orgID so
// requireOrgIDFromURL passes. body may be nil.
func fleetReq(t *testing.T, method, target, callerOrgID string, params map[string]string, body string) *http.Request {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	if params == nil {
		params = map[string]string{}
	}
	if _, ok := params["orgID"]; !ok {
		params["orgID"] = callerOrgID
	}
	if rdr == nil {
		return buildRequest(t, method, target, callerOrgID, params, nil)
	}
	return buildRequest(t, method, target, callerOrgID, params, rdr)
}

// ---------- GET /fleet ----------

func TestListAssets_OK(t *testing.T) {
	svc := &fakeFleetService{listResult: []models.FleetAsset{{ID: uuid.New(), Name: "Excavator 1", AssetType: "excavator"}}}
	h := NewFleetHandler(svc)
	r := fleetReq(t, "GET", "/api/v1/org/"+testOrgID+"/fleet?status=available,maintenance", testOrgID, nil, "")
	w := httptest.NewRecorder()
	h.ListAssets(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if got := svc.gotStatusFilter; len(got) != 2 || got[0] != "available" || got[1] != "maintenance" {
		t.Errorf("status filter = %v, want [available maintenance]", got)
	}
	if svc.gotOrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s", svc.gotOrgID, testOrgID)
	}
}

func TestListAssets_OrgMismatch403(t *testing.T) {
	h := NewFleetHandler(&fakeFleetService{})
	// URL orgID differs from the caller's claims org.
	r := fleetReq(t, "GET", "/api/v1/org/"+otherOrgID+"/fleet", testOrgID, map[string]string{"orgID": otherOrgID}, "")
	w := httptest.NewRecorder()
	h.ListAssets(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if code := decodeErrCode(t, w); code != "FORBIDDEN" {
		t.Errorf("code=%q, want FORBIDDEN", code)
	}
}

func TestListAssets_ServiceErr500(t *testing.T) {
	h := NewFleetHandler(&fakeFleetService{listErr: errInternal()})
	r := fleetReq(t, "GET", "/api/v1/org/"+testOrgID+"/fleet", testOrgID, nil, "")
	w := httptest.NewRecorder()
	h.ListAssets(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

// ---------- POST /fleet ----------

func TestCreateAsset_OK(t *testing.T) {
	svc := &fakeFleetService{createResult: models.FleetAsset{ID: uuid.New(), Name: "Grader 1", AssetType: "grader"}}
	h := NewFleetHandler(svc)
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet", testOrgID, nil,
		`{"name":"Grader 1","asset_type":"grader","serial_number":"SN-9"}`)
	w := httptest.NewRecorder()
	h.CreateAsset(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotCreateInput.Name != "Grader 1" || svc.gotCreateInput.AssetType != "grader" ||
		svc.gotCreateInput.SerialNumber == nil || *svc.gotCreateInput.SerialNumber != "SN-9" {
		t.Errorf("create input = %+v", svc.gotCreateInput)
	}
	if svc.gotSub != "test-sub" {
		t.Errorf("sub = %q, want test-sub", svc.gotSub)
	}
}

func TestCreateAsset_BadJSON(t *testing.T) {
	h := NewFleetHandler(&fakeFleetService{})
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet", testOrgID, nil, "{bad")
	w := httptest.NewRecorder()
	h.CreateAsset(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestCreateAsset_ValidationError(t *testing.T) {
	h := NewFleetHandler(&fakeFleetService{createErr: wrapInvalid("name required")})
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet", testOrgID, nil, `{"asset_type":"grader"}`)
	w := httptest.NewRecorder()
	h.CreateAsset(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

func TestCreateAsset_OrgMismatch403(t *testing.T) {
	h := NewFleetHandler(&fakeFleetService{})
	r := fleetReq(t, "POST", "/api/v1/org/"+otherOrgID+"/fleet", testOrgID,
		map[string]string{"orgID": otherOrgID}, `{"name":"x","asset_type":"grader"}`)
	w := httptest.NewRecorder()
	h.CreateAsset(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if code := decodeErrCode(t, w); code != "FORBIDDEN" {
		t.Errorf("code=%q, want FORBIDDEN", code)
	}
}

// ---------- POST /fleet/{assetID}/allocate ----------

func TestAllocateAsset_OK(t *testing.T) {
	assetID := uuid.New()
	projID := uuid.New()
	svc := &fakeFleetService{allocateResult: models.EquipmentAllocation{ID: uuid.New(), AssetID: assetID}}
	h := NewFleetHandler(svc)
	body := `{"project_id":"` + projID.String() + `","start_date":"2026-03-01","end_date":"2026-03-05T00:00:00Z"}`
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet/"+assetID.String()+"/allocate",
		testOrgID, map[string]string{"assetID": assetID.String()}, body)
	w := httptest.NewRecorder()
	h.AllocateAsset(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotAllocInput.AssetID != assetID || svc.gotAllocInput.ProjectID != projID {
		t.Errorf("alloc input = %+v, want asset=%s proj=%s", svc.gotAllocInput, assetID, projID)
	}
	// start_date parsed as YYYY-MM-DD, end_date as RFC3339 — both reach the service.
	wantStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !svc.gotAllocInput.StartDate.Equal(wantStart) {
		t.Errorf("start = %v, want %v", svc.gotAllocInput.StartDate, wantStart)
	}
}

func TestAllocateAsset_BadDate(t *testing.T) {
	assetID := uuid.New()
	h := NewFleetHandler(&fakeFleetService{})
	body := `{"project_id":"` + uuid.New().String() + `","start_date":"nope","end_date":"2026-03-05"}`
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet/"+assetID.String()+"/allocate",
		testOrgID, map[string]string{"assetID": assetID.String()}, body)
	w := httptest.NewRecorder()
	h.AllocateAsset(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

func TestAllocateAsset_Conflict409(t *testing.T) {
	assetID := uuid.New()
	h := NewFleetHandler(&fakeFleetService{allocateErr: service.ErrAllocationConflict})
	body := `{"project_id":"` + uuid.New().String() + `","start_date":"2026-03-01","end_date":"2026-03-05"}`
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet/"+assetID.String()+"/allocate",
		testOrgID, map[string]string{"assetID": assetID.String()}, body)
	w := httptest.NewRecorder()
	h.AllocateAsset(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409", w.Code)
	}
	if code := decodeErrCode(t, w); code != "CONFLICT" {
		t.Errorf("code=%q, want CONFLICT", code)
	}
}

func TestAllocateAsset_OrgMismatch403(t *testing.T) {
	assetID := uuid.New()
	h := NewFleetHandler(&fakeFleetService{})
	body := `{"project_id":"` + uuid.New().String() + `","start_date":"2026-03-01","end_date":"2026-03-05"}`
	r := fleetReq(t, "POST", "/api/v1/org/"+otherOrgID+"/fleet/"+assetID.String()+"/allocate",
		testOrgID, map[string]string{"orgID": otherOrgID, "assetID": assetID.String()}, body)
	w := httptest.NewRecorder()
	h.AllocateAsset(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if code := decodeErrCode(t, w); code != "FORBIDDEN" {
		t.Errorf("code=%q, want FORBIDDEN", code)
	}
}

func TestAllocateAsset_BadAssetID400(t *testing.T) {
	h := NewFleetHandler(&fakeFleetService{})
	body := `{"project_id":"` + uuid.New().String() + `","start_date":"2026-03-01","end_date":"2026-03-05"}`
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet/not-a-uuid/allocate",
		testOrgID, map[string]string{"assetID": "not-a-uuid"}, body)
	w := httptest.NewRecorder()
	h.AllocateAsset(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestAllocateAsset_BadJSON400(t *testing.T) {
	assetID := uuid.New()
	h := NewFleetHandler(&fakeFleetService{})
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet/"+assetID.String()+"/allocate",
		testOrgID, map[string]string{"assetID": assetID.String()}, "{bad")
	w := httptest.NewRecorder()
	h.AllocateAsset(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

// TestAllocateAsset_BadEndDate400 covers the end_date parse leg specifically:
// start_date is valid (passing the first parseRequiredDate), so the handler
// reaches the SECOND parse and rejects the malformed end_date. The sibling
// TestAllocateAsset_BadDate trips on start_date and never gets here.
func TestAllocateAsset_BadEndDate400(t *testing.T) {
	assetID := uuid.New()
	h := NewFleetHandler(&fakeFleetService{})
	body := `{"project_id":"` + uuid.New().String() + `","start_date":"2026-03-01","end_date":"nope"}`
	r := fleetReq(t, "POST", "/api/v1/org/"+testOrgID+"/fleet/"+assetID.String()+"/allocate",
		testOrgID, map[string]string{"assetID": assetID.String()}, body)
	w := httptest.NewRecorder()
	h.AllocateAsset(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

// ---------- fleet writeServiceError mapping ----------

func TestFleetWriteServiceError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"asset not found", service.ErrFleetAssetNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"project not found", service.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"allocation conflict", service.ErrAllocationConflict, http.StatusConflict, "CONFLICT"},
		{"validation", wrapInvalid("bad"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"default", errInternal(), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	h := NewFleetHandler(&fakeFleetService{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			h.writeServiceError(w, r, tt.err)
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d, want %d", w.Code, tt.wantStatus)
			}
			if code := decodeErrCode(t, w); code != tt.wantCode {
				t.Errorf("code=%q, want %q", code, tt.wantCode)
			}
		})
	}
}

// ---------- parseRequiredDate ----------

func TestParseRequiredDate(t *testing.T) {
	if _, err := parseRequiredDate(""); err == nil {
		t.Error("empty string should error")
	}
	if _, err := parseRequiredDate("2026-13-99"); err == nil {
		t.Error("unparseable date should error")
	}
	got, err := parseRequiredDate("2026-03-01")
	if err != nil {
		t.Fatalf("YYYY-MM-DD: %v", err)
	}
	if !got.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("YYYY-MM-DD parsed = %v", got)
	}
	if _, err := parseRequiredDate("2026-03-01T12:30:00Z"); err != nil {
		t.Errorf("RFC3339: %v", err)
	}
}

// ---------- HR handler ----------

type fakeHRService struct {
	employees []models.Employee
	empErr    error
	certs     []models.Certification
	certErr   error

	createEmpResult  models.Employee
	createEmpErr     error
	createCertResult models.Certification
	createCertErr    error

	gotOrgID, gotEmployeeID uuid.UUID
	gotCreateEmpInput       service.CreateEmployeeInput
	gotCreateCertInput      service.CreateCertificationInput
}

func (f *fakeHRService) ListEmployees(_ context.Context, orgID uuid.UUID) ([]models.Employee, error) {
	f.gotOrgID = orgID
	return f.employees, f.empErr
}

func (f *fakeHRService) ListCertifications(_ context.Context, orgID, employeeID uuid.UUID) ([]models.Certification, error) {
	f.gotOrgID, f.gotEmployeeID = orgID, employeeID
	return f.certs, f.certErr
}

func (f *fakeHRService) CreateEmployee(_ context.Context, in service.CreateEmployeeInput) (models.Employee, error) {
	f.gotCreateEmpInput = in
	return f.createEmpResult, f.createEmpErr
}

func (f *fakeHRService) CreateCertification(_ context.Context, in service.CreateCertificationInput) (models.Certification, error) {
	f.gotCreateCertInput = in
	return f.createCertResult, f.createCertErr
}

func TestListEmployees_OK(t *testing.T) {
	svc := &fakeHRService{employees: []models.Employee{{ID: uuid.New(), FirstName: "Ada", LastName: "Lovelace"}}}
	h := NewHRHandler(svc)
	r := fleetReq(t, "GET", "/api/v1/org/"+testOrgID+"/employees", testOrgID, nil, "")
	w := httptest.NewRecorder()
	h.ListEmployees(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotOrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s", svc.gotOrgID, testOrgID)
	}
}

func TestListEmployees_ServiceErr500(t *testing.T) {
	h := NewHRHandler(&fakeHRService{empErr: errInternal()})
	r := fleetReq(t, "GET", "/api/v1/org/"+testOrgID+"/employees", testOrgID, nil, "")
	w := httptest.NewRecorder()
	h.ListEmployees(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

func TestListEmployees_OrgMismatch403(t *testing.T) {
	h := NewHRHandler(&fakeHRService{})
	r := fleetReq(t, "GET", "/api/v1/org/"+otherOrgID+"/employees", testOrgID,
		map[string]string{"orgID": otherOrgID}, "")
	w := httptest.NewRecorder()
	h.ListEmployees(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if code := decodeErrCode(t, w); code != "FORBIDDEN" {
		t.Errorf("code=%q, want FORBIDDEN", code)
	}
}

func TestListCertifications_OK(t *testing.T) {
	empID := uuid.New()
	svc := &fakeHRService{certs: []models.Certification{{ID: uuid.New(), EmployeeID: empID, CertType: "osha-30"}}}
	h := NewHRHandler(svc)
	r := fleetReq(t, "GET", "/api/v1/org/"+testOrgID+"/employees/"+empID.String()+"/certifications",
		testOrgID, map[string]string{"employeeID": empID.String()}, "")
	w := httptest.NewRecorder()
	h.ListCertifications(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotEmployeeID != empID {
		t.Errorf("employeeID = %s, want %s", svc.gotEmployeeID, empID)
	}
}

func TestListCertifications_BadEmployeeID(t *testing.T) {
	h := NewHRHandler(&fakeHRService{})
	r := fleetReq(t, "GET", "/api/v1/org/"+testOrgID+"/employees/not-a-uuid/certifications",
		testOrgID, map[string]string{"employeeID": "not-a-uuid"}, "")
	w := httptest.NewRecorder()
	h.ListCertifications(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestListCertifications_OrgMismatch403(t *testing.T) {
	empID := uuid.New()
	h := NewHRHandler(&fakeHRService{})
	r := fleetReq(t, "GET", "/api/v1/org/"+otherOrgID+"/employees/"+empID.String()+"/certifications",
		testOrgID, map[string]string{"orgID": otherOrgID, "employeeID": empID.String()}, "")
	w := httptest.NewRecorder()
	h.ListCertifications(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if code := decodeErrCode(t, w); code != "FORBIDDEN" {
		t.Errorf("code=%q, want FORBIDDEN", code)
	}
}

func TestListCertifications_ServiceErr500(t *testing.T) {
	empID := uuid.New()
	h := NewHRHandler(&fakeHRService{certErr: errInternal()})
	r := fleetReq(t, "GET", "/api/v1/org/"+testOrgID+"/employees/"+empID.String()+"/certifications",
		testOrgID, map[string]string{"employeeID": empID.String()}, "")
	w := httptest.NewRecorder()
	h.ListCertifications(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

func TestHRWriteServiceError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"employee not found", service.ErrEmployeeNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"validation", wrapInvalid("bad"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"default", errInternal(), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	h := NewHRHandler(&fakeHRService{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			h.writeServiceError(w, r, tt.err)
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d, want %d", w.Code, tt.wantStatus)
			}
			if code := decodeErrCode(t, w); code != tt.wantCode {
				t.Errorf("code=%q, want %q", code, tt.wantCode)
			}
		})
	}
}

// errInternal is a non-sentinel error for exercising the default 500 branch.
func errInternal() error { return errors.New("boom") }

// wrapInvalid wraps the shared ErrInvalidInput sentinel with context, matching
// how the service layer surfaces validation failures.
func wrapInvalid(msg string) error { return fmt.Errorf("%w: %s", service.ErrInvalidInput, msg) }
