/**
 * authStore session-persistence tests — the HttpOnly refresh-cookie rehydration.
 *
 * The bug: on a hard reload / deep-link the in-memory tokens are gone, so boot
 * fell back to /login. The fix: initSession() POSTs /api/v1/auth/refresh with NO
 * body; the browser replays the HttpOnly buildos_refresh cookie, and a returned
 * token pair rehydrates the in-memory access token + schedules proactive refresh.
 *
 * Here we mock the auth-endpoints module (the cookie itself is browser-managed
 * and invisible to JS — we assert the call shape + the resulting session state).
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../src/api/endpoints/auth.js', () => ({
  refresh: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  claimFirstOwner: vi.fn(),
}));

import * as authApi from '../src/api/endpoints/auth.js';
import { initSession, authStatus, authClaims, refreshSession } from '../src/state/authStore.js';
import { getAccessToken, getRefreshToken, clearTokens } from '../src/api/tokens.js';
import type { TokenPair } from '../src/types/models.js';

/** Unsigned JWT-shaped token (test-only; client never verifies the signature). */
function makeToken(payload: Record<string, unknown>): string {
  const b64url = (obj: Record<string, unknown>) =>
    btoa(JSON.stringify(obj)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64url({ alg: 'RS256', typ: 'JWT' })}.${b64url(payload)}.sig`;
}

function tokenPair(): TokenPair {
  return {
    access_token: makeToken({
      sub: 'user-1',
      org_id: 'org-9',
      role: 'owner',
      exp: Math.floor(Date.now() / 1000) + 900,
    }),
    token_type: 'Bearer',
    expires_in: 900,
    refresh_token: 'rotated-refresh-token',
    user: {} as TokenPair['user'],
  };
}

const mockRefresh = vi.mocked(authApi.refresh);

describe('initSession — cookie-based silent rehydration on boot', () => {
  beforeEach(() => {
    clearTokens();
    mockRefresh.mockReset();
  });
  afterEach(() => {
    clearTokens();
    vi.useRealTimers();
  });

  it('POSTs refresh with NO body (cookie transport) when memory is empty', async () => {
    mockRefresh.mockResolvedValue(tokenPair());
    expect(getRefreshToken()).toBeNull(); // simulate a hard reload — memory empty

    await initSession();

    // Called with undefined → endpoint omits the body so the browser replays
    // the HttpOnly cookie.
    expect(mockRefresh).toHaveBeenCalledTimes(1);
    expect(mockRefresh).toHaveBeenCalledWith(undefined);
  });

  it('hydrates the in-memory access token + authenticated status on success', async () => {
    mockRefresh.mockResolvedValue(tokenPair());

    await initSession();

    expect(authStatus.get()).toBe('authenticated');
    expect(getAccessToken()).not.toBeNull();
    expect(authClaims.get()?.sub).toBe('user-1');
    expect(authClaims.get()?.role).toBe('owner');
  });

  it('falls back to anonymous when the cookie is missing/expired (refresh 401s)', async () => {
    mockRefresh.mockRejectedValue(new Error('401'));

    await initSession();

    expect(authStatus.get()).toBe('anonymous');
    expect(getAccessToken()).toBeNull();
    expect(authClaims.get()).toBeNull();
  });
});

describe('refreshSession transport selection', () => {
  beforeEach(() => {
    clearTokens();
    mockRefresh.mockReset();
  });
  afterEach(() => {
    clearTokens();
    vi.useRealTimers();
  });

  it('uses cookie transport (no body) when no in-memory token', async () => {
    mockRefresh.mockResolvedValue(tokenPair());
    const ok = await refreshSession();
    expect(ok).toBe(true);
    expect(mockRefresh).toHaveBeenCalledWith(undefined);
  });

  it('clears session and returns false when refresh fails', async () => {
    mockRefresh.mockRejectedValue(new Error('dead'));
    const ok = await refreshSession();
    expect(ok).toBe(false);
    expect(authStatus.get()).toBe('anonymous');
  });
});
