import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// The endpoint modules are mocked so pages never hit the network. authStore is
// real (it calls these mocked endpoints), so login/claim error branches flow
// through the store exactly as in production.
vi.mock('../src/api/endpoints/auth.js', () => ({
  login: vi.fn(),
  claimFirstOwner: vi.fn(),
  refresh: vi.fn(),
  logout: vi.fn(),
  requestPasswordReset: vi.fn(),
  confirmPasswordReset: vi.fn(),
  authState: vi.fn(),
}));
vi.mock('../src/api/endpoints/setup.js', () => ({
  getSetupState: vi.fn(),
  updateCompanyInfo: vi.fn(),
  createTrade: vi.fn(),
  createCostCode: vi.fn(),
  createCalendar: vi.fn(),
  addHoliday: vi.fn(),
  addJurisdiction: vi.fn(),
  completeSetup: vi.fn(),
}));
vi.mock('../src/api/endpoints/integrations.js', () => ({
  listIntegrations: vi.fn(),
  setCredential: vi.fn(),
  deleteCredential: vi.fn(),
}));

import '../src/components/pages/index.js';
import * as authApi from '../src/api/endpoints/auth.js';
import * as setupApi from '../src/api/endpoints/setup.js';
import * as integrationsApi from '../src/api/endpoints/integrations.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import type { SetupState } from '../src/types/models.js';

