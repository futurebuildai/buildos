# `internal/brain`

Typed HTTP client for [The Brain](../../../../futurebuild-brain) — BuildOS's only path to auth/AI/3p/billing.

## What this package is for

Every BuildOS surface that needs Maestro AI, billing data, Hub-credentialed 3p calls, or A2A orchestration goes through here. Service-layer code looks like:

```go
type DailyFocusService struct {
    brain *brain.Client
    // ... other deps
}

func (s *DailyFocusService) Generate(ctx context.Context, projectID uuid.UUID) (Briefing, error) {
    resp, err := s.brain.Maestro.Chat(ctx, brain.ChatRequest{
        Message: fmt.Sprintf("morning briefing for project %s", projectID),
    })
    if err != nil {
        return Briefing{}, err
    }
    return Briefing{Body: resp.Reply}, nil
}
```

The token comes from `ctx`. The auth middleware stashes the validated Bearer token via `brain.ContextWithToken` right after JWT validation, so service methods never see raw tokens.

## What's wired today

| Sub-client | Brain endpoints | BuildOS callers |
|---|---|---|
| `brain.Client.Maestro` | `POST /api/maestro/chat`, `GET /api/maestro/sessions`, `GET /api/maestro/sessions/{id}` | none yet — Sprint 5 agents are first consumers |
| `brain.Client.Billing` | `GET /api/billing/usage`, `GET /api/billing/usage/daily` | none yet — corporate financials view will surface |

Hub MCP wrapper (D6 ADR) and A2A outbound emitter (Sprint 4 callbacks) will land as additional sub-clients in future PRs.

## Transport semantics

- **Auth**: Bearer JWT pulled from `ctx` via `TokenFromContext`. Returns `ErrUnauthenticated` (no HTTP call) when absent.
- **Retries**: 3 attempts default; exponential backoff (100ms → 200ms → 400ms). Retries on 5xx + transport errors only; 4xx is returned immediately.
- **Cancellation**: Honors `ctx` cancellation between attempts — won't sleep through a cancelled deadline.
- **Envelope unwrap**: Brain returns `{data, error, meta}`. The client unwraps `data` for the caller. Errors map to `*HTTPError` (typed, supports `errors.Is(err, brain.ErrNotFound)` etc.).

## Testing

`client_test.go` exercises the transport against `httptest.Server` — no external deps. The tests cover happy path, 4xx-no-retry, 5xx-retry-then-transient, 5xx-then-200, ctx cancel during backoff, missing token, query encoding, and `HTTPError.Is` matching.

When a service-layer test wants to mock Brain calls, define a small interface at the consumer side and have the test inject a fake — same pattern as `BudgetServicer` / `PipelineServicer` in this repo.

## Cross-repo coordination

Some surfaces (notably the wire-protocol values `iss="fb-brain"` / `aud="fb-os"`) need coordinated changes in both repos. See [ADR-001](../../.agents/handoff/ADR-001-vision-alignment.md) D4 for the cutover plan. Until then, this client validates whatever Brain emits — no special handling needed here.
