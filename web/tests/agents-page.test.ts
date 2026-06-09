import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ---------------------------------------------------------------------------
// Module mocks. The page reaches the network only through endpoints/admin.js,
// and reads two stores: capabilityStore (`aiConfigured`, a computed signal) and
// authStore (`hasRole`). We mock all three so the page renders deterministically
// off in-test fixtures. `aiConfigured` is exposed as a plain object with a
// `.get()` method — the page reads it as a signal (`aiConfigured.get()`), so a
// per-test return value is all that's needed; SignalWatcher tracking is moot.
// ---------------------------------------------------------------------------
vi.mock('../src/api/endpoints/admin.js', () => ({
  listAgents: vi.fn(),
  setAgent: vi.fn(),
  resetAgent: vi.fn(),
}));

let aiConfiguredValue = true;
vi.mock('../src/state/capabilityStore.js', () => ({
  aiConfigured: { get: () => aiConfiguredValue },
}));

let ownerValue = false;
vi.mock('../src/state/authStore.js', () => ({
  hasRole: (...roles: string[]) => roles.includes('owner') && ownerValue,
}));

import '../src/components/pages/fb-agents-page.js';

import * as adminApi from '../src/api/endpoints/admin.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import type { AgentCapability, EffectiveAgentConfig, AgentConfig } from '../src/types/models.js';

// ----------------------------- harness helpers -----------------------------

