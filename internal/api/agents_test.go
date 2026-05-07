package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/service"
)

// mockAgentsService implements AgentsServicer for handler tests. The
// daily-briefing surface stays out of test scope here (covered by the
// service's own validation tests + the existing brain client tests);
// only the new RecommendScheduleAdjustments path is exercised below.
type mockAgentsService struct {
	briefingResult service.DailyBriefing
	briefingErr    error

	recResult service.ScheduleAdjustmentSet
	recErr    error

	// Captured args
	lastBriefOrg     uuid.UUID
	lastBriefSub     string
	lastBriefRole    string
	lastRecOrg       uuid.UUID
	lastRecUserSub   string
	lastRecProjectID uuid.UUID
}

func (m *mockAgentsService) GenerateDailyBriefing(_ context.Context, callerOrgID uuid.UUID, callerOIDCSubject, callerRole string) (service.DailyBriefing, error) {
	m.lastBriefOrg = callerOrgID
	m.lastBriefSub = callerOIDCSubject
	m.lastBriefRole = callerRole
	return m.briefingResult, m.briefingErr
}

func (m *mockAgentsService) RecommendScheduleAdjustments(_ context.Context, callerOrgID uuid.UUID, callerUserSub string, projectID uuid.UUID) (service.ScheduleAdjustmentSet, error) {
	m.lastRecOrg = callerOrgID
	m.lastRecUserSub = callerUserSub
	m.lastRecProjectID = projectID
	return m.recResult, m.recErr
}

func TestRecommendScheduleAdjustments_HappyPath(t *testing.T) {
	runID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	five := 5
	svc := &mockAgentsService{
		recResult: service.ScheduleAdjustmentSet{
			Adjustments: []brain.ScheduleAdjustment{
				{TaskID: uuid.MustParse(testTaskID), NewDurationDays: &five, Rationale: "weather slip"},
			},
			AppliedDeltas:        1,
			SkippedRationaleOnly: 0,
			RunID:                runID,
			TokensUsed:           420,
			CostCents:            13,
			CurrencyCode:         "USD",
		},
	}
	h := NewAgentsHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastRecOrg.String() != testOrgID {
		t.Errorf("service got org=%s, want %s", svc.lastRecOrg, testOrgID)
	}
	if svc.lastRecProjectID.String() != testProjID {
		t.Errorf("service got project=%s, want %s", svc.lastRecProjectID, testProjID)
	}
	if svc.lastRecUserSub != "test-sub" {
		t.Errorf("service got user_sub=%q, want test-sub", svc.lastRecUserSub)
	}
	body := w.Body.String()
	for _, want := range []string{`"applied_deltas":1`, `"run_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"`, `"cost_cents":13`, `"currency_code":"USD"`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
}

func TestRecommendScheduleAdjustments_BadProjectIDReturns400(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{})
	r := buildRequest(t, "POST", "/api/v1/projects/not-a-uuid/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": "not-a-uuid"}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestRecommendScheduleAdjustments_NotFoundReturns404(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{recErr: service.ErrNotFound})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestRecommendScheduleAdjustments_InvalidInputReturns400(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{recErr: service.ErrInvalidInput})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestRecommendScheduleAdjustments_MaestroUnavailableReturns503(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{recErr: service.ErrAgentsMaestroUnavailable})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"SERVICE_UNAVAILABLE"`) {
		t.Errorf("body missing SERVICE_UNAVAILABLE code: %s", w.Body.String())
	}
}

func TestRecommendScheduleAdjustments_ScheduleServiceUnavailableReturns503(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{recErr: service.ErrAgentsScheduleServiceUnavailable})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d, want 503", w.Code)
	}
}

func TestRecommendScheduleAdjustments_BrainTransientReturns502(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{recErr: brain.ErrTransient})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status=%d, want 502", w.Code)
	}
}

func TestRecommendScheduleAdjustments_RecalcDeferredReturns200(t *testing.T) {
	// Service's "apply succeeded; recalc deferred" path returns a
	// non-zero result alongside a wrapped error. The handler must
	// surface 200 with the result body — the deltas were persisted;
	// reporting 5xx would mislead the caller into thinking the
	// adjustments weren't applied.
	runID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	three := 3
	svc := &mockAgentsService{
		recResult: service.ScheduleAdjustmentSet{
			Adjustments:   []brain.ScheduleAdjustment{{TaskID: uuid.MustParse(testTaskID), NewDurationDays: &three}},
			AppliedDeltas: 1,
			RunID:         runID,
			CurrencyCode:  "USD",
		},
		recErr: errors.New("apply succeeded; recalc deferred: deadline exceeded"),
	}
	h := NewAgentsHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/schedule/recommend-adjustments",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.RecommendScheduleAdjustments(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d (recalc-deferred should still 200), body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"applied_deltas":1`) {
		t.Errorf("body missing applied_deltas: %s", w.Body.String())
	}
}
