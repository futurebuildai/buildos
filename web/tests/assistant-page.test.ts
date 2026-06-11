import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ---------------------------------------------------------------------------
// Module mocks. The page reaches the network only through endpoints/assistant.js
// (sendChat), and reads two stores (capabilityStore `aiConfigured`, authStore
// `hasRole`/`hasMinRole`) + the router `navigate`. We mock all of them so the
// page renders deterministically off in-test fixtures. `aiConfigured` is exposed
// as a plain object with a `.get()` — the page reads it imperatively in
// connectedCallback, so a per-test return value is all that's needed.
// ---------------------------------------------------------------------------
vi.mock('../src/api/endpoints/assistant.js', () => ({
  sendChat: vi.fn(),
}));

let aiConfiguredValue = true;
const markAiUnconfigured = vi.fn();
vi.mock('../src/state/capabilityStore.js', () => ({
  aiConfigured: { get: () => aiConfiguredValue },
  markAiUnconfigured: () => markAiUnconfigured(),
}));

let ownerValue = false;
let minRole: 'superintendent' | 'admin' = 'superintendent';
const ROLE_RANK = { field_worker: 0, superintendent: 1, admin: 2, owner: 3 } as const;
vi.mock('../src/state/authStore.js', () => ({
  hasRole: (...roles: string[]) => roles.includes('owner') && ownerValue,
  hasMinRole: (min: keyof typeof ROLE_RANK) => ROLE_RANK[minRole] >= ROLE_RANK[min],
}));

const navigate = vi.fn();
vi.mock('../src/router.js', () => ({
  navigate: (path: string) => navigate(path),
}));

import '../src/components/pages/fb-assistant-page.js';

import * as assistantApi from '../src/api/endpoints/assistant.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import type { ChatResponse } from '../src/types/models.js';

// ----------------------------- harness helpers -----------------------------

async function mount<T extends HTMLElement>(tag: string): Promise<T> {
  const el = document.createElement(tag) as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

/** Flush the microtask queue (mocked async sends) then the Lit render. */
async function flush(page: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

const sr = (page: HTMLElement): ShadowRoot => page.shadowRoot!;

function textarea(page: HTMLElement): HTMLTextAreaElement {
  const el = sr(page).querySelector('textarea');
  if (!el) throw new Error('no composer textarea');
  return el;
}

/** Type a draft into the composer (mirrors the @input path). */
function type(page: HTMLElement, value: string): void {
  const ta = textarea(page);
  ta.value = value;
  ta.dispatchEvent(new Event('input'));
}

/** Press Enter (send) on the composer. */
function pressEnter(page: HTMLElement): void {
  textarea(page).dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }),
  );
}

function chatOk(over: Partial<ChatResponse> = {}): ChatResponse {
  return {
    reply: 'Birchwood Estate is on track. **No critical risks.**',
    tools_used: [{ name: 'list_projects', is_error: false }],
    iterations: 1,
    truncated: false,
    ...over,
  };
}

function apiError(code: string, status = 400): ApiError {
  return new ApiError({ code, message: code, status });
}

const userBubbles = (page: HTMLElement): Element[] => [...sr(page).querySelectorAll('.turn.user')];

beforeEach(() => {
  vi.clearAllMocks();
  aiConfiguredValue = true;
  ownerValue = false;
  minRole = 'superintendent';
  vi.mocked(assistantApi.sendChat).mockResolvedValue(chatOk());
});

afterEach(() => {
  document.body.innerHTML = '';
});

describe('fb-assistant-page — empty state', () => {
  it('renders the composer and starter prompts when AI is on', async () => {
    const el = await mount('fb-assistant-page');
    await flush(el);
    expect(sr(el).querySelector('.composer')).not.toBeNull();
    expect(sr(el).querySelector('textarea')?.getAttribute('aria-label')).toBe('Ask the assistant');
    expect(sr(el).querySelectorAll('.starter-chips fb-chip').length).toBeGreaterThanOrEqual(3);
    // The conversation thread is a live log region for SR users.
    const log = sr(el).querySelector('.thread');
    expect(log?.getAttribute('role')).toBe('log');
    expect(log?.getAttribute('aria-live')).toBe('polite');
  });

  it('hides the admin-only starter for a superintendent and shows it for admin', async () => {
    minRole = 'superintendent';
    const supe = await mount<HTMLElement>('fb-assistant-page');
    await flush(supe);
    const supeStarters = [...sr(supe).querySelectorAll('.starter-chips fb-chip')].map((c) =>
      c.textContent?.trim(),
    );
    expect(supeStarters.some((t) => t?.includes('budget variance'))).toBe(false);

    minRole = 'admin';
    const admin = await mount<HTMLElement>('fb-assistant-page');
    await flush(admin);
    const adminStarters = [...sr(admin).querySelectorAll('.starter-chips fb-chip')].map((c) =>
      c.textContent?.trim(),
    );
    expect(adminStarters.some((t) => t?.includes('budget variance'))).toBe(true);
  });
});