async function mount<T extends HTMLElement>(tag: string): Promise<T> {
  const el = document.createElement(tag) as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

/** Flush the microtask queue (mocked async loads) then the Lit render. */
async function flush(page: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

const sr = (page: HTMLElement): ShadowRoot => page.shadowRoot!;

/** The fb-switch host for a capability, resolved by its data-cap join key. */
function switchFor(page: HTMLElement, cap: AgentCapability): HTMLElement {
  const el = sr(page).querySelector<HTMLElement>(`fb-switch[data-cap='${cap}']`);
  if (!el) throw new Error(`no fb-switch for ${cap}`);
  return el;
}

/**
 * The role=switch input nested inside a capability's fb-switch shadow root —
 * the element a `getByRole('switch', { name: /Enable/ })` query would resolve.
 * Returning it lets us assert both its role and accessible name (aria-label).
 */
function switchInput(page: HTMLElement, cap: AgentCapability): HTMLInputElement {
  const input = switchFor(page, cap).shadowRoot!.querySelector<HTMLInputElement>(
    "input[role='switch']",
  );
  if (!input) throw new Error(`no role=switch input for ${cap}`);
  return input;
}

/** Dispatch the composed `change` { checked } the page's onToggle listens for. */
function toggle(page: HTMLElement, cap: AgentCapability, checked: boolean): void {
  switchFor(page, cap).dispatchEvent(
    new CustomEvent('change', { detail: { checked }, bubbles: true, composed: true }),
  );
}

/** Drive the foresight fb-form submit with raw STRING control values. */
function submitForesight(page: HTMLElement, values: Record<string, string>): void {
  const form = sr(page).querySelector<HTMLElement>("fb-form[data-cap='foresight']")!;
  form.dispatchEvent(
    new CustomEvent('submit', { detail: { values }, bubbles: true, composed: true }),
  );
}

function apiError(
  code: string,
  status = 400,
  details: { field: string; reason: string }[] = [],
): ApiError {
  return new ApiError({ code, message: code, status, details });
}

// ------------------------------- fixtures ----------------------------------

function effective(
  cap: AgentCapability,
  over: Partial<EffectiveAgentConfig> = {},
): EffectiveAgentConfig {
  const base: Record<AgentCapability, EffectiveAgentConfig> = {
    delay_cascade: {
      capability: 'delay_cascade',
      description: 'delay cascade',
      enabled: true,
      config: {},
      source: 'default',
    },
    foresight: {
      capability: 'foresight',
      description: 'foresight',
      enabled: true,
      config: { schedule_float_days: 2, budget_burn_percent: 80 },
      source: 'default',
    },
    experience: {
      capability: 'experience',
      description: 'experience',
      enabled: true,
      config: {},
      source: 'default',
    },
  };
  return { ...base[cap], ...over };
}

function defaultAgents(): EffectiveAgentConfig[] {
  return [effective('delay_cascade'), effective('foresight'), effective('experience')];
}

function agentRow(cap: AgentCapability, over: Partial<AgentConfig> = {}): AgentConfig {
  return {
    id: 'ag-1',
    org_id: 'org-1',
    capability: cap,
    enabled: true,
    config: cap === 'foresight' ? { schedule_float_days: 2, budget_burn_percent: 80 } : {},
    updated_by: 'u-1',
    created_at: '2026-01-01',
    updated_at: '2026-01-01',
    ...over,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  aiConfiguredValue = true;
  ownerValue = false;
  vi.mocked(adminApi.listAgents).mockResolvedValue(defaultAgents());
  vi.mocked(adminApi.setAgent).mockResolvedValue(agentRow('foresight'));
  vi.mocked(adminApi.resetAgent).mockResolvedValue(undefined);
});

afterEach(() => {
  document.body.innerHTML = '';
});

// ============================================================================
// §9.1 — agents-page core
// ============================================================================

describe('fb-agents-page — render', () => {
  it('renders one card per catalog capability (3) after a successful load', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    expect(sr(el).querySelectorAll('section.card').length).toBe(3);
    // Each capability switch is present, keyed by its join key.
    for (const cap of ['delay_cascade', 'foresight', 'experience'] as AgentCapability[]) {
      expect(switchFor(el, cap)).toBeTruthy();
    }
  });

  it('exposes each switch as role=switch with an "Enable …" accessible name', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    // The element a getByRole('switch', { name: /Enable/ }) query resolves.
    const input = switchInput(el, 'foresight');
    expect(input.getAttribute('role')).toBe('switch');
    expect(input.getAttribute('aria-label')).toMatch(/^Enable /);
    expect(input.getAttribute('aria-label')).toBe('Enable Risk early-warning');
  });

  it('renders the foresight tuning form prefilled from server config (only foresight)', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    const forms = sr(el).querySelectorAll("fb-form[data-cap='foresight']");
    expect(forms.length).toBe(1);
    const inputs = sr(el).querySelectorAll("fb-form[data-cap='foresight'] fb-input[name]");
    const names = Array.from(inputs).map((i) => i.getAttribute('name'));
    expect(names).toEqual(['schedule_float_days', 'budget_burn_percent']);
    // One fb-field per number input (a11y: shared field leaves input #2 unwired).
    expect(sr(el).querySelectorAll("fb-form[data-cap='foresight'] fb-field").length).toBe(2);
  });

  it('shows a retryable error state when the GET fails', async () => {
    vi.mocked(adminApi.listAgents).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mount('fb-agents-page');
    await flush(el);
    const state = sr(el).querySelector('fb-state');
    expect(state?.getAttribute('mode')).toBe('error');
    expect(state?.getAttribute('error-code')).toBe(ErrorCode.SERVICE_UNAVAILABLE);
    expect(state?.hasAttribute('retryable')).toBe(true);
  });
});

// ============================================================================
// §9.1 — foresight Save: coercion + client validation + 400 mapping
// ============================================================================

