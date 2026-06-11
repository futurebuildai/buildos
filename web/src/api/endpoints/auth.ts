/**
 * Auth endpoints — unauthenticated surface under /api/v1/auth
 * (internal/api/auth.go MountAuthRoutes). All use `skipAuth` because they mint
 * the credentials the rest of the API requires.
 *
 * Refresh-token transport: the backend sets an `HttpOnly; Secure;
 * SameSite=Strict; Path=/api/v1/auth` cookie (`buildos_refresh`) on
 * claim/login/refresh AND still returns the token in the JSON body. These four
 * calls set `withCredentials: true` so the browser stores/replays that cookie —
 * which is what survives a reload/deep-link. `refresh()` takes an OPTIONAL token:
 * on a hard reload the SPA calls it with no argument and the browser replays the
 * cookie (silent rehydration); the same-session interceptor still passes the
 * in-memory token. The OTHER API endpoints stay cookie-free (Bearer header).
 */
import { api } from '../client.js';
import type { TokenPair, AuthState } from '../../types/models.js';

export function claimFirstOwner(input: {
  token: string;
  email: string;
  password: string;
  display_name: string;
}): Promise<TokenPair> {
  return api.post<TokenPair>('/api/v1/auth/claim', input, {
    skipAuth: true,
    withCredentials: true,
  });
}

export function login(input: { email: string; password: string }): Promise<TokenPair> {
  return api.post<TokenPair>('/api/v1/auth/login', input, {
    skipAuth: true,
    withCredentials: true,
  });
}

/**
 * Rotate the refresh token. With a token argument the body carries it (the
 * same-session 401 interceptor + proactive refresh path). With NO argument the
 * body is omitted entirely and the backend reads the HttpOnly cookie — this is
 * the boot-time silent rehydration after a hard reload/deep-link.
 */
export function refresh(refreshToken?: string): Promise<TokenPair> {
  const body = refreshToken !== undefined ? { refresh_token: refreshToken } : undefined;
  return api.post<TokenPair>('/api/v1/auth/refresh', body, {
    skipAuth: true,
    withCredentials: true,
  });
}

export function logout(refreshToken?: string): Promise<void> {
  const body = refreshToken !== undefined ? { refresh_token: refreshToken } : undefined;
  return api.post<void>('/api/v1/auth/logout', body, {
    skipAuth: true,
    withCredentials: true,
  });
}

export function requestPasswordReset(email: string): Promise<void> {
  return api.post<void>('/api/v1/auth/password-reset/request', { email }, { skipAuth: true });
}

export function confirmPasswordReset(token: string, password: string): Promise<void> {
  return api.post<void>(
    '/api/v1/auth/password-reset/confirm',
    { token, password },
    { skipAuth: true },
  );
}

/**
 * GET /api/v1/auth/state — cold-start routing probe (OQ-2). NOT yet mounted on
 * the backend; callers must handle a thrown ApiError and fall back to the
 * silent-refresh-then-/login flow (FRONTEND_ARCHITECTURE §3.1 / §10 OQ-2).
 */
export function authState(): Promise<AuthState> {
  return api.get<AuthState>('/api/v1/auth/state', { skipAuth: true });
}
