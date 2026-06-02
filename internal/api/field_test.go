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
	"github.com/futurebuildai/buildos/internal/service"
)

// ---------- field fake ----------

type fakeFieldService struct {
	syncResult     models.FieldSyncResponse
	syncErr        error
	progressResult models.TaskProgress
	progressErr    error
	checkinResult  models.CrewCheckin
	checkinErr     error
	dailyLogResult models.DailyLog
	dailyLogErr    error

	gotSyncOpts   service.SyncOptions
	gotOrgID      uuid.UUID
	gotSub        string
	gotProgressIn service.ReportProgressInput
	gotCheckinIn  service.CheckinInput
	gotDailyLogIn service.DailyLogInput
}

func (f *fakeFieldService) Sync(_ context.Context, opts service.SyncOptions) (models.FieldSyncResponse, error) {
	f.gotSyncOpts = opts
	return f.syncResult, f.syncErr
}

func (f *fakeFieldService) ReportProgress(_ context.Context, orgID uuid.UUID, sub string, in service.ReportProgressInput) (models.TaskProgress, error) {
	f.gotOrgID, f.gotSub, f.gotProgressIn = orgID, sub, in
	return f.progressResult, f.progressErr
}

func (f *fakeFieldService) Checkin(_ context.Context, orgID uuid.UUID, sub string, in service.CheckinInput) (models.CrewCheckin, error) {
	f.gotOrgID, f.gotSub, f.gotCheckinIn = orgID, sub, in
	return f.checkinResult, f.checkinErr
}

func (f *fakeFieldService) DailyLog(_ context.Context, orgID uuid.UUID, sub string, in service.DailyLogInput) (models.DailyLog, error) {
	f.gotOrgID, f.gotSub, f.gotDailyLogIn = orgID, sub, in
	return f.dailyLogResult, f.dailyLogErr
}

// fieldReq builds a caller-scoped request (field endpoints read the org
// from claims, never the URL). body may be empty.
func fieldReq(t *testing.T, method, target, callerOrgID, body string) *http.Request {
	t.Helper()
	if body == "" {
		return buildRequest(t, method, target, callerOrgID, nil, nil)
	}
	return buildRequest(t, method, target, callerOrgID, nil, strings.NewReader(body))
}

// ---------- GET /field/sync ----------

func TestFieldSync_OK(t *testing.T) {
	svc := &fakeFieldService{syncResult: models.FieldSyncResponse{ServerTime: time.Now()}}
	h := NewFieldHandler(svc)
	r := fieldReq(t, "GET", "/api/v1/field/sync?since=2026-03-01T00:00:00Z", testOrgID, "")
	w := httptest.NewRecorder()
	h.Sync(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotSyncOpts.CallerOrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s", svc.gotSyncOpts.CallerOrgID, testOrgID)
	}
	wantSince := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !svc.gotSyncOpts.Since.Equal(wantSince) {
		t.Errorf("since = %v, want %v", svc.gotSyncOpts.Since, wantSince)
	}
}

