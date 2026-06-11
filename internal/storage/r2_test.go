package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func newTestStore(t *testing.T, client *http.Client) *R2Store {
	t.Helper()
	s, err := NewR2Store(Config{
		Endpoint:        "https://acct123.r2.cloudflarestorage.com",
		Bucket:          "photos",
		Region:          "auto",
		AccessKeyID:     "AKIATESTKEY",
		SecretAccessKey: "testsecret/value",
		HTTPClient:      client,
		now:             fixedClock(time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("NewR2Store: %v", err)
	}
	return s
}

func TestNewR2Store_Unconfigured(t *testing.T) {
	cases := []Config{
		{Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s"},           // no endpoint
		{Endpoint: "https://x", AccessKeyID: "k", SecretAccessKey: "s"}, // no bucket
		{Endpoint: "https://x", Bucket: "b", SecretAccessKey: "s"},      // no access key
		{Endpoint: "https://x", Bucket: "b", AccessKeyID: "k"},          // no secret
	}
	for i, c := range cases {
		if _, err := NewR2Store(c); !errors.Is(err, ErrUnconfigured) {
			t.Errorf("case %d: want ErrUnconfigured, got %v", i, err)
		}
	}
}

func TestPresignPut_ShapeAndSignedHeaders(t *testing.T) {
	s := newTestStore(t, &http.Client{})
	gotURL, headers, err := s.PresignPut(context.Background(), "org/o1/proj/p1/abc.jpg", "image/jpeg", 12345, 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	u, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Path is bucket-rooted, key segments preserved.
	if u.Path != "/photos/org/o1/proj/p1/abc.jpg" {
		t.Errorf("path = %q", u.Path)
	}
	q := u.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		t.Errorf("algo = %q", q.Get("X-Amz-Algorithm"))
	}
	if q.Get("X-Amz-Signature") == "" {
		t.Error("missing signature")
	}
	if q.Get("X-Amz-Expires") != "300" {
		t.Errorf("expires = %q", q.Get("X-Amz-Expires"))
	}
	// Content-Type + Content-Length must be signed into the URL (so R2 rejects a
	// mismatched body) — assert they appear in the SignedHeaders list.
	sh := q.Get("X-Amz-SignedHeaders")
	for _, want := range []string{"host", "content-length", "content-type"} {
		if !strings.Contains(sh, want) {
			t.Errorf("SignedHeaders %q missing %q", sh, want)
		}
	}
	// The returned headers are what the client must echo on PUT.
	if headers["Content-Type"] != "image/jpeg" {
		t.Errorf("header Content-Type = %q", headers["Content-Type"])
	}
	if headers["Content-Length"] != "12345" {
		t.Errorf("header Content-Length = %q", headers["Content-Length"])
	}
}

func TestPresignGet_Deterministic(t *testing.T) {
	s := newTestStore(t, &http.Client{})
	a, err := s.PresignGet(context.Background(), "k/obj.png", 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	b, _ := s.PresignGet(context.Background(), "k/obj.png", 15*time.Minute)
	if a != b {
		t.Fatal("presigned GET not deterministic for fixed clock")
	}
	u, _ := url.Parse(a)
	if u.Query().Get("X-Amz-Expires") != "900" {
		t.Errorf("expires = %q", u.Query().Get("X-Amz-Expires"))
	}
}

// roundTripFunc lets a test stub the http.Client transport (no network).
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGet_SignsAndStreams(t *testing.T) {
	var sawAuth, sawDate, sawSha string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sawAuth = r.Header.Get("Authorization")
		sawDate = r.Header.Get("X-Amz-Date")
		sawSha = r.Header.Get("X-Amz-Content-Sha256")
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"image/png"}},
			Body:       io.NopCloser(strings.NewReader("PNGBYTES")),
		}, nil
	})}
	s := newTestStore(t, client)
	body, ct, err := s.Get(context.Background(), "k/obj.png")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer body.Close()
	if ct != "image/png" {
		t.Errorf("content-type = %q", ct)
	}
	data, _ := io.ReadAll(body)
	if string(data) != "PNGBYTES" {
		t.Errorf("body = %q", data)
	}
	if !strings.HasPrefix(sawAuth, "AWS4-HMAC-SHA256 Credential=AKIATESTKEY/") {
		t.Errorf("Authorization = %q", sawAuth)
	}
	if sawDate == "" || sawSha != unsignedPayload {
		t.Errorf("date=%q sha=%q", sawDate, sawSha)
	}
}

func TestGet_NotFound(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	s := newTestStore(t, client)
	if _, _, err := s.Get(context.Background(), "missing"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("want ErrObjectNotFound, got %v", err)
	}
}
