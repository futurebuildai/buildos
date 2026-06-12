package api

import (
	"context"
	"html"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// publicShareCSP is the Content-Security-Policy stamped on EVERY public
// progress-page response. It is the strictest CSP in the codebase and the
// load-bearing defense for the first surface outside everything-behind-auth:
//
//   - default-src 'none'        — nothing loads by default; each source is
//     opted in explicitly. No JS at all (no script-src), so the page CANNOT
//     call /api/* — it is a self-contained, server-rendered document.
//   - img-src 'self'            — photos come ONLY from the same-origin proxy
//     (GET /p/{token}/photos/{id}); the R2 host/key never appears in the HTML
//     and never widens img-src.
//   - style-src 'self' 'unsafe-inline' — the page ships one inline <style>
//     block (no external stylesheet, no fonts host); 'unsafe-inline' covers it.
//     There is NO script-src, so this does not weaken the XSS posture.
//   - base-uri 'self'; form-action 'none'; frame-ancestors 'none' — no base
//     hijack, no form posting, no framing (the last mirrors the global
//     X-Frame-Options: DENY).
//   - object-src 'none'; connect-src 'none' — no plugins, no fetch/XHR/WS.
const publicShareCSP = "default-src 'none'; " +
	"img-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"base-uri 'self'; " +
	"form-action 'none'; " +
	"frame-ancestors 'none'; " +
	"object-src 'none'; " +
	"connect-src 'none'"

// PublicShareResolver is the consumer-side slice of *service.ShareLinkService the
// public handler needs. Both methods take a CLEARTEXT token (never an org/id) and
// return ONLY redaction-safe data — the service is the boundary that builds the
// allowlisted projection and verifies curated-photo membership.
type PublicShareResolver interface {
	ResolvePublicUpdate(ctx context.Context, cleartext string) (models.PublicUpdate, error)
	ResolvePublicPhoto(ctx context.Context, cleartext string, assetID uuid.UUID) (service.ResolvePublicPhotoTarget, error)
}

// PublicAssetServer is the consumer-side slice of *service.AssetService the photo
// proxy needs: stream an asset's bytes EXIF-stripped (the Chunk A ServeAsset). It
// is org+asset scoped; the org/asset come from the resolver, never the caller.
type PublicAssetServer interface {
	ServeAsset(ctx context.Context, orgID, assetID uuid.UUID) (io.ReadCloser, string, error)
}

// PublicShareHandler serves the UNAUTHENTICATED, token-gated homeowner progress
// page (Chunk E) — the FIRST surface outside everything-behind-auth. It renders
// a minimal, self-contained HTML document (NOT the Lit SPA) from a redaction-safe
// models.PublicUpdate, and proxies curated photos same-origin EXIF-stripped. It
// is mounted as a SIBLING of MountAuthRoutes (outside the auth group) so it
// inherits the global stack (RealIP, rate limiter, security headers) but bypasses
// Auth + SetupGate WITHOUT weakening either.
type PublicShareHandler struct {
	resolver PublicShareResolver
	assets   PublicAssetServer // may be nil (storage off) — the page renders text-only
}

// NewPublicShareHandler binds the handler. assets may be nil (storage
// unconfigured) — the page then renders without the photo strip (graceful
// degrade, D-7).
func NewPublicShareHandler(resolver PublicShareResolver, assets PublicAssetServer) *PublicShareHandler {
	return &PublicShareHandler{resolver: resolver, assets: assets}
}

// Page renders GET /p/{token}. A valid, active token yields a minimal HTML page
// (project name, period date, client-safe prose, curated photo <img> tags
// pointing at the same-origin proxy). Any invalid/expired/revoked/malformed
// token yields a UNIFORM 404 page (the service returns ErrInvalidShareToken for
// all failure reasons). NO auth required, NO cookies, NO /api/* references.
func (h *PublicShareHandler) Page(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	pub, err := h.resolver.ResolvePublicUpdate(r.Context(), token)
	if err != nil {
		h.writeNotFoundPage(w, r)
		return
	}
	h.writePage(w, r, token, pub)
}

// Photo proxies GET /p/{token}/photos/{assetID}. It streams the asset bytes
// EXIF-stripped, but ONLY if the service confirms the asset is in THIS token's
// update's operator-curated photo set. Any other asset — even a real, ready blob
// from the same project/org — yields a uniform 404. The R2 host never appears in
// the client; the homeowner only ever sees same-origin image URLs.
func (h *PublicShareHandler) Photo(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	assetID, err := uuid.Parse(chi.URLParam(r, "assetID"))
	if err != nil {
		h.writeNotFoundImage(w)
		return
	}
	target, err := h.resolver.ResolvePublicPhoto(r.Context(), token, assetID)
	if err != nil || h.assets == nil {
		h.writeNotFoundImage(w)
		return
	}
	body, contentType, err := h.assets.ServeAsset(r.Context(), target.OrgID, target.AssetID)
	if err != nil {
		h.writeNotFoundImage(w)
		return
	}
	defer func() { _ = body.Close() }()

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	// nosniff is also set globally; restate the disposition so the byte stream is
	// rendered inline as an image and never treated as a downloadable document.
	w.Header().Set("Content-Disposition", "inline")
	// PII discipline: photos may carry incidental detail; never let a shared cache
	// retain them. Private + no-store keeps the image out of intermediary caches.
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Content-Security-Policy", publicShareCSP)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// writePage renders the success HTML for a resolved PublicUpdate.
func (h *PublicShareHandler) writePage(w http.ResponseWriter, r *http.Request, token string, pub models.PublicUpdate) {
	var b strings.Builder
	b.WriteString(publicPageHead(pub.ProjectName))
	b.WriteString(`<main class="card">`)
	b.WriteString(`<h1>`)
	b.WriteString(html.EscapeString(pub.ProjectName))
	b.WriteString(`</h1>`)
	if !pub.UpdateDate.IsZero() {
		b.WriteString(`<p class="date">`)
		b.WriteString(html.EscapeString(pub.UpdateDate.Format("January 2, 2006")))
		b.WriteString(`</p>`)
	}
	// Body: escape, then render \n\n as paragraphs and \n as <br>. We deliberately
	// do NOT run a markdown renderer that could emit <a>/<img>/raw HTML — the body
	// is plain prose under a 'default-src none' CSP, so escaping + paragraphs is
	// the whole render. Belt-and-suspenders against any injected markup.
	b.WriteString(`<div class="body">`)
	for _, para := range strings.Split(pub.Body, "\n\n") {
		p := strings.TrimSpace(para)
		if p == "" {
			continue
		}
		b.WriteString(`<p>`)
		b.WriteString(strings.ReplaceAll(html.EscapeString(p), "\n", "<br>"))
		b.WriteString(`</p>`)
	}
	b.WriteString(`</div>`)

	if len(pub.PhotoAssetIDs) > 0 && h.assets != nil {
		b.WriteString(`<div class="photos">`)
		for _, id := range pub.PhotoAssetIDs {
			// Same-origin proxy URL. token + id are both path-safe (base64url token,
			// canonical UUID); escape defensively anyway.
			src := "/p/" + html.EscapeString(token) + "/photos/" + html.EscapeString(id.String())
			b.WriteString(`<img src="`)
			b.WriteString(src)
			b.WriteString(`" alt="Site photo" loading="lazy">`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</main>`)
	b.WriteString(publicPageFoot())

	h.writeHTML(w, http.StatusOK, b.String())
}

// writeNotFoundPage renders the uniform 404 page for any bad/expired/revoked
// token. It is intentionally generic — it never reveals whether a token ever
// existed, expired, or was revoked (enumeration defense).
func (h *PublicShareHandler) writeNotFoundPage(w http.ResponseWriter, _ *http.Request) {
	var b strings.Builder
	b.WriteString(publicPageHead("Link unavailable"))
	b.WriteString(`<main class="card">`)
	b.WriteString(`<h1>This link is unavailable</h1>`)
	b.WriteString(`<p class="body">This progress link is invalid or has expired. Please contact your builder for an updated link.</p>`)
	b.WriteString(`</main>`)
	b.WriteString(publicPageFoot())
	h.writeHTML(w, http.StatusNotFound, b.String())
}

// writeNotFoundImage returns a 404 for a photo that is not in the curated set or
// otherwise unavailable. No body — the <img> simply shows broken; no JSON
// envelope (this is an image endpoint, not an API one).
func (h *PublicShareHandler) writeNotFoundImage(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", publicShareCSP)
	w.WriteHeader(http.StatusNotFound)
}

// writeHTML writes a self-contained HTML response with the strict per-response
// CSP and a no-store cache policy (the page may carry incidental PII; do not let
// a shared cache retain it).
func (h *PublicShareHandler) writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", publicShareCSP)
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// publicPageHead is the document head + opening body. Dark, branded, read-only.
// One inline <style> block (covered by style-src 'unsafe-inline'); NO external
// stylesheet/font and NO script.
func publicPageHead(title string) string {
	return `<!DOCTYPE html><html lang="en"><head>` +
		`<meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<meta name="robots" content="noindex, nofollow">` +
		`<title>` + html.EscapeString(title) + `</title>` +
		`<style>` + publicPageStyle + `</style>` +
		`</head><body>`
}

func publicPageFoot() string {
	return `<footer>Shared by your builder via BuildOS</footer></body></html>`
}

// publicPageStyle is the page's only CSS. Dark theme, mobile-first, accessible
// contrast. No external resources.
const publicPageStyle = `
:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;background:#0c0d10;color:#e7e9ee;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;line-height:1.6}
.card{max-width:720px;margin:0 auto;padding:32px 20px 24px}
h1{font-size:1.6rem;margin:0 0 4px;color:#fff}
.date{margin:0 0 24px;color:#a7adba;font-size:.95rem}
.body p{margin:0 0 16px}
.photos{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:12px;margin-top:24px}
.photos img{width:100%;height:auto;border-radius:10px;display:block;background:#16181d}
footer{max-width:720px;margin:32px auto 40px;padding:0 20px;color:#7a8090;font-size:.8rem;text-align:center}
`

// MountPublicShareRoutes wires the UNAUTHENTICATED public progress page + its
// same-origin photo proxy. CRITICAL: the caller mounts this as a SIBLING of
// MountAuthRoutes — at the router ROOT, OUTSIDE the r.Group that applies
// authMiddleware + SetupGate. It therefore inherits the global stack (RequestID,
// RealIP, otelhttp, SecurityHeaders, RateLimiter, Sentry, Recoverer) — so it is
// rate-limited and security-headed automatically — while bypassing Auth +
// SetupGate WITHOUT touching either (no exempt-prefix entry, because the route
// never enters the auth group at all).
//
// An OPTIONAL dedicated stricter limiter (§9-11 default) may be applied on top of
// the inherited global limiter by passing a non-nil publicLimiter — the public
// route is the brute-force surface and legit homeowner traffic is low.
func MountPublicShareRoutes(r chi.Router, h *PublicShareHandler, publicLimiter func(http.Handler) http.Handler) {
	r.Route("/p", func(r chi.Router) {
		if publicLimiter != nil {
			r.Use(publicLimiter)
		}
		r.Get("/{token}", h.Page)
		r.Get("/{token}/photos/{assetID}", h.Photo)
	})
}

// compile-time assertion that the concrete service satisfies the consumer-side
// interface (catches a signature drift at build, not at wire time).
var _ PublicShareResolver = (*service.ShareLinkService)(nil)
