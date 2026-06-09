import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Endpoint modules are mocked so the page never hits the network. The connectors
// page owns TWO endpoint modules: the admin connector registry (primary) and the
// BYOK vault (secondary, partial-load tolerant).
vi.mock('../src/api/endpoints/admin.js', () => ({
  listConnectors: vi.fn(),
  setConnector: vi.fn(),
  refreshConnector: vi.fn(),
  deleteConnector: vi.fn(),
}));
vi.mock('../src/api/endpoints/integrations.js', () => ({
  listIntegrations: vi.fn(),
  setCredential: vi.fn(),
  deleteCredential: vi.fn(),
}));

import '../src/components/pages/fb-connectors-page.js';
import '../src/components/molecules/fb-connector-card.js';

import * as adminApi from '../src/api/endpoints/admin.js';
import * as integrationsApi from '../src/api/endpoints/integrations.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import type { EffectiveConnector, IntegrationCredential } from '../src/types/models.js';
import type { FbConnectorCard } from '../src/components/molecules/fb-connector-card.js';

// NOTE ON TEST SHAPE: happy-dom mis-parses a custom element nested inside a
// conditionally-rendered wrapper `<div>` (it emits the literal text `<?>`), so the
// page's MCP card grid does NOT materialize as queryable `fb-connector-card`
// elements under this environment (it renders fine in a real browser — the live
// Playwright a11y sweep covers the rendered grid). We therefore drive the page's
// intent handlers + derived getters DIRECTLY (the established codebase pattern —
// cf. portfolio-pages.test.ts `submitAllocate`/`onConfirmDelete`), asserting on
// the resulting endpoint calls + page state. Card-level rendering concerns (the
// kind branch, the tools-status badge, disabled-MCP affordances, the credential
// `.submit()` wipe) are asserted on a STANDALONE-mounted `fb-connector-card`,
// which renders correctly under happy-dom.

interface PageInternals {
  connectors: EffectiveConnector[];
  integrations: IntegrationCredential[];
  integrationsUnknown: boolean;
  modalOpen: boolean;
  notice: { kind: 'ok' | 'err'; text: string } | null;
  cardErrors: Map<string, string>;
  mcps: EffectiveConnector[];
  builtins: EffectiveConnector[];
  credStateFor(name: string): string;
  credLast4For(name: string): string | undefined;
  onToggle(c: EffectiveConnector, enabled: boolean): Promise<void>;
  onRefresh(name: string): Promise<void>;
  onSaveCredential(card: FbConnectorCard, name: string, value: string): Promise<void>;
  onRequestDelete(name: string): void;
  onConfirmDelete(): Promise<void>;
  onModalSubmit(e: Event): Promise<void>;
  openAdd(): void;
}

function internals(el: HTMLElement): PageInternals {
  return el as unknown as PageInternals;
}

