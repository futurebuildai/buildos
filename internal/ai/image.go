package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Image-handling sentinels. Distinct typed errors so callers can map an
// oversize / unsupported document to a precise 4xx rather than a generic
// 500.
var (
	// ErrImageTooLarge is returned when a fetched document image exceeds
	// the client's MaxImageBytes ceiling.
	ErrImageTooLarge = errors.New("ai: document image exceeds size limit")

	// ErrUnsupportedMediaType is returned when a fetched document is not
	// one of the Anthropic-supported image media types.
	ErrUnsupportedMediaType = errors.New("ai: unsupported document media type")
)

// supportedImageMediaTypes is the set Anthropic accepts for image
// content blocks.
var supportedImageMediaTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// fetchDocumentImage GETs url via the client's shared HTTP client,
// enforces the MaxImageBytes ceiling, validates the media type, and
// returns the detected media type plus the base64-encoded bytes ready
// to drop into an image content block.
//
// The raw bytes are NEVER logged. Oversize and unsupported-media-type
// failures return the typed sentinels above.
func (c *Client) fetchDocumentImage(ctx context.Context, rawURL string) (mediaType string, base64Data string, err error) {
	// SSRF defense layer 1 (scheme): only https — reject http/file/gopher/etc.
	// Layer 2 (the resolved private-IP denylist + no-redirect) is enforced by
	// c.docHTTPClient (the guarded egress client) at dial time.
	u, perr := url.Parse(rawURL)
	if perr != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", fmt.Errorf("%w: document_url must be an absolute https URL", ErrUnsupportedMediaType)
	}
	// The AUTHORITATIVE SSRF guard is the egress dial Control hook on
	// c.docHTTPClient: it blocks the RESOLVED private/metadata IP at connect
	// time (so a literal private IP OR a hostname that resolves to one is
	// refused, defeating DNS-rebind), and refuses redirects. The scheme check
	// above is the cheap first layer.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("ai: build image request: %w", err)
	}

	resp, err := c.docHTTPClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("ai: fetch document image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("ai: fetch document image: HTTP %d", resp.StatusCode)
	}

	// Read at most MaxImageBytes+1 so we can detect an over-limit body
	// without buffering arbitrarily large attacker-controlled input.
	limited := io.LimitReader(resp.Body, c.maxImageBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", "", fmt.Errorf("ai: read document image: %w", err)
	}
	if int64(len(data)) > c.maxImageBytes {
		return "", "", ErrImageTooLarge
	}

	// Determine the media type. Prefer the server's Content-Type when it
	// names a supported image type; otherwise sniff the bytes. Sniffing
	// guards against a mislabeled or generic (octet-stream) header.
	mediaType = normalizeMediaType(resp.Header.Get("Content-Type"))
	if !supportedImageMediaTypes[mediaType] {
		mediaType = normalizeMediaType(http.DetectContentType(data))
	}
	if !supportedImageMediaTypes[mediaType] {
		return "", "", fmt.Errorf("%w: %q", ErrUnsupportedMediaType, mediaType)
	}

	return mediaType, base64.StdEncoding.EncodeToString(data), nil
}

// normalizeMediaType strips any parameters (e.g. "; charset=...") and
// surrounding whitespace from a Content-Type value, returning just the
// media type token.
func normalizeMediaType(ct string) string {
	for i := 0; i < len(ct); i++ {
		if ct[i] == ';' {
			ct = ct[:i]
			break
		}
	}
	// trim spaces
	start, end := 0, len(ct)
	for start < end && ct[start] == ' ' {
		start++
	}
	for end > start && ct[end-1] == ' ' {
		end--
	}
	return ct[start:end]
}
