package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Hand-rolled AWS Signature Version 4 (zero external SDK; stdlib crypto only).
//
// Implements the two shapes BuildOS needs against an S3-compatible endpoint
// (Cloudflare R2):
//
//   - QUERY-PARAM PRESIGNING (presignedURL): a self-contained URL carrying the
//     signature in X-Amz-* query params, handed to a client to PUT/GET directly.
//     Used by PresignPut / PresignGet.
//   - AUTHORIZATION-HEADER SIGNING (signRequest): the canonical signature in an
//     Authorization header on a server-side request. Used by Get / Delete (the
//     same-origin proxy + GC paths that run from the Go server).
//
// Spec: AWS "Signature Version 4 signing process". The algorithm is stable and
// shared verbatim by R2; the only S3-specific knobs are service="s3" and the
// UNSIGNED-PAYLOAD content-sha for streamed/presigned bodies.

const (
	awsV4Algorithm  = "AWS4-HMAC-SHA256"
	unsignedPayload = "UNSIGNED-PAYLOAD"
	emptyPayloadSHA = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // sha256("")
	iso8601Basic    = "20060102T150405Z"
	yyyymmdd        = "20060102"
)

// signerCreds is the static credential pair used to sign. Region for R2 is
// conventionally "auto". service is "s3".
type signerCreds struct {
	accessKeyID     string
	secretAccessKey string
	region          string
	service         string // "s3"
}

// hmacSHA256 computes HMAC-SHA256(key, data).
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// sha256Hex returns the lowercase hex of sha256(data).
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// signingKey derives the SigV4 signing key:
//
//	kDate    = HMAC("AWS4"+secret, yyyymmdd)
//	kRegion  = HMAC(kDate, region)
//	kService = HMAC(kRegion, service)
//	kSigning = HMAC(kService, "aws4_request")
func (c signerCreds) signingKey(dateStamp string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+c.secretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(c.region))
	kService := hmacSHA256(kRegion, []byte(c.service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

// credentialScope is "<yyyymmdd>/<region>/<service>/aws4_request".
func (c signerCreds) credentialScope(dateStamp string) string {
	return dateStamp + "/" + c.region + "/" + c.service + "/aws4_request"
}

// objectPath joins a bucket + key into the object's URI path. An empty bucket
// (path-rooted endpoint, or the AWS canonical test vector's virtual-host shape)
// yields "/<key>"; otherwise "/<bucket>/<key>".
func objectPath(bucket, key string) string {
	if bucket == "" {
		return "/" + key
	}
	return "/" + bucket + "/" + key
}

// uriEncodePath encodes a URI path per the SigV4 rules: every segment is
// rfc3986-escaped, but the path separators '/' are preserved. AWS keeps '/'
// literal in the canonical URI for S3.
func uriEncodePath(p string) string {
	if p == "" {
		return "/"
	}
	segments := strings.Split(p, "/")
	for i, s := range segments {
		segments[i] = rfc3986Escape(s, false)
	}
	return strings.Join(segments, "/")
}

// rfc3986Escape escapes a string per RFC 3986, the encoding SigV4 requires for
// both canonical URI segments and canonical query values. Unreserved chars
// (A-Z a-z 0-9 - _ . ~) pass through; everything else is %XX uppercase. When
// encodeSlash is false, '/' is preserved (path-segment use).
func rfc3986Escape(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9',
			ch == '-', ch == '_', ch == '.', ch == '~':
			b.WriteByte(ch)
		case ch == '/' && !encodeSlash:
			b.WriteByte(ch)
		default:
			b.WriteByte('%')
			b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{ch})))
		}
	}
	return b.String()
}