async function mountPage(): Promise<HTMLElement> {
  const el = document.createElement('fb-connectors-page');
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  // Two ticks: the connectedCallback load() is async (admin + integrations).
  await new Promise((r) => setTimeout(r, 0));
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

/** Settle a chained reload + follow-up async (e.g. enable -> auto-refresh). */
async function settle(el: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  await new Promise((r) => setTimeout(r, 0));
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

/** Mount a standalone connector card (renders fine under happy-dom). */
async function mountCard(
  connector: EffectiveConnector,
  props: Partial<FbConnectorCard> = {},
): Promise<FbConnectorCard> {
  const card = document.createElement('fb-connector-card') as FbConnectorCard;
  card.connector = connector;
  Object.assign(card, props);
  document.body.appendChild(card);
  await (card as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return card;
}

function apiError(
  code: string,
  status = 400,
  details: { field: string; reason: string }[] = [],
): ApiError {
  return new ApiError({ code, message: code, status, details });
}

// --------------------------------- Fixtures ---------------------------------

const BUILTIN: EffectiveConnector = {
  connector: 'reference',
  kind: 'builtin',
  description: 'A read-only glossary and currency helper that runs inside BuildOS.',
  enabled: false,
  config: {},
  tools_count: 0,
  source: 'default',
};

const BUILTIN_ON: EffectiveConnector = { ...BUILTIN, enabled: true, source: 'override' };

/** An MCP instance, enabled, never refreshed (no tools_fetched_at). */
const MCP_NEVER: EffectiveConnector = {
  connector: 'estimator',
  kind: 'mcp',
  description: '',
  enabled: true,
  config: { endpoint: 'https://tools.example.com' },
  endpoint: 'https://tools.example.com',
  tools_count: 0,
  source: 'override',
};

/** An MCP instance, DISABLED — all affordances must still render. */
const MCP_DISABLED: EffectiveConnector = { ...MCP_NEVER, enabled: false };

/** An MCP instance refreshed with a genuine zero tools (working empty server). */
const MCP_ZERO: EffectiveConnector = {
  ...MCP_NEVER,
  enabled: true,
  tools_count: 0,
  tools_fetched_at: '2026-06-09T10:00:00Z',
};

function credRow(name: string, active: boolean, last4 = 'cd34'): IntegrationCredential {
  return {
    id: `cred-${name}`,
    provider: `connector:${name}`,
    label: `${name} access token`,
    last4,
    is_active: active,
    created_by: 'u-1',
    created_at: '2026-01-01',
    updated_at: '2026-01-01',
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  // Default: only the built-in connector, no MCPs, no vault credentials.
  vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN]);
  vi.mocked(adminApi.setConnector).mockResolvedValue({
    id: 'c-1',
    org_id: 'org-1',
    connector_name: 'reference',
    kind: 'builtin',
    enabled: true,
    config: {},
    updated_by: 'u-1',
    created_at: '2026-01-01',
    updated_at: '2026-01-01',
  });
  vi.mocked(adminApi.refreshConnector).mockResolvedValue({
    connector: 'estimator',
    tools_count: 3,
  });
  vi.mocked(adminApi.deleteConnector).mockResolvedValue(undefined);
  vi.mocked(integrationsApi.listIntegrations).mockResolvedValue([]);
  vi.mocked(integrationsApi.setCredential).mockResolvedValue(credRow('estimator', true));
  vi.mocked(integrationsApi.deleteCredential).mockResolvedValue(undefined);
});

afterEach(() => {
  document.body.innerHTML = '';
  window.history.replaceState({}, '', '/');
});

// ------------------ Card-level rendering (standalone fb-connector-card) ------------------

describe('fb-connector-card — kind branch + affordances', () => {
  it('renders the built-in via the kind branch (enable-only chrome, no refresh)', async () => {
    const card = await mountCard(BUILTIN);
    expect(card.connector.kind).toBe('builtin');
    // Built-in: a switch + source badge, but NO endpoint / refresh affordances.
    expect(card.shadowRoot!.querySelector('fb-switch')).not.toBeNull();
    expect(card.shadowRoot!.querySelector('[icon="refresh"]')).toBeNull();
    expect(card.shadowRoot!.querySelector('.endpoint')).toBeNull();
  });

  it('a DISABLED MCP still renders endpoint/refresh/cred/delete (never stranded)', async () => {
    const card = await mountCard(MCP_DISABLED, { credState: 'none' });
    expect(card.connector.enabled).toBe(false);
    expect(card.shadowRoot!.querySelector('.endpoint')).not.toBeNull();
    expect(card.shadowRoot!.querySelector('[label^="Refresh tools"]')).not.toBeNull();
    expect(card.shadowRoot!.querySelector('[label^="Delete connector"]')).not.toBeNull();
    expect(card.shadowRoot!.querySelector('fb-secret-input')).not.toBeNull();
  });

  it('shows last4 only for an ACTIVE credential (cred-state=set)', async () => {
    const setCard = await mountCard(MCP_NEVER, { credState: 'set', credLast4: 'ab12' });
    const secret = setCard.shadowRoot!.querySelector('fb-secret-input');
    expect(secret?.hasAttribute('has-value')).toBe(true);
    expect(secret?.getAttribute('last4')).toBe('ab12');
  });

  it('renders the masked write-only state without "set" when cred-state=none', async () => {
    const noneCard = await mountCard(MCP_NEVER, { credState: 'none' });
    const secret = noneCard.shadowRoot!.querySelector('fb-secret-input');
    expect(secret?.hasAttribute('has-value')).toBe(false);
  });
});

describe('fb-connector-card — tools-status badge', () => {
  it('enabled + never-refreshed nags "No tools loaded" (warning)', async () => {
    const card = await mountCard(MCP_NEVER);
    const warning = [...card.shadowRoot!.querySelectorAll('fb-badge')].find(
      (b) => b.getAttribute('status') === 'warning',
    );
    expect(warning?.textContent?.trim()).toBe('No tools loaded');
  });

  it('refreshed with a genuine ZERO tools does NOT warn (working empty server)', async () => {
    const card = await mountCard(MCP_ZERO);
    const warning = [...card.shadowRoot!.querySelectorAll('fb-badge')].find(
      (b) => b.getAttribute('status') === 'warning',
    );
    expect(warning).toBeUndefined();
    const refreshed = [...card.shadowRoot!.querySelectorAll('fb-badge')].find((b) =>
      b.textContent?.includes('refreshed'),
    );
    expect(refreshed?.textContent).toContain('0 tools');
  });

  it('surfaces a persistent refresh-error region (alert) when the page sets refresh-error', async () => {
    const card = await mountCard(MCP_NEVER, {
      refreshError: 'Last refresh failed — could not reach https://tools.example.com.',
    });
    const errEl = card.shadowRoot!.querySelector('.card-error');
    expect(errEl).not.toBeNull();
    expect(errEl!.getAttribute('role')).toBe('alert');
    expect(errEl!.textContent).toContain('Last refresh failed');
  });
});

// ------------------------------- Built-in toggle -------------------------------

describe('fb-connectors-page — built-in toggle', () => {
  it('toggling a default-OFF built-in ON calls setConnector{enabled:true}', async () => {
    const el = await mountPage();
    await internals(el).onToggle(BUILTIN, true);
    await settle(el);
    expect(adminApi.setConnector).toHaveBeenCalledWith('reference', { enabled: true });
    expect(adminApi.deleteConnector).not.toHaveBeenCalled();
  });

  it('toggling the built-in OFF DELETEs the override row (not PUT{enabled:false})', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN_ON]);
    const el = await mountPage();
    await internals(el).onToggle(BUILTIN_ON, false);
    await settle(el);
    expect(adminApi.deleteConnector).toHaveBeenCalledWith('reference');
    expect(adminApi.setConnector).not.toHaveBeenCalled();
  });

  it('re-derives the switch from server truth after any toggle (load() in finally)', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN_ON]);
    const el = await mountPage();
    vi.clearAllMocks();
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN_ON]);
    vi.mocked(adminApi.deleteConnector).mockRejectedValueOnce(
      apiError(ErrorCode.INTERNAL_ERROR, 500),
    );
    // Even a REJECTED toggle must reload so the switch resyncs from the server.
    await internals(el).onToggle(BUILTIN_ON, false);
    await settle(el);
    expect(adminApi.listConnectors).toHaveBeenCalled();
  });
});

