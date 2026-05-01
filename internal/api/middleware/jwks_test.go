package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// stubJWKSEndpoint returns an httptest server that serves a JWKS with
// `nKeys` keys.
func stubJWKSEndpoint(t *testing.T, nKeys int) *httptest.Server {
	t.Helper()
	keys := make([]jose.JSONWebKey, 0, nKeys)
	for i := 0; i < nKeys; i++ {
		priv, err := rsa.GenerateKey(rand.Reader, 1024) // small for test speed
		if err != nil {
			t.Fatalf("rsa.GenerateKey: %v", err)
		}
		keys = append(keys, jose.JSONWebKey{Key: &priv.PublicKey, KeyID: "k", Use: "sig", Algorithm: "RS256"})
	}
	body, err := json.Marshal(jose.JSONWebKeySet{Keys: keys})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestJWKSProvider_CacheStatus_BeforeFetch(t *testing.T) {
	p := NewJWKSProvider("http://nowhere", quietLog())
	count, age := p.CacheStatus()
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if age != 0 {
		t.Errorf("age = %s, want 0", age)
	}
}

func TestJWKSProvider_CacheStatus_AfterFetch(t *testing.T) {
	srv := stubJWKSEndpoint(t, 2)
	defer srv.Close()

	p := NewJWKSProvider(srv.URL, quietLog())
	if _, err := p.GetKeySet(context.Background()); err != nil {
		t.Fatalf("GetKeySet: %v", err)
	}

	count, age := p.CacheStatus()
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if age <= 0 {
		t.Errorf("age = %s, want >0", age)
	}
	if age > 5*time.Second {
		t.Errorf("age = %s, suspiciously old for a just-fetched cache", age)
	}
}

func TestJWKSProvider_CacheTTL_Default5Min(t *testing.T) {
	p := NewJWKSProvider("http://nowhere", quietLog())
	if got := p.CacheTTL(); got != 5*time.Minute {
		t.Errorf("CacheTTL = %s, want 5m (constructor default)", got)
	}
}