describe('fb-agents-page — foresight Save coercion & validation', () => {
  it('coerces string form values to ints and PUTs them as an OBJECT', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    submitForesight(el, { schedule_float_days: '5', budget_burn_percent: '90' });
    await flush(el);

    expect(adminApi.setAgent).toHaveBeenCalledTimes(1);
    const [cap, input] = vi.mocked(adminApi.setAgent).mock.calls[0]!;
    expect(cap).toBe('foresight');
    // Coerced to integers (not the raw strings).
    expect(input.config).toEqual({ schedule_float_days: 5, budget_burn_percent: 90 });
    expect(input.config!['schedule_float_days']).toBe(5);
    expect(input.config!['budget_burn_percent']).toBe(90);
    // config is an OBJECT on the wire, never a stringified JSON.
    expect(typeof input.config).toBe('object');
    expect(typeof input.config!['schedule_float_days']).toBe('number');
  });

  it('rejects empty values client-side BEFORE any setAgent call', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    submitForesight(el, { schedule_float_days: '', budget_burn_percent: '80' });
    await flush(el);
    expect(adminApi.setAgent).not.toHaveBeenCalled();
    // Error surfaced through the fb-form summary, keyed by the human label.
    const summary = sr(el)
      .querySelector("fb-form[data-cap='foresight']")!
      .shadowRoot!.querySelector('.summary');
    expect(summary?.textContent).toContain('Schedule float (days)');
  });

  it('rejects NaN / non-integer values client-side BEFORE any setAgent call', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    submitForesight(el, { schedule_float_days: 'abc', budget_burn_percent: '2.5' });
    await flush(el);
    expect(adminApi.setAgent).not.toHaveBeenCalled();
    const summary = sr(el)
      .querySelector("fb-form[data-cap='foresight']")!
      .shadowRoot!.querySelector('.summary');
    expect(summary?.textContent).toContain('Schedule float (days)');
    expect(summary?.textContent).toContain('Budget spent warning (%)');
  });

  it('rejects values below 1 (e.g. 0) client-side BEFORE any setAgent call', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    submitForesight(el, { schedule_float_days: '0', budget_burn_percent: '80' });
    await flush(el);
    expect(adminApi.setAgent).not.toHaveBeenCalled();
  });

  it('maps a foresight 400 detail to the matching fb-field via its human label', async () => {
    vi.mocked(adminApi.setAgent).mockRejectedValueOnce(
      apiError(ErrorCode.VALIDATION_ERROR, 400, [
        { field: 'schedule_float_days', reason: 'must be a positive integer' },
      ]),
    );
    const el = await mount('fb-agents-page');
    await flush(el);
    // Valid client-side input so the PUT actually fires and 400s.
    submitForesight(el, { schedule_float_days: '3', budget_burn_percent: '85' });
    await flush(el);

    expect(adminApi.setAgent).toHaveBeenCalledTimes(1);
    const summary = sr(el)
      .querySelector("fb-form[data-cap='foresight']")!
      .shadowRoot!.querySelector('.summary');
    // Wire field → human label, so aria-invalid lands on the right input.
    expect(summary?.textContent).toContain('Schedule float (days)');
    expect(summary?.textContent).toContain('must be a positive integer');
    // The other field's label is NOT keyed by this single-field error.
    expect(summary?.textContent).not.toContain('Budget spent warning (%)');
  });

  it('binds a 400 error onto the offending fb-field (aria-invalid), not only the summary', async () => {
    vi.mocked(adminApi.setAgent).mockRejectedValueOnce(
      apiError(ErrorCode.VALIDATION_ERROR, 400, [
        { field: 'budget_burn_percent', reason: 'must be a positive integer' },
      ]),
    );
    const el = await mount('fb-agents-page');
    await flush(el);
    submitForesight(el, { schedule_float_days: '3', budget_burn_percent: '85' });
    await flush(el);

    // The budget field's fb-field carries the error (so its inner input gets
    // aria-invalid + aria-describedby), and the float field does NOT.
    const fields = sr(el).querySelectorAll<HTMLElement & { error?: string }>(
      "fb-form[data-cap='foresight'] fb-field",
    );
    const errs = [...fields].map((f) => f.getAttribute('error') ?? '');
    expect(errs.some((e) => e.includes('must be a positive integer'))).toBe(true);
    // Exactly one field is flagged (the budget one), not both.
    expect(errs.filter((e) => e.length > 0).length).toBe(1);
  });

  it('clears per-field errors after a subsequent successful save', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    // First: a client-side rejection flags both fields.
    submitForesight(el, { schedule_float_days: '', budget_burn_percent: '' });
    await flush(el);
    let fields = sr(el).querySelectorAll("fb-form[data-cap='foresight'] fb-field");
    expect([...fields].some((f) => (f.getAttribute('error') ?? '').length > 0)).toBe(true);
    // Then: a valid save clears them.
    submitForesight(el, { schedule_float_days: '3', budget_burn_percent: '85' });
    await flush(el);
    fields = sr(el).querySelectorAll("fb-form[data-cap='foresight'] fb-field");
    expect([...fields].every((f) => (f.getAttribute('error') ?? '').length === 0)).toBe(true);
  });
});