// canonicalQuery builds the canonical query string: each key+value
// rfc3986-escaped (slashes encoded), sorted by encoded key (then value).
func canonicalQuery(q url.Values) string {
	type kv struct{ k, v string }
	var pairs []kv
	for k, vs := range q {
		ek := rfc3986Escape(k, true)
		for _, v := range vs {
			pairs = append(pairs, kv{ek, rfc3986Escape(v, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(p.v)
	}
	return b.String()
}

// canonicalHeaders builds the canonical headers block + the signed-headers list.
// Header names are lowercased and sorted; values are trimmed. headers maps a
// lowercase header name to its value.
func canonicalHeaders(headers map[string]string) (canonical, signedList string) {
	names := make([]string, 0, len(headers))
	for n := range headers {
		names = append(names, strings.ToLower(n))
	}
	sort.Strings(names)
	var cb strings.Builder
	for _, n := range names {
		cb.WriteString(n)
		cb.WriteByte(':')
		cb.WriteString(strings.TrimSpace(headers[n]))
		cb.WriteByte('\n')
	}
	return cb.String(), strings.Join(names, ";")
}

// presignParams is the input for presignedURL.
type presignParams struct {
	creds         signerCreds
	method        string // "GET" / "PUT"
	endpoint      string // scheme://host (no path), e.g. https://<acct>.r2.cloudflarestorage.com
	bucket        string
	key           string            // opaque object key (no leading slash)
	now           time.Time         // signing time (UTC); tests inject a fixed instant
	ttl           time.Duration     // X-Amz-Expires
	signedHeaders map[string]string // extra headers signed into the URL (e.g. content-type, content-length); host is always added
}

// presignedURL builds a SigV4 query-param-presigned URL. The signature covers
// the canonical request whose payload hash is UNSIGNED-PAYLOAD (the standard
// for presigned PUT/GET — the client supplies the body the URL grants access
// to). Headers in signedHeaders are bound into the signature, so a client that
// omits or changes them (e.g. a different Content-Type or Content-Length on a
// PUT) is rejected by R2.
func presignedURL(p presignParams) (string, error) {
	host, scheme, err := splitEndpoint(p.endpoint)
	if err != nil {
		return "", err
	}
	canonicalURI := uriEncodePath(objectPath(p.bucket, p.key))
	amzDate := p.now.UTC().Format(iso8601Basic)
	dateStamp := p.now.UTC().Format(yyyymmdd)

	// Signed headers always include host; merge any caller-supplied headers.
	hdrs := map[string]string{"host": host}
	for k, v := range p.signedHeaders {
		hdrs[strings.ToLower(k)] = v
	}
	canonicalHdrs, signedHdrList := canonicalHeaders(hdrs)

	// X-Amz-* presign query params. Per spec, X-Amz-SignedHeaders lists the
	// headers folded into the canonical request.
	q := url.Values{}
	q.Set("X-Amz-Algorithm", awsV4Algorithm)
	q.Set("X-Amz-Credential", p.creds.accessKeyID+"/"+p.creds.credentialScope(dateStamp))
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", strconv.Itoa(int(p.ttl.Seconds())))
	q.Set("X-Amz-SignedHeaders", signedHdrList)

	canonicalRequest := strings.Join([]string{
		p.method,
		canonicalURI,
		canonicalQuery(q),
		canonicalHdrs,
		signedHdrList,
		unsignedPayload,
	}, "\n")

	stringToSign := strings.Join([]string{
		awsV4Algorithm,
		amzDate,
		p.creds.credentialScope(dateStamp),
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signature := hex.EncodeToString(hmacSHA256(p.creds.signingKey(dateStamp), []byte(stringToSign)))
	q.Set("X-Amz-Signature", signature)

	return scheme + "://" + host + canonicalURI + "?" + canonicalQuery(q), nil
}

// signRequest signs an *http.Request in place with an Authorization header
// (header-based SigV4). payloadHash is the hex sha256 of the body, or
// UNSIGNED-PAYLOAD for streamed bodies. Used by the server-side Get/Delete
// paths. The request's Host, X-Amz-Date, and X-Amz-Content-Sha256 headers are
// set here.
func signRequest(req *http.Request, creds signerCreds, payloadHash string, now time.Time) {
	amzDate := now.UTC().Format(iso8601Basic)
	dateStamp := now.UTC().Format(yyyymmdd)

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	// Sign host + x-amz-date + x-amz-content-sha256 (the minimal stable set).
	hdrs := map[string]string{
		"host":                 host,
		"x-amz-date":           amzDate,
		"x-amz-content-sha256": payloadHash,
	}
	canonicalHdrs, signedHdrList := canonicalHeaders(hdrs)

	canonicalURI := uriEncodePath(req.URL.Path)
	canonicalQ := canonicalQuery(req.URL.Query())

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQ,
		canonicalHdrs,
		signedHdrList,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		awsV4Algorithm,
		amzDate,
		creds.credentialScope(dateStamp),
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signature := hex.EncodeToString(hmacSHA256(creds.signingKey(dateStamp), []byte(stringToSign)))

	auth := awsV4Algorithm +
		" Credential=" + creds.accessKeyID + "/" + creds.credentialScope(dateStamp) +
		", SignedHeaders=" + signedHdrList +
		", Signature=" + signature
	req.Header.Set("Authorization", auth)
}

// splitEndpoint parses "scheme://host" (path/query ignored) into host + scheme.
// Defaults to https when the scheme is omitted.
func splitEndpoint(endpoint string) (host, scheme string, err error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}
	if u.Host == "" {
		// Tolerate a bare host with no scheme: re-parse as https.
		u, err = url.Parse("https://" + endpoint)
		if err != nil {
			return "", "", err
		}
	}
	scheme = u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return u.Host, scheme, nil
}
