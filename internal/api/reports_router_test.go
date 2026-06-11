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

// fakeReportsService is a router-level fake ReportsServicer. Records the org id
// (org-scope assertion) and returns canned results/errors.
type fakeReportsService struct {
	listErr   error
	listRes   []models.DailyReportSummary
	getErr    error
	getRes    models.DailyReport
	digestErr error
	digestRes string
	draftErr  error
	draftRes  service.ClientUpdateDraft

	gotOrgID uuid.UUID
}

func (f *fakeReportsService) ListProjectReports(_ context.Context, orgID, _ uuid.UUID, _, _ time.Time) ([]models.DailyReportSummary, error) {
	f.gotOrgID = orgID
	return f.listRes, f.listErr
}
func (f *fakeReportsService) GetProjectReport(_ context.Context, orgID, _ uuid.UUID, _ time.Time) (models.DailyReport, error) {
	f.gotOrgID = orgID
	return f.getRes, f.getErr
}
func (f *fakeReportsService) GenerateDigest(_ context.Context, orgID uuid.UUID, _ string, _ uuid.UUID, _ time.Time) (string, error) {
	f.gotOrgID = orgID
	return f.digestRes, f.digestErr
}
func (f *fakeReportsService) DraftClientUpdate(_ context.Context, orgID uuid.UUID, _ string, _ uuid.UUID, _ time.Time) (service.ClientUpdateDraft, error) {
	f.gotOrgID = orgID
	return f.draftRes, f.draftErr
}

const reportProjID = "44444444-4444-4444-4444-444444444444"
const reportDate = "2026-06-09"

func reportsRouter(svc ReportsServicer) http.Handler {
	return NewRouter(RouterConfig{DevAuthMode: "header", ReportsService: svc})
}

// List/Get/Digest = superintendent+; field_worker is 403'd.
func TestReportRoutes_List_FieldWorkerForbidden(t *testing.T) {
	h := reportsRouter(&fakeReportsService{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/projects/"+reportProjID+"/daily-reports", "field_worker", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("list as field_worker = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestReportRoutes_List_SuperintendentOK_OrgScoped(t *testing.T) {
	fake := &fakeReportsService{listRes: []models.DailyReportSummary{{ProjectID: uuid.New(), WorkSummary: "framed"}}}
	h := reportsRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/projects/"+reportProjID+"/daily-reports?since=2026-06-01&until=2026-06-09", "superintendent", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list as superintendent = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.gotOrgID.String() != rbacOrgID {
		t.Fatalf("handler passed org %s, want caller's claim org", fake.gotOrgID)
	}
}

func TestReportRoutes_Get_CrossOrgNotFound(t *testing.T) {
	fake := &fakeReportsService{getErr: service.ErrNotFound}
	h := reportsRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/projects/"+reportProjID+"/daily-reports/"+reportDate, "superintendent", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET cross-org report = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestReportRoutes_Get_BadDate(t *testing.T) {
	h := reportsRouter(&fakeReportsService{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/projects/"+reportProjID+"/daily-reports/06-09-2026", "superintendent", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET bad date = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// Digest: superintendent+ clears; AI-unconfigured → 503.
func TestReportRoutes_Digest_FieldWorkerForbidden(t *testing.T) {
	h := reportsRouter(&fakeReportsService{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+reportProjID+"/daily-reports/"+reportDate+"/digest", "field_worker", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("digest as field_worker = %d, want 403", rec.Code)
	}
}

func TestReportRoutes_Digest_Unconfigured503(t *testing.T) {
	fake := &fakeReportsService{digestErr: service.ErrReportsAIUnavailable}
	h := reportsRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+reportProjID+"/daily-reports/"+reportDate+"/digest", "superintendent", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("digest unconfigured = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SERVICE_UNAVAILABLE") {
		t.Fatalf("body missing SERVICE_UNAVAILABLE: %s", rec.Body.String())
	}
}

func TestReportRoutes_Digest_SuperintendentOK(t *testing.T) {
	fake := &fakeReportsService{digestRes: "office digest text"}
	h := reportsRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+reportProjID+"/daily-reports/"+reportDate+"/digest", "superintendent", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("digest = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "office digest text") {
		t.Fatalf("body missing digest: %s", rec.Body.String())
	}
}

// Client-update draft: owner/admin only — superintendent is 403'd (§9-1).
func TestReportRoutes_ClientDraft_SuperintendentForbidden(t *testing.T) {
	h := reportsRouter(&fakeReportsService{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+reportProjID+"/daily-reports/"+reportDate+"/client-update-draft", "superintendent", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("client draft as superintendent = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestReportRoutes_ClientDraft_OwnerOK(t *testing.T) {
	fake := &fakeReportsService{draftRes: service.ClientUpdateDraft{Subject: "Progress!", Body: "All good."}}
	h := reportsRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+reportProjID+"/daily-reports/"+reportDate+"/client-update-draft", "owner", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("client draft as owner = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Progress!") {
		t.Fatalf("body missing draft subject: %s", rec.Body.String())
	}
}

func TestReportRoutes_SkippedWhenNil(t *testing.T) {
	h := NewRouter(RouterConfig{DevAuthMode: "header"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/projects/"+reportProjID+"/daily-reports", "owner", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("daily-reports with ReportsService nil = %d, want 404", rec.Code)
	}
}
