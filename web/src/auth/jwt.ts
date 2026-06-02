/**
 * Minimal JWT claim decoding (no signature verification — that is the server's
 * job; internal/auth/token.go is both issuer and verifier). The client decodes
 * the access token only to mirror RBAC for UX (FRONTEND_ARCHITECTURE §6.2).
 *
 * Claim shape from internal/auth/token.go TokenClaims:
 *   { sub, org_id, role, plan_tier?, iss, aud, exp, iat, nbf }
 */

export type Role = 'owner' | 'admin' | 'superintendent' | 'field_worker';
export type PlanTier = 'free' | 'pro' | 'enterprise';

export interface AccessClaims {
  sub: string;
  orgId: string;
  role: Role;
  planTier: PlanTier | undefined;
  /** Expiry as epoch milliseconds (derived from the `exp` second-claim). */
  expiresAtMs: number | undefined;
}

interface RawClaims {
  sub?: string;
  org_id?: string;
  role?: string;
  plan_tier?: string;
  exp?: number;
}

/** Decodes a base64url segment to a UTF-8 string. */
function base64UrlDecode(segment: string): string {
  const padded = segment.replace(/-/g, '+').replace(/_/g, '/');
  const base64 = padded + '='.repeat((4 - (padded.length % 4)) % 4);
  const binary = atob(base64);
  // Reconstruct UTF-8 (atob yields a binary string).
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

/**
 * Decodes the payload of a JWT access token. Returns null on any malformed
 * input — callers treat null as "not authenticated".
 */
export function decodeAccessToken(token: string): AccessClaims | null {
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  const payloadSegment = parts[1];
  if (!payloadSegment) return null;
  let raw: RawClaims;
  try {
    raw = JSON.parse(base64UrlDecode(payloadSegment)) as RawClaims;
  } catch {
    return null;
  }
  if (!raw.sub || !raw.org_id || !raw.role) return null;
  return {
    sub: raw.sub,
    orgId: raw.org_id,
    role: raw.role as Role,
    planTier: raw.plan_tier as PlanTier | undefined,
    expiresAtMs: typeof raw.exp === 'number' ? raw.exp * 1000 : undefined,
  };
}

/** Role precedence: owner > admin > superintendent > field_worker. */
const ROLE_RANK: Record<Role, number> = {
  owner: 3,
  admin: 2,
  superintendent: 1,
  field_worker: 0,
};

/** True when `role` is at least `min` in the precedence order. */
export function roleAtLeast(role: Role, min: Role): boolean {
  return ROLE_RANK[role] >= ROLE_RANK[min];
}

/** True when `role` is exactly one of `allowed`. */
export function roleIn(role: Role, allowed: readonly Role[]): boolean {
  return allowed.includes(role);
}
