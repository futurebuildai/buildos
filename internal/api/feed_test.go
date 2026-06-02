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
)

// ---------- feed fake ----------

type fakeFeedService struct {
	listResult  service.FeedListResult
	listErr     error
	dismissCard models.FeedCard
	dismissErr  error
	actionCard  models.FeedCard
	actionErr   error

	gotListOpts service.FeedListOptions
	gotOrgID    uuid.UUID
	gotSub      string
	gotCardID   uuid.UUID
	gotActionIn service.FeedActionInput
}

func (f *fakeFeedService) ListFeed(_ context.Context, opts service.FeedListOptions) (service.FeedListResult, error) {
	f.gotListOpts = opts
	return f.listResult, f.listErr
}

func (f *fakeFeedService) DismissCard(_ context.Context, orgID uuid.UUID, sub string, cardID uuid.UUID) (models.FeedCard, error) {
	f.gotOrgID, f.gotSub, f.gotCardID = orgID, sub, cardID
	return f.dismissCard, f.dismissErr
}

func (f *fakeFeedService) ActionCard(_ context.Context, orgID uuid.UUID, sub string, cardID uuid.UUID, in service.FeedActionInput) (models.FeedCard, error) {
	f.gotOrgID, f.gotSub, f.gotCardID, f.gotActionIn = orgID, sub, cardID, in
	return f.actionCard, f.actionErr
}

// feedReq builds a caller-scoped request (feed endpoints read the org
// from claims, never the URL). body may be empty.
func feedReq(t *testing.T, method, target, callerOrgID string, params map[string]string, body string) *http.Request {
	t.Helper()
	if body == "" {
		return buildRequest(t, method, target, callerOrgID, params, nil)
	}
	return buildRequest(t, method, target, callerOrgID, params, strings.NewReader(body))
}

// ---------- GET /feed ----------

func TestFeedList_OK(t *testing.T) {
	svc := &fakeFeedService{listResult: service.FeedListResult{
		Cards: []models.FeedCard{{ID: uuid.New(), Title: "Weather alert", Priority: "urgent"}},
		Total: 1,
	}}
	h := NewFeedHandler(svc)
	r := feedReq(t, "GET", "/api/v1/feed?status=active,dismissed&priority=urgent&page=2&per_page=10", testOrgID, nil, "")
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotListOpts.CallerOrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s", svc.gotListOpts.CallerOrgID, testOrgID)
	}
	if svc.gotListOpts.CallerOIDCSubject != "test-sub" || svc.gotListOpts.CallerRole != "owner" {
		t.Errorf("caller = %q/%q, want test-sub/owner", svc.gotListOpts.CallerOIDCSubject, svc.gotListOpts.CallerRole)
	}
	if got := svc.gotListOpts.StatusFilter; len(got) != 2 || got[0] != "active" || got[1] != "dismissed" {
		t.Errorf("status filter = %v, want [active dismissed]", got)
	}
	if got := svc.gotListOpts.PriorityFilter; len(got) != 1 || got[0] != "urgent" {
		t.Errorf("priority filter = %v, want [urgent]", got)
	}
	if svc.gotListOpts.Page != 2 || svc.gotListOpts.PerPage != 10 {
		t.Errorf("pagination = %d/%d, want 2/10", svc.gotListOpts.Page, svc.gotListOpts.PerPage)
	}
}

func TestFeedList_InvalidOrgClaim401(t *testing.T) {
	h := NewFeedHandler(&fakeFeedService{})
	r := feedReq(t, "GET", "/api/v1/feed", "not-a-uuid", nil, "")
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
}

