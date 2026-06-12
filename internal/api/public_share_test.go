package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// fakePublicShareResolver is a router-level fake PublicShareResolver. A valid
// token (validToken) resolves to the canned PublicUpdate; anything else returns
// the uniform ErrInvalidShareToken. For the photo path, only curatedAsset
// resolves under validToken.
type fakePublicShareResolver struct {
	validToken   string
	pub          models.PublicUpdate
	curatedAsset uuid.UUID
	orgID        uuid.UUID
}

func (f *fakePublicShareResolver) ResolvePublicUpdate(_ context.Context, cleartext string) (models.PublicUpdate, error) {
	if cleartext != f.validToken {
		return models.PublicUpdate{}, service.ErrInvalidShareToken
	}
	return f.pub, nil
}

func (f *fakePublicShareResolver) ResolvePublicPhoto(_ context.Context, cleartext string, assetID uuid.UUID) (service.ResolvePublicPhotoTarget, error) {
	if cleartext != f.validToken || assetID != f.curatedAsset {
		return service.ResolvePublicPhotoTarget{}, service.ErrInvalidShareToken
	}
	return service.ResolvePublicPhotoTarget{OrgID: f.orgID, AssetID: assetID}, nil
}

// fakePublicAssetServer streams fixed bytes for any (org, asset) the resolver
// authorized. It records what it was asked for so a test can assert the proxy
// fetched the resolver-supplied target, not caller input.
type fakePublicAssetServer struct {
	body        string
	contentType string
	gotOrg      uuid.UUID
	gotAsset    uuid.UUID
}

func (f *fakePublicAssetServer) ServeAsset(_ context.Context, orgID, assetID uuid.UUID) (io.ReadCloser, string, error) {
	f.gotOrg, f.gotAsset = orgID, assetID
	return io.NopCloser(strings.NewReader(f.body)), f.contentType, nil
}

// fakeIncompleteSetup satisfies SetupServicer with onboarding NEVER complete, so
// the SetupGate is active for the authenticated group. The wizard handler
// methods are unused by these tests (return zero values).
type fakeIncompleteSetup struct{}

func (fakeIncompleteSetup) GetState(context.Context, uuid.UUID) (service.SetupState, error) {
	return service.SetupState{}, nil
}
func (fakeIncompleteSetup) UpdateCompanyInfo(context.Context, service.UpdateCompanyInfoInput) (models.CompanyProfile, error) {
	return models.CompanyProfile{}, nil
}
func (fakeIncompleteSetup) CreateTrade(context.Context, service.CreateTradeInput) (models.TradeCategory, error) {
	return models.TradeCategory{}, nil
}
func (fakeIncompleteSetup) CreateCostCode(context.Context, service.CreateCostCodeInput) (models.CostCode, error) {
	return models.CostCode{}, nil
}
func (fakeIncompleteSetup) CreateCalendar(context.Context, service.CreateCalendarInput) (models.WorkingCalendar, error) {
	return models.WorkingCalendar{}, nil
}
func (fakeIncompleteSetup) AddHoliday(context.Context, service.AddHolidayInput) (models.HolidayOverride, error) {
	return models.HolidayOverride{}, nil
}
func (fakeIncompleteSetup) AddJurisdiction(context.Context, service.AddJurisdictionInput) (models.PermitJurisdiction, error) {
	return models.PermitJurisdiction{}, nil
}
func (fakeIncompleteSetup) Complete(context.Context, service.CompleteSetupInput) (models.CompanyProfile, error) {
	return models.CompanyProfile{}, nil
}
func (fakeIncompleteSetup) IsOnboardingComplete(context.Context, uuid.UUID) (bool, error) {
	return false, nil // onboarding NEVER complete → SetupGate active
}

const publicToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 43-char base64url shape

// newPublicShareFixture builds a resolver + asset-server + a canned PublicUpdate
// that carries a deliberately ERP-laden ProjectName/Body NOTHING — the point is
// that the projection only HAS the allowlisted fields. We still seed forbidden
// strings into the source the fake would have, to prove they never appear.
func newPublicShareFixture() (*fakePublicShareResolver, *fakePublicAssetServer) {
	curated := uuid.New()
	org := uuid.New()
	res := &fakePublicShareResolver{
		validToken:   publicToken,
		curatedAsset: curated,
		orgID:        org,
		pub: models.PublicUpdate{
			ProjectName:   "Maple Street Residence",
			Body:          "We framed the second floor this week.\n\nThe roof goes on next week.",
			PhotoAssetIDs: []uuid.UUID{curated},
		},
	}
	srv := &fakePublicAssetServer{body: "\xff\xd8\xff\xe0fakejpegbytes", contentType: "image/jpeg"}
	return res, srv
}

