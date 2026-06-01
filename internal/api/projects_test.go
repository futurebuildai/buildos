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

// mockProjectService implements ProjectServicer for handler tests. Each
// method records its input on a captured-args field and returns the
// corresponding result/err. Mirrors mockSetupService / mockBudgetService.
type mockProjectService struct {
	listResult []models.Project
	listErr    error

	getResult models.Project
	getErr    error

	createResult models.Project
	createErr    error

	updateResult models.Project
	updateErr    error

	// Captured args for assertions.
	lastListInput   service.ListProjectsInput
	lastGetOrgID    uuid.UUID
	lastGetID       uuid.UUID
	lastCreateInput service.CreateProjectInput
	lastUpdateInput service.UpdateProjectInput
}

func (m *mockProjectService) ListProjects(_ context.Context, in service.ListProjectsInput) ([]models.Project, error) {
	m.lastListInput = in
	return m.listResult, m.listErr
}
func (m *mockProjectService) GetProject(_ context.Context, orgID, projectID uuid.UUID) (models.Project, error) {
	m.lastGetOrgID = orgID
	m.lastGetID = projectID
	return m.getResult, m.getErr
}
func (m *mockProjectService) CreateProject(_ context.Context, in service.CreateProjectInput) (models.Project, error) {
	m.lastCreateInput = in
	return m.createResult, m.createErr
}
func (m *mockProjectService) UpdateProject(_ context.Context, in service.UpdateProjectInput) (models.Project, error) {
	m.lastUpdateInput = in
	return m.updateResult, m.updateErr
}

// ---------- GET /projects ----------

