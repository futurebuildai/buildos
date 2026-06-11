/**
 * Proves the refresh-cookie wiring at the fetch boundary:
 *  - the auth endpoints (claim/login/refresh/logout) send `credentials:'include'`
 *    so the browser stores/replays the HttpOnly buildos_refresh cookie;
 *  - refresh() with no argument omits the body (cookie-only silent refresh);
 *  - every OTHER API call stays cookie-free (no credentials field).
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import * as authApi from '../src/api/endpoints/auth.js';
import { api } from '../src/api/client.js';

function envelope(data: unknown): Response {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

let fetchMock: ReturnType<typeof vi.fn>;

function lastInit(): RequestInit {
  return fetchMock.mock.calls.at(-1)?.[1] as RequestInit;
}

describe('refresh-cookie fetch wiring', () => {
  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(envelope({}));
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('login sends credentials:include', async () => {
    await authApi.login({ email: 'a@b.c', password: 'pw1234567890' });
    expect(lastInit().credentials).toBe('include');
  });

  it('refresh() with no token omits the body and sends credentials:include', async () => {
    await authApi.refresh();
    const init = lastInit();
    expect(init.credentials).toBe('include');
    expect(init.body).toBeUndefined();
  });

  it('refresh(token) carries the token in the body (same-session path)', async () => {
    await authApi.refresh('in-memory-token');
    const init = lastInit();
    expect(init.credentials).toBe('include');
    expect(JSON.parse(init.body as string)).toEqual({ refresh_token: 'in-memory-token' });
  });

  it('logout sends credentials:include so the server can clear the cookie', async () => {
    await authApi.logout();
    expect(lastInit().credentials).toBe('include');
  });

  it('non-auth API calls do NOT send credentials (Bearer-header surface stays cookie-free)', async () => {
    await api.get('/api/v1/projects');
    expect(lastInit().credentials).toBeUndefined();
  });
});