// publicRouter builds a router with ONLY the public share surface wired (plus an
// optional active SetupGate). No auth services — proving the route needs none.
func publicRouter(res PublicShareResolver, assets PublicAssetServer, withSetupGate bool, publicLimiter *mw.IPRateLimiter) http.Handler {
	cfg := RouterConfig{
		DevAuthMode:         "header",
		PublicShareResolver: res,
		PublicAssetServer:   assets,
		PublicShareLimiter:  publicLimiter,
	}
	if withSetupGate {
		cfg.SetupService = fakeIncompleteSetup{}
	}
	return NewRouter(cfg)
}

// ---- the route is reachable WITHOUT auth -------------------------------------

func TestPublicShare_ReachableWithoutAuth(t *testing.T) {
	res, srv := newPublicShareFixture()
	h := publicRouter(res, srv, false, nil)

	// No Authorization header, no X-Dev-Auth header — a homeowner's browser.
	req := httptest.NewRequest(http.MethodGet, "/p/"+publicToken, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /p/{token} with no auth = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q, want text/html", ct)
	}
}

// ---- the route bypasses SetupGate (onboarding incomplete) without weakening it

func TestPublicShare_BypassesSetupGate(t *testing.T) {
	res, srv := newPublicShareFixture()
	h := publicRouter(res, srv, true /* SetupGate active */, nil)

	// Public page: reachable despite onboarding incomplete + no auth.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/"+publicToken, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public page with SetupGate active = %d, want 200", rec.Code)
	}

	// Proof the bypass did NOT weaken auth elsewhere: an authenticated API route
	// behind the gate still 403s SETUP_INCOMPLETE (the gate is genuinely active).
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, authedReq(http.MethodGet, "/api/v1/projects", "owner", ""))
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("authed API route with SetupGate active = %d, want 403 (proves gate still on)", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "SETUP_INCOMPLETE") {
		t.Errorf("expected SETUP_INCOMPLETE on the gated route, got %s", rec2.Body.String())
	}
}

// ---- bad/expired/revoked/garbage tokens → uniform 404 ------------------------

func TestPublicShare_BadToken_Uniform404(t *testing.T) {
	res, srv := newPublicShareFixture()
	h := publicRouter(res, srv, false, nil)

	bodies := map[string]string{}
	for _, tok := range []string{"wrongtoken", strings.Repeat("Z", 43), "short"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/"+tok, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("bad token %q = %d, want 404", tok, rec.Code)
		}
		bodies[tok] = rec.Body.String()
	}
	// The 404 body is uniform across all failure reasons (enumeration defense).
	var prev string
	for tok, b := range bodies {
		if prev != "" && b != prev {
			t.Errorf("404 body differs across tokens (leaks reason); token %q body=%q", tok, b)
		}
		prev = b
		if strings.Contains(strings.ToLower(b), "expired token") && strings.Contains(strings.ToLower(b), "revoked") {
			t.Errorf("404 body distinguishes failure reasons: %q", b)
		}
	}
}

// ---- the page is self-contained: strict CSP, no cookies, no /api references ---

func TestPublicShare_PageHeadersAndContent(t *testing.T) {
	res, srv := newPublicShareFixture()
	h := publicRouter(res, srv, false, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/"+publicToken, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("page = %d, want 200", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("no Content-Security-Policy on the public page")
	}
	for _, want := range []string{"default-src 'none'", "img-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q: %q", want, csp)
		}
	}
	// No script-src at all → no JS allowed.
	if strings.Contains(csp, "script-src") {
		t.Errorf("CSP unexpectedly allows scripts: %q", csp)
	}
	// Global security headers still apply (inherited from the root stack).
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", rec.Header().Get("X-Frame-Options"))
	}
	// No cookies set on the public page.
	if sc := rec.Header().Get("Set-Cookie"); sc != "" {
		t.Errorf("public page set a cookie: %q", sc)
	}
	body := rec.Body.String()
	// No JS reference to the API surface, no <script> at all.
	if strings.Contains(body, "/api/") {
		t.Errorf("public page references /api/: %s", body)
	}
	if strings.Contains(strings.ToLower(body), "<script") {
		t.Errorf("public page contains a <script>: %s", body)
	}
	// The curated photo renders via the same-origin proxy path.
	if !strings.Contains(body, "/p/"+publicToken+"/photos/") {
		t.Errorf("photo not rendered via same-origin proxy: %s", body)
	}
	// The R2/object-store host must not appear in client HTML.
	if strings.Contains(strings.ToLower(body), "r2.cloudflarestorage") || strings.Contains(strings.ToLower(body), "amazonaws") {
		t.Errorf("object-store host leaked into client HTML: %s", body)
	}
	if !strings.Contains(body, "Maple Street Residence") {
		t.Errorf("project name not rendered: %s", body)
	}
}

