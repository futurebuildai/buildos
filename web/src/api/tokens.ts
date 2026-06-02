/**
 * Token storage (web).
 *
 * OQ-4 resolution: the backend (internal/api/auth.go) carries the refresh token
 * in the JSON request/response BODY, not an HttpOnly cookie. Per
 * FRONTEND_ARCHITECTURE §4.1, refresh tokens MUST NOT be persisted to
 * localStorage/sessionStorage (XSS exfiltration risk). Therefore BOTH tokens are
 * held in memory only. A hard reload loses the session and the boot flow falls
 * back to /login (the documented fallback). If the backend later adds
 * `HttpOnly; Secure; SameSite=Strict` cookie transport, swap this module's impl
 * — it is the single choke-point the rest of the app depends on.
 *
 * The access token is never logged (PII-Restricted; CLAUDE.md PII handling).
 */

interface TokenState {
  accessToken: string | null;
  refreshToken: string | null;
  /** Epoch milliseconds at which the access token expires. */
  expiresAtMs: number | null;
}

const state: TokenState = {
  accessToken: null,
  refreshToken: null,
  expiresAtMs: null,
};

export function setTokens(args: {
  accessToken: string;
  refreshToken: string;
  /** Seconds until expiry, as returned by the backend (`expires_in`). */
  expiresIn: number;
}): void {
  state.accessToken = args.accessToken;
  state.refreshToken = args.refreshToken;
  state.expiresAtMs = Date.now() + args.expiresIn * 1000;
}

export function clearTokens(): void {
  state.accessToken = null;
  state.refreshToken = null;
  state.expiresAtMs = null;
}

export function getAccessToken(): string | null {
  return state.accessToken;
}

export function getRefreshToken(): string | null {
  return state.refreshToken;
}

export function hasRefreshToken(): boolean {
  return state.refreshToken !== null;
}

export function getExpiresAtMs(): number | null {
  return state.expiresAtMs;
}

/** Milliseconds until the access token expires (negative if already expired). */
export function msUntilExpiry(): number {
  if (state.expiresAtMs === null) return -1;
  return state.expiresAtMs - Date.now();
}