// ============================================================================
// §9.1 — toggle semantics (full-document PUT; config snapshot; resync)
// ============================================================================

describe('fb-agents-page — enable toggle', () => {
  it('toggling foresight resends config===savedConfig (both ints preserved) as an OBJECT', async () => {
    vi.mocked(adminApi.listAgents).mockResolvedValue([
      effective('delay_cascade'),
      effective('foresight', {
        enabled: false,
        source: 'override',
        config: { schedule_float_days: 7, budget_burn_percent: 65 },
      }),
      effective('experience'),
    ]);
    const el = await mount('fb-agents-page');
    await flush(el);

    toggle(el, 'foresight', true);
    await flush(el);

    expect(adminApi.setAgent).toHaveBeenCalledTimes(1);
    const [cap, input] = vi.mocked(adminApi.setAgent).mock.calls[0]!;
    expect(cap).toBe('foresight');
    expect(input.enabled).toBe(true);
    // The full-document PUT carries the last server-confirmed snapshot verbatim —
    // never live inputs, never an omitted/{} config (which would RESET tuning).
    expect(input.config).toEqual({ schedule_float_days: 7, budget_burn_percent: 65 });
    expect(typeof input.config).toBe('object');
  });

  it('toggling delay_cascade / experience sends config==={} (no tunable keys)', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);

    toggle(el, 'delay_cascade', false);
    await flush(el);
    toggle(el, 'experience', false);
    await flush(el);

    expect(adminApi.setAgent).toHaveBeenCalledTimes(2);
    const delayInput = vi.mocked(adminApi.setAgent).mock.calls[0]![1];
    const expInput = vi.mocked(adminApi.setAgent).mock.calls[1]![1];
    expect(vi.mocked(adminApi.setAgent).mock.calls[0]![0]).toBe('delay_cascade');
    expect(delayInput.config).toEqual({});
    expect(typeof delayInput.config).toBe('object');
    expect(vi.mocked(adminApi.setAgent).mock.calls[1]![0]).toBe('experience');
    expect(expInput.config).toEqual({});
  });

  it('reloads after a SUCCESSFUL toggle to re-derive the switch from server truth', async () => {
    const el = await mount('fb-agents-page');
    await flush(el);
    expect(adminApi.listAgents).toHaveBeenCalledTimes(1); // initial load
    toggle(el, 'delay_cascade', false);
    await flush(el);
    // finally → await this.load() resyncs the switch.
    expect(adminApi.listAgents).toHaveBeenCalledTimes(2);
  });

  it('a REJECTED toggle still reloads and the switch reflects server truth', async () => {
    // Server still reports enabled=true after the failed write.
    vi.mocked(adminApi.listAgents).mockResolvedValue(defaultAgents());
    vi.mocked(adminApi.setAgent).mockRejectedValueOnce(apiError(ErrorCode.INTERNAL_ERROR, 500));
    const el = await mount('fb-agents-page');
    await flush(el);
    // fb-switch self-mutates checked → user-driven false.
    toggle(el, 'delay_cascade', false);
    await flush(el);

    // finally → load() runs even on failure (resync is the proven pattern).
    expect(adminApi.listAgents).toHaveBeenCalledTimes(2);
    // The freshly-rendered switch input is back to server truth (enabled=true).
    expect(switchInput(el, 'delay_cascade').checked).toBe(true);
    // And an error notice is announced.
    const toast = sr(el).querySelector('.toast.err');
    expect(toast?.getAttribute('role')).toBe('alert');
  });
});

// ============================================================================
// §9.1 — reset (override DELETE; foresight via fb-confirm)
// ============================================================================