describe('fb-assistant-page — send flow', () => {
  it('shows the user turn, calls sendChat with {message, history}, and renders the reply', async () => {
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, "What's at risk on Birchwood Estate?");
    pressEnter(el);
    await flush(el);

    expect(assistantApi.sendChat).toHaveBeenCalledTimes(1);
    const [message, history] = vi.mocked(assistantApi.sendChat).mock.calls[0]!;
    expect(message).toBe("What's at risk on Birchwood Estate?");
    expect(history).toEqual([]); // first turn — no prior history

    // User bubble carries the typed text; assistant reply rendered via fb-markdown.
    expect(userBubbles(el).length).toBe(1);
    const md = sr(el).querySelector('.turn.assistant fb-markdown');
    expect(md).not.toBeNull();
    expect((md as unknown as { source: string }).source).toContain('Birchwood Estate is on track');
  });

  it('renders tools_used as friendly grounding chips and never leaks args/results', async () => {
    vi.mocked(assistantApi.sendChat).mockResolvedValue(
      chatOk({
        reply: 'Done.',
        tools_used: [
          { name: 'get_schedule_gantt', is_error: false },
          { name: 'list_feed_cards', is_error: false },
        ],
      }),
    );
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, 'Summarize the schedule');
    pressEnter(el);
    await flush(el);

    const group = sr(el).querySelector('.sources[aria-label="Sources used"]');
    expect(group).not.toBeNull();
    const chipText = [...group!.querySelectorAll('fb-chip')].map((c) => c.textContent?.trim());
    expect(chipText.some((t) => t?.includes('Schedule'))).toBe(true);
    expect(chipText.some((t) => t?.includes('Feed'))).toBe(true);
    // Raw tool names are mapped to labels, never shown verbatim.
    expect(group!.textContent).not.toContain('get_schedule_gantt');
  });

  it('marks a failed tool chip with the (failed) affix', async () => {
    vi.mocked(assistantApi.sendChat).mockResolvedValue(
      chatOk({ tools_used: [{ name: 'get_org_financials', is_error: true }] }),
    );
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, 'Budget?');
    pressEnter(el);
    await flush(el);

    const chip = sr(el).querySelector('.sources fb-chip');
    expect(chip?.classList.contains('source-failed')).toBe(true);
    expect(chip?.textContent).toContain('(failed)');
  });

  it('renders the incomplete note when truncated=true', async () => {
    vi.mocked(assistantApi.sendChat).mockResolvedValue(chatOk({ truncated: true }));
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, 'Long question');
    pressEnter(el);
    await flush(el);
    expect(sr(el).querySelector('.truncated')?.textContent).toContain('may be incomplete');
  });

  it('keeps the thread across multiple turns (second send includes the first exchange)', async () => {
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, 'First question');
    pressEnter(el);
    await flush(el);

    type(el, 'Follow-up');
    pressEnter(el);
    await flush(el);

    expect(assistantApi.sendChat).toHaveBeenCalledTimes(2);
    const [, secondHistory] = vi.mocked(assistantApi.sendChat).mock.calls[1]!;
    expect(secondHistory).toEqual([
      { role: 'user', text: 'First question' },
      { role: 'assistant', text: 'Birchwood Estate is on track. **No critical risks.**' },
    ]);
    expect(userBubbles(el).length).toBe(2);
  });

  it('caps history to the last 10 turns', async () => {
    const el = await mount('fb-assistant-page');
    await flush(el);
    // 6 exchanges = 12 turns of prior history before the 7th send.
    for (let i = 0; i < 6; i++) {
      type(el, `q${i}`);
      pressEnter(el);
      await flush(el);
    }
    type(el, 'final');
    pressEnter(el);
    await flush(el);

    const calls = vi.mocked(assistantApi.sendChat).mock.calls;
    const [, history] = calls[calls.length - 1]!;
    expect(history.length).toBeLessThanOrEqual(10);
  });

  it('does not send an empty/whitespace draft', async () => {
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, '   ');
    pressEnter(el);
    await flush(el);
    expect(assistantApi.sendChat).not.toHaveBeenCalled();
  });
});