// ------------------------- Credential presence derivation -------------------------

describe('fb-connectors-page — credential presence', () => {
  it('derives presence from the ACTIVE-only integrations row', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    vi.mocked(integrationsApi.listIntegrations).mockResolvedValue([credRow('estimator', true)]);
    const el = await mountPage();
    expect(internals(el).credStateFor('estimator')).toBe('set');
    expect(internals(el).credLast4For('estimator')).toBe('cd34');
  });

  it('an INACTIVE bearer row reads as "none" (orthogonal to enabled)', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    // last4 present but is_active:false — a rotated/deactivated bearer.
    vi.mocked(integrationsApi.listIntegrations).mockResolvedValue([credRow('estimator', false)]);
    const el = await mountPage();
    expect(internals(el).credStateFor('estimator')).toBe('none');
  });

  it('partial load (integrations rejects) yields credential "unknown", not "none"', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    vi.mocked(integrationsApi.listIntegrations).mockRejectedValue(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mountPage();
    // The page must NOT block on the secondary fetch — connectors still loaded.
    expect(internals(el).integrationsUnknown).toBe(true);
    expect(internals(el).mcps.map((c) => c.connector)).toContain('estimator');
    expect(internals(el).credStateFor('estimator')).toBe('unknown');
  });
});

// ---------------------------------- Refresh ----------------------------------

