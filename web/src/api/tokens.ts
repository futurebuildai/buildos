/**
 * Token storage (web).
 *
 * Persistence model (updated): the backend (internal/api/auth.go) now sets the
 * refresh token as an `HttpOnly; Secure; SameSite=Strict; Path=/api/v1/auth`
 * cookie (`buildos_refresh`) on claim/login/refresh, in ADDITION to returning it
 * in the JSON body. The cookie is the source of truth ACROSS reloads/deep-links:
 * the SPA holds the access token in memory only (never localStorage —
 * FRONTEND_ARCHITECTURE §4.1, XSS exfiltration risk), and on boot it POSTs
 * /api/v1/auth/refresh with NO body so the browser replays the cookie
 * (initSession() in authStore.ts). The in-memory refresh token below is kept
 * only as a convenience for the SAME-SESSION 401→refresh→retry interceptor; it
 * is NOT what survives a reload (it can't — memory is cleared) and is no longer
 * load-bearing for persistence.
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
