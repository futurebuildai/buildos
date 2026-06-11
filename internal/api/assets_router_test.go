package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// fakeAssetService is a router-level fake AssetServicer. It records the org id
// it was called with (to assert org-scoping) and returns canned results/errors.
type fakeAssetService struct {
	requestErr error
	requestRes service.RequestUploadResult
	confirmErr error
	confirmRes models.Asset
	getErr     error
	getRes     models.Asset
	signErr    error
	signURL    string
	listErr    error
	listRes    []models.Asset

	gotOrgID uuid.UUID
}

func (f *fakeAssetService) RequestUpload(_ context.Context, orgID uuid.UUID, _ string, _ service.RequestUploadInput) (service.RequestUploadResult, error) {
	f.gotOrgID = orgID
	return f.requestRes, f.requestErr
}
func (f *fakeAssetService) ConfirmUpload(_ context.Context, orgID uuid.UUID, _ string, _ uuid.UUID, _ *string) (models.Asset, error) {
	f.gotOrgID = orgID
	return f.confirmRes, f.confirmErr
}
func (f *fakeAssetService) GetAsset(_ context.Context, orgID, _ uuid.UUID) (models.Asset, error) {
	f.gotOrgID = orgID
	return f.getRes, f.getErr
}
func (f *fakeAssetService) SignedGetURL(_ context.Context, orgID, _ uuid.UUID, _ time.Duration) (string, error) {
	f.gotOrgID = orgID
	return f.signURL, f.signErr
}
func (f *fakeAssetService) ServeAsset(_ context.Context, orgID, _ uuid.UUID) (io.ReadCloser, string, error) {
	f.gotOrgID = orgID
	return nil, "", f.getErr
}
func (f *fakeAssetService) ListProjectAssets(_ context.Context, orgID, _ uuid.UUID, _ bool) ([]models.Asset, error) {
	f.gotOrgID = orgID
	return f.listRes, f.listErr
}

// assetProjID / assetTestID are fixed UUIDs for the asset router tests. The
// shared authedReq (ingress_rbac_test.go) bakes rbacOrgID into the X-Dev-Auth
// header, so org-scope assertions compare against rbacOrgID.
const assetProjID = "22222222-2222-2222-2222-222222222222"
const assetTestID = "33333333-3333-3333-3333-333333333333"

// TestAssetRoutes_PresignPut_FieldWorkerForbidden proves the operator presign
// surface is gated at superintendent: a field_worker is 403'd before the
// handler runs (the field_worker-facing variant is Chunk B).
func TestAssetRoutes_PresignPut_FieldWorkerForbidden(t *testing.T) {
	handler := NewRouter(RouterConfig{DevAuthMode: "header", AssetService: &fakeAssetService{}})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq(http.MethodPost,
		"/api/v1/projects/"+assetProjID+"/assets/presign-put", "field_worker",
		`{"content_type":"image/jpeg","byte_size":1024}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("presign-put as field_worker = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestAssetRoutes_PresignPut_SuperintendentClearsRBAC proves a superintendent
// clears the gate and reaches the handler (here the fake returns a result → 201).
func TestAssetRoutes_PresignPut_SuperintendentClearsRBAC(t *testing.T) {
	fake := &fakeAssetService{requestRes: service.RequestUploadResult{
		Asset:     models.Asset{ID: uuid.New()},
		UploadURL: "https://r2.test/put",
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}}
	handler := NewRouter(RouterConfig{DevAuthMode: "header", AssetService: fake})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq(http.MethodPost,
		"/api/v1/projects/"+assetProjID+"/assets/presign-put", "superintendent",
		`{"content_type":"image/jpeg","byte_size":1024}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("presign-put as superintendent = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	// Org-scoping: the handler must pass the caller's org id (from claims).
	if fake.gotOrgID.String() != rbacOrgID {
		t.Fatalf("handler passed org %s, want caller's claim org", fake.gotOrgID)
	}
}

// TestAssetRoutes_PresignPut_Unconfigured503 proves the soft-fail posture:
// ErrStorageUnavailable maps to 503 STORAGE_UNAVAILABLE.
func TestAssetRoutes_PresignPut_Unconfigured503(t *testing.T) {
	fake := &fakeAssetService{requestErr: service.ErrStorageUnavailable}
	handler := NewRouter(RouterConfig{DevAuthMode: "header", AssetService: fake})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq(http.MethodPost,
		"/api/v1/projects/"+assetProjID+"/assets/presign-put", "superintendent",
		`{"content_type":"image/jpeg","byte_size":1024}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("presign-put unconfigured = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "STORAGE_UNAVAILABLE") {
		t.Fatalf("body missing STORAGE_UNAVAILABLE: %s", rec.Body.String())
	}
}

// TestAssetRoutes_Get_CrossOrgNotFound proves a cross-org / missing asset
// surfaces as 404 (the service returns ErrNotFound; the handler maps to 404).
func TestAssetRoutes_Get_CrossOrgNotFound(t *testing.T) {
	fake := &fakeAssetService{signErr: service.ErrNotFound}
	handler := NewRouter(RouterConfig{DevAuthMode: "header", AssetService: fake})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq(http.MethodGet,
		"/api/v1/assets/"+assetTestID, "superintendent", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET cross-org asset = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestAssetRoutes_Get_RedirectsToSignedURL proves a ready asset 302-redirects to
// the short-lived signed URL.
func TestAssetRoutes_Get_RedirectsToSignedURL(t *testing.T) {
	fake := &fakeAssetService{signURL: "https://r2.test/get/obj?X-Amz-Signature=abc"}
	handler := NewRouter(RouterConfig{DevAuthMode: "header", AssetService: fake})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq(http.MethodGet,
		"/api/v1/assets/"+assetTestID, "superintendent", ""))
	if rec.Code != http.StatusFound {
		t.Fatalf("GET ready asset = %d, want 302 (body=%s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != fake.signURL {
		t.Fatalf("Location = %q, want %q", loc, fake.signURL)
	}
}

// TestAssetRoutes_SkippedWhenNil proves the conditional mount: with AssetService
// nil the asset routes don't mount, so they 404.
func TestAssetRoutes_SkippedWhenNil(t *testing.T) {
	handler := NewRouter(RouterConfig{DevAuthMode: "header"})
	for _, tc := range []struct {
		method, target string
	}{
		{http.MethodPost, "/api/v1/projects/" + assetProjID + "/assets/presign-put"},
		{http.MethodGet, "/api/v1/assets/" + assetTestID},
		{http.MethodGet, "/api/v1/projects/" + assetProjID + "/assets"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, authedReq(tc.method, tc.target, "owner", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s with AssetService nil = %d, want 404", tc.method, tc.target, rec.Code)
		}
	}
}
