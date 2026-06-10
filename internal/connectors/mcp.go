package connectors

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/futurebuildai/buildos/internal/agentic"
)

// MCPConnectorParams hydrates an mcp connector instance for one (org, instance).
// The SERVICE supplies the cached tool list (from its connector_tools store), the
// endpoint + name (from connectors_config), the vault SecretResolver, the
// SSRF-guarded HTTP client, and the per-(org,endpoint) breaker. The connectors
// package never touches the DB.
type MCPConnectorParams struct {
	Name           string
	Description    string
	Endpoint       string
	CachedTools    []ToolDef
	Secret         SecretResolver
	HTTP           *http.Client // MUST be an SSRF-guarded NewEgressClient
	Breaker        *Breaker
	PerCall        time.Duration
	MaxResultBytes int
	ClientVersion  string
	Logger         *slog.Logger // optional; detailed connector errors log here, NOT into the model
}

// mcpConnector implements Connector for an MCP server instance. BuildTools wraps
// each CACHED tool (NOT a live tools/list — refresh is operator-driven) into an
// agentic.Tool whose executor calls the remote tools/call at invocation time,
// gated by the circuit breaker, bounded by the per-call timeout + result cap, and
// authenticated by the per-org vault secret resolved lazily. EVERY failure is a
// soft IsError result; the executor never returns a Go error or panics.
type mcpConnector struct {
	p MCPConnectorParams
}

// NewMCPConnector constructs an mcp connector instance.
func NewMCPConnector(p MCPConnectorParams) Connector {
	if p.PerCall <= 0 {
		p.PerCall = defaultEgressTimeout
	}
	if p.MaxResultBytes <= 0 {
		p.MaxResultBytes = defaultMaxResultBytes
	}
	return &mcpConnector{p: p}
}

func (c *mcpConnector) Name() string { return c.p.Name }

func (c *mcpConnector) Description() string {
	if c.p.Description != "" {
		return c.p.Description
	}
	return "MCP server connector (" + c.p.Endpoint + ")"
}

// BuildTools turns the cached tool defs into agentic tools. The remote tool name
// is sealed into the executor closure (the registry namespaces the model-facing
// name; the executor uses the original remote name for tools/call).
func (c *mcpConnector) BuildTools(_ context.Context, caller Caller) ([]agentic.Tool, error) {
	out := make([]agentic.Tool, 0, len(c.p.CachedTools))
	for _, td := range c.p.CachedTools {
		remoteName := td.Name
		schema := td.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, agentic.Tool{
			Spec: agentic.ToolSpec{
				Name:        td.Name, // the SERVICE namespaces this (conn__<instance>__<name>)
				Description: td.Description,
				InputSchema: schema,
			},
			// Natural MinRole; ConnectorService floors connector tools to admin.
			MinRole:  "admin",
			Executor: c.executor(caller, remoteName),
		})
	}
	return out, nil
}

// executor returns the per-tool closure. It seals caller.OrgID (for the per-org
// secret) and the remote tool name. It NEVER returns a Go error — any failure is
// a soft IsError result so the chat loop self-corrects in prose.
func (c *mcpConnector) executor(caller Caller, remoteName string) agentic.ToolExecutor {
	return executorFunc(func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
		// Circuit breaker: short-circuit a known-bad endpoint.
		ok, gen := c.p.Breaker.Allow()
		if !ok {
			return softError("connector temporarily unavailable (circuit open); try again shortly"), nil
		}

		// Resolve the per-org credential lazily (best-effort; "" => unauthenticated).
		bearer := ""
		if c.p.Secret != nil {
			if s, err := c.p.Secret.ResolveConnectorSecret(ctx, caller.OrgID, c.p.Name); err == nil {
				bearer = s
			}
		}

		client := NewMCPClient(MCPClientParams{
			HTTP:           c.p.HTTP,
			Endpoint:       c.p.Endpoint,
			Bearer:         bearer,
			PerCall:        c.p.PerCall,
			MaxResultBytes: c.p.MaxResultBytes,
			ClientVersion:  c.p.ClientVersion,
		})
		if err := client.Initialize(ctx); err != nil {
			c.p.Breaker.RecordFailure(gen)
			// NEVER surface the raw transport error to the model — it embeds the
			// resolved IP / DNS resolver / TLS cert host / "blocked private range"
			// confirmation, which would turn the egress guard into a blind-SSRF
			// oracle. Log the detail server-side; give the model a generic message.
			c.logError(ctx, "connect", err)
			return softError("the connector is unavailable right now"), nil
		}
		content, isErr, err := client.CallTool(ctx, remoteName, input)
		if err != nil {
			c.p.Breaker.RecordFailure(gen)
			c.logError(ctx, "call", err)
			return softError("the connector call failed"), nil
		}
		// A reachable server (even a tool-level isError) is a healthy endpoint.
		c.p.Breaker.RecordSuccess(gen)
		return agentic.ToolResult{Content: content, IsError: isErr}, nil
	})
}

// logError records the connector failure server-side (never to the model). The
// raw error embeds resolved IPs / TLS hosts; the field-name slog scrubber can't
// mask values inside an error string, so the WARN line (always emitted) omits
// the raw error and only the DEBUG line (off in prod) carries the detail.
func (c *mcpConnector) logError(ctx context.Context, op string, err error) {
	if c.p.Logger == nil {
		return
	}
	c.p.Logger.WarnContext(ctx, "mcp connector "+op+" failed",
		slog.String("connector", c.p.Name))
	c.p.Logger.DebugContext(ctx, "mcp connector "+op+" error detail",
		slog.String("connector", c.p.Name), slog.Any("error", err))
}
