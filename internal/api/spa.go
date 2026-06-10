package api

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// spaIndexPage is the SPA entry document. Every client-routed path
// (e.g. /projects/123/schedule) falls back to it; the web console's
// router takes over from there.
const spaIndexPage = "index.html"

// spaCSP is the Content-Security-Policy stamped on the SPA's HTML
// responses. The Go server is the SPA host in production
// (docs/security-posture.md L-2), so the CSP for the browser-rendered
// console is owned here, not by an external static host.
//
// Allowances beyond 'self', and why:
//   - style-src 'unsafe-inline' — Lit templates render style="…"
//     attributes (fb-state skeletons, fb-toast tone vars); inline style
//     ATTRIBUTES require it. Inline <script> stays blocked — script-src
//     'self' is the load-bearing XSS defense.
//   - fonts.googleapis.com / fonts.gstatic.com — web/index.html loads
//     Outfit + JetBrains Mono from Google Fonts (DESIGN_SYSTEM §3).
//   - img-src data: — inline SVG/data URIs (favicons, generated icons).
//   - connect-src 'self' — the API is same-origin by construction.
//   - frame-ancestors 'none' — CSP-level mirror of X-Frame-Options:
//     DENY (already set globally by mw.SecurityHeaders).
//
// Known limitation: the console's opt-in browser-Sentry hook
// (web/src/obs/sentry.ts — host-page-loaded SDK + VITE_SENTRY_DSN) is
// blocked by this CSP on both legs (CDN script load + ingest POST). A
// fork enabling browser telemetry must extend script-src/connect-src
// here with its Sentry origins; see docs/security-posture.md.
const spaCSP = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"object-src 'none'"

// spaHandler serves the built web console (web/dist) same-origin from
// the Go server. It mounts as the router's NotFound handler. Registered
// routes (/api/*, /health, /ready, /metrics) always win — but chi
// PROPAGATES a root NotFound into every mounted subrouter, so unmatched
// paths INSIDE API subtrees (e.g. GET /api/v1/projects/{id}/bogus) DO
// execute this handler, after the group's middleware (auth, SetupGate)
// has run. The /api guard below keys on r.URL.Path — which chi mounts
// never rewrite (they only shift the routing context's RoutePath) — and
// that is load-bearing: switching it to chi's RoutePath, or reordering
// it below the file-serving logic, would serve HTML inside API subtrees.
//
// Behavior:
//   - /api/* misses get a JSON 404 (the standard error envelope), never
//     HTML — an API typo must not look like a successful page load.
//   - Real files under the dist dir are served as-is. Vite's hashed
//     bundles under /assets/* get an immutable year-long cache.
//   - Everything else falls back to index.html (SPA client routing)
//     with Cache-Control: no-cache so deploys take effect on reload.
//   - HTML responses carry the SPA Content-Security-Policy; the rest of
//     the security-header baseline (nosniff, X-Frame-Options, referrer
//     policy) is already applied globally by mw.SecurityHeaders.
type spaHandler struct {
	fsys fs.FS
}

// newSPAHandler builds a handler rooted at distDir. The caller
// (cmd/server) validates the directory exists before wiring it.
func newSPAHandler(distDir string) *spaHandler {
	return &spaHandler{fsys: os.DirFS(distDir)}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// API misses stay JSON. Guard by prefix so a typo'd or removed
	// endpoint returns the standard error envelope (and ticks the
	// error-response metric), not a 200 with index.html.
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}

	// The SPA surface is read-only. A non-GET to an unregistered path
	// is a client error, not a page load.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}

	// Resolve the request to a file inside the dist root. path.Clean +
	// fs.ValidPath reject traversal ("..", absolute paths); anything
	// invalid or missing falls back to the SPA entry document rather
	// than erroring — that's the SPA contract for client-routed paths.
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || !fs.ValidPath(name) {
		name = spaIndexPage
	}
	if info, err := fs.Stat(h.fsys, name); err != nil || info.IsDir() {
		// Missing hashed bundles must 404, not fall back to HTML: a
		// stale index.html referencing a gone asset would otherwise get
		// text/html where the browser expected JS/CSS — an opaque MIME
		// error instead of an obvious miss.
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		name = spaIndexPage
	}

	switch {
	case name == spaIndexPage:
		// no-cache ≠ no-store: the browser may cache but must
		// revalidate, so a new deploy's index (pointing at new hashed
		// bundles) is picked up on the next load.
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", spaCSP)
	case strings.HasPrefix(name, "assets/"):
		// Vite content-hashes every file under assets/ — a changed
		// file is a new URL, so the old one is immutable forever.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	// ServeFileFS handles Content-Type, Last-Modified/conditional
	// requests, ranges, and HEAD. It independently rejects any
	// lingering ".." in the raw URL path with a 400.
	http.ServeFileFS(w, r, h.fsys, name)
}
