package connectors

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Egress security (Phase 3b-ii). An MCP connector connects to an
// operator-configured, otherwise-untrusted URL — the repo's first arbitrary-URL
// outbound path. These guards are the ONLY thing standing between a curious or
// compromised admin (or a prompt-injected model supplying tools/call arguments)
// and the deployment's own metadata endpoint / private network.

var (
	// errBlockedAddress is returned by the dial Control hook when the resolved
	// IP is in a denied range. It fires at CONNECT time on the ACTUAL dialed IP,
	// so it closes DNS-rebinding/TOCTOU (a host that resolves public at check
	// time but private at dial time is rejected on the dial).
	errBlockedAddress = errors.New("connectors: blocked egress address (private/loopback/metadata range)")
	// errBlockedNetwork rejects non-tcp dials.
	errBlockedNetwork = errors.New("connectors: blocked egress network")
	// errNoRedirect refuses HTTP redirects — a 30x to a private host would
	// otherwise bypass the per-request guard.
	errNoRedirect = errors.New("connectors: redirects are not allowed for egress")
)

// defaultEgressTimeout bounds a single connector HTTP call. It MUST stay well
// under the 30s chat tool-loop budget (the loop deadline does not interrupt a
// tool mid-call), so a slow connector self-limits before it can stall the chat.
const defaultEgressTimeout = 8 * time.Second

// isBlockedIP reports whether an IP must NOT be dialed for egress. It is the
// single source of egress truth and is exhaustively unit-tested. Blocks:
//   - loopback (127/8, ::1)
//   - RFC1918 private + ULA fc00::/7 (net.IP.IsPrivate)
//   - link-local unicast 169.254/16 + fe80::/10 — the cloud-metadata range
//     (169.254.169.254) lives here (net.IP.IsLinkLocalUnicast)
//   - link-local + general multicast, and the unspecified address
//   - CGNAT 100.64.0.0/10 (NOT covered by IsPrivate; checked explicitly)
//
// A nil/unparseable IP is treated as blocked (fail closed). IPv4-mapped IPv6
// forms (e.g. ::ffff:127.0.0.1) are normalized by the net.IP methods + To4.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64.0.0/10 (shared address space, RFC 6598).
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}

// IsBlockedIP is the exported guard for a best-effort, write-time check (e.g. an
// admin configuring an endpoint whose host is a literal private IP). The
// authoritative guard remains the dial Control hook at connect time.
func IsBlockedIP(ip net.IP) bool { return isBlockedIP(ip) }

// egressControl is the net.Dialer.Control hook: it runs AFTER DNS resolution,
// at connect time, with the actual ip:port about to be dialed. Rejecting a
// blocked IP here is what defeats DNS-rebinding (re-resolution between a
// config-time check and the dial cannot smuggle in a private IP).
func egressControl(network, address string, _ syscall.RawConn) error {
	if network != "tcp4" && network != "tcp6" {
		return errBlockedNetwork
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errBlockedAddress
	}
	ip := net.ParseIP(host)
	if ip == nil || isBlockedIP(ip) {
		return errBlockedAddress
	}
	return nil
}

// NewEgressClient returns an *http.Client safe for connecting to an
// operator-configured, untrusted URL: resolve-and-pin private-IP denylist (via
// the dial Control hook), redirects refused, bounded dial/TLS/response timeouts,
// otelhttp-wrapped transport, TLS verification on. The caller still rejects
// non-https schemes at config + request time (this client does not see the URL
// until the request). perCallTimeout <= 0 takes the default.
func NewEgressClient(perCallTimeout time.Duration) *http.Client {
	if perCallTimeout <= 0 {
		perCallTimeout = defaultEgressTimeout
	}
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   egressControl,
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext, // Control runs on the raw TCP dial, before TLS
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: perCallTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Transport: otelhttp.NewTransport(transport,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		),
		Timeout: perCallTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errNoRedirect
		},
	}
}