describe('fb-connectors-page — refresh', () => {
  it('a successful refresh runs refreshConnector for that connector', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    const el = await mountPage();
    await internals(el).onRefresh('estimator');
    await settle(el);
    expect(adminApi.refreshConnector).toHaveBeenCalledWith('estimator');
  });

  it('refresh 502 UPSTREAM_ERROR sets PERSISTENT connector-specific card copy (not generic)', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    vi.mocked(adminApi.refreshConnector).mockRejectedValueOnce(
      apiError(ErrorCode.UPSTREAM_ERROR, 502),
    );
    const el = await mountPage();
    await internals(el).onRefresh('estimator');
    await settle(el);
    // A persistent per-card error (cardErrors map), not a vanishing notice, and
    // NOT the generic "our end" copy from userMessageForCode.
    const copy = internals(el).cardErrors.get('estimator') ?? '';
    expect(copy).toContain('Last refresh failed');
    expect(copy).toContain('tools.example.com');
    expect(copy).not.toContain('our end');
    // It is a CARD error, not a page-level error notice.
    expect(internals(el).notice?.kind).not.toBe('err');
  });
});

// ----------------------------- Credential save -----------------------------

describe('fb-connectors-page — credential save', () => {
  it('clears the secret-input value (submit()) and refetches integrations after Save', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    const el = await mountPage();
    // The page passes the card element to onSaveCredential so it can wipe the
    // plaintext; use a standalone card (renders under happy-dom) with a typed value.
    const card = await mountCard(MCP_NEVER, { credState: 'none' });
    const secret = card.shadowRoot!.querySelector('fb-secret-input') as HTMLElement & {
      value: string;
    };
    secret.value = 'sk-bearer-123';
    await internals(el).onSaveCredential(card, 'estimator', 'sk-bearer-123');
    await settle(el);
    expect(integrationsApi.setCredential).toHaveBeenCalledWith('connector:estimator', {
      label: 'estimator access token',
      key: 'sk-bearer-123',
    });
    // submit() wiped the plaintext from the DOM (page calls card.clearCredential()).
    expect(secret.value).toBe('');
    // integrations refetched (initial load + post-save) to refresh last4.
    expect(integrationsApi.listIntegrations).toHaveBeenCalledTimes(2);
  });

  it('still clears the plaintext when setCredential REJECTS', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    vi.mocked(integrationsApi.setCredential).mockRejectedValueOnce(
      apiError(ErrorCode.INTERNAL_ERROR, 500),
    );
    const el = await mountPage();
    const card = await mountCard(MCP_NEVER, { credState: 'none' });
    const secret = card.shadowRoot!.querySelector('fb-secret-input') as HTMLElement & {
      value: string;
    };
    secret.value = 'sk-bad';
    await internals(el).onSaveCredential(card, 'estimator', 'sk-bad');
    await settle(el);
    expect(secret.value).toBe('');
  });

  it('does not call setCredential for an empty value', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    const el = await mountPage();
    const card = await mountCard(MCP_NEVER, { credState: 'none' });
    await internals(el).onSaveCredential(card, 'estimator', '');
    await settle(el);
    expect(integrationsApi.setCredential).not.toHaveBeenCalled();
  });
});