async function mount<T extends HTMLElement>(
  tag: string,
  attrs: Record<string, string> = {},
): Promise<T> {
  const el = document.createElement(tag) as T;
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

function submit(page: HTMLElement, values: Record<string, string>): void {
  const form = page.shadowRoot!.querySelector('fb-form')!;
  form.dispatchEvent(new CustomEvent('submit', { detail: { values } }));
}

/** Flush the microtask queue (mocked async handlers) then the Lit render. */
async function flush(page: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

function text(page: HTMLElement, selector: string): string {
  return page.shadowRoot!.querySelector(selector)?.textContent?.trim() ?? '';
}

function apiError(code: string, status = 400): ApiError {
  return new ApiError({ code, message: code, status });
}

const EMPTY_STATE: SetupState = {
  org_id: 'org-1',
  onboarding_complete: false,
  company_profile: { onboarding_complete: false },
  trades: [],
  cost_codes: [],
  permit_jurisdictions: [],
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(setupApi.getSetupState).mockResolvedValue(EMPTY_STATE);
  vi.mocked(integrationsApi.listIntegrations).mockResolvedValue([]);
});

afterEach(() => {
  document.body.innerHTML = '';
  window.history.replaceState({}, '', '/');
});

describe('fb-login-page', () => {
  it('renders the email + password fields and the forgot-password link', async () => {
    const el = await mount('fb-login-page');
    const root = el.shadowRoot!;
    expect(root.querySelector('fb-input[name="email"]')).not.toBeNull();
    expect(root.querySelector('fb-password-input[name="password"]')).not.toBeNull();
    expect(root.querySelector('a[href="/forgot-password"]')).not.toBeNull();
    expect(text(el, '.auth-title')).toContain('Sign in to BuildOS');
  });

  it('shows uniform copy for invalid credentials (no field oracle)', async () => {
    vi.mocked(authApi.login).mockRejectedValueOnce(apiError(ErrorCode.INVALID_CREDENTIALS, 401));
    const el = await mount('fb-login-page');
    submit(el, { email: 'a@b.co', password: 'wrong' });
    await flush(el);
    expect(text(el, '.auth-error')).toContain('Email or password is incorrect.');
  });

  it('maps a non-credential API error to generic copy', async () => {
    vi.mocked(authApi.login).mockRejectedValueOnce(apiError(ErrorCode.SERVICE_UNAVAILABLE, 503));
    const el = await mount('fb-login-page');
    submit(el, { email: 'a@b.co', password: 'pw' });
    await flush(el);
    expect(text(el, '.auth-error')).toContain('Something went wrong on our end.');
  });

  it('validates that both fields are present before calling the API', async () => {
    const el = await mount('fb-login-page');
    submit(el, { email: '', password: '' });
    await flush(el);
    expect(text(el, '.auth-error')).toContain('Enter your email and password.');
    expect(authApi.login).not.toHaveBeenCalled();
  });
});

describe('fb-first-run-page', () => {
  it('renders the bootstrap token + owner fields', async () => {
    const el = await mount('fb-first-run-page');
    const root = el.shadowRoot!;
    expect(root.querySelector('fb-secret-input[name="token"]')).not.toBeNull();
    expect(root.querySelector('fb-input[name="display_name"]')).not.toBeNull();
    expect(root.querySelector('fb-input[name="email"]')).not.toBeNull();
    expect(root.querySelectorAll('fb-password-input').length).toBe(2);
  });

  it('rejects a password/confirm mismatch before calling the API', async () => {
    const el = await mount('fb-first-run-page');
    submit(el, {
      token: 'tok',
      display_name: 'Owner',
      email: 'o@b.co',
      password: 'aaaaaaaa',
      confirm: 'bbbbbbbb',
    });
    await flush(el);
    expect(text(el, '.auth-error')).toContain('don’t match');
    expect(authApi.claimFirstOwner).not.toHaveBeenCalled();
  });

  it('shows a terminal state when an owner already exists', async () => {
    vi.mocked(authApi.claimFirstOwner).mockRejectedValueOnce(
      apiError(ErrorCode.FIRST_OWNER_EXISTS, 409),
    );
    const el = await mount('fb-first-run-page');
    submit(el, {
      token: 'tok',
      display_name: 'Owner',
      email: 'o@b.co',
      password: 'aaaaaaaa',
      confirm: 'aaaaaaaa',
    });
    await flush(el);
    expect(text(el, '.auth-title')).toContain('already set up');
    expect(el.shadowRoot!.querySelector('.auth-error')).toBeNull();
  });
});

describe('fb-forgot-password-page (enumeration-safe)', () => {
  it('shows the neutral confirmation on success', async () => {
    vi.mocked(authApi.requestPasswordReset).mockResolvedValueOnce(undefined);
    const el = await mount('fb-forgot-password-page');
    submit(el, { email: 'real@b.co' });
    await flush(el);
    expect(text(el, '.auth-notice')).toContain('If an account exists for that email');
  });

  it('shows the SAME confirmation even when the request errors', async () => {
    vi.mocked(authApi.requestPasswordReset).mockRejectedValueOnce(
      apiError(ErrorCode.NOT_FOUND, 404),
    );
    const el = await mount('fb-forgot-password-page');
    submit(el, { email: 'nobody@b.co' });
    await flush(el);
    expect(text(el, '.auth-notice')).toContain('If an account exists for that email');
  });
});

describe('fb-reset-password-page', () => {
  it('captures and scrubs the token from the URL on connect', async () => {
    window.history.replaceState({}, '', '/reset-password?token=secret-123');
    const el = await mount('fb-reset-password-page');
    // Token must not linger in the address bar.
    expect(window.location.search).toBe('');
    expect(text(el, '.auth-title')).toContain('Set a new password');
  });

  it('shows the incomplete-link state when no token is present', async () => {
    window.history.replaceState({}, '', '/reset-password');
    const el = await mount('fb-reset-password-page');
    expect(text(el, '.auth-title')).toContain('Reset link incomplete');
  });

  it('rejects a password/confirm mismatch before calling the API', async () => {
    window.history.replaceState({}, '', '/reset-password?token=abc');
    const el = await mount('fb-reset-password-page');
    submit(el, { password: 'aaaaaaaa', confirm: 'zzzzzzzz' });
    await flush(el);
    expect(text(el, '.auth-error')).toContain('don’t match');
    expect(authApi.confirmPasswordReset).not.toHaveBeenCalled();
  });
});

describe('fb-use-mobile-page', () => {
  it('renders the field-worker landing with a sign-out action', async () => {
    const el = await mount('fb-use-mobile-page');
    expect(text(el, '.auth-title')).toContain('Use the BuildOS mobile app');
    expect(el.shadowRoot!.querySelector('fb-button')).not.toBeNull();
  });
});

describe('fb-integrations-page (BYOK)', () => {
  it('lists the Anthropic and Resend providers with off-notices when no keys are set', async () => {
    const el = await mount('fb-integrations-page');
    await flush(el);
    const cards = el.shadowRoot!.querySelectorAll('fb-integration-card');
    const providers = Array.from(cards).map((c) => c.getAttribute('provider'));
    expect(providers).toContain('Anthropic');
    expect(providers).toContain('Resend');
    expect(el.shadowRoot!.querySelectorAll('.off').length).toBe(2);
  });
});

describe('fb-setup-page (wizard)', () => {
  it('renders the stepper after loading server state', async () => {
    const el = await mount('fb-setup-page', { step: 'company' });
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-wizard-stepper')).not.toBeNull();
    expect(setupApi.getSetupState).toHaveBeenCalled();
  });
});
