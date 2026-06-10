package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
	"github.com/futurebuildai/buildos/internal/store"
)

// mockFeedbackService implements FeedbackServicer for handler tests.
type mockFeedbackService struct {
	submitResult models.Feedback
	submitErr    error
	listResult   store.FeedbackPage
	listErr      error
	triageResult models.Feedback
	triageErr    error

	lastSubmit service.SubmitFeedbackInput
	lastList   service.ListFeedbackInput
	lastTriage service.TriageFeedbackInput
}

func (m *mockFeedbackService) Submit(_ context.Context, in service.SubmitFeedbackInput) (models.Feedback, error) {
	m.lastSubmit = in
	return m.submitResult, m.submitErr
}
func (m *mockFeedbackService) ListForAdmin(_ context.Context, in service.ListFeedbackInput) (store.FeedbackPage, error) {
	m.lastList = in
	return m.listResult, m.listErr
}
func (m *mockFeedbackService) Triage(_ context.Context, in service.TriageFeedbackInput) (models.Feedback, error) {
	m.lastTriage = in
	return m.triageResult, m.triageErr
}

// ---------- POST /api/v1/feedback ----------

func TestFeedbackSubmit_Created(t *testing.T) {
	svc := &mockFeedbackService{submitResult: models.Feedback{Category: "bug", Status: "new"}}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/feedback", testOrgID, nil,
		strings.NewReader(`{"category":"bug","message":"Gantt bars misalign on resize","context":{"route":"/projects/x/schedule","role":"superintendent"}}`))
	w := httptest.NewRecorder()
	h.Submit(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	// Identity comes from claims, never the body.
	if svc.lastSubmit.OrgID.String() != testOrgID {
		t.Errorf("Submit org = %s, want %s (from claims)", svc.lastSubmit.OrgID, testOrgID)
	}
	if svc.lastSubmit.UserSub != "test-sub" {
		t.Errorf("Submit UserSub = %q, want test-sub (from claims)", svc.lastSubmit.UserSub)
	}
	if svc.lastSubmit.Category != "bug" || !strings.Contains(svc.lastSubmit.Message, "Gantt") {
		t.Errorf("Submit input = %+v", svc.lastSubmit)
	}
	if !strings.Contains(string(svc.lastSubmit.Context), `"route"`) {
		t.Errorf("Submit context not threaded: %s", svc.lastSubmit.Context)
	}
}

func TestFeedbackSubmit_BadJSON400(t *testing.T) {
	h := NewFeedbackHandler(&mockFeedbackService{})
	r := buildRequest(t, "POST", "/api/v1/feedback", testOrgID, nil, strings.NewReader(`{nope`))
	w := httptest.NewRecorder()
	h.Submit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestFeedbackSubmit_ValidationErr400(t *testing.T) {
	svc := &mockFeedbackService{submitErr: service.ErrInvalidInput}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/feedback", testOrgID, nil,
		strings.NewReader(`{"category":"rant","message":"x"}`))
	w := httptest.NewRecorder()
	h.Submit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "VALIDATION_ERROR") {
		t.Errorf("body = %s, want VALIDATION_ERROR", w.Body.String())
	}
}

func TestFeedbackSubmit_InvalidOrgClaim401(t *testing.T) {
	svc := &mockFeedbackService{}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/feedback", "not-a-uuid", nil,
		strings.NewReader(`{"category":"bug","message":"x"}`))
	w := httptest.NewRecorder()
	h.Submit(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
	if svc.lastSubmit.OrgID != uuid.Nil {
		t.Error("service should not be invoked on an unparseable org claim")
	}
}

// ---------- GET /api/v1/admin/feedback ----------

func TestFeedbackList_OK_FilterAndPaginationThreaded(t *testing.T) {
	svc := &mockFeedbackService{listResult: store.FeedbackPage{
		Feedback: []models.Feedback{{Category: "idea", Status: "new"}},
		Total:    742, Page: 3, PerPage: 200, TotalPages: 4,
	}}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/admin/feedback?status=new&page=3&per_page=200", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastList.Status != "new" || svc.lastList.Page != 3 || svc.lastList.PerPage != 200 {
		t.Errorf("list input = %+v, want status=new page=3 per_page=200", svc.lastList)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"idea"`) {
		t.Errorf("body should carry the row: %s", body)
	}
	// Pagination meta must surface so the harvest poller can detect
	// and drain a backlog (no silent truncation).
	for _, want := range []string{`"total":742`, `"total_pages":4`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing pagination meta %s: %s", want, body)
		}
	}
}

func TestFeedbackList_EmptyIsArrayNotNull(t *testing.T) {
	h := NewFeedbackHandler(&mockFeedbackService{listResult: store.FeedbackPage{Page: 1, PerPage: 100}})
	r := buildRequest(t, "GET", "/api/v1/admin/feedback", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"feedback":[]`) {
		t.Errorf("empty list must serialize as [], got %s", w.Body.String())
	}
}