// --------------------------------- Add MCP ---------------------------------

describe('fb-connectors-page — Add MCP', () => {
  /** Drive the modal submit via the CustomEvent shape fb-form emits. */
  function submitModal(el: HTMLElement, values: Record<string, string>): Promise<void> {
    return internals(el).onModalSubmit(new CustomEvent('submit', { detail: { values } }));
  }

  it('blocks an invalid name CLIENT-SIDE before any PUT (modal stays open)', async () => {
    const el = await mountPage();
    internals(el).openAdd();
    await submitModal(el, { name: 'Bad Name!', endpoint: 'https://ok.example.com' });
    await settle(el);
    expect(adminApi.setConnector).not.toHaveBeenCalled();
    expect(internals(el).modalOpen).toBe(true);
  });

  it('PUT 400 keeps the modal open and does NOT chain reload/refresh', async () => {
    vi.mocked(adminApi.setConnector).mockRejectedValueOnce(
      apiError(ErrorCode.VALIDATION_ERROR, 400, [{ field: 'endpoint', reason: 'must be https' }]),
    );
    const el = await mountPage();
    internals(el).openAdd();
    await submitModal(el, { name: 'estimator', endpoint: 'https://tools.example.com' });
    await settle(el);
    expect(adminApi.setConnector).toHaveBeenCalledOnce();
    expect(internals(el).modalOpen).toBe(true);
    // The create did not land -> no auto-refresh.
    expect(adminApi.refreshConnector).not.toHaveBeenCalled();
  });

  it('PUT success closes the modal + reloads FIRST, THEN fires refresh', async () => {
    vi.mocked(adminApi.listConnectors)
      .mockResolvedValueOnce([BUILTIN]) // initial
      .mockResolvedValue([BUILTIN, MCP_NEVER]); // post-create reload
    const el = await mountPage();
    internals(el).openAdd();
    await submitModal(el, { name: 'estimator', endpoint: 'https://tools.example.com' });
    await settle(el);
    expect(adminApi.setConnector).toHaveBeenCalledWith('estimator', {
      enabled: true,
      kind: 'mcp',
      config: { endpoint: 'https://tools.example.com' },
    });
    expect(internals(el).modalOpen).toBe(false);
    expect(vi.mocked(adminApi.listConnectors).mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(adminApi.refreshConnector).toHaveBeenCalledWith('estimator');
  });

  it('sends config as an OBJECT (never a stringified JSON)', async () => {
    const el = await mountPage();
    internals(el).openAdd();
    await submitModal(el, { name: 'estimator', endpoint: 'https://tools.example.com' });
    await settle(el);
    const arg = vi.mocked(adminApi.setConnector).mock.calls[0]![1];
    expect(typeof arg.config).toBe('object');
    expect(arg.config).toEqual({ endpoint: 'https://tools.example.com' });
  });

  it('passes the bare (validated) name to setConnector; admin.ts encodes the path', async () => {
    const el = await mountPage();
    internals(el).openAdd();
    await submitModal(el, { name: 'my-estimator', endpoint: 'https://x.example.com' });
    await settle(el);
    expect(adminApi.setConnector).toHaveBeenCalledWith('my-estimator', expect.any(Object));
  });
});

// --------------------------- Enable auto-refresh ---------------------------

describe('fb-connectors-page — enable auto-refresh', () => {
  it('enabling a disabled MCP auto-fires refresh (PUT does not run tools/list)', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_DISABLED]);
    vi.mocked(adminApi.setConnector).mockResolvedValue({
      id: 'c-2',
      org_id: 'org-1',
      connector_name: 'estimator',
      kind: 'mcp',
      enabled: true,
      config: { endpoint: 'https://tools.example.com' },
      updated_by: 'u-1',
      created_at: '2026-01-01',
      updated_at: '2026-01-01',
    });
    const el = await mountPage();
    await internals(el).onToggle(MCP_DISABLED, true);
    await settle(el);
    // PUT preserves the endpoint snapshot, then refresh auto-runs.
    expect(adminApi.setConnector).toHaveBeenCalledWith('estimator', {
      enabled: true,
      kind: 'mcp',
      config: { endpoint: 'https://tools.example.com' },
    });
    expect(adminApi.refreshConnector).toHaveBeenCalledWith('estimator');
  });

  it('DISABLING an MCP does NOT auto-fire refresh', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    const el = await mountPage();
    await internals(el).onToggle(MCP_NEVER, false);
    await settle(el);
    expect(adminApi.setConnector).toHaveBeenCalledWith('estimator', {
      enabled: false,
      kind: 'mcp',
      config: { endpoint: 'https://tools.example.com' },
    });
    expect(adminApi.refreshConnector).not.toHaveBeenCalled();
  });
});

