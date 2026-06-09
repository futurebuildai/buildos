/**
 * Admin config endpoints — /api/v1/admin/* (Phase 3c).
 *
 * Two surfaces: the agentic-harness capability config (`/admin/agents`,
 * API_CONTRACT §13b) and the connector registry (`/admin/connectors`,
 * §13c). Both are RequireMinRole=admin + SetupGate, and — per ESC-002 —
 * NOT plan-gated, so the kill-switches reach admins regardless of tier.
 *
 * `config` is an EMBEDDED JSON OBJECT on the wire (json.RawMessage). It is
 * always sent as an object, never JSON.stringify'd (the server 400s
 * "config not an object"). On a FULL-DOCUMENT PUT, an omitted/null `config`
 * RESETS tuning to the catalog default — so callers must pass the last
 * server-confirmed snapshot, never live form inputs. Path segments are
 * encodeURIComponent'd (mirror integrations.ts).
 */
import { api } from '../client.js';
import type {
  AgentConfig,
  ConnectorConfig,
  EffectiveAgentConfig,
  EffectiveConnector,
} from '../../types/models.js';

// ------------------------------- Agents -------------------------------

export function listAgents(): Promise<EffectiveAgentConfig[]> {
  return api.get<{ agents: EffectiveAgentConfig[] }>('/api/v1/admin/agents').then((r) => r.agents);
}

export interface SetAgentInput {
  enabled: boolean;
  /** Embedded object (never stringified). Omitted ⇒ server RESETS tuning to default. */
  config?: Record<string, unknown>;
}
export function setAgent(capability: string, input: SetAgentInput): Promise<AgentConfig> {
  return api
    .put<{
      agent: AgentConfig;
    }>(`/api/v1/admin/agents/${encodeURIComponent(capability)}`, input)
    .then((r) => r.agent);
}

/** Idempotent reset to the catalog default (204; 404 on an unknown capability). */
export function resetAgent(capability: string): Promise<void> {
  return api.delete<void>(`/api/v1/admin/agents/${encodeURIComponent(capability)}`);
}

// ----------------------------- Connectors -----------------------------

export function listConnectors(): Promise<EffectiveConnector[]> {
  return api
    .get<{ connectors: EffectiveConnector[] }>('/api/v1/admin/connectors')
    .then((r) => r.connectors);
}

export interface SetConnectorInput {
  enabled: boolean;
  /** 'mcp' for an instance; omitted/ignored for the built-in `reference`. */
  kind?: 'mcp';
  /** Embedded object (never stringified). MCP carries `{ endpoint: 'https://…' }`. */
  config?: Record<string, unknown>;
}
export function setConnector(
  connector: string,
  input: SetConnectorInput,
): Promise<ConnectorConfig> {
  return api
    .put<{
      connector: ConnectorConfig;
    }>(`/api/v1/admin/connectors/${encodeURIComponent(connector)}`, input)
    .then((r) => r.connector);
}

/** MCP only — runs tools/list upstream. 404 unknown; 400 not-an-mcp; 502 UPSTREAM_ERROR. */
export function refreshConnector(
  connector: string,
): Promise<{ connector: string; tools_count: number }> {
  return api.post<{ connector: string; tools_count: number }>(
    `/api/v1/admin/connectors/${encodeURIComponent(connector)}/refresh`,
  );
}

/** Idempotent reset to default-OFF + clears cached tools (204; 404 if neither builtin nor instance). */
export function deleteConnector(connector: string): Promise<void> {
  return api.delete<void>(`/api/v1/admin/connectors/${encodeURIComponent(connector)}`);
}
