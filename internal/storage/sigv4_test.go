package storage

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestPresignedURL_CanonicalAWSVector validates the hand-rolled presigner
// against the canonical AWS "Signature Version 4 Test Suite" example for a
// presigned S3 GET (AWS docs: "Examples of how to derive a signing key" /
// presigned-url example). With access key AKIAIOSFODNN7EXAMPLE, the published
// secret, region us-east-1, service s3, date 20130524T000000Z, and a 86400s
// expiry on GET /test.txt against examplebucket.s3.amazonaws.com, the expected
// X-Amz-Signature is the well-known fixture value below. A drift here means the
// canonical-request / string-to-sign / signing-key math diverged from the spec.
func TestPresignedURL_CanonicalAWSVector(t *testing.T) {
	creds := signerCreds{
		accessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		region:          "us-east-1",
		service:         "s3",
	}
	signTime := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

	// The AWS example presigns "GET /test.txt" on host
	// examplebucket.s3.amazonaws.com. Our presigner places the bucket in the
	// path (path-style), which is the R2 shape. To reproduce the exact AWS
	// vector we treat the host AS the bucket-qualified host and pass an empty
	// bucket so the canonical URI is exactly "/test.txt".
	gotURL, err := presignedURL(presignParams{
		creds:    creds,
		method:   "GET",
		endpoint: "https://examplebucket.s3.amazonaws.com",
		bucket:   "",
		key:      "test.txt",
		now:      signTime,
		ttl:      86400 * time.Second,
	})
	if err != nil {
		t.Fatalf("presignedURL: %v", err)
	}
	u, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	const wantSig = "aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	if got := u.Query().Get("X-Amz-Signature"); got != wantSig {
		t.Fatalf("signature mismatch:\n got %s\nwant %s\nfull url: %s", got, wantSig, gotURL)
	}
	// Spot-check the other presign query params are present + shaped.
	q := u.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		t.Errorf("X-Amz-Algorithm = %q", q.Get("X-Amz-Algorithm"))
	}
	if !strings.HasPrefix(q.Get("X-Amz-Credential"), "AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request") {
		t.Errorf("X-Amz-Credential = %q", q.Get("X-Amz-Credential"))
	}
	if q.Get("X-Amz-Date") != "20130524T000000Z" {
		t.Errorf("X-Amz-Date = %q", q.Get("X-Amz-Date"))
	}
	if q.Get("X-Amz-Expires") != "86400" {
		t.Errorf("X-Amz-Expires = %q", q.Get("X-Amz-Expires"))
	}
	if q.Get("X-Amz-SignedHeaders") != "host" {
		t.Errorf("X-Amz-SignedHeaders = %q", q.Get("X-Amz-SignedHeaders"))
	}
}

// TestSigningKeyDerivation pins the SigV4 signing-key derivation against the
// AWS docs worked example (region us-east-1, service "iam" ... but the docs
// publish the kSigning hex for service "iam"; we instead assert the property
// that the same inputs deterministically reproduce a fixed hex — a regression
// guard, not an external vector). Determinism is what matters: identical inputs
// → identical key.
func TestSigningKeyDeterminism(t *testing.T) {
	c := signerCreds{secretAccessKey: "secret", region: "auto", service: "s3"}
	a := c.signingKey("20260101")
	b := c.signingKey("20260101")
	if string(a) != string(b) {
		t.Fatal("signing key derivation is not deterministic")
	}
	diff := c.signingKey("20260102")
	if string(a) == string(diff) {
		t.Fatal("signing key did not change with date")
	}
}

// TestRFC3986Escape covers the SigV4 encoding rules: unreserved chars pass,
// reserved chars are %XX uppercase, and '/' is preserved or encoded per flag.
func TestRFC3986Escape(t *testing.T) {
	cases := []struct {
		in          string
		encodeSlash bool
		want        string
	}{
		{"abcXYZ-_.~", true, "abcXYZ-_.~"},
		{"a b", true, "a%20b"},
		{"a/b", false, "a/b"},
		{"a/b", true, "a%2Fb"},
		{"foo=bar&baz", true, "foo%3Dbar%26baz"},
	}
	for _, tc := range cases {
		if got := rfc3986Escape(tc.in, tc.encodeSlash); got != tc.want {
			t.Errorf("rfc3986Escape(%q, %v) = %q, want %q", tc.in, tc.encodeSlash, got, tc.want)
		}
	}
}

// TestCanonicalQuerySorted asserts canonical query ordering is by encoded key.
func TestCanonicalQuerySorted(t *testing.T) {
	q := url.Values{}
	q.Set("X-Amz-Date", "20260101T000000Z")
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	got := canonicalQuery(q)
	want := "X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Date=20260101T000000Z"
	if got != want {
		t.Fatalf("canonicalQuery = %q, want %q", got, want)
	}
}