// ----------------------------------- Delete -----------------------------------

describe('fb-connectors-page — delete', () => {
  it('confirm-delete removes the connector and drops the active orphan credential', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    vi.mocked(integrationsApi.listIntegrations).mockResolvedValue([credRow('estimator', true)]);
    const el = await mountPage();
    internals(el).onRequestDelete('estimator');
    await internals(el).onConfirmDelete();
    await settle(el);
    expect(adminApi.deleteConnector).toHaveBeenCalledWith('estimator');
    expect(integrationsApi.deleteCredential).toHaveBeenCalledWith('connector:estimator');
  });

  it('does not delete a credential when none is active (no orphan to clean up)', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    vi.mocked(integrationsApi.listIntegrations).mockResolvedValue([]); // no cred
    const el = await mountPage();
    internals(el).onRequestDelete('estimator');
    await internals(el).onConfirmDelete();
    await settle(el);
    expect(adminApi.deleteConnector).toHaveBeenCalledWith('estimator');
    expect(integrationsApi.deleteCredential).not.toHaveBeenCalled();
  });

  it('a 404 on delete is treated as success (idempotent), not an error', async () => {
    vi.mocked(adminApi.listConnectors).mockResolvedValue([BUILTIN, MCP_NEVER]);
    vi.mocked(adminApi.deleteConnector).mockRejectedValueOnce(apiError(ErrorCode.NOT_FOUND, 404));
    const el = await mountPage();
    internals(el).onRequestDelete('estimator');
    await internals(el).onConfirmDelete();
    await settle(el);
    expect(internals(el).notice?.kind).toBe('ok');
  });
});

// ------------------------------ Load states ------------------------------

describe('fb-connectors-page — load states', () => {
  it('shows a retryable error when GET /admin/connectors fails', async () => {
    vi.mocked(adminApi.listConnectors).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mountPage();
    const state = el.shadowRoot!.querySelector('fb-state');
    expect(state?.getAttribute('mode')).toBe('error');
    expect(state?.getAttribute('error-code')).toBe(ErrorCode.SERVICE_UNAVAILABLE);
  });

  it('renders the built-in card after a successful load', async () => {
    const el = await mountPage();
    expect(internals(el).builtins.map((c) => c.connector)).toEqual(['reference']);
    // The built-in card materializes (it is not inside a conditional wrapper div).
    expect(el.shadowRoot!.querySelector('fb-connector-card')).not.toBeNull();
  });

  it('has no MCP instances by default (the page renders its empty-state branch)', async () => {
    // The empty-state `fb-state` lives inside the same conditionally-rendered
    // wrapper happy-dom mis-parses, so we assert the derived condition that drives
    // the empty branch rather than the (unrenderable-here) element. The live
    // Playwright sweep covers the rendered empty state.
    const el = await mountPage();
    expect(internals(el).mcps).toHaveLength(0);
  });
});