func TestProjects_List_OK(t *testing.T) {
	orgUUID := uuid.MustParse(testOrgID)
	svc := &mockProjectService{
		listResult: []models.Project{
			{ID: uuid.MustParse(testProjID), OrgID: orgUUID, Name: "Maple St", Status: "active"},
		},
	}
	h := NewProjectHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects?status=active&page=2&per_page=10", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastListInput.OrgID != orgUUID {
		t.Errorf("List got org=%s, want %s", svc.lastListInput.OrgID, orgUUID)
	}
	if svc.lastListInput.Status != "active" || svc.lastListInput.Page != 2 || svc.lastListInput.PerPage != 10 {
		t.Errorf("query params not forwarded: %+v", svc.lastListInput)
	}
	var env struct {
		Data struct {
			Projects []models.Project `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data.Projects) != 1 || env.Data.Projects[0].Name != "Maple St" {
		t.Errorf("projects not surfaced: %+v", env.Data.Projects)
	}
}

func TestProjects_List_EmptyIsStableArray(t *testing.T) {
	svc := &mockProjectService{listResult: nil}
	h := NewProjectHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	// A nil slice must serialize as [] not null.
	if !strings.Contains(w.Body.String(), `"projects":[]`) {
		t.Errorf("empty list not stable []: %s", w.Body.String())
	}
}

func TestProjects_List_MalformedPagingFallsBack(t *testing.T) {
	svc := &mockProjectService{}
	h := NewProjectHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects?page=abc&per_page=xyz", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	// Malformed page/per_page parse to 0 → service defaults.
	if svc.lastListInput.Page != 0 || svc.lastListInput.PerPage != 0 {
		t.Errorf("malformed paging not zeroed: %+v", svc.lastListInput)
	}
}

func TestProjects_List_InvalidOrgIDClaim_401(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{})
	r := buildRequest(t, "GET", "/api/v1/projects", "not-a-uuid", nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", w.Code)
	}
}

// ---------- POST /projects ----------

func TestProjects_Create_HappyPath(t *testing.T) {
	orgUUID := uuid.MustParse(testOrgID)
	svc := &mockProjectService{
		createResult: models.Project{ID: uuid.MustParse(testProjID), OrgID: orgUUID, Name: "Maple St", Status: "active"},
	}
	h := NewProjectHandler(svc)
	body := strings.NewReader(`{"name":"Maple St","address":"1 Maple","permit_issued_date":"2026-03-01","project_start_date":"2026-04-01","gsf":3000}`)
	r := buildRequest(t, "POST", "/api/v1/projects", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCreateInput.OrgID != orgUUID || svc.lastCreateInput.Name != "Maple St" {
		t.Errorf("create input not forwarded: %+v", svc.lastCreateInput)
	}
	if svc.lastCreateInput.UserSub != "test-sub" {
		t.Errorf("UserSub=%q, want test-sub", svc.lastCreateInput.UserSub)
	}
	if svc.lastCreateInput.Address == nil || *svc.lastCreateInput.Address != "1 Maple" {
		t.Errorf("address not forwarded: %+v", svc.lastCreateInput.Address)
	}
	if svc.lastCreateInput.GSF == nil || *svc.lastCreateInput.GSF != 3000 {
		t.Errorf("gsf not forwarded: %+v", svc.lastCreateInput.GSF)
	}
	if svc.lastCreateInput.PermitIssuedDate == nil ||
		svc.lastCreateInput.PermitIssuedDate.Year() != 2026 ||
		svc.lastCreateInput.PermitIssuedDate.Month() != 3 {
		t.Errorf("permit_issued_date parse wrong: %v", svc.lastCreateInput.PermitIssuedDate)
	}
	if svc.lastCreateInput.ProjectStartDate == nil ||
		svc.lastCreateInput.ProjectStartDate.Month() != 4 {
		t.Errorf("project_start_date parse wrong: %v", svc.lastCreateInput.ProjectStartDate)
	}
}

func TestProjects_Create_InvalidJSON_400(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{})
	r := buildRequest(t, "POST", "/api/v1/projects", testOrgID, nil, strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestProjects_Create_BadPermitDate_400(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{})
	body := strings.NewReader(`{"name":"x","permit_issued_date":"yesterday"}`)
	r := buildRequest(t, "POST", "/api/v1/projects", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestProjects_Create_BadStartDate_400(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{})
	body := strings.NewReader(`{"name":"x","project_start_date":"someday"}`)
	r := buildRequest(t, "POST", "/api/v1/projects", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestProjects_Create_ServiceValidationError_400(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{createErr: service.ErrInvalidInput})
	body := strings.NewReader(`{"name":""}`)
	r := buildRequest(t, "POST", "/api/v1/projects", testOrgID, nil, body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ---------- GET /projects/{projectID} ----------

func TestProjects_Get_HappyPath(t *testing.T) {
	orgUUID := uuid.MustParse(testOrgID)
	projUUID := uuid.MustParse(testProjID)
	svc := &mockProjectService{
		getResult: models.Project{ID: projUUID, OrgID: orgUUID, Name: "Maple St"},
	}
	h := NewProjectHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID, testOrgID,
		map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.Get(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastGetOrgID != orgUUID || svc.lastGetID != projUUID {
		t.Errorf("get args not forwarded: org=%s id=%s", svc.lastGetOrgID, svc.lastGetID)
	}
}

func TestProjects_Get_BadProjectID_400(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{})
	r := buildRequest(t, "GET", "/api/v1/projects/not-a-uuid", testOrgID,
		map[string]string{"projectID": "not-a-uuid"}, nil)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestProjects_Get_NotFound_404(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{getErr: service.ErrNotFound})
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID, testOrgID,
		map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

// ---------- PUT /projects/{projectID} ----------

func TestProjects_Update_HappyPath(t *testing.T) {
	orgUUID := uuid.MustParse(testOrgID)
	projUUID := uuid.MustParse(testProjID)
	svc := &mockProjectService{
		updateResult: models.Project{ID: projUUID, OrgID: orgUUID, Name: "Renamed", Status: "completed"},
	}
	h := NewProjectHandler(svc)
	body := strings.NewReader(`{"name":"Renamed","status":"completed","gsf":4000}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID, testOrgID,
		map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastUpdateInput.OrgID != orgUUID || svc.lastUpdateInput.ProjectID != projUUID {
		t.Errorf("update scope not forwarded: org=%s id=%s", svc.lastUpdateInput.OrgID, svc.lastUpdateInput.ProjectID)
	}
	if svc.lastUpdateInput.UserSub != "test-sub" {
		t.Errorf("UserSub=%q, want test-sub", svc.lastUpdateInput.UserSub)
	}
	if svc.lastUpdateInput.Name == nil || *svc.lastUpdateInput.Name != "Renamed" {
		t.Errorf("name not forwarded: %+v", svc.lastUpdateInput.Name)
	}
	if svc.lastUpdateInput.Status == nil || *svc.lastUpdateInput.Status != "completed" {
		t.Errorf("status not forwarded: %+v", svc.lastUpdateInput.Status)
	}
	if svc.lastUpdateInput.GSF == nil || *svc.lastUpdateInput.GSF != 4000 {
		t.Errorf("gsf not forwarded: %+v", svc.lastUpdateInput.GSF)
	}
}

func TestProjects_Update_BadProjectID_400(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{})
	body := strings.NewReader(`{"name":"x"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/not-a-uuid", testOrgID,
		map[string]string{"projectID": "not-a-uuid"}, body)
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestProjects_Update_InvalidJSON_400(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{})
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID, testOrgID,
		map[string]string{"projectID": testProjID}, strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestProjects_Update_NotFound_404(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{updateErr: service.ErrNotFound})
	body := strings.NewReader(`{"name":"x"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID, testOrgID,
		map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

// ---------- error mapping coverage ----------

func TestProjects_ErrorMapping_UnknownErrorIs500(t *testing.T) {
	h := NewProjectHandler(&mockProjectService{listErr: assertErr("storage exploded")})
	r := buildRequest(t, "GET", "/api/v1/projects", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
	// Body must NOT leak the underlying error message.
	if strings.Contains(w.Body.String(), "exploded") {
		t.Errorf("internal error leaked to client: %s", w.Body.String())
	}
}
