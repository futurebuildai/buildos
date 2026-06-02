import { describe, it, expect } from 'vitest';
import { decodeAccessToken, roleAtLeast, roleIn } from '../src/auth/jwt.js';

/** Builds an unsigned JWT-shaped string with the given payload (test-only). */
function makeToken(payload: Record<string, unknown>): string {
  const b64url = (obj: Record<string, unknown>) =>
    btoa(JSON.stringify(obj)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64url({ alg: 'RS256', typ: 'JWT' })}.${b64url(payload)}.sig`;
}

describe('decodeAccessToken', () => {
  it('decodes sub/org_id/role/plan_tier and exp→ms', () => {
    const token = makeToken({
      sub: 'user-1',
      org_id: 'org-9',
      role: 'owner',
      plan_tier: 'pro',
      exp: 1_700_000_000,
    });
    const claims = decodeAccessToken(token);
    expect(claims).not.toBeNull();
    expect(claims?.sub).toBe('user-1');
    expect(claims?.orgId).toBe('org-9');
    expect(claims?.role).toBe('owner');
    expect(claims?.planTier).toBe('pro');
    expect(claims?.expiresAtMs).toBe(1_700_000_000_000);
  });

  it('returns null for malformed tokens', () => {
    expect(decodeAccessToken('not-a-jwt')).toBeNull();
    expect(decodeAccessToken('a.b')).toBeNull();
  });

  it('returns null when required claims are missing', () => {
    const token = makeToken({ sub: 'u', role: 'owner' }); // no org_id
    expect(decodeAccessToken(token)).toBeNull();
  });
});

describe('role precedence', () => {
  it('roleAtLeast respects owner>admin>super>field', () => {
    expect(roleAtLeast('owner', 'admin')).toBe(true);
    expect(roleAtLeast('superintendent', 'admin')).toBe(false);
    expect(roleAtLeast('admin', 'admin')).toBe(true);
  });

  it('roleIn matches exact allow-lists', () => {
    expect(roleIn('owner', ['owner', 'admin'])).toBe(true);
    expect(roleIn('superintendent', ['owner', 'admin'])).toBe(false);
  });
});