func TestFeedbackSubmit_Throttled429WithRetryAfter(t *testing.T) {
	svc := &mockFeedbackService{submitErr: service.ErrRateLimited}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/feedback", testOrgID, nil,
		strings.NewReader(`{"category":"bug","message":"x"}`))
	w := httptest.NewRecorder()
	h.Submit(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d, want 429", w.Code)
	}
	if !strings.Contains(w.Body.String(), "RATE_LIMITED") {
		t.Errorf("body = %s, want RATE_LIMITED", w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 must carry Retry-After")
	}
}

// ---------- PATCH /api/v1/admin/feedback/{feedbackID} ----------

func TestFeedbackTriage_OK(t *testing.T) {
	id := uuid.New()
	svc := &mockFeedbackService{triageResult: models.Feedback{ID: id, Status: "planned"}}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "PATCH", "/api/v1/admin/feedback/"+id.String(), testOrgID,
		map[string]string{"feedbackID": id.String()},
		strings.NewReader(`{"status":"planned","triage_note":"queued for 0b"}`))
	w := httptest.NewRecorder()
	h.Triage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastTriage.ID != id || svc.lastTriage.Status != "planned" {
		t.Errorf("Triage input = %+v", svc.lastTriage)
	}
	if svc.lastTriage.TriageNote == nil || *svc.lastTriage.TriageNote != "queued for 0b" {
		t.Errorf("TriageNote = %v, want set", svc.lastTriage.TriageNote)
	}
	if svc.lastTriage.UserSub != "test-sub" {
		t.Errorf("UserSub = %q, want test-sub (from claims)", svc.lastTriage.UserSub)
	}
}

func TestFeedbackTriage_OmittedNoteIsNil(t *testing.T) {
	id := uuid.New()
	svc := &mockFeedbackService{triageResult: models.Feedback{ID: id}}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "PATCH", "/api/v1/admin/feedback/"+id.String(), testOrgID,
		map[string]string{"feedbackID": id.String()},
		strings.NewReader(`{"status":"triaged"}`))
	w := httptest.NewRecorder()
	h.Triage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if svc.lastTriage.TriageNote != nil {
		t.Errorf("omitted triage_note must stay nil (keep existing), got %q", *svc.lastTriage.TriageNote)
	}
}

func TestFeedbackTriage_NotFound404(t *testing.T) {
	id := uuid.New()
	svc := &mockFeedbackService{triageErr: service.ErrNotFound}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "PATCH", "/api/v1/admin/feedback/"+id.String(), testOrgID,
		map[string]string{"feedbackID": id.String()},
		strings.NewReader(`{"status":"triaged"}`))
	w := httptest.NewRecorder()
	h.Triage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestFeedbackTriage_BadUUID400(t *testing.T) {
	svc := &mockFeedbackService{}
	h := NewFeedbackHandler(svc)
	r := buildRequest(t, "PATCH", "/api/v1/admin/feedback/nope", testOrgID,
		map[string]string{"feedbackID": "nope"},
		strings.NewReader(`{"status":"triaged"}`))
	w := httptest.NewRecorder()
	h.Triage(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

// ---------- router wiring / RBAC ----------

// TestNewRouter_FeedbackSubmit_AnyRole proves the submit route mounts
// auth-only: a field_worker — the lowest role — can file feedback.
func TestNewRouter_FeedbackSubmit_AnyRole(t *testing.T) {
	handler := NewRouter(RouterConfig{
		DevAuthMode:     "header",
		FeedbackService: &mockFeedbackService{},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback",
		strings.NewReader(`{"category":"bug","message":"door"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dev-Auth", "fw-sub,11111111-1111-1111-1111-111111111111,field_worker")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/feedback as field_worker = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestNewRouter_FeedbackAdmin_RBAC proves the harvest surface is
// admin-gated: superintendent 403s, admin clears.
func TestNewRouter_FeedbackAdmin_RBAC(t *testing.T) {
	handler := NewRouter(RouterConfig{
		DevAuthMode:     "header",
		FeedbackService: &mockFeedbackService{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedGet("/api/v1/admin/feedback", "superintendent"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /api/v1/admin/feedback as superintendent = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, authedGet("/api/v1/admin/feedback", "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/feedback as admin = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}
