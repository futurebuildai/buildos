package connectors

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},             // loopback
		{"::1", true},                   // loopback v6
		{"10.0.0.5", true},              // RFC1918
		{"172.16.4.4", true},            // RFC1918
		{"192.168.1.1", true},           // RFC1918
		{"169.254.169.254", true},       // cloud metadata (link-local)
		{"fe80::1", true},               // link-local v6
		{"fc00::1", true},               // ULA
		{"fd12:3456::1", true},          // ULA
		{"100.64.0.1", true},            // CGNAT
		{"100.127.255.255", true},       // CGNAT upper
		{"0.0.0.0", true},               // unspecified
		{"224.0.0.1", true},             // multicast
		{"::ffff:127.0.0.1", true},      // v4-mapped loopback
		{"::ffff:10.0.0.1", true},       // v4-mapped private
		{"8.8.8.8", false},              // public
		{"1.1.1.1", false},              // public
		{"100.63.255.255", false},       // just below CGNAT
		{"100.128.0.0", false},          // just above CGNAT
		{"2001:4860:4860::8888", false}, // public v6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("test IP %q did not parse", c.ip)
		}
		if got := isBlockedIP(ip); got != c.blocked {
			t.Errorf("isBlockedIP(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
	// A nil IP must fail closed.
	if !isBlockedIP(nil) {
		t.Error("isBlockedIP(nil) must be true (fail closed)")
	}
}

func TestEgressControl(t *testing.T) {
	cases := []struct {
		network, address string
		wantErr          error
	}{
		{"tcp4", "169.254.169.254:80", errBlockedAddress}, // metadata
		{"tcp4", "10.0.0.1:443", errBlockedAddress},       // private
		{"tcp4", "127.0.0.1:8080", errBlockedAddress},     // loopback
		{"udp", "8.8.8.8:53", errBlockedNetwork},          // non-tcp
		{"tcp4", "8.8.8.8:443", nil},                      // public, allowed
		{"tcp6", "[2001:4860:4860::8888]:443", nil},       // public v6
		{"tcp4", "not-an-ip:443", errBlockedAddress},      // unresolved host (should never reach Control, fail closed)
	}
	for _, c := range cases {
		err := egressControl(c.network, c.address, nil)
		if !errors.Is(err, c.wantErr) {
			t.Errorf("egressControl(%s, %s) = %v, want %v", c.network, c.address, err, c.wantErr)
		}
	}
}

// The egress client must REFUSE to connect to a loopback-bound server — the most
// direct end-to-end proof that the dial guard fires (httptest binds 127.0.0.1).
func TestEgressClient_RefusesLoopbackServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewEgressClient(0)
	resp, err := client.Get(srv.URL)
	if err == nil {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatal("egress client must refuse a loopback server, but the request succeeded")
	}
	if !errors.Is(err, errBlockedAddress) {
		t.Errorf("error = %v, want it to wrap errBlockedAddress", err)
	}
}

// Redirects must be refused so a 30x to a private host can't bypass the guard.
func TestEgressClient_RefusesRedirect(t *testing.T) {
	client := NewEgressClient(0)
	req, _ := http.NewRequest("GET", "https://example.com/", nil)
	if err := client.CheckRedirect(req, nil); !errors.Is(err, errNoRedirect) {
		t.Errorf("CheckRedirect = %v, want errNoRedirect", err)
	}
}