describe('fb-agents-page — reset', () => {
  it('reset on a non-foresight override card calls resetAgent + refetches directly', async () => {
    vi.mocked(adminApi.listAgents).mockResolvedValue([
      effective('delay_cascade', { source: 'override', enabled: false }),
      effective('foresight'),
      effective('experience'),
    ]);
    const el = await mount('fb-agents-page');
    await flush(el);

    // Reset affordance shows only on override cards.
    const resetBtn = sr(el).querySelector<HTMLElement>(
      'fb-button[label="Reset delay_cascade to default"]',
    );
    expect(resetBtn).toBeTruthy();
    resetBtn!.dispatchEvent(new CustomEvent('click', { bubbles: true, composed: true }));
    await flush(el);

    expect(adminApi.resetAgent).toHaveBeenCalledTimes(1);
    expect(adminApi.resetAgent).toHaveBeenCalledWith('delay_cascade');
    // Refetch after reset (initial load + post-reset reload).
    expect(adminApi.listAgents).toHaveBeenCalledTimes(2);
  });

  it('foresight reset opens fb-confirm first and only DELETEs on confirm', async () => {
    vi.mocked(adminApi.listAgents).mockResolvedValue([
      effective('delay_cascade'),
      effective('foresight', {
        source: 'override',
        config: { schedule_float_days: 7, budget_burn_percent: 65 },
      }),
      effective('experience'),
    ]);
    const el = await mount('fb-agents-page');
    await flush(el);

    const confirm = sr(el).querySelector('fb-confirm')!;
    expect(confirm.hasAttribute('open')).toBe(false);

    const resetBtn = sr(el).querySelector<HTMLElement>(
      'fb-button[label="Reset foresight to default"]',
    )!;
    resetBtn.dispatchEvent(new CustomEvent('click', { bubbles: true, composed: true }));
    await flush(el);

    // Confirm opens; no DELETE has fired yet (foresight tuning is gated).
    expect(sr(el).querySelector('fb-confirm')!.hasAttribute('open')).toBe(true);
    expect(adminApi.resetAgent).not.toHaveBeenCalled();

    // Confirm → DELETE fires.
    sr(el)
      .querySelector('fb-confirm')!
      .dispatchEvent(new CustomEvent('confirm', { bubbles: true, composed: true }));
    await flush(el);

    expect(adminApi.resetAgent).toHaveBeenCalledTimes(1);
    expect(adminApi.resetAgent).toHaveBeenCalledWith('foresight');
  });

  it('does not render a reset affordance on a default-source card', async () => {
    const el = await mount('fb-agents-page'); // all default
    await flush(el);
    expect(sr(el).querySelector("fb-button[label$='to default']")).toBeNull();
  });
});

// ============================================================================
// §9.2 — AI-dependency states
// ============================================================================

describe('fb-agents-page — AI dependency states', () => {
  it('shows the prominent banner when aiConfigured === false', async () => {
    aiConfiguredValue = false;
    const el = await mount('fb-agents-page');
    await flush(el);
    const banner = sr(el).querySelector('.banner');
    expect(banner).toBeTruthy();
    expect(banner?.getAttribute('role')).toBe('status');
    expect(banner?.textContent).toContain('Anthropic key');
  });

  it('does NOT show the banner when aiConfigured (assume-on)', async () => {
    aiConfiguredValue = true;
    const el = await mount('fb-agents-page');
    await flush(el);
    expect(sr(el).querySelector('.banner')).toBeNull();
    // The persistent neutral per-card dependency row is still present on every card.
    expect(sr(el).querySelectorAll('.dep-row').length).toBe(3);
  });

  it('renders the integrations link as a real <a> ONLY for an owner', async () => {
    ownerValue = true;
    const el = await mount('fb-agents-page');
    await flush(el);
    const links = sr(el).querySelectorAll<HTMLAnchorElement>(
      ".dep-row a[href='/settings/integrations']",
    );
    expect(links.length).toBe(3); // one per card
    expect(links[0]!.tagName).toBe('A');
  });

  it('renders plain text (no link) for a non-owner admin', async () => {
    ownerValue = false;
    const el = await mount('fb-agents-page');
    await flush(el);
    expect(sr(el).querySelector('.dep-row a')).toBeNull();
    const row = sr(el).querySelector('.dep-row');
    expect(row?.textContent).toContain('Ask your owner to add an Anthropic key');
  });
});
