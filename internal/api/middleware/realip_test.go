package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return n
}

func TestRealIP(t *testing.T) {
	lb := []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}

	tests := []struct {
		name     string
		trusted  []*net.IPNet
		peer     string // r.RemoteAddr (TCP peer)
		xff      string
		xRealIP  string
		wantHost string // clientIP() after the middleware
	}{
		{
			name:     "no trusted proxies: spoofed XFF is ignored, real peer wins",
			trusted:  nil,
			peer:     "203.0.113.9:55000",
			xff:      "1.2.3.4", // attacker-forged
			wantHost: "203.0.113.9",
		},
		{
			name:     "untrusted peer: spoofed XFF ignored even with an allowlist",
			trusted:  lb,
			peer:     "203.0.113.9:55000", // not in 10/8
			xff:      "1.2.3.4",
			wantHost: "203.0.113.9",
		},
		{
			name:     "trusted proxy: the forwarded client IP is honored",
			trusted:  lb,
			peer:     "10.0.0.5:443", // the LB
			xff:      "198.51.100.7",
			wantHost: "198.51.100.7",
		},
		{
			name:     "trusted proxy, multi-hop XFF: walk right-to-left past trusted hops",
			trusted:  lb,
			peer:     "10.0.0.5:443",
			xff:      "198.51.100.7, 10.0.0.9", // client, then an internal hop
			wantHost: "198.51.100.7",
		},
		{
			name:     "trusted proxy, X-Real-IP fallback when no XFF",
			trusted:  lb,
			peer:     "10.0.0.5:443",
			xRealIP:  "198.51.100.42",
			wantHost: "198.51.100.42",
		},
		{
			// XFF present but all-trusted → use the peer, do NOT honor a
			// (potentially forged) X-Real-IP. Regression for the spoof gap.
			name:     "trusted proxy, all-trusted XFF: ignore X-Real-IP, use peer",
			trusted:  lb,
			peer:     "10.0.0.5:443",
			xff:      "10.0.0.9", // an internal hop, trusted
			xRealIP:  "1.2.3.4",  // forged
			wantHost: "10.0.0.5",
		},
		{
			name:     "IPv6 trusted proxy: forwarded IPv6 client is honored",
			trusted:  []*net.IPNet{mustCIDR(t, "2001:db8::/32")},
			peer:     "[2001:db8::5]:443",
			xff:      "2001:db9::99", // a different /32 → the real client
			wantHost: "2001:db9::99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			h := RealIP(tt.trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = clientIP(r)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.peer
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != tt.wantHost {
				t.Errorf("clientIP = %q, want %q", got, tt.wantHost)
			}
		})
	}
}
