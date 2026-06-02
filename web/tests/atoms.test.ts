import { describe, it, expect, afterEach } from 'vitest';
import '../src/components/atoms/index.js';
import type { FbButton } from '../src/components/atoms/fb-button.js';
import type { FbBadge } from '../src/components/atoms/fb-badge.js';
import type { FbRoleBadge } from '../src/components/atoms/fb-role-badge.js';
import type { FbCheckbox } from '../src/components/atoms/fb-checkbox.js';
import type { FbChip } from '../src/components/atoms/fb-chip.js';

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

describe('fb-button', () => {
  it('renders a native button with the slotted label', async () => {
    const el = await mount<FbButton>('<fb-button>Save</fb-button>');
    const btn = el.shadowRoot!.querySelector('button')!;
    expect(btn).toBeTruthy();
    expect(btn.type).toBe('button');
    expect(el.textContent).toContain('Save');
  });

  it('disables and marks busy while loading', async () => {
    const el = await mount<FbButton>('<fb-button>Go</fb-button>');
    el.loading = true;
    await el.updateComplete;
    const btn = el.shadowRoot!.querySelector('button')!;
    expect(btn.disabled).toBe(true);
    expect(btn.getAttribute('aria-busy')).toBe('true');
  });
});

describe('fb-badge', () => {
  it('pairs an icon with the slotted text (never color-only)', async () => {
    const el = await mount<FbBadge>('<fb-badge status="critical">Overdue</fb-badge>');
    expect(el.shadowRoot!.querySelector('fb-icon')).toBeTruthy();
    expect(el.textContent).toContain('Overdue');
  });
});

describe('fb-role-badge', () => {
  it('maps a role to its human label', async () => {
    const el = document.createElement('fb-role-badge') as FbRoleBadge;
    el.role = 'superintendent';
    document.body.appendChild(el);
    await el.updateComplete;
    expect(el.shadowRoot!.textContent).toContain('Superintendent');
    // Must not leak an invalid ARIA role onto the host.
    expect(el.getAttribute('role')).toBeNull();
  });
});

describe('fb-checkbox', () => {
  it('emits change with the new checked state', async () => {
    const el = await mount<FbCheckbox>('<fb-checkbox>Agree</fb-checkbox>');
    let detail: { checked: boolean } | null = null;
    el.addEventListener('change', (e) => {
      detail = (e as CustomEvent).detail;
    });
    const input = el.shadowRoot!.querySelector('input')!;
    input.checked = true;
    input.dispatchEvent(new Event('change'));
    expect(detail).toEqual({ checked: true });
  });
});

describe('fb-chip', () => {
  it('toggles selection and exposes aria-pressed when selectable', async () => {
    const el = await mount<FbChip>('<fb-chip selectable>Active</fb-chip>');
    const chip = el.shadowRoot!.querySelector('.chip')!;
    expect(chip.getAttribute('aria-pressed')).toBe('false');
    (chip as HTMLElement).click();
    await el.updateComplete;
    expect(el.selected).toBe(true);
    expect(el.shadowRoot!.querySelector('.chip')!.getAttribute('aria-pressed')).toBe('true');
  });
});
