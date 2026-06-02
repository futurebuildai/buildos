import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  setTokens,
  clearTokens,
  getAccessToken,
  getRefreshToken,
  hasRefreshToken,
  getExpiresAtMs,
  msUntilExpiry,
} from '../src/api/tokens.js';

describe('token store', () => {
  beforeEach(() => clearTokens());
  afterEach(() => {
    clearTokens();
    vi.useRealTimers();
  });

  it('starts empty', () => {
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
    expect(hasRefreshToken()).toBe(false);
    expect(getExpiresAtMs()).toBeNull();
    expect(msUntilExpiry()).toBe(-1);
  });

  it('stores tokens and computes expiry from expires_in seconds', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));
    setTokens({ accessToken: 'a', refreshToken: 'r', expiresIn: 900 });
    expect(getAccessToken()).toBe('a');
    expect(getRefreshToken()).toBe('r');
    expect(hasRefreshToken()).toBe(true);
    expect(getExpiresAtMs()).toBe(Date.parse('2026-01-01T00:00:00.000Z') + 900_000);
    expect(msUntilExpiry()).toBe(900_000);
  });

  it('reports negative msUntilExpiry once past expiry', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));
    setTokens({ accessToken: 'a', refreshToken: 'r', expiresIn: 10 });
    vi.setSystemTime(new Date('2026-01-01T00:00:20.000Z'));
    expect(msUntilExpiry()).toBeLessThan(0);
  });

  it('clears all state', () => {
    setTokens({ accessToken: 'a', refreshToken: 'r', expiresIn: 900 });
    clearTokens();
    expect(getAccessToken()).toBeNull();
    expect(hasRefreshToken()).toBe(false);
    expect(getExpiresAtMs()).toBeNull();
  });
});