describe('fb-assistant-page — gated / disabled states', () => {
  it('renders the gated panel without calling the endpoint when AI is off at connect', async () => {
    aiConfiguredValue = false;
    const el = await mount('fb-assistant-page');
    await flush(el);
    expect(assistantApi.sendChat).not.toHaveBeenCalled();
    expect(sr(el).querySelector('fb-state')?.getAttribute('mode')).toBe('gated');
  });

  it('shows the gated panel + owner link on a 503 and marks AI unconfigured', async () => {
    ownerValue = true;
    vi.mocked(assistantApi.sendChat).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, 'hello');
    pressEnter(el);
    await flush(el);

    expect(markAiUnconfigured).toHaveBeenCalled();
    const state = sr(el).querySelector('fb-state');
    expect(state?.getAttribute('mode')).toBe('gated');
    // Owner sees the configure affordance (can-configure reflected on fb-state).
    expect(state?.hasAttribute('can-configure')).toBe(true);
  });

  it('shows the admin-off copy with NO configure link on a 403 CAPABILITY_DISABLED', async () => {
    ownerValue = true; // even an owner gets no key-link: it is not a missing key.
    vi.mocked(assistantApi.sendChat).mockRejectedValueOnce(
      apiError(ErrorCode.CAPABILITY_DISABLED, 403),
    );
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, 'hello');
    pressEnter(el);
    await flush(el);

    const state = sr(el).querySelector('fb-state');
    expect(state?.getAttribute('mode')).toBe('gated');
    expect(state?.hasAttribute('can-configure')).toBe(false);
    expect(markAiUnconfigured).not.toHaveBeenCalled();
    expect(state?.getAttribute('message')).toContain('turned off by an admin');
  });

  it('attaches an inline error to the user turn on a transient failure (keeps the typed message)', async () => {
    vi.mocked(assistantApi.sendChat).mockRejectedValueOnce(apiError(ErrorCode.UPSTREAM_ERROR, 502));
    const el = await mount('fb-assistant-page');
    await flush(el);
    type(el, 'will fail');
    pressEnter(el);
    await flush(el);

    // The thread stays in chat mode (not gated) and the user turn is preserved.
    expect(sr(el).querySelector('fb-state')).toBeNull();
    expect(userBubbles(el).length).toBe(1);
    expect(sr(el).querySelector('.turn-error')).not.toBeNull();
  });
});

// Regression (adversarial review MAJOR): a long assistant reply (>8000 bytes)
// must be DROPPED from the resent history (not re-sent → server per-turn 400 →
// permanent wedge). It stays visible in the thread, just isn't resent.
describe('fb-assistant-page — history per-turn cap (wedge regression)', () => {
  it('drops an oversized assistant reply from resent history but keeps it visible', async () => {
    const huge = 'x'.repeat(8001);
    vi.mocked(assistantApi.sendChat).mockResolvedValueOnce(chatOk({ reply: huge }));
    const el = await mount('fb-assistant-page');

    type(el, 'first question');
    pressEnter(el);
    await flush(el);

    type(el, 'second question');
    pressEnter(el);
    await flush(el);

    const calls = vi.mocked(assistantApi.sendChat).mock.calls;
    expect(calls.length).toBe(2);
    const history = (calls[1]?.[1] ?? []) as { role: string; text: string }[];
    // the oversized assistant turn is NOT resent (would 400 + wedge the thread)
    expect(history.some((t) => t.text.length > 8000)).toBe(false);
    // the prior user turn IS resent
    expect(history.some((t) => t.role === 'user' && t.text === 'first question')).toBe(true);
    // but the long reply still renders in the thread (display ≠ resend): both
    // assistant turns are present (its body lives in fb-markdown's shadow root).
    expect(sr(el).querySelectorAll('.turn.assistant').length).toBe(2);
  });
});
