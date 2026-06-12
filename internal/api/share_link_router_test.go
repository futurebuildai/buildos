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

// fakeShareLinkService is a router-level fake ShareLinkServicer.
type fakeShareLinkService struct {
	createErr error
	createRes service.IssuedShareLink
	revokeErr error
	listErr   error
	listRes   []models.ShareLink
	gotOrgID  uuid.UUID
}

func (f *fakeShareLinkService) CreateShareLink(_ context.Context, orgID uuid.UUID, _ string, _ uuid.UUID, _ time.Duration) (service.IssuedShareLink, error) {
	f.gotOrgID = orgID
	return f.createRes, f.createErr
}
func (f *fakeShareLinkService) RevokeShareLink(_ context.Context, orgID uuid.UUID, _ string, _ uuid.UUID) (models.ShareLink, error) {
	f.gotOrgID = orgID
	return models.ShareLink{}, f.revokeErr
}
func (f *fakeShareLinkService) ListShareLinks(_ context.Context, orgID, _ uuid.UUID) ([]models.ShareLink, error) {
	f.gotOrgID = orgID
	return f.listRes, f.listErr
}

const slCUID = "66666666-6666-6666-6666-666666666666"
const slLinkID = "77777777-7777-7777-7777-777777777777"

func slRouter(svc ShareLinkServicer) http.Handler {
	return NewRouter(RouterConfig{DevAuthMode: "header", ShareLinkService: svc, PublicBaseURL: "https://acme.example"})
}

// ---- RBAC: owner/admin only (§9-1) -------------------------------------------

func TestShareLinkRoutes_Create_RBAC(t *testing.T) {
	cases := []struct {
		role string
		want int
	}{
		{"field_worker", http.StatusForbidden},
		{"superintendent", http.StatusForbidden},
		{"admin", http.StatusCreated},
		{"owner", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fake := &fakeShareLinkService{createRes: service.IssuedShareLink{
				Cleartext: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
				Link:      models.ShareLink{ID: uuid.New(), ExpiresAt: time.Now().Add(time.Hour)},
			}}
			h := slRouter(fake)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+slCUID+"/share-links", tc.role, `{"ttl_days":30}`))
			if rec.Code != tc.want {
				t.Fatalf("create as %s = %d, want %d (body=%s)", tc.role, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestShareLinkRoutes_Revoke_RBAC(t *testing.T) {
	for _, role := range []string{"field_worker", "superintendent"} {
		t.Run(role+" forbidden", func(t *testing.T) {
			h := slRouter(&fakeShareLinkService{})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authedReq(http.MethodDelete, "/api/v1/share-links/"+slLinkID, role, ""))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("revoke as %s = %d, want 403", role, rec.Code)
			}
		})
	}
}

// ---- create returns the one-time URL built from the public base + cleartext --

func TestShareLinkRoutes_Create_ReturnsURLOnce(t *testing.T) {
	clear := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	fake := &fakeShareLinkService{createRes: service.IssuedShareLink{
		Cleartext: clear,
		Link:      models.ShareLink{ID: uuid.New(), ExpiresAt: time.Now().Add(720 * time.Hour)},
	}}
	h := slRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+slCUID+"/share-links", "owner", `{}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "https://acme.example/p/"+clear) {
		t.Errorf("response missing the one-time URL: %s", body)
	}
	if fake.gotOrgID.String() != rbacOrgID {
		t.Errorf("handler passed org %s, want caller's claim org", fake.gotOrgID)
	}
}

// ---- not-sent → 422 UPDATE_NOT_SENT ------------------------------------------

func TestShareLinkRoutes_Create_NotSent422(t *testing.T) {
	h := slRouter(&fakeShareLinkService{createErr: service.ErrShareLinkNotSent})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+slCUID+"/share-links", "owner", `{}`))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create on draft = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "UPDATE_NOT_SENT") {
		t.Errorf("body missing UPDATE_NOT_SENT: %s", rec.Body.String())
	}
}

// ---- revoke happy path → 204 -------------------------------------------------

func TestShareLinkRoutes_Revoke_NoContent(t *testing.T) {
	h := slRouter(&fakeShareLinkService{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete, "/api/v1/share-links/"+slLinkID, "owner", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

// ---- list omits the cleartext + the hash ------------------------------------

func TestShareLinkRoutes_List_NoSecretLeak(t *testing.T) {
	fake := &fakeShareLinkService{listRes: []models.ShareLink{{
		ID: uuid.New(), TokenHash: "secret-hash-value", ExpiresAt: time.Now().Add(time.Hour), ViewCount: 3,
	}}}
	h := slRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/client-updates/"+slCUID+"/share-links", "admin", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret-hash-value") {
		t.Errorf("token hash leaked into list response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"active"`) {
		t.Errorf("expected derived status in list: %s", rec.Body.String())
	}
}

// ---- the share-link surface does NOT mount when the service is nil -----------

func TestShareLinkRoutes_SkippedWhenNil(t *testing.T) {
	h := NewRouter(RouterConfig{DevAuthMode: "header"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+slCUID+"/share-links", "owner", `{}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("share-links with nil service = %d, want 404", rec.Code)
	}
}
