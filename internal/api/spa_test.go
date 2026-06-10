package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// spaTestRouter builds a router with SPA serving rooted at a fixture
// dist dir: index.html (with a recognizable marker), one hashed bundle
// under assets/, and a root-level favicon. Returns the handler plus the
// fixture dir so tests can plant extra files.
func spaTestRouter(t *testing.T) (http.Handler, string) {
	t.Helper()
	dist := t.TempDir()
	writeFixture(t, filepath.Join(dist, "index.html"),
		`<!doctype html><html><body>BUILDOS-SPA-INDEX</body></html>`)
	writeFixture(t, filepath.Join(dist, "assets", "app-abc123.js"),
		`console.log("bundle")`)
	writeFixture(t, filepath.Join(dist, "favicon.svg"), `<svg></svg>`)

	return NewRouter(RouterConfig{
		DevAuthMode: "header",
		WebDistDir:  dist,
	}), dist
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(handler http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// TestSPA_RootServesIndex proves GET / serves the SPA entry document
// with the HTML-response contract: no-cache (deploys take effect on
// reload), the SPA CSP, and the global security-header baseline.
func TestSPA_RootServesIndex(t *testing.T) {
	handler, _ := spaTestRouter(t)
	rec := get(handler, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "BUILDOS-SPA-INDEX") {
		t.Errorf("GET / body does not contain the index marker: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"frame-ancestors 'none'",
		"https://fonts.googleapis.com",
		"https://fonts.gstatic.com",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q; got %q", want, csp)
		}
	}
	// The global SecurityHeaders middleware wraps the SPA handler too.
	if xfo := rec.Header().Get("X-Frame-Options"); xfo != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", xfo)
	}
	if nosniff := rec.Header().Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", nosniff)
	}
}

// TestSPA_ClientRouteFallsBackToIndex proves deep links (client-routed
// paths with no file on disk) serve index.html — the SPA contract —
// without authentication (the login page must load logged-out).
func TestSPA_ClientRouteFallsBackToIndex(t *testing.T) {
	handler, _ := spaTestRouter(t)

	for _, target := range []string{
		"/login",
		"/projects/3f2c9a40-0000-0000-0000-000000000000/schedule",
		"/admin/agents",
	} {
		rec := get(handler, target)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", target, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), "BUILDOS-SPA-INDEX") {
			t.Errorf("GET %s did not fall back to index.html", target)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("GET %s Cache-Control = %q, want no-cache", target, cc)
		}
	}
}

// TestSPA_HashedAssetImmutableCache proves Vite's content-hashed
// bundles get the year-long immutable cache header and a real
// JavaScript content type — and no CSP (it's not an HTML response).
func TestSPA_HashedAssetImmutableCache(t *testing.T) {
	handler, _ := spaTestRouter(t)
	rec := get(handler, "/assets/app-abc123.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/app-abc123.js = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable year-long", cc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want javascript", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "" {
		t.Errorf("asset response should not carry CSP; got %q", csp)
	}
}

// TestSPA_MissingAssetIs404 proves a miss under /assets/ 404s instead
// of falling back to HTML: a stale index referencing a gone bundle must
// fail loudly, not produce a MIME-type error from an HTML body.
func TestSPA_MissingAssetIs404(t *testing.T) {
	handler, _ := spaTestRouter(t)
	rec := get(handler, "/assets/app-gone999.js")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /assets/app-gone999.js = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "BUILDOS-SPA-INDEX") {
		t.Error("missing asset fell back to index.html; want plain 404")
	}
}

// TestSPA_RootLevelFileServed proves real non-asset files at the dist
// root (favicon, robots.txt) are served as themselves, not index.html.
func TestSPA_RootLevelFileServed(t *testing.T) {
	handler, _ := spaTestRouter(t)
	rec := get(handler, "/favicon.svg")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /favicon.svg = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<svg>") {
		t.Errorf("GET /favicon.svg body = %q, want the svg fixture", rec.Body.String())
	}
}

// TestSPA_APIMissStaysJSON proves unmatched /api/* paths return the
// standard JSON error envelope — an API typo must never get HTML.
func TestSPA_APIMissStaysJSON(t *testing.T) {
	handler, _ := spaTestRouter(t)

	// Note: unmatched paths UNDER an authenticated subtree (e.g.
	// /api/v1/projects/x/typo) 401 before routing concludes — chi runs
	// the group middleware first. That pre-existing behavior is kept;
	// these cases are API paths no route group claims at all.
	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/api/v1/nope"},
		{http.MethodPost, "/api/v2/anything"},
		{http.MethodGet, "/api"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("%s %s Content-Type = %q, want application/json", tc.method, tc.target, ct)
		}
		if !strings.Contains(rec.Body.String(), `"NOT_FOUND"`) {
			t.Errorf("%s %s body = %q, want NOT_FOUND error envelope", tc.method, tc.target, rec.Body.String())
		}
	}
}

// TestSPA_NonGETIsJSON404 proves write methods to unregistered non-API
// paths get a JSON 404, never the HTML page.
func TestSPA_NonGETIsJSON404(t *testing.T) {
	handler, _ := spaTestRouter(t)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/projects/123", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /projects/123 = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "BUILDOS-SPA-INDEX") {
		t.Error("POST got the SPA page; want JSON 404")
	}
}

// TestSPA_ProbesNotShadowed proves the registered probe routes still
// win over the catch-all (NotFound only fires for unmatched paths).
func TestSPA_ProbesNotShadowed(t *testing.T) {
	handler, _ := spaTestRouter(t)
	rec := get(handler, "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("GET /health body = %q, want the JSON liveness payload", rec.Body.String())
	}
}

// TestSPA_DisabledWhenEmpty proves WebDistDir="" serves no SPA — but
// unmatched paths still get the standard JSON 404 envelope, so 404
// bodies are identical between dev (no console) and prod (console
// baked in). API clients must never see chi's text/plain default in
// one environment and the envelope in the other.
func TestSPA_DisabledWhenEmpty(t *testing.T) {
	handler := NewRouter(RouterConfig{DevAuthMode: "header"})

	for _, target := range []string{"/login", "/api/v1/nope"} {
		rec := get(handler, target)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s with SPA disabled = %d, want 404", target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("GET %s Content-Type = %q, want application/json", target, ct)
		}
		if !strings.Contains(rec.Body.String(), `"NOT_FOUND"`) {
			t.Errorf("GET %s body = %q, want NOT_FOUND envelope", target, rec.Body.String())
		}
	}
}

// TestSPA_TraversalDoesNotEscapeRoot plants a secret file OUTSIDE the
// dist dir and proves dot-dot paths (raw and percent-encoded) can never
// read it: every response either falls back to index.html or errors —
// the secret bytes never appear.
func TestSPA_TraversalDoesNotEscapeRoot(t *testing.T) {
	handler, dist := spaTestRouter(t)
	secret := "TOPSECRET-OUTSIDE-DIST"
	writeFixture(t, filepath.Join(filepath.Dir(dist), "secret.txt"), secret)

	for _, target := range []string{
		"/../secret.txt",
		"/%2e%2e/secret.txt",
		"/assets/../../secret.txt",
		"/..%2fsecret.txt",
	} {
		rec := get(handler, target)
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("GET %s leaked file content outside the dist root", target)
		}
	}
}
