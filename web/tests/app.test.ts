import { describe, it, expect, beforeEach, vi } from 'vitest';
import '../src/components/app/fb-app.js';
import { navigate, currentRoute } from '../src/router.js';
import { login } from '../src/state/authStore.js';

/** Builds an unsigned JWT-shaped string with the given payload (test-only). */
function makeToken(payload: Record<string, unknown>): string {
  const b64url = (obj: Record<string, unknown>) =>
    btoa(JSON.stringify(obj)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64url({ alg: 'RS256', typ: 'JWT' })}.${b64url(payload)}.sig`;
}

// Stub the auth endpoint so `login()` resolves offline with owner claims — enough
// for the router's `isAuthenticated` gate to admit the org-shell routes below.
vi.mock('../src/api/endpoints/auth.js', () => ({
  login: vi.fn(async () => ({
    access_token: makeToken({
      sub: 'owner-1',
      org_id: 'org-1',
      role: 'owner',
      plan_tier: 'pro',
      exp: Math.floor(Date.now() / 1000) + 3600,
    }),
    refresh_token: 'refresh-token',
    expires_in: 3600,
  })),
}));

/**
 * Phase F: the density preference (DSC §2.3) must survive a reload. fb-app reads
 * it from localStorage at construction and mirrors it onto `data-density`, the
 * ancestor the density tokens cascade from. With no route resolved yet the app
 * renders its loading canvas, which is enough to assert the attribute wiring.
 */
async function mountApp(): Promise<HTMLElement> {
  const el = document.createElement('fb-app');
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

describe('fb-app density persistence', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    localStorage.clear();
  });

  it('defaults to comfortable when nothing is stored', async () => {
    const el = await mountApp();
    expect(el.getAttribute('data-density')).toBe('comfortable');
  });

  it('restores a persisted compact preference on boot', async () => {
    localStorage.setItem('fb-density', 'compact');
    const el = await mountApp();
    expect(el.getAttribute('data-density')).toBe('compact');
  });

  it('ignores a corrupt stored value and falls back to comfortable', async () => {
    localStorage.setItem('fb-density', 'banana');
    const el = await mountApp();
    expect(el.getAttribute('data-density')).toBe('comfortable');
  });
});

/**
 * Regression: the org shell must translate the composed `navigate` event into a
 * router navigation. `fb-nav-item` (and breadcrumbs) call preventDefault on the
 * anchor and emit `navigate` instead — the router's document-level click
 * interceptor can't see those anchors because they live in `fb-nav-item`'s
 * shadow root, so the click target is retargeted to the host. If `fb-app` does
 * not wire `@navigate` on `fb-org-shell`, the entire nav rail goes dead. The
 * live E2E caught this; this locks it in backend-free.
 */
describe('fb-app shell navigation', () => {
  beforeEach(async () => {
    document.body.innerHTML = '';
    await login('owner@example.com', 'pw'); // authenticate so org routes resolve (not /login)
    navigate('/portfolio/projects'); // an org-shell route → fb-app renders fb-org-shell
  });

  it('routes when the shell emits a composed navigate event', async () => {
    const el = await mountApp();
    const shell = el.shadowRoot!.querySelector('fb-org-shell');
    expect(shell, 'fb-app should render fb-org-shell for an org route').not.toBeNull();

    shell!.dispatchEvent(
      new CustomEvent('navigate', {
        detail: { href: '/command/schedule' },
        bubbles: true,
        composed: true,
      }),
    );

    expect(currentRoute.get()?.path).toBe('/command/schedule');
  });
});
