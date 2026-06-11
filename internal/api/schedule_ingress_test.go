package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// TestImport_PassesClaimsAndDefaultsRecalc proves the handler derives org+sub
// from the JWT claims (never the body) and that `recalculate` defaults to TRUE
// when omitted (the keystone goal is a populated Gantt).
func TestImport_PassesClaimsAndDefaultsRecalc(t *testing.T) {
	svc := &mockScheduleService{importResult: &service.ImportScheduleResult{DependencyCount: 1}}
	h := NewScheduleHandler(svc)
	body := strings.NewReader(`{"tasks":[{"wbs_code":"01-00","name":"Site","duration_days":3}],
		"dependencies":[{"predecessor_code":"01-00","successor_code":"01-00"}]}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/import",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Import(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastImportOrg.String() != testOrgID {
		t.Errorf("import org = %s, want %s (from claims)", svc.lastImportOrg, testOrgID)
	}
	if svc.lastImportUserSub != "test-sub" {
		t.Errorf("import user_sub = %q, want test-sub", svc.lastImportUserSub)
	}
	if !svc.lastImportInput.Recalculate {
		t.Error("recalculate defaulted to false, want true when omitted")
	}
}

// TestImport_RecalcExplicitFalse proves an explicit recalculate:false is honored.
func TestImport_RecalcExplicitFalse(t *testing.T) {
	svc := &mockScheduleService{importResult: &service.ImportScheduleResult{}}
	h := NewScheduleHandler(svc)
	body := strings.NewReader(`{"tasks":[{"wbs_code":"01-00","name":"Site","duration_days":3}],"recalculate":false}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/import",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Import(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastImportInput.Recalculate {
		t.Error("recalculate = true, want false (explicit)")
	}
}

// TestImport_BadJSON400 proves a malformed body is a 400 before the service.
func TestImport_BadJSON400(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{})
	body := strings.NewReader(`{not json`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/import",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Import(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// TestImport_ServiceInvalidInputMaps400 proves an ErrInvalidInput from the
// service (e.g. cycle, self-loop, bad duration) maps to 400.
func TestImport_ServiceInvalidInputMaps400(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{importErr: service.ErrInvalidInput})
	body := strings.NewReader(`{"tasks":[{"wbs_code":"01-00","name":"Site","duration_days":3}]}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/import",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Import(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// TestImport_NotFoundMaps404 proves a cross-tenant project surfaces as 404.
func TestImport_NotFoundMaps404(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{importErr: service.ErrNotFound})
	body := strings.NewReader(`{"tasks":[{"wbs_code":"01-00","name":"Site","duration_days":3}]}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/import",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Import(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

// TestCreateTask_PassesClaims proves the single-task create derives org+sub
// from claims and returns 201.
func TestCreateTask_PassesClaims(t *testing.T) {
	svc := &mockScheduleService{createTaskResult: models.ProjectTask{WBSCode: "01-00"}}
	h := NewScheduleHandler(svc)
	body := strings.NewReader(`{"wbs_code":"01-00","name":"Site","duration_days":3}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/tasks",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.CreateTask(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCreateTaskIn.OrgID.String() != testOrgID {
		t.Errorf("create task org = %s, want %s (from claims)", svc.lastCreateTaskIn.OrgID, testOrgID)
	}
	if svc.lastCreateTaskIn.CallerUserSub != "test-sub" {
		t.Errorf("create task user_sub = %q, want test-sub", svc.lastCreateTaskIn.CallerUserSub)
	}
}
