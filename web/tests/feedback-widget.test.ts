import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// The endpoint module is mocked so the widget never hits the network
// (portfolio-pages.test.ts idiom).
vi.mock('../src/api/endpoints/feedback.js', () => ({
  submitFeedback: vi.fn(),
}));

import '../src/components/organisms/fb-feedback-widget.js';

import * as feedbackApi from '../src/api/endpoints/feedback.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import { navigate } from '../src/router.js';
import type { FbFeedbackWidget } from '../src/components/organisms/fb-feedback-widget.js';
import type { Feedback } from '../src/types/models.js';

const FEEDBACK: Feedback = {
  id: 'fb-1',
  org_id: 'org-1',
  user_sub: 'user-1',
  category: 'idea',
  message: 'The gantt zoom is great',
  context: {
    route: '/login',
    role: '',
    app_version: 'dev',
    user_agent: 'test',
    viewport: '1024x768',
  },
  status: 'new',
  triage_note: '',
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

async function mount<T extends HTMLElement>(html: string): Promise<T> {
  const host = document.createElement('div');
  host.innerHTML = html;
  const el = host.firstElementChild as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

/** Flush the microtask queue (mocked async submit) then the Lit render. */
async function flush(el: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

function trigger(el: FbFeedbackWidget): HTMLButtonElement {
  return el.shadowRoot!.querySelector('.trigger')!;
}

async function openWidget(el: FbFeedbackWidget): Promise<void> {
  trigger(el).click();
  await el.updateComplete;
}

function setMessage(el: FbFeedbackWidget, text: string): void {
  const ta = el.shadowRoot!.querySelector('textarea')!;
  ta.value = text;
  ta.dispatchEvent(new Event('input'));
}

async function submit(el: FbFeedbackWidget): Promise<void> {
  el.shadowRoot!.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
  await flush(el);
}

beforeEach(() => {
  vi.mocked(feedbackApi.submitFeedback).mockReset();
});

afterEach(() => {
  document.body.innerHTML = '';
});

describe('fb-feedback-widget', () => {
  it('is closed by default and opens on trigger click, flipping aria-expanded', async () => {
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    expect(el.shadowRoot!.querySelector('[role="dialog"]')).toBeNull();
    expect(trigger(el).getAttribute('aria-expanded')).toBe('false');

    await openWidget(el);
    expect(el.shadowRoot!.querySelector('[role="dialog"]')).toBeTruthy();
    expect(trigger(el).getAttribute('aria-expanded')).toBe('true');
  });

  it('closes on Escape and restores focus to the trigger', async () => {
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    await openWidget(el);
    // Focus moved INTO the panel on open (inner focusable, not the host).
    const within = el.shadowRoot!.querySelector('.panel')!.contains(el.shadowRoot!.activeElement);
    expect(within).toBe(true);

    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    await flush(el);
    expect(el.shadowRoot!.querySelector('[role="dialog"]')).toBeNull();
    expect(trigger(el).getAttribute('aria-expanded')).toBe('false');
    expect(el.shadowRoot!.activeElement).toBe(trigger(el));
  });

  it('submits category + trimmed message + auto-captured context, then shows success', async () => {
    navigate('/login'); // public route → deterministic context.route
    vi.mocked(feedbackApi.submitFeedback).mockResolvedValue(FEEDBACK);
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    await openWidget(el);

    const select = el.shadowRoot!.querySelector('select')!;
    select.value = 'idea';
    select.dispatchEvent(new Event('change'));
    setMessage(el, '  The gantt zoom is great  ');
    await submit(el);

    expect(feedbackApi.submitFeedback).toHaveBeenCalledTimes(1);
    const body = vi.mocked(feedbackApi.submitFeedback).mock.calls[0]![0];
    expect(body.category).toBe('idea');
    expect(body.message).toBe('The gantt zoom is great'); // trimmed
    expect(Object.keys(body.context).sort()).toEqual([
      'app_version',
      'role',
      'route',
      'user_agent',
      'viewport',
    ]);
    expect(body.context.route).toBe('/login');
    // app_version is injected at build time (vite `define` → package.json
    // version; 'dev' only when the define doesn't run) — assert presence,
    // not a specific value.
    expect(typeof body.context.app_version).toBe('string');
    expect(body.context.app_version).not.toBe('');
    expect(body.context.viewport).toMatch(/^\d+x\d+$/);
    expect(typeof body.context.user_agent).toBe('string');

    // Success state + polite announcement.
    expect(el.shadowRoot!.querySelector('.success')!.textContent).toContain('Thanks');
    expect(el.shadowRoot!.querySelector('[role="status"]')!.textContent).toContain('Thanks');
  });

  it('blocks an empty message with aria-invalid + described-by error, no POST', async () => {
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    await openWidget(el);
    setMessage(el, '   '); // whitespace-only trims to empty
    await submit(el);

    expect(feedbackApi.submitFeedback).not.toHaveBeenCalled();
    const ta = el.shadowRoot!.querySelector('textarea')!;
    expect(ta.getAttribute('aria-invalid')).toBe('true');
    const describedBy = ta.getAttribute('aria-describedby')!;
    const error = el.shadowRoot!.getElementById(describedBy)!;
    expect(error.textContent).toContain('Enter a message');
  });

  it('shows an inline error on API failure and KEEPS the user text', async () => {
    vi.mocked(feedbackApi.submitFeedback).mockRejectedValueOnce(
      new ApiError({ code: ErrorCode.RATE_LIMITED, message: 'rate limited', status: 429 }),
    );
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    await openWidget(el);
    setMessage(el, 'Scheduling keeps double-booking crews');
    await submit(el);

    const error = el.shadowRoot!.querySelector('.error')!;
    expect(error.textContent).toContain('Too many requests');
    // The form survives and the message is untouched.
    expect(el.shadowRoot!.querySelector('textarea')!.value).toBe(
      'Scheduling keeps double-booking crews',
    );
    expect(el.shadowRoot!.querySelector('[role="status"]')!.textContent).toContain(
      'Too many requests',
    );
  });

  it('discards a late success resolution when the panel was closed mid-submit', async () => {
    // A controllable promise: the POST stays pending until WE resolve it.
    let resolveSubmit!: (fb: Feedback) => void;
    vi.mocked(feedbackApi.submitFeedback).mockImplementation(
      () => new Promise<Feedback>((resolve) => (resolveSubmit = resolve)),
    );
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    const sent = vi.fn();
    el.addEventListener('feedback-sent', sent);
    await openWidget(el);
    setMessage(el, 'About to abandon this');
    el.shadowRoot!.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
    await el.updateComplete;
    expect(el.shadowRoot!.querySelector('.send')!.textContent).toContain('Sending…');

    // Close WHILE the POST is in flight (allowed — abandons its UI effects).
    el.shadowRoot!.querySelector<HTMLButtonElement>('.close')!.click();
    await flush(el);
    expect(el.shadowRoot!.querySelector('[role="dialog"]')).toBeNull();

    // Park focus outside the widget so any focus steal is observable.
    const outside = document.createElement('input');
    document.body.appendChild(outside);
    outside.focus();

    // The late SUCCESS resolution arrives — and must be discarded entirely.
    resolveSubmit(FEEDBACK);
    await flush(el);
    expect(el.shadowRoot!.querySelector('[role="dialog"]')).toBeNull(); // stays closed
    expect(el.shadowRoot!.querySelector('.success')).toBeNull(); // no success state
    expect(el.shadowRoot!.querySelector('[role="status"]')!.textContent).not.toContain('Thanks');
    expect(sent).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(outside); // no focus steal
  });

  it('keeps a freshly typed draft when a stale resolution lands after close + reopen', async () => {
    let resolveSubmit!: (fb: Feedback) => void;
    vi.mocked(feedbackApi.submitFeedback).mockImplementation(
      () => new Promise<Feedback>((resolve) => (resolveSubmit = resolve)),
    );
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    await openWidget(el);
    setMessage(el, 'first draft');
    el.shadowRoot!.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
    await el.updateComplete;

    // Close mid-flight, reopen, start typing a NEW message.
    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    await flush(el);
    await openWidget(el);
    setMessage(el, 'second draft');
    await el.updateComplete;

    // The stale resolution lands: no transition, the new text is intact.
    resolveSubmit(FEEDBACK);
    await flush(el);
    expect(el.shadowRoot!.querySelector('.success')).toBeNull();
    expect(el.shadowRoot!.querySelector('form')).toBeTruthy(); // still the form
    expect(el.shadowRoot!.querySelector('textarea')!.value).toBe('second draft');
    expect(el.shadowRoot!.querySelector('.send')!.textContent).toContain('Send feedback'); // idle, not Sending…
  });

  it('exposes the dialog/labelling a11y contract', async () => {
    const el = await mount<FbFeedbackWidget>('<fb-feedback-widget></fb-feedback-widget>');
    const t = trigger(el);
    expect(t.getAttribute('aria-label')).toBe('Send feedback');
    expect(t.getAttribute('aria-haspopup')).toBe('dialog');

    await openWidget(el);
    const dialog = el.shadowRoot!.querySelector('[role="dialog"]')!;
    expect(dialog.getAttribute('aria-modal')).toBe('false');
    const labelId = dialog.getAttribute('aria-labelledby')!;
    expect(el.shadowRoot!.getElementById(labelId)!.textContent).toContain('Send feedback');
    // Every control carries a real <label for>.
    expect(el.shadowRoot!.querySelector('label[for="fb-feedback-category"]')).toBeTruthy();
    expect(el.shadowRoot!.querySelector('label[for="fb-feedback-message"]')).toBeTruthy();
  });
});
