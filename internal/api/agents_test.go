package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/service"
)

// mockAgentsService implements AgentsServicer for handler tests. Both
// the daily-briefing and recommend-adjustments surfaces are exercised
// below (error-mapping is shared via writeServiceError, but each
// endpoint needs its own coverage of the soft-fail / upstream paths).
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

// ---- DailyBriefing ----------------------------------------------------

func TestDailyBriefing_HappyPath(t *testing.T) {
	svc := &mockAgentsService{
		briefingResult: service.DailyBriefing{
			Reply:      "Lead with the framing inspection, then pour footings.",
			SessionID:  uuid.MustParse(testProjID),
			TaskCount:  4,
			AlertCount: 1,
		},
	}
	h := NewAgentsHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/agents/daily-briefing", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.DailyBriefing(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	// Claims threaded through to the service: org (from X-Dev-Auth),
	// sub, and role.
	if svc.lastBriefOrg.String() != testOrgID {
		t.Errorf("service got org=%s, want %s", svc.lastBriefOrg, testOrgID)
	}
	if svc.lastBriefSub != "test-sub" {
		t.Errorf("service got sub=%q, want test-sub", svc.lastBriefSub)
	}
	if svc.lastBriefRole != "owner" {
		t.Errorf("service got role=%q, want owner", svc.lastBriefRole)
	}
	body := w.Body.String()
	for _, want := range []string{`"briefing"`, `"reply":"Lead with the framing`, `"task_count":4`, `"alert_count":1`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
}

func TestDailyBriefing_Unconfigured503(t *testing.T) {
	// No Anthropic key configured for the org — the native AI client
	// returns ai.ErrUnconfigured; the handler must map it to 503 (a
	// configuration gap, not a caller error) so the operator knows to
	// set a key in the vault.
	h := NewAgentsHandler(&mockAgentsService{briefingErr: ai.ErrUnconfigured})
	r := buildRequest(t, "POST", "/api/v1/agents/daily-briefing", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.DailyBriefing(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"SERVICE_UNAVAILABLE"`) {
		t.Errorf("body missing SERVICE_UNAVAILABLE code: %s", w.Body.String())
	}
}

func TestDailyBriefing_Transient502(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{briefingErr: ai.ErrTransient})
	r := buildRequest(t, "POST", "/api/v1/agents/daily-briefing", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.DailyBriefing(w, r)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status=%d, want 502", w.Code)
	}
}

func TestDailyBriefing_RateLimited429(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{briefingErr: ai.ErrRateLimited})
	r := buildRequest(t, "POST", "/api/v1/agents/daily-briefing", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.DailyBriefing(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status=%d, want 429", w.Code)
	}
}

func TestDailyBriefing_BadStoredKey503(t *testing.T) {
	// A 401 from Anthropic means the STORED key is bad — operator-fixable,
	// not the caller's token being rejected. The handler maps it to 503
	// so the client doesn't treat it as its own auth failure.
	h := NewAgentsHandler(&mockAgentsService{briefingErr: &ai.HTTPError{StatusCode: http.StatusUnauthorized, Type: "authentication_error"}})
	r := buildRequest(t, "POST", "/api/v1/agents/daily-briefing", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.DailyBriefing(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503, body=%s", w.Code, w.Body.String())
	}
}

func TestRecommendScheduleAdjustments_HappyPath(t *testing.T) {
	five := 5
	svc := &mockAgentsService{
		recResult: service.ScheduleAdjustmentSet{
			Adjustments: []ai.ScheduleAdjustment{
				{TaskID: uuid.MustParse(testTaskID), NewDurationDays: &five, Rationale: "weather slip"},
			},
			AppliedDeltas:        1,
			SkippedRationaleOnly: 0,
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
	for _, want := range []string{`"applied_deltas":1`, `"skipped_rationale_only":0`} {
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

func TestRecommendScheduleAdjustments_AIUnavailableReturns503(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{recErr: service.ErrAgentsAIUnavailable})
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

func TestRecommendScheduleAdjustments_AITransientReturns502(t *testing.T) {
	h := NewAgentsHandler(&mockAgentsService{recErr: ai.ErrTransient})
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
	three := 3
	svc := &mockAgentsService{
		recResult: service.ScheduleAdjustmentSet{
			Adjustments:   []ai.ScheduleAdjustment{{TaskID: uuid.MustParse(testTaskID), NewDurationDays: &three}},
			AppliedDeltas: 1,
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
