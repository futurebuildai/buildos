package brain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// newHubTestClient is a HubClient-aware variant of newTestClient.
// directMode is forwarded into Config so the proxy-vs-direct branch
// can be exercised per test.
func newHubTestClient(t *testing.T, directMode bool, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := NewClient(Config{
		BaseURL:       srv.URL,
		Retry:         RetryConfig{MaxAttempts: 3, BaseDelayMs: 1, Multiplier: 2.0},
		HubDirectMode: directMode,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv.Close
}

func TestHub_GetCredential_ProxyMode_PathAndResponse(t *testing.T) {
	wantID := uuid.New()
	expiresAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	var seenPath, seenQuery string
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		writeEnvelope(w, http.StatusOK, Credential{
			ID:          wantID,
			Provider:    "gable",
			Scope:       "default",
			ExpiresAt:   expiresAt,
			ProxyHandle: "ph_abc123",
			// Secret intentionally absent in proxy-mode response.
		})
	})
	defer cleanup()

	cred, err := c.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{
		Provider: "gable",
	})
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if seenPath != "/api/hub/credentials/gable/default" {
		t.Errorf("path = %q", seenPath)
	}
	if seenQuery != "" {
		t.Errorf("proxy mode should not send ?mode=direct; got %q", seenQuery)
	}
	if cred.ID != wantID {
		t.Errorf("id = %s, want %s", cred.ID, wantID)
	}
	if cred.ProxyHandle != "ph_abc123" {
		t.Errorf("proxy_handle = %q", cred.ProxyHandle)
	}
	if cred.Secret != "" {
		t.Error("Secret must be empty in proxy mode")
	}
}

func TestHub_GetCredential_DirectMode_AppendsQueryAndDecodesSecret(t *testing.T) {
	var seenQuery string
	c, cleanup := newHubTestClient(t, true, func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.RawQuery
		writeEnvelope(w, http.StatusOK, Credential{
			ID:       uuid.New(),
			Provider: "gable",
			Scope:    "default",
			Secret:   "sk_live_supersecret",
		})
	})
	defer cleanup()

	cred, err := c.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{
		Provider: "gable",
	})
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !strings.Contains(seenQuery, "mode=direct") {
		t.Errorf("query = %q, want mode=direct", seenQuery)
	}
	if cred.Secret != "sk_live_supersecret" {
		t.Errorf("Secret should be populated in direct mode; got %q", cred.Secret)
	}
}

func TestHub_GetCredential_DefaultScope(t *testing.T) {
	var seenPath string
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		writeEnvelope(w, http.StatusOK, Credential{ID: uuid.New(), Provider: "twilio", Scope: "default"})
	})
	defer cleanup()

	// Empty scope must default to "default".
	if _, err := c.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{Provider: "twilio"}); err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if seenPath != "/api/hub/credentials/twilio/default" {
		t.Errorf("path = %q, want /api/hub/credentials/twilio/default", seenPath)
	}
}

func TestHub_GetCredential_CustomScope(t *testing.T) {
	var seenPath string
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		writeEnvelope(w, http.StatusOK, Credential{ID: uuid.New(), Provider: "twilio", Scope: "sandbox"})
	})
	defer cleanup()

	if _, err := c.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{
		Provider: "twilio",
		Scope:    "sandbox",
	}); err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if seenPath != "/api/hub/credentials/twilio/sandbox" {
		t.Errorf("path = %q", seenPath)
	}
}

func TestHub_GetCredential_RequiresProvider(t *testing.T) {
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject empty provider")
	})
	defer cleanup()

	_, err := c.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHub_GetCredential_NotFoundMaps(t *testing.T) {
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"CREDENTIAL_NOT_FOUND","message":"no credential for provider"}}`))
	})
	defer cleanup()

	_, err := c.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{Provider: "unknown"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound chain", err)
	}
}

func TestHub_RefreshIfExpired_PathAndOK(t *testing.T) {
	credID := uuid.New()
	calls := int32(0)
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		want := "/api/hub/credentials/" + credID.String() + "/refresh"
		if r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		// Brain returns 204 / empty envelope on success.
		writeEnvelope(w, http.StatusOK, map[string]any{"refreshed": true})
	})
	defer cleanup()

	if err := c.Hub.RefreshIfExpired(ctxWithToken(t), credID); err != nil {
		t.Fatalf("RefreshIfExpired: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestHub_RefreshIfExpired_RequiresID(t *testing.T) {
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject zero UUID")
	})
	defer cleanup()

	if err := c.Hub.RefreshIfExpired(ctxWithToken(t), uuid.Nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestHub_RefreshIfExpired_TransientPropagates(t *testing.T) {
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	defer cleanup()

	err := c.Hub.RefreshIfExpired(ctxWithToken(t), uuid.New())
	if !errors.Is(err, ErrTransient) {
		t.Errorf("err = %v, want ErrTransient chain", err)
	}
}

func TestHub_NoTokenInContext(t *testing.T) {
	c, cleanup := newHubTestClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called without token")
	})
	defer cleanup()

	_, err := c.Hub.GetCredential(context.Background(), GetCredentialRequest{Provider: "gable"})
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
}

func TestHub_DirectModeIsForkStatic(t *testing.T) {
	// Two separate clients with different HubDirectMode values
	// must each route to their configured mode regardless of
	// shared backing types.
	srvProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "mode=direct") {
			t.Error("proxy-mode client must not send mode=direct")
		}
		writeEnvelope(w, http.StatusOK, Credential{ID: uuid.New(), Provider: "gable", Scope: "default"})
	}))
	defer srvProxy.Close()
	srvDirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "mode=direct") {
			t.Error("direct-mode client must send mode=direct")
		}
		writeEnvelope(w, http.StatusOK, Credential{ID: uuid.New(), Provider: "gable", Scope: "default"})
	}))
	defer srvDirect.Close()

	cProxy, _ := NewClient(Config{BaseURL: srvProxy.URL, HubDirectMode: false})
	cDirect, _ := NewClient(Config{BaseURL: srvDirect.URL, HubDirectMode: true})

	if _, err := cProxy.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{Provider: "gable"}); err != nil {
		t.Fatalf("proxy GetCredential: %v", err)
	}
	if _, err := cDirect.Hub.GetCredential(ctxWithToken(t), GetCredentialRequest{Provider: "gable"}); err != nil {
		t.Fatalf("direct GetCredential: %v", err)
	}
}
