package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/physics"
	"github.com/futurebuildai/buildos/internal/service"
)

// mockScheduleService implements ScheduleServicer for handler tests.
type mockScheduleService struct {
	recalcResult *physics.CPMResult
	recalcTook   time.Duration
	recalcErr    error

	ganttResult service.GanttView
	ganttErr    error

	listResult []models.ProjectTask
	listErr    error

	updateResult models.ProjectTask
	updateErr    error

	// Captured args
	lastListInput   service.ListProjectTasksInput
	lastUpdateInput service.UpdateTaskInput
	lastRecalcOrg   uuid.UUID
	lastGanttOrg    uuid.UUID
}

func (m *mockScheduleService) RecalculateSchedule(_ context.Context, _, callerOrgID uuid.UUID) (*physics.CPMResult, time.Duration, error) {
	m.lastRecalcOrg = callerOrgID
	return m.recalcResult, m.recalcTook, m.recalcErr
}
func (m *mockScheduleService) GetGantt(_ context.Context, _, callerOrgID uuid.UUID) (service.GanttView, error) {
	m.lastGanttOrg = callerOrgID
	return m.ganttResult, m.ganttErr
}
func (m *mockScheduleService) ListProjectTasks(_ context.Context, in service.ListProjectTasksInput) ([]models.ProjectTask, error) {
	m.lastListInput = in
	return m.listResult, m.listErr
}
func (m *mockScheduleService) UpdateTask(_ context.Context, in service.UpdateTaskInput) (models.ProjectTask, error) {
	m.lastUpdateInput = in
	return m.updateResult, m.updateErr
}

const testTaskID = "88888888-8888-8888-8888-888888888888"

func TestRecalculate_HappyPath(t *testing.T) {
	svc := &mockScheduleService{
		recalcResult: &physics.CPMResult{
			ProjectEnd:          time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			CriticalPath:        []string{"6.0", "9.0"},
			CriticalPathChanged: true,
		},
		recalcTook: 187 * time.Millisecond,
	}
	h := NewScheduleHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recalculate",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.Recalculate(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastRecalcOrg.String() != testOrgID {
		t.Errorf("service got org=%s, want %s", svc.lastRecalcOrg, testOrgID)
	}
	if !strings.Contains(w.Body.String(), `"recalculation_ms":187`) {
		t.Errorf("response missing recalculation_ms: %s", w.Body.String())
	}
}

func TestRecalculate_NotFoundReturns404(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{
		recalcErr: service.ErrNotFound,
	})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recalculate",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.Recalculate(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestGantt_OK(t *testing.T) {
	svc := &mockScheduleService{
		ganttResult: service.GanttView{
			Tasks:        []models.ProjectTask{{Name: "Foundation", IsCritical: true}},
			CriticalPath: []uuid.UUID{uuid.MustParse(testTaskID)},
			ProjectEnd:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	h := NewScheduleHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID+"/schedule/gantt",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.Gantt(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
}

func TestListTasks_FilterParsing(t *testing.T) {
	svc := &mockScheduleService{}
	h := NewScheduleHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID+"/tasks?status=pending&is_critical=true",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.ListTasks(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastListInput.Status != "pending" {
		t.Errorf("status filter = %q, want pending", svc.lastListInput.Status)
	}
	if svc.lastListInput.IsCritical == nil || *svc.lastListInput.IsCritical != true {
		t.Errorf("is_critical filter not parsed; got %v", svc.lastListInput.IsCritical)
	}
}

func TestListTasks_BadIsCriticalReturns400(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{})
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID+"/tasks?is_critical=maybe",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.ListTasks(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestUpdateTask_HappyPath(t *testing.T) {
	svc := &mockScheduleService{
		updateResult: models.ProjectTask{Status: "completed", PercentComplete: 100},
	}
	h := NewScheduleHandler(svc)
	body := strings.NewReader(`{"status":"completed","percent_complete":100}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/tasks/"+testTaskID,
		testOrgID, map[string]string{"projectID": testProjID, "taskID": testTaskID}, body)
	w := httptest.NewRecorder()
	h.UpdateTask(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastUpdateInput.Status == nil || *svc.lastUpdateInput.Status != "completed" {
		t.Errorf("status passed = %v", svc.lastUpdateInput.Status)
	}
	if svc.lastUpdateInput.PercentComplete == nil || *svc.lastUpdateInput.PercentComplete != 100 {
		t.Errorf("percent_complete passed = %v", svc.lastUpdateInput.PercentComplete)
	}
}

func TestUpdateTask_InvalidInputMaps400(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{
		updateErr: service.ErrInvalidInput,
	})
	body := strings.NewReader(`{"percent_complete":150}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/tasks/"+testTaskID,
		testOrgID, map[string]string{"projectID": testProjID, "taskID": testTaskID}, body)
	w := httptest.NewRecorder()
	h.UpdateTask(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestUpdateTask_NotFoundReturns404(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{
		updateErr: service.ErrNotFound,
	})
	body := strings.NewReader(`{"status":"in_progress"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/tasks/"+testTaskID,
		testOrgID, map[string]string{"projectID": testProjID, "taskID": testTaskID}, body)
	w := httptest.NewRecorder()
	h.UpdateTask(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestUpdateTask_BadJSONReturns400(t *testing.T) {
	h := NewScheduleHandler(&mockScheduleService{})
	body := strings.NewReader("not-json")
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/tasks/"+testTaskID,
		testOrgID, map[string]string{"projectID": testProjID, "taskID": testTaskID}, body)
	w := httptest.NewRecorder()
	h.UpdateTask(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}
