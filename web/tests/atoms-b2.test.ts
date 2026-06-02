import { describe, it, expect, afterEach } from 'vitest';
import '../src/components/atoms/index.js';
import type { FbMoney } from '../src/components/atoms/fb-money.js';
import type { FbMoneyInput } from '../src/components/atoms/fb-money-input.js';
import type { FbPasswordInput } from '../src/components/atoms/fb-password-input.js';
import type { FbSecretInput } from '../src/components/atoms/fb-secret-input.js';

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

describe('fb-money', () => {
  it('formats cents in the chosen currency, mono+tabular', async () => {
    const el = await mount<FbMoney>('<fb-money cents="1234567" currency-code="USD"></fb-money>');
    expect(el.shadowRoot!.textContent).toContain('$12,345.67');
  });

  it('renders an em-dash placeholder on invalid input rather than crashing', async () => {
    const el = await mount<FbMoney>('<fb-money cents="not-a-number"></fb-money>');
    expect(el.shadowRoot!.textContent).toContain('—');
  });

  it('prefixes an explicit + in variance mode (color is never the only cue)', async () => {
    const el = await mount<FbMoney>('<fb-money cents="500" variance></fb-money>');
    expect(el.shadowRoot!.textContent).toContain('+$5.00');
  });
});

describe('fb-money-input', () => {
  it('emits integer cents (string) on blur, parsed via string math', async () => {
    const el = await mount<FbMoneyInput>('<fb-money-input></fb-money-input>');
    let detail: { cents: string | null; currencyCode: string } | null = null;
    el.addEventListener('change', (e) => {
      detail = (e as CustomEvent).detail;
    });
    const input = el.shadowRoot!.querySelector('input')!;
    input.value = '12.5';
    input.dispatchEvent(new Event('input'));
    input.dispatchEvent(new Event('blur'));
    expect(detail).toEqual({ cents: '1250', currencyCode: 'USD' });
  });
});

describe('fb-password-input', () => {
  it('toggles masking and announces the toggle state', async () => {
    const el = await mount<FbPasswordInput>('<fb-password-input></fb-password-input>');
    const toggle = el.shadowRoot!.querySelector('.toggle') as HTMLButtonElement;
    const input = el.shadowRoot!.querySelector('input')!;
    expect(input.type).toBe('password');
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    toggle.click();
    await el.updateComplete;
    expect(el.shadowRoot!.querySelector('input')!.type).toBe('text');
    expect(el.shadowRoot!.querySelector('.toggle')!.getAttribute('aria-pressed')).toBe('true');
  });
});

describe('fb-secret-input', () => {
  it('shows a write-only masked state when a key already exists (never echoes)', async () => {
    const el = await mount<FbSecretInput>(
      '<fb-secret-input has-value last4="cd34"></fb-secret-input>',
    );
    const group = el.shadowRoot!.querySelector('[aria-label="API key set"]');
    expect(group).toBeTruthy();
    // No input is rendered until the user clicks Replace.
    expect(el.shadowRoot!.querySelector('input')).toBeNull();
  });

  it('validates the 43-char base64url bootstrap token client-side', async () => {
    const el = await mount<FbSecretInput>('<fb-secret-input bootstrap></fb-secret-input>');
    el.value = 'too-short';
    expect(el.valid).toBe(false);
    el.value = 'A'.repeat(43);
    expect(el.valid).toBe(true);
    el.value = '!'.repeat(43); // valid length, invalid charset
    expect(el.valid).toBe(false);
  });

  it('clears its plaintext on submit()', async () => {
    const el = await mount<FbSecretInput>('<fb-secret-input></fb-secret-input>');
    el.value = 'sk-secret-value';
    el.submit();
    expect(el.value).toBe('');
  });
});
