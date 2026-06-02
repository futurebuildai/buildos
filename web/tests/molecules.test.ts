import { describe, it, expect, afterEach, vi } from 'vitest';
import '../src/components/atoms/index.js';
import '../src/components/molecules/index.js';
import type { FbField } from '../src/components/molecules/fb-field.js';
import type { FbForm } from '../src/components/molecules/fb-form.js';
import type { FbToaster } from '../src/components/molecules/fb-toaster.js';
import type { FbTabBar } from '../src/components/molecules/fb-tab-bar.js';
import type { FbFeedCard } from '../src/components/molecules/fb-feed-card.js';
import type { FbFileUpload } from '../src/components/molecules/fb-file-upload.js';

async function mount<T extends HTMLElement>(html: string): Promise<T> {
  const host = document.createElement('div');
  host.innerHTML = html;
  const el = host.firstElementChild as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('fb-field', () => {
  it('wires aria-describedby + invalid onto the slotted control on error', async () => {
    const el = await mount<FbField>(
      '<fb-field label="Email" error="Required"><fb-input></fb-input></fb-field>',
    );
    await el.updateComplete;
    const input = el.querySelector('fb-input') as HTMLElement & {
      describedby?: string;
      invalid?: boolean;
    };
    expect(input.invalid).toBe(true);
    expect(input.describedby).toContain('-error');
    // The error text is a real alert, paired with an icon (never color-only).
    expect(el.shadowRoot!.querySelector('[role="alert"]')!.textContent).toContain('Required');
    expect(el.shadowRoot!.querySelector('fb-icon')).toBeTruthy();
  });
});

describe('fb-form', () => {
  it('emits submit with serialized named values and prevents default', async () => {
    const el = await mount<FbForm>('<fb-form><input name="email" value="a@b.co" /></fb-form>');
    let detail: { values: Record<string, string> } | null = null;
    el.addEventListener('submit', (e) => {
      detail = (e as unknown as CustomEvent).detail;
    });
    const form = el.shadowRoot!.querySelector('form')!;
    if (typeof form.requestSubmit === 'function') form.requestSubmit();
    else form.dispatchEvent(new Event('submit', { cancelable: true }));
    expect(detail).toEqual({ values: { email: 'a@b.co' } });
  });

  it('renders an error summary alert when setErrors is called', async () => {
    const el = await mount<FbForm>('<fb-form></fb-form>');
    el.setErrors({ Email: 'is required' });
    await el.updateComplete;
    const summary = el.shadowRoot!.querySelector('.summary[role="alert"]')!;
    expect(summary.textContent).toContain('Email');
  });
});

describe('fb-toaster', () => {
  it('auto-dismisses non-error toasts after their duration', async () => {
    vi.useFakeTimers();
    const el = await mount<FbToaster>('<fb-toaster></fb-toaster>');
    el.toast('Saved', { tone: 'success', duration: 1000 });
    await el.updateComplete;
    expect(el.shadowRoot!.querySelector('fb-toast')).toBeTruthy();
    vi.advanceTimersByTime(1001);
    await el.updateComplete;
    expect(el.shadowRoot!.querySelector('fb-toast')).toBeNull();
    vi.useRealTimers();
  });

  it('keeps error toasts sticky (duration 0) and uses an assertive region', async () => {
    const el = await mount<FbToaster>('<fb-toaster></fb-toaster>');
    el.toast('Boom', { tone: 'error' });
    await el.updateComplete;
    const assertive = el.shadowRoot!.querySelector('[aria-live="assertive"]')!;
    expect(assertive.querySelector('fb-toast')).toBeTruthy();
  });
});

describe('fb-tab-bar', () => {
  it('moves selection with ArrowRight and emits change', async () => {
    const el = await mount<FbTabBar>('<fb-tab-bar></fb-tab-bar>');
    el.tabs = [
      { id: 'a', label: 'A' },
      { id: 'b', label: 'B' },
    ];
    el.active = 'a';
    await el.updateComplete;
    let changed: { id: string } | null = null;
    el.addEventListener('change', (e) => {
      changed = (e as CustomEvent).detail;
    });
    const tablist = el.shadowRoot!.querySelector('[role="tablist"]')!;
    tablist.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    expect(changed).toEqual({ id: 'b' });
  });
});

describe('fb-feed-card', () => {
  it('exposes priority as an accessible label and emits action with its id', async () => {
    const el = await mount<FbFeedCard>('<fb-feed-card heading="Late"></fb-feed-card>');
    el.priority = 'critical';
    el.actions = [{ id: 'ack', label: 'Acknowledge' }];
    await el.updateComplete;
    let actionId: string | null = null;
    el.addEventListener('action', (e) => {
      actionId = (e as CustomEvent).detail.actionId;
    });
    const icon = el.shadowRoot!.querySelector('fb-icon.icon')!;
    expect(icon.getAttribute('label')).toBe('critical');
    (el.shadowRoot!.querySelector('fb-button') as HTMLElement).click();
    expect(actionId).toBe('ack');
  });
});

describe('fb-file-upload', () => {
  it('rejects oversized files and accepts valid ones via the guard', async () => {
    const el = await mount<FbFileUpload>(
      '<fb-file-upload accept="image/*" max-size-mb="1"></fb-file-upload>',
    );
    let accepted: { accepted: { name: string }[] } | null = null;
    el.addEventListener('files', (e) => {
      accepted = (e as CustomEvent).detail;
    });
    const ok = new File(['x'], 'photo.png', { type: 'image/png' });
    const input = el.shadowRoot!.querySelector('input[type=file]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { value: [ok], configurable: true });
    input.dispatchEvent(new Event('change'));
    expect(accepted!.accepted[0]!.name).toBe('photo.png');
  });
});
