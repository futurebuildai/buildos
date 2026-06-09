package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeSecret struct {
	key      string
	gotOrg   uuid.UUID
	gotConn  string
	resolved bool
}

func (f *fakeSecret) ResolveConnectorSecret(_ context.Context, orgID uuid.UUID, name string) (string, error) {
	f.gotOrg, f.gotConn, f.resolved = orgID, name, true
	return f.key, nil
}

func newMCPConnectorForTest(t *testing.T, h http.HandlerFunc, secret SecretResolver, breaker *Breaker) Connector {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	if breaker == nil {
		breaker = newBreaker(BreakerConfig{})
	}
	return NewMCPConnector(MCPConnectorParams{
		Name:        "acme",
		Endpoint:    srv.URL,
		CachedTools: []ToolDef{{Name: "search", Description: "search", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		Secret:      secret,
		HTTP:        srv.Client(), // SSRF guard proven separately; protocol exercise here
		Breaker:     breaker,
	})
}

func TestMCPConnector_BuildTools_AdminFlooredNamespacedLater(t *testing.T) {
	c := newMCPConnectorForTest(t, mcpStub("json", nil, callToolResult{}, nil), nil, nil)
	tools, err := c.BuildTools(context.Background(), Caller{OrgID: uuid.New(), Role: "admin"})
	if err != nil {
		t.Fatalf("BuildTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	if tools[0].Spec.Name != "search" { // un-namespaced here; the service namespaces it
		t.Errorf("tool name = %q, want the raw remote name", tools[0].Spec.Name)
	}
	if tools[0].MinRole != "admin" {
		t.Errorf("MinRole = %q, want admin", tools[0].MinRole)
	}
	if tools[0].Executor == nil {
		t.Error("executor must be non-nil")
	}
}

func TestMCPConnector_Executor_HappyPath(t *testing.T) {
	res := callToolResult{Content: []mcpContent{{Type: "text", Text: "result text"}}}
	c := newMCPConnectorForTest(t, mcpStub("json", nil, res, nil), nil, nil)
	tools, _ := c.BuildTools(context.Background(), Caller{OrgID: uuid.New(), Role: "admin"})

	out, err := tools[0].Executor.Execute(context.Background(), json.RawMessage(`{"q":"x"}`))
	if err != nil {
		t.Fatalf("executor must never return a Go error: %v", err)
	}
	if out.IsError || !strings.Contains(out.Content, "result text") {
		t.Errorf("out = %+v, want the result text, not an error", out)
	}
}

func TestMCPConnector_Executor_ResolvesSecret(t *testing.T) {
	var gotAuth string
	h := func(w http.ResponseWriter, r *http.Request) {
		if a := r.Header.Get("Authorization"); a != "" {
			gotAuth = a
		}
		mcpStub("json", nil, callToolResult{Content: []mcpContent{{Type: "text", Text: "ok"}}}, nil)(w, r)
	}
	secret := &fakeSecret{key: "s3cr3t"}
	c := newMCPConnectorForTest(t, h, secret, nil)
	tools, _ := c.BuildTools(context.Background(), Caller{OrgID: uuid.New(), Role: "admin"})

	if _, err := tools[0].Executor.Execute(context.Background(), nil); err != nil {
		t.Fatalf("executor: %v", err)
	}
	if !secret.resolved {
		t.Error("the executor must resolve the per-org secret")
	}
	if gotAuth != "Bearer s3cr3t" {
		t.Errorf("server saw Authorization %q, want Bearer s3cr3t", gotAuth)
	}
}

func TestMCPConnector_Executor_ServerError_SoftFailsAndTripsBreaker(t *testing.T) {
	h := mcpStub("json", nil, callToolResult{}, map[string]func(http.ResponseWriter){
		"tools/call": func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
	})
	breaker := newBreaker(BreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute})
	c := newMCPConnectorForTest(t, h, nil, breaker)
	tools, _ := c.BuildTools(context.Background(), Caller{OrgID: uuid.New(), Role: "admin"})

	out, err := tools[0].Executor.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("a server error must be a soft IsError, not a Go error: %v", err)
	}
	if !out.IsError {
		t.Error("a 500 from the server must surface as IsError")
	}
	// One failure tripped the (threshold-1) breaker → next call short-circuits.
	out2, _ := tools[0].Executor.Execute(context.Background(), nil)
	if !out2.IsError || !strings.Contains(out2.Content, "circuit open") {
		t.Errorf("breaker should be open: out = %+v", out2)
	}
}