func TestFieldSync_NoSinceIsZero(t *testing.T) {
	svc := &fakeFieldService{}
	h := NewFieldHandler(svc)
	r := fieldReq(t, "GET", "/api/v1/field/sync", testOrgID, "")
	w := httptest.NewRecorder()
	h.Sync(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if !svc.gotSyncOpts.Since.IsZero() {
		t.Errorf("since = %v, want zero", svc.gotSyncOpts.Since)
	}
}

func TestFieldSync_BadSince400(t *testing.T) {
	h := NewFieldHandler(&fakeFieldService{})
	r := fieldReq(t, "GET", "/api/v1/field/sync?since=nope", testOrgID, "")
	w := httptest.NewRecorder()
	h.Sync(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestFieldSync_InvalidOrgClaim401(t *testing.T) {
	h := NewFieldHandler(&fakeFieldService{})
	r := fieldReq(t, "GET", "/api/v1/field/sync", "not-a-uuid", "")
	w := httptest.NewRecorder()
	h.Sync(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
}

func TestFieldSync_ServiceErr500(t *testing.T) {
	h := NewFieldHandler(&fakeFieldService{syncErr: errInternal()})
	r := fieldReq(t, "GET", "/api/v1/field/sync", testOrgID, "")
	w := httptest.NewRecorder()
	h.Sync(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

// ---------- POST /field/progress ----------

func TestFieldReportProgress_OK(t *testing.T) {
	taskID := uuid.New()
	idemKey := uuid.New()
	svc := &fakeFieldService{progressResult: models.TaskProgress{ID: uuid.New(), TaskID: taskID}}
	h := NewFieldHandler(svc)
	body := `{"task_id":"` + taskID.String() + `","percent_complete":75,"idempotency_key":"` + idemKey.String() + `"}`
	r := fieldReq(t, "POST", "/api/v1/field/progress", testOrgID, body)
	w := httptest.NewRecorder()
	h.ReportProgress(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotProgressIn.TaskID != taskID || svc.gotProgressIn.PercentComplete != 75 ||
		svc.gotProgressIn.IdempotencyKey != idemKey {
		t.Errorf("progress input = %+v", svc.gotProgressIn)
	}
	if svc.gotSub != "test-sub" {
		t.Errorf("sub = %q, want test-sub", svc.gotSub)
	}
}

func TestFieldReportProgress_BadJSON(t *testing.T) {
	h := NewFieldHandler(&fakeFieldService{})
	r := fieldReq(t, "POST", "/api/v1/field/progress", testOrgID, "{bad")
	w := httptest.NewRecorder()
	h.ReportProgress(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestFieldReportProgress_Conflict409(t *testing.T) {
	taskID := uuid.New()
	h := NewFieldHandler(&fakeFieldService{progressErr: service.ErrIdempotencyConflict})
	body := `{"task_id":"` + taskID.String() + `","percent_complete":10,"idempotency_key":"` + uuid.New().String() + `"}`
	r := fieldReq(t, "POST", "/api/v1/field/progress", testOrgID, body)
	w := httptest.NewRecorder()
	h.ReportProgress(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409", w.Code)
	}
	if code := decodeErrCode(t, w); code != "CONFLICT" {
		t.Errorf("code=%q, want CONFLICT", code)
	}
}

// ---------- POST /field/checkin ----------

func TestFieldCheckin_OK(t *testing.T) {
	projID := uuid.New()
	idemKey := uuid.New()
	svc := &fakeFieldService{checkinResult: models.CrewCheckin{ID: uuid.New(), ProjectID: projID}}
	h := NewFieldHandler(svc)
	body := `{"project_id":"` + projID.String() + `","crew_members":[{"worker_id":"w1"}],"idempotency_key":"` + idemKey.String() + `"}`
	r := fieldReq(t, "POST", "/api/v1/field/checkin", testOrgID, body)
	w := httptest.NewRecorder()
	h.Checkin(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotCheckinIn.ProjectID != projID || svc.gotCheckinIn.IdempotencyKey != idemKey {
		t.Errorf("checkin input = %+v", svc.gotCheckinIn)
	}
	if string(svc.gotCheckinIn.CrewMembers) != `[{"worker_id":"w1"}]` {
		t.Errorf("crew_members = %s", svc.gotCheckinIn.CrewMembers)
	}
}

func TestFieldCheckin_BadJSON(t *testing.T) {
	h := NewFieldHandler(&fakeFieldService{})
	r := fieldReq(t, "POST", "/api/v1/field/checkin", testOrgID, "{bad")
	w := httptest.NewRecorder()
	h.Checkin(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

// ---------- POST /field/daily-log ----------

func TestFieldDailyLog_OK(t *testing.T) {
	projID := uuid.New()
	idemKey := uuid.New()
	svc := &fakeFieldService{dailyLogResult: models.DailyLog{ID: uuid.New(), ProjectID: projID}}
	h := NewFieldHandler(svc)
	body := `{"project_id":"` + projID.String() + `","log_date":"2026-03-01","work_summary":"poured slab","idempotency_key":"` + idemKey.String() + `"}`
	r := fieldReq(t, "POST", "/api/v1/field/daily-log", testOrgID, body)
	w := httptest.NewRecorder()
	h.DailyLog(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotDailyLogIn.ProjectID != projID || svc.gotDailyLogIn.WorkSummary != "poured slab" ||
		svc.gotDailyLogIn.IdempotencyKey != idemKey {
		t.Errorf("daily-log input = %+v", svc.gotDailyLogIn)
	}
	wantDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !svc.gotDailyLogIn.LogDate.Equal(wantDate) {
		t.Errorf("log_date = %v, want %v", svc.gotDailyLogIn.LogDate, wantDate)
	}
}

func TestFieldDailyLog_BadJSON(t *testing.T) {
	h := NewFieldHandler(&fakeFieldService{})
	r := fieldReq(t, "POST", "/api/v1/field/daily-log", testOrgID, "{bad")
	w := httptest.NewRecorder()
	h.DailyLog(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestFieldDailyLog_BadDate400(t *testing.T) {
	projID := uuid.New()
	h := NewFieldHandler(&fakeFieldService{})
	body := `{"project_id":"` + projID.String() + `","log_date":"nope","work_summary":"x","idempotency_key":"` + uuid.New().String() + `"}`
	r := fieldReq(t, "POST", "/api/v1/field/daily-log", testOrgID, body)
	w := httptest.NewRecorder()
	h.DailyLog(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

// ---------- field writeServiceError mapping ----------

func TestFieldWriteServiceError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"idempotency conflict", service.ErrIdempotencyConflict, http.StatusConflict, "CONFLICT"},
		{"not found", service.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"validation", wrapInvalid("bad"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"default", errInternal(), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	h := NewFieldHandler(&fakeFieldService{})
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
