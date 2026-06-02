/**
 * authStore — the single source of truth for session state (Lit Signals).
 *
 * Holds the decoded access-token claims (sub/org_id/role/plan_tier), drives the
 * RBAC mirror, and owns the refresh + logout side-effects. Wires the injected
 * refresh handler into the API client so the 401 interceptor (client.ts §4.2)
 * can call back here single-flight.
 *
 * Multi-tab logout uses BroadcastChannel (UX_AUTH_ONBOARDING §4): when one tab
 * logs out (or a refresh dies), other tabs clear their session too.
 */
import { signal, computed } from '@lit-labs/signals';
import {
  setTokens,
  clearTokens,
  getRefreshToken,
  getAccessToken,
  hasRefreshToken,
  msUntilExpiry,
} from '../api/tokens.js';
import { setRefreshHandler } from '../api/client.js';
import {
  decodeAccessToken,
  roleAtLeast,
  roleIn,
  type AccessClaims,
  type Role,
} from '../auth/jwt.js';
import * as authApi from '../api/endpoints/auth.js';
import type { TokenPair } from '../types/models.js';

export type AuthStatus = 'unknown' | 'authenticated' | 'anonymous';

const claimsSignal = signal<AccessClaims | null>(null);
const statusSignal = signal<AuthStatus>('unknown');

export const authClaims = computed(() => claimsSignal.get());
export const authStatus = computed(() => statusSignal.get());
export const isAuthenticated = computed(() => statusSignal.get() === 'authenticated');
export const currentRole = computed<Role | null>(() => claimsSignal.get()?.role ?? null);

/** Channel for cross-tab session coordination. Guarded for non-browser test envs. */
const channel =
  typeof BroadcastChannel !== 'undefined' ? new BroadcastChannel('buildos-auth') : null;

let refreshTimer: ReturnType<typeof setTimeout> | null = null;

/** Applies a fresh token pair: stores tokens, decodes claims, schedules refresh. */
function applyTokenPair(pair: TokenPair): void {
  setTokens({
    accessToken: pair.access_token,
    refreshToken: pair.refresh_token,
    expiresIn: pair.expires_in,
  });
  claimsSignal.set(decodeAccessToken(pair.access_token));
  statusSignal.set('authenticated');
  scheduleProactiveRefresh();
}

/** Clears all session state locally (no server call, no broadcast). */
function clearLocal(): void {
  if (refreshTimer) {
    clearTimeout(refreshTimer);
    refreshTimer = null;
  }
  clearTokens();
  claimsSignal.set(null);
  statusSignal.set('anonymous');
}

/**
 * Proactive refresh at T-60s (UX_AUTH_ONBOARDING §3) so the user never hits a
 * reactive 401 mid-action. Falls back to the interceptor if this misses.
 */
function scheduleProactiveRefresh(): void {
  if (refreshTimer) clearTimeout(refreshTimer);
  const lead = 60_000;
  const delay = Math.max(msUntilExpiry() - lead, 0);
  refreshTimer = setTimeout(() => {
    void refreshSession();
  }, delay);
}

/**
 * Single attempt to rotate the refresh token. Returns true on success. On
 * failure clears the session and broadcasts logout. The API client calls this
 * single-flight (it dedupes concurrent callers).
 */
export async function refreshSession(): Promise<boolean> {
  const token = getRefreshToken();
  if (!token) {
    handleSessionDeath();
    return false;
  }
  try {
    const pair = await authApi.refresh(token);
    applyTokenPair(pair);
    return true;
  } catch {
    handleSessionDeath();
    return false;
  }
}

/** Session is dead: clear locally + tell other tabs. */
function handleSessionDeath(): void {
  clearLocal();
  channel?.postMessage({ type: 'logout' });
}

export async function login(email: string, password: string): Promise<void> {
  const pair = await authApi.login({ email, password });
  applyTokenPair(pair);
}

export async function claimFirstOwner(input: {
  token: string;
  email: string;
  password: string;
  display_name: string;
}): Promise<void> {
  const pair = await authApi.claimFirstOwner(input);
  applyTokenPair(pair);
}

export async function logout(): Promise<void> {
  const token = getRefreshToken();
  if (token) {
    try {
      await authApi.logout(token);
    } catch {
      // Best-effort server-side revocation; clear locally regardless.
    }
  }
  clearLocal();
  channel?.postMessage({ type: 'logout' });
}

/**
 * Cold-start: attempt a silent refresh if we somehow still hold a refresh token
 * in memory (rare on a hard reload given in-memory storage), else mark anonymous.
 * The actual cold-start ROUTING decision (login vs first-run) lives in the
 * router, which may also consult GET /auth/state when the backend adds it.
 */
export async function initSession(): Promise<void> {
  if (hasRefreshToken()) {
    const ok = await refreshSession();
    statusSignal.set(ok ? 'authenticated' : 'anonymous');
  } else {
    statusSignal.set('anonymous');
  }
}

// ---- RBAC mirror helpers (UX-only; server is authoritative) ----
export function hasMinRole(min: Role): boolean {
  const role = claimsSignal.get()?.role;
  return role ? roleAtLeast(role, min) : false;
}
export function hasRole(...allowed: Role[]): boolean {
  const role = claimsSignal.get()?.role;
  return role ? roleIn(role, allowed) : false;
}
export function isPro(): boolean {
  return claimsSignal.get()?.planTier === 'pro' || claimsSignal.get()?.planTier === 'enterprise';
}

// Wire the interceptor + cross-tab listener exactly once at module load.
setRefreshHandler(refreshSession);
if (channel) {
  channel.onmessage = (e: MessageEvent) => {
    if (e.data?.type === 'logout' && getAccessToken()) {
      clearLocal();
    }
  };
}