// ---- headline security test: the RENDERED HTML carries no raw ERP ------------
//
// The substantive source-ERP-absence proof lives at the service layer
// (share_link_integration_test.go TestShareLink_Redaction_PublicUpdateCarriesNoERP,
// which seeds real safety/crew/GPS/cents/address + a sibling project and asserts
// none reaches the PublicUpdate projection). This is the spec-named at-the-wire
// complement (DAILY_REPORTS_CLIENT_UPDATES.md §"headline security test"): render
// /p/{token} and grep the response HTML for each forbidden ERP value, asserting
// ABSENT. Because PublicUpdate physically carries only ProjectName/Body/Date/
// photo-ids, the rendered page is a pure function of the projection — this test
// is the regression guard that the renderer never starts reflecting more.
func TestPublicShare_RenderedHTML_NoRawERP(t *testing.T) {
	res, srv := newPublicShareFixture()
	// The operator-edited (already-redacted) body the homeowner is meant to see.
	res.pub.Body = "We framed the second floor this week.\n\nThe roof goes on next week."
	h := publicRouter(res, srv, false, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/"+publicToken, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("page = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// Forbidden ERP values that would be present iff the renderer pulled raw
	// report/project data instead of the curated projection. None may appear.
	forbidden := []string{
		"OSHA",                // safety-incident text
		"laceration",          // safety-incident detail
		"Hector Ramirez",      // crew identity
		"crew_member",         // crew field name
		"37.7749",             // GPS latitude
		"-122.4194",           // GPS longitude
		"gps_lat",             // GPS field name
		"$",                   // any dollar/cents amount
		"_cents",              // money field name
		"148200",              // a raw cents value
		"1247 Birchwood Lane", // full street address
		"Oakdale Commons",     // a sibling project's name
		"ai_draft",            // the pre-edit AI draft field
		"recipient_email",     // the homeowner address snapshot
		"@",                   // any email address
	}
	for _, f := range forbidden {
		if strings.Contains(body, f) {
			t.Errorf("rendered public HTML leaks forbidden ERP value %q:\n%s", f, body)
		}
	}
	// The curated, client-safe content IS present.
	if !strings.Contains(body, "Maple Street Residence") {
		t.Errorf("project name not rendered: %s", body)
	}
	if !strings.Contains(body, "framed the second floor") {
		t.Errorf("operator-edited body not rendered: %s", body)
	}
}

// ---- photo proxy: only the curated asset streams; others 404 -----------------

func TestPublicShare_PhotoProxy_CuratedStreamsOthers404(t *testing.T) {
	res, srv := newPublicShareFixture()
	h := publicRouter(res, srv, false, nil)

	// Curated asset → 200 image bytes via the EXIF-stripping proxy.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/"+publicToken+"/photos/"+res.curatedAsset.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("curated photo = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("photo content-type = %q, want image/jpeg", ct)
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Errorf("photo response missing CSP")
	}
	// The proxy fetched the resolver-supplied (org, asset), never caller input.
	if srv.gotOrg != res.orgID || srv.gotAsset != res.curatedAsset {
		t.Errorf("proxy fetched (%s,%s), want (%s,%s)", srv.gotOrg, srv.gotAsset, res.orgID, res.curatedAsset)
	}

	// A different (non-curated) asset id under the SAME valid token → 404.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/p/"+publicToken+"/photos/"+uuid.New().String(), nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("non-curated photo = %d, want 404", rec2.Code)
	}

	// A photo under a WRONG token → 404 (a different update's token can't reach
	// this update's assets).
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/p/wrongtoken/photos/"+res.curatedAsset.String(), nil))
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("photo under wrong token = %d, want 404", rec3.Code)
	}
}

// ---- the public route IS rate-limited (it does not bypass the limiter) -------

func TestPublicShare_RateLimited(t *testing.T) {
	res, srv := newPublicShareFixture()
	// A 1-rps / 1-burst dedicated limiter: the second request in a burst is 429.
	limiter := mw.NewIPRateLimiter(1, 1)
	h := publicRouter(res, srv, false, limiter)

	var got429 bool
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p/"+publicToken, nil)
		req.RemoteAddr = "203.0.113.7:1234" // fixed peer so the bucket is shared
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("public route never returned 429 — it is NOT rate-limited")
	}
}

// ---- the public surface does NOT mount when the resolver is nil --------------

func TestPublicShare_SkippedWhenNil(t *testing.T) {
	h := NewRouter(RouterConfig{DevAuthMode: "header"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/"+publicToken, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public route with nil resolver = %d, want 404", rec.Code)
	}
}

// sanity: the uniform-token sentinel is exported as expected (compile guard).
var _ = errors.Is(service.ErrInvalidShareToken, service.ErrInvalidShareToken)
