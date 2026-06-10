package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tinyPNG returns the bytes of a 1x1 PNG. Used by image tests and the
// invoice-image round-trip test in client_test.go.
func tinyPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// imageTestClient injects docClient as the DocumentFetchClient so tests reach
// an httptest TLS server (the real default is the SSRF-guarded egress client,
// which deliberately refuses loopback — the SSRF guard itself is covered in
// internal/connectors). Document URLs must be https (the scheme allowlist).
func imageTestClient(t *testing.T, maxBytes int64, docClient *http.Client) *Client {
	t.Helper()
	c, err := NewClient(Config{
		KeyResolver:         staticKey("k"),
		MaxImageBytes:       maxBytes,
		DocumentFetchClient: docClient,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestFetchDocumentImage_HappyPath(t *testing.T) {
	pngBytes := tinyPNG()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	c := imageTestClient(t, defaultMaxImageBytes, srv.Client())
	mt, b64, err := c.fetchDocumentImage(context.Background(), srv.URL+"/doc.png")
	if err != nil {
		t.Fatalf("fetchDocumentImage: %v", err)
	}
	if mt != "image/png" {
		t.Errorf("mediaType = %q, want image/png", mt)
	}
	want := base64.StdEncoding.EncodeToString(pngBytes)
	if b64 != want {
		t.Errorf("base64 mismatch")
	}
}

// Content-Type missing/generic but bytes sniff as PNG → accepted.
func TestFetchDocumentImage_SniffsWhenContentTypeGeneric(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(tinyPNG())
	}))
	defer srv.Close()

	c := imageTestClient(t, defaultMaxImageBytes, srv.Client())
	mt, _, err := c.fetchDocumentImage(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchDocumentImage: %v", err)
	}
	if mt != "image/png" {
		t.Errorf("mediaType = %q, want sniffed image/png", mt)
	}
}

func TestFetchDocumentImage_OversizeRejected(t *testing.T) {
	// Serve more bytes than the ceiling.
	big := make([]byte, 2048)
	copy(big, tinyPNG()) // keep a valid PNG header so it's not also a media-type fail
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	c := imageTestClient(t, 1024, srv.Client()) // 1KB ceiling, body is 2KB
	_, _, err := c.fetchDocumentImage(context.Background(), srv.URL)
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("err = %v, want ErrImageTooLarge", err)
	}
}

// Exactly-at-limit is accepted (the limit is inclusive).
func TestFetchDocumentImage_AtLimitAccepted(t *testing.T) {
	pngBytes := tinyPNG()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	c := imageTestClient(t, int64(len(pngBytes)), srv.Client()) // ceiling == exact size
	if _, _, err := c.fetchDocumentImage(context.Background(), srv.URL); err != nil {
		t.Fatalf("at-limit should be accepted; err = %v", err)
	}
}

func TestFetchDocumentImage_UnsupportedMediaTypeRejected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7\n%fake pdf bytes that won't sniff as an image"))
	}))
	defer srv.Close()

	c := imageTestClient(t, defaultMaxImageBytes, srv.Client())
	_, _, err := c.fetchDocumentImage(context.Background(), srv.URL)
	if !errors.Is(err, ErrUnsupportedMediaType) {
		t.Fatalf("err = %v, want ErrUnsupportedMediaType", err)
	}
}

func TestFetchDocumentImage_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := imageTestClient(t, defaultMaxImageBytes, srv.Client())
	if _, _, err := c.fetchDocumentImage(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error on 404")
	}
}

// TestFetchDocumentImage_TransportError covers the httpClient.Do error
// leg: a connection to a closed server fails before any status check.
func TestFetchDocumentImage_TransportError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // connections now refused

	c := imageTestClient(t, defaultMaxImageBytes, srv.Client())
	if _, _, err := c.fetchDocumentImage(context.Background(), url); err == nil {
		t.Fatal("expected a transport error fetching from a closed server")
	}
}

// TestFetchDocumentImage_BodyReadError covers the io.ReadAll error leg: a
// 200 whose body fails mid-read surfaces a read error. The erroringBody /
// stubTransport helpers live in client_test.go (same package).
func TestFetchDocumentImage_BodyReadError(t *testing.T) {
	rt := stubTransport{fn: func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       erroringBody{},
			Header:     make(http.Header),
		}, nil
	}}
	c, err := NewClient(Config{
		KeyResolver:         staticKey("k"),
		DocumentFetchClient: &http.Client{Transport: rt},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, _, ferr := c.fetchDocumentImage(context.Background(), "https://doc.invalid/x.png"); ferr == nil || !strings.Contains(ferr.Error(), "read document image") {
		t.Fatalf("err = %v, want a read document image error", ferr)
	}
}

// SSRF scheme guard: only https document URLs are fetched.
func TestFetchDocumentImage_RejectsNonHTTPS(t *testing.T) {
	c := imageTestClient(t, defaultMaxImageBytes, &http.Client{})
	for _, u := range []string{
		"http://example.com/x.png",
		"file:///etc/passwd",
		"gopher://internal/x",
		"//example.com/x.png", // scheme-relative → no scheme
		"not a url",
	} {
		if _, _, err := c.fetchDocumentImage(context.Background(), u); !errors.Is(err, ErrUnsupportedMediaType) {
			t.Errorf("url %q: err = %v, want ErrUnsupportedMediaType", u, err)
		}
	}
}

func TestNormalizeMediaType(t *testing.T) {
	cases := map[string]string{
		"image/png":                "image/png",
		"image/png; charset=utf-8": "image/png",
		"  image/jpeg ":            "image/jpeg",
		"":                         "",
	}
	for in, want := range cases {
		if got := normalizeMediaType(in); got != want {
			t.Errorf("normalizeMediaType(%q) = %q, want %q", in, got, want)
		}
	}
}