func TestFeedList_ServiceErr500(t *testing.T) {
	h := NewFeedHandler(&fakeFeedService{listErr: errInternal()})
	r := feedReq(t, "GET", "/api/v1/feed", testOrgID, nil, "")
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

// ---------- POST /feed/{cardID}/action ----------

func TestFeedAction_OK(t *testing.T) {
	cardID := uuid.New()
	svc := &fakeFeedService{actionCard: models.FeedCard{ID: cardID, Status: "actioned"}}
	h := NewFeedHandler(svc)
	r := feedReq(t, "POST", "/api/v1/feed/"+cardID.String()+"/action", testOrgID,
		map[string]string{"cardID": cardID.String()}, `{"action_type":"approve","payload":{"k":"v"}}`)
	w := httptest.NewRecorder()
	h.Action(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotCardID != cardID {
		t.Errorf("cardID = %s, want %s", svc.gotCardID, cardID)
	}
	if svc.gotActionIn.ActionType != "approve" {
		t.Errorf("action_type = %q, want approve", svc.gotActionIn.ActionType)
	}
	if string(svc.gotActionIn.Payload) != `{"k":"v"}` {
		t.Errorf("payload = %s, want {\"k\":\"v\"}", svc.gotActionIn.Payload)
	}
	if svc.gotSub != "test-sub" {
		t.Errorf("sub = %q, want test-sub", svc.gotSub)
	}
}

func TestFeedAction_BadCardID(t *testing.T) {
	h := NewFeedHandler(&fakeFeedService{})
	r := feedReq(t, "POST", "/api/v1/feed/not-a-uuid/action", testOrgID,
		map[string]string{"cardID": "not-a-uuid"}, `{"action_type":"approve"}`)
	w := httptest.NewRecorder()
	h.Action(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestFeedAction_BadJSON(t *testing.T) {
	cardID := uuid.New()
	h := NewFeedHandler(&fakeFeedService{})
	r := feedReq(t, "POST", "/api/v1/feed/"+cardID.String()+"/action", testOrgID,
		map[string]string{"cardID": cardID.String()}, "{bad")
	w := httptest.NewRecorder()
	h.Action(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestFeedAction_NotFound404(t *testing.T) {
	cardID := uuid.New()
	h := NewFeedHandler(&fakeFeedService{actionErr: service.ErrFeedCardNotFound})
	r := feedReq(t, "POST", "/api/v1/feed/"+cardID.String()+"/action", testOrgID,
		map[string]string{"cardID": cardID.String()}, `{"action_type":"approve"}`)
	w := httptest.NewRecorder()
	h.Action(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
	if code := decodeErrCode(t, w); code != "NOT_FOUND" {
		t.Errorf("code=%q, want NOT_FOUND", code)
	}
}

// ---------- POST /feed/{cardID}/dismiss ----------

func TestFeedDismiss_OK(t *testing.T) {
	cardID := uuid.New()
	svc := &fakeFeedService{dismissCard: models.FeedCard{ID: cardID, Status: "dismissed"}}
	h := NewFeedHandler(svc)
	r := feedReq(t, "POST", "/api/v1/feed/"+cardID.String()+"/dismiss", testOrgID,
		map[string]string{"cardID": cardID.String()}, "")
	w := httptest.NewRecorder()
	h.Dismiss(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotCardID != cardID || svc.gotSub != "test-sub" {
		t.Errorf("dismiss got card=%s sub=%q", svc.gotCardID, svc.gotSub)
	}
}

func TestFeedDismiss_BadCardID(t *testing.T) {
	h := NewFeedHandler(&fakeFeedService{})
	r := feedReq(t, "POST", "/api/v1/feed/not-a-uuid/dismiss", testOrgID,
		map[string]string{"cardID": "not-a-uuid"}, "")
	w := httptest.NewRecorder()
	h.Dismiss(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestFeedDismiss_ServiceErr500(t *testing.T) {
	cardID := uuid.New()
	h := NewFeedHandler(&fakeFeedService{dismissErr: errInternal()})
	r := feedReq(t, "POST", "/api/v1/feed/"+cardID.String()+"/dismiss", testOrgID,
		map[string]string{"cardID": cardID.String()}, "")
	w := httptest.NewRecorder()
	h.Dismiss(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

// ---------- feed writeServiceError mapping ----------

func TestFeedWriteServiceError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"card not found", service.ErrFeedCardNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"validation", wrapInvalid("bad"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"default", errInternal(), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	h := NewFeedHandler(&fakeFeedService{})
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

// ---------- splitCSVParam ----------

func TestSplitCSVParam(t *testing.T) {
	mk := func(q string) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/x?k="+q, nil)
	}
	if got := splitCSVParam(mk(""), "k"); got != nil {
		t.Errorf("missing param = %v, want nil", got)
	}
	got := splitCSVParam(mk("a,%20b%20,,c"), "k") // " b " trimmed, empty segments dropped
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("got %v, want [a b c]", got)
	}
}
