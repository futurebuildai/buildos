package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Config configures an R2Store (and any S3-compatible endpoint). All five
// fields are per-fork (ADR-002): endpoint+bucket+region come from the operator's
// env via internal/config.SecretSource; AccessKeyID+SecretAccessKey are sealed
// in the encrypted vault (provider "object_store") and resolved at construction.
type Config struct {
	Endpoint        string // scheme://host, e.g. https://<acct>.r2.cloudflarestorage.com
	Bucket          string
	Region          string // R2 convention: "auto"
	AccessKeyID     string
	SecretAccessKey string
	// HTTPClient is the client used for the server-side Get/Delete paths. When
	// nil, a default SSRF-discipline client is built (https-only, no redirects,
	// bounded timeouts). Presigned URLs are handed to the CLIENT and never use
	// this — the endpoint host is operator-configured trust.
	HTTPClient *http.Client
	// now is the signing clock; nil uses time.Now. Tests inject a fixed instant
	// for deterministic presigned-URL assertions.
	now func() time.Time
}

// R2Store implements ObjectStore against an S3-compatible endpoint using the
// hand-rolled SigV4 presigner. Constructed per-fork; stateless after build and
// safe to share across goroutines.
type R2Store struct {
	creds  signerCreds
	bucket string
	scheme string
	host   string
	http   *http.Client
	now    func() time.Time
}

// NewR2Store builds an R2Store from per-fork Config. Returns ErrUnconfigured
// when any required field (endpoint, bucket, access key, secret) is empty — the
// caller treats that as soft-fail (storage disabled, endpoints 503). Region
// defaults to "auto" (R2 convention).
func NewR2Store(cfg Config) (*R2Store, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	bucket := strings.TrimSpace(cfg.Bucket)
	if endpoint == "" || bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, ErrUnconfigured
	}
	host, scheme, err := splitEndpoint(endpoint)
	if err != nil {
		return nil, fmt.Errorf("storage: parse endpoint: %w", err)
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "auto"
	}
	clock := cfg.now
	if clock == nil {
		clock = time.Now
	}
	client := cfg.HTTPClient
	if client == nil {
		client = newServerSideClient()
	}
	return &R2Store{
		creds: signerCreds{
			accessKeyID:     cfg.AccessKeyID,
			secretAccessKey: cfg.SecretAccessKey,
			region:          region,
			service:         "s3",
		},
		bucket: bucket,
		scheme: scheme,
		host:   host,
		http:   client,
		now:    clock,
	}, nil
}

// PresignPut returns a presigned PUT URL with Content-Type + Content-Length
// signed in (R2 rejects a mismatched body). The returned signedHeaders are the
// exact headers the client must echo on the PUT.
func (s *R2Store) PresignPut(_ context.Context, key, contentType string, contentLength int64, ttl time.Duration) (string, map[string]string, error) {
	if key == "" {
		return "", nil, fmt.Errorf("storage: empty key")
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	signed := map[string]string{
		"content-type":   contentType,
		"content-length": strconv.FormatInt(contentLength, 10),
	}
	u, err := presignedURL(presignParams{
		creds:         s.creds,
		method:        http.MethodPut,
		endpoint:      s.scheme + "://" + s.host,
		bucket:        s.bucket,
		key:           key,
		now:           s.now(),
		ttl:           ttl,
		signedHeaders: signed,
	})
	if err != nil {
		return "", nil, err
	}
	// Return the client-facing header names (canonical-case for HTTP wire use).
	return u, map[string]string{
		"Content-Type":   contentType,
		"Content-Length": strconv.FormatInt(contentLength, 10),
	}, nil
}

// PresignGet returns a presigned GET URL valid for ttl.
func (s *R2Store) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	if key == "" {
		return "", fmt.Errorf("storage: empty key")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return presignedURL(presignParams{
		creds:    s.creds,
		method:   http.MethodGet,
		endpoint: s.scheme + "://" + s.host,
		bucket:   s.bucket,
		key:      key,
		now:      s.now(),
		ttl:      ttl,
	})
}

// Get streams object bytes via a header-signed GET from the Go server (the
// same-origin proxy path). The caller MUST Close the returned reader.
func (s *R2Store) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	if key == "" {
		return nil, "", fmt.Errorf("storage: empty key")
	}
	objURL := s.scheme + "://" + s.host + uriEncodePath(objectPath(s.bucket, key))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("storage: build get: %w", err)
	}
	req.Host = s.host
	signRequest(req, s.creds, unsignedPayload, s.now())

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("storage: get: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, "", ErrObjectNotFound
	}
	if resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		return nil, "", fmt.Errorf("storage: get status %d", resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// Delete removes an object (header-signed DELETE). A 404/204 is success.
func (s *R2Store) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("storage: empty key")
	}
	objURL := s.scheme + "://" + s.host + uriEncodePath(objectPath(s.bucket, key))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, objURL, nil)
	if err != nil {
		return fmt.Errorf("storage: build delete: %w", err)
	}
	req.Host = s.host
	signRequest(req, s.creds, emptyPayloadSHA, s.now())

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("storage: delete: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<14))
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("storage: delete status %d", resp.StatusCode)
	}
	return nil
}

// ErrObjectNotFound is returned by Get when the object key does not exist.
var ErrObjectNotFound = errors.New("storage: object not found")

// newServerSideClient builds the http.Client for the server-side Get/Delete
// paths. The endpoint is operator-configured (per-fork) so this is less
// adversarial than the connector egress client, but we keep the same
// discipline: https-only enforced at request build, no redirects (a 30x to a
// private host would otherwise bypass intent), bounded timeouts, and a dial
// Control hook rejecting loopback/link-local so a misconfigured endpoint can't
// be pointed at the metadata service.
func newServerSideClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errBlockedEgress
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
				ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return errBlockedEgress
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errNoRedirectEgress
		},
	}
}

var (
	errBlockedEgress    = errors.New("storage: blocked egress address")
	errNoRedirectEgress = errors.New("storage: redirects not allowed")
)
