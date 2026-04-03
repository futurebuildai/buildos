package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// withChiParam returns a context with the given chi URL param set.
func withChiParam(ctx context.Context, key, value string) context.Context {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

// withChiParams returns a context with multiple chi URL params set.
func withChiParams(ctx context.Context, params map[string]string) context.Context {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

// validClaims returns a Claims value with valid UUIDs for Sub and OrgID.
func validClaims() mw.Claims {
	return mw.Claims{
		Sub:   uuid.New().String(),
		OrgID: uuid.New().String(),
		Role:  "foreman",
	}
}

// assertStatus checks the HTTP status code.
func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rr.Code != expected {
		t.Errorf("expected status %d, got %d; body: %s", expected, rr.Code, rr.Body.String())
	}
}

// assertErrorCode checks that the response body contains the expected error code.
func assertErrorCode(t *testing.T, rr *httptest.ResponseRecorder, expectedCode string) {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if env.Error == nil {
		t.Fatal("expected an error in response body, got nil")
	}
	if env.Error.Code != expectedCode {
		t.Errorf("expected error code %q, got %q", expectedCode, env.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// FieldHandler tests
// ---------------------------------------------------------------------------

func TestFieldHandler_Sync_MissingClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/field/sync", nil)

	h.Sync(rr, req)

	assertStatus(t, rr, http.StatusUnauthorized)
	assertErrorCode(t, rr, "UNAUTHORIZED")
}

func TestFieldHandler_Sync_InvalidOrgIDInClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/field/sync", nil)

	claims := mw.Claims{
		Sub:   uuid.New().String(),
		OrgID: "not-a-uuid",
		Role:  "foreman",
	}
	ctx := mw.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	h.Sync(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_Sync_InvalidSinceTimestamp(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/field/sync?since=not-a-timestamp", nil)

	ctx := mw.ContextWithClaims(req.Context(), validClaims())
	req = req.WithContext(ctx)

	h.Sync(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_ReportProgress_MissingClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"project_id":"` + uuid.New().String() + `","task_id":"` + uuid.New().String() + `","percent_complete":50}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/progress", body)

	h.ReportProgress(rr, req)

	assertStatus(t, rr, http.StatusUnauthorized)
	assertErrorCode(t, rr, "UNAUTHORIZED")
}

func TestFieldHandler_ReportProgress_InvalidJSONBody(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{invalid json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/progress", body)

	ctx := mw.ContextWithClaims(req.Context(), validClaims())
	req = req.WithContext(ctx)

	h.ReportProgress(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_Checkin_MissingClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"project_id":"` + uuid.New().String() + `","latitude":40.7,"longitude":-74.0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/checkin", body)

	h.Checkin(rr, req)

	assertStatus(t, rr, http.StatusUnauthorized)
	assertErrorCode(t, rr, "UNAUTHORIZED")
}

func TestFieldHandler_Checkin_InvalidJSONBody(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`not json at all`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/checkin", body)

	ctx := mw.ContextWithClaims(req.Context(), validClaims())
	req = req.WithContext(ctx)

	h.Checkin(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_DailyLog_MissingClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"project_id":"` + uuid.New().String() + `","summary":"good day"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/daily-log", body)

	h.DailyLog(rr, req)

	assertStatus(t, rr, http.StatusUnauthorized)
	assertErrorCode(t, rr, "UNAUTHORIZED")
}

func TestFieldHandler_DailyLog_InvalidJSONBody(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{{{`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/daily-log", body)

	ctx := mw.ContextWithClaims(req.Context(), validClaims())
	req = req.WithContext(ctx)

	h.DailyLog(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

// ---------------------------------------------------------------------------
// FleetHandler tests
// ---------------------------------------------------------------------------

func TestFleetHandler_ListAssets_InvalidOrgID(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/not-a-uuid/fleet", nil)

	ctx := withChiParam(req.Context(), "orgID", "not-a-uuid")
	req = req.WithContext(ctx)

	h.ListAssets(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_CreateAsset_InvalidOrgID(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name":"Excavator","asset_type":"heavy","serial_number":"SN-001"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/bad-id/fleet", body)

	ctx := withChiParam(req.Context(), "orgID", "bad-id")
	req = req.WithContext(ctx)

	h.CreateAsset(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_CreateAsset_InvalidJSONBody(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`not json`)
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/"+orgID+"/fleet", body)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	h.CreateAsset(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_AllocateAsset_InvalidAssetID(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"project_id":"` + uuid.New().String() + `","start_date":"2026-05-01","end_date":"2026-05-15"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/fleet/bad-asset-id/allocate", body)

	ctx := withChiParams(req.Context(), map[string]string{
		"orgID":   uuid.New().String(),
		"assetID": "bad-asset-id",
	})
	req = req.WithContext(ctx)

	h.AllocateAsset(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_AllocateAsset_InvalidJSONBody(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	assetID := uuid.New().String()
	body := bytes.NewBufferString(`{broken json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/fleet/"+assetID+"/allocate", body)

	ctx := withChiParams(req.Context(), map[string]string{
		"orgID":   uuid.New().String(),
		"assetID": assetID,
	})
	req = req.WithContext(ctx)

	h.AllocateAsset(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_AllocateAsset_InvalidProjectID(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	assetID := uuid.New().String()
	body := bytes.NewBufferString(`{"project_id":"not-a-uuid","start_date":"2026-05-01","end_date":"2026-05-15"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/fleet/"+assetID+"/allocate", body)

	ctx := withChiParams(req.Context(), map[string]string{
		"orgID":   uuid.New().String(),
		"assetID": assetID,
	})
	req = req.WithContext(ctx)

	h.AllocateAsset(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

// ---------------------------------------------------------------------------
// HRHandler tests
// ---------------------------------------------------------------------------

func TestHRHandler_ListEmployees_InvalidOrgID(t *testing.T) {
	h := NewHRHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/invalid/employees", nil)

	ctx := withChiParam(req.Context(), "orgID", "invalid")
	req = req.WithContext(ctx)

	h.ListEmployees(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestHRHandler_ListCertifications_InvalidEmployeeID(t *testing.T) {
	h := NewHRHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/xxx/employees/bad-emp-id/certifications", nil)

	ctx := withChiParam(req.Context(), "employeeID", "bad-emp-id")
	req = req.WithContext(ctx)

	h.ListCertifications(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

// ---------------------------------------------------------------------------
// Additional edge case tests
// ---------------------------------------------------------------------------

func TestFieldHandler_Sync_InvalidSubInClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/field/sync", nil)

	claims := mw.Claims{
		Sub:   "not-a-uuid",
		OrgID: uuid.New().String(),
		Role:  "foreman",
	}
	ctx := mw.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	h.Sync(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_ReportProgress_InvalidSubInClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"project_id":"` + uuid.New().String() + `","task_id":"` + uuid.New().String() + `","percent_complete":50}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/progress", body)

	claims := mw.Claims{
		Sub:   "bad-sub",
		OrgID: uuid.New().String(),
		Role:  "foreman",
	}
	ctx := mw.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	h.ReportProgress(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_Checkin_InvalidSubInClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"project_id":"` + uuid.New().String() + `","latitude":40.7,"longitude":-74.0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/checkin", body)

	claims := mw.Claims{
		Sub:   "bad-sub",
		OrgID: uuid.New().String(),
		Role:  "foreman",
	}
	ctx := mw.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	h.Checkin(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_DailyLog_InvalidSubInClaims(t *testing.T) {
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"project_id":"` + uuid.New().String() + `","summary":"good day"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/field/daily-log", body)

	claims := mw.Claims{
		Sub:   "bad-sub",
		OrgID: uuid.New().String(),
		Role:  "foreman",
	}
	ctx := mw.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	h.DailyLog(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_AllocateAsset_InvalidStartDate(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	assetID := uuid.New().String()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{"project_id":"` + projectID + `","start_date":"not-a-date","end_date":"2026-05-15"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/fleet/"+assetID+"/allocate", body)

	ctx := withChiParams(req.Context(), map[string]string{
		"orgID":   uuid.New().String(),
		"assetID": assetID,
	})
	req = req.WithContext(ctx)

	h.AllocateAsset(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_AllocateAsset_InvalidEndDate(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	assetID := uuid.New().String()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{"project_id":"` + projectID + `","start_date":"2026-05-01","end_date":"not-a-date"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/fleet/"+assetID+"/allocate", body)

	ctx := withChiParams(req.Context(), map[string]string{
		"orgID":   uuid.New().String(),
		"assetID": assetID,
	})
	req = req.WithContext(ctx)

	h.AllocateAsset(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFleetHandler_ListAssets_EmptyOrgID(t *testing.T) {
	h := NewFleetHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org//fleet", nil)

	ctx := withChiParam(req.Context(), "orgID", "")
	req = req.WithContext(ctx)

	h.ListAssets(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestFieldHandler_Sync_EmptySinceDefaultsToLast24h(t *testing.T) {
	// When "since" is empty, the handler defaults to last 24 hours
	// and proceeds to call the service. Since the service is nil,
	// this will panic if we reach the service call. The absence of
	// a 400 means the since parsing succeeded.
	// We only test that it does NOT return a 400 for an empty since param.
	h := NewFieldHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/field/sync", nil)

	ctx := mw.ContextWithClaims(req.Context(), validClaims())
	req = req.WithContext(ctx)

	// This will panic when calling nil service, but we recover to verify
	// we got past the validation stage.
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service panic means we passed validation
			}
		}()
		h.Sync(rr, req)
	}()

	// If we get a 400, the since validation failed unexpectedly
	if rr.Code == http.StatusBadRequest {
		t.Errorf("empty since param should default to last 24h, got 400: %s", rr.Body.String())
	}
}
