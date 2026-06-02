/**
 * Auth endpoints — unauthenticated surface under /api/v1/auth
 * (internal/api/auth.go MountAuthRoutes). All use `skipAuth` because they mint
 * the credentials the rest of the API requires. Refresh tokens ride in the body
 * (OQ-4 resolved: no cookie transport).
 */
import { api } from '../client.js';
import type { TokenPair, AuthState } from '../../types/models.js';

export function claimFirstOwner(input: {
  token: string;
  email: string;
  password: string;
  display_name: string;
}): Promise<TokenPair> {
  return api.post<TokenPair>('/api/v1/auth/claim', input, { skipAuth: true });
}

export function login(input: { email: string; password: string }): Promise<TokenPair> {
  return api.post<TokenPair>('/api/v1/auth/login', input, { skipAuth: true });
}

export function refresh(refreshToken: string): Promise<TokenPair> {
  return api.post<TokenPair>(
    '/api/v1/auth/refresh',
    { refresh_token: refreshToken },
    { skipAuth: true },
  );
}

export function logout(refreshToken: string): Promise<void> {
  return api.post<void>('/api/v1/auth/logout', { refresh_token: refreshToken }, { skipAuth: true });
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
