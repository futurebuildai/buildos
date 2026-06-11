import { describe, it, expect, afterEach } from 'vitest';
import '../src/components/atoms/index.js';
import type { FbButton } from '../src/components/atoms/fb-button.js';
import type { FbBadge } from '../src/components/atoms/fb-badge.js';
import type { FbRoleBadge } from '../src/components/atoms/fb-role-badge.js';
import type { FbCheckbox } from '../src/components/atoms/fb-checkbox.js';
import type { FbChip } from '../src/components/atoms/fb-chip.js';
import type { FbLogo } from '../src/components/atoms/fb-logo.js';

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

describe('fb-logo', () => {
  it('renders mark + wordmark by default and labels the host as the BuildOS image', async () => {
    const el = await mount<FbLogo>('<fb-logo></fb-logo>');
    // Host carries the single accessible name.
    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBe('BuildOS');
    const svg = el.shadowRoot!.querySelector('svg')!;
    expect(svg).toBeTruthy();
    expect(svg.getAttribute('viewBox')).toBe('0 0 1857 1606');
    expect(svg.querySelector('path')).toBeTruthy();
    // Wordmark visible with "OS" emphasized.
    const wordmark = el.shadowRoot!.querySelector('.wordmark')!;
    expect(wordmark.textContent).toBe('BuildOS');
    expect(wordmark.querySelector('.os')!.textContent).toBe('OS');
  });

  it('does not double-announce: with the wordmark shown the mark is decorative', async () => {
    const el = await mount<FbLogo>('<fb-logo variant="full"></fb-logo>');
    const svg = el.shadowRoot!.querySelector('svg')!;
    // Mark hidden from AT (host label is the sole "BuildOS"); wordmark hidden too.
    expect(svg.getAttribute('aria-hidden')).toBe('true');
    expect(svg.getAttribute('role')).toBe('presentation');
    expect(svg.hasAttribute('aria-label')).toBe(false);
    expect(el.shadowRoot!.querySelector('.wordmark')!.getAttribute('aria-hidden')).toBe('true');
  });

  it('variant="mark" renders only the mark (no wordmark) and the mark names itself', async () => {
    const el = await mount<FbLogo>('<fb-logo variant="mark"></fb-logo>');
    expect(el.shadowRoot!.querySelector('.wordmark')).toBeNull();
    const svg = el.shadowRoot!.querySelector('svg')!;
    expect(svg).toBeTruthy();
    // Sole content: the mark itself carries the accessible name.
    expect(svg.getAttribute('role')).toBe('img');
    expect(svg.getAttribute('aria-label')).toBe('BuildOS');
    expect(svg.hasAttribute('aria-hidden')).toBe(false);
  });

  it('variant="wordmark" renders only the wordmark (no mark svg)', async () => {
    const el = await mount<FbLogo>('<fb-logo variant="wordmark"></fb-logo>');
    expect(el.shadowRoot!.querySelector('svg')).toBeNull();
    expect(el.shadowRoot!.querySelector('.wordmark')!.textContent).toBe('BuildOS');
    // Host still provides the accessible name.
    expect(el.getAttribute('aria-label')).toBe('BuildOS');
  });

  it('scales the mark width by the intrinsic aspect ratio for a given size', async () => {
    const el = await mount<FbLogo>('<fb-logo variant="mark" size="100"></fb-logo>');
    const svg = el.shadowRoot!.querySelector('svg')!;
    expect(svg.getAttribute('height')).toBe('100');
    // 1857 / 1606 ≈ 1.1563 → width ≈ 115.63
    expect(Number(svg.getAttribute('width'))).toBeCloseTo(115.63, 1);
  });
});
