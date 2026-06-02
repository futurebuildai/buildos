import { describe, it, expect } from 'vitest';
import { ApiError, ErrorCode, userMessageForCode } from '../src/api/errors.js';

describe('ApiError', () => {
  it('defaults details to [] when omitted', () => {
    const e = new ApiError({ code: ErrorCode.NOT_FOUND, message: 'x', status: 404 });
    expect(e.details).toEqual([]);
    expect(e.name).toBe('ApiError');
    expect(e instanceof Error).toBe(true);
  });

  it('flags AI_UNCONFIGURED as a soft notice, not transient', () => {
    const e = new ApiError({ code: ErrorCode.AI_UNCONFIGURED, message: 'x', status: 503 });
    expect(e.isAiUnconfigured).toBe(true);
    expect(e.isTransient).toBe(false);
  });

  it('flags SETUP_INCOMPLETE', () => {
    const e = new ApiError({ code: ErrorCode.SETUP_INCOMPLETE, message: 'x', status: 403 });
    expect(e.isSetupIncomplete).toBe(true);
  });

  it('treats 5xx and known upstream codes as transient', () => {
    expect(
      new ApiError({ code: ErrorCode.SERVICE_UNAVAILABLE, message: 'x', status: 503 }).isTransient,
    ).toBe(true);
    expect(
      new ApiError({ code: ErrorCode.UPSTREAM_ERROR, message: 'x', status: 502 }).isTransient,
    ).toBe(true);
    expect(new ApiError({ code: 'SOME_UNKNOWN_5XX', message: 'x', status: 500 }).isTransient).toBe(
      true,
    );
    expect(
      new ApiError({ code: ErrorCode.VALIDATION_ERROR, message: 'x', status: 400 }).isTransient,
    ).toBe(false);
  });
});

describe('userMessageForCode', () => {
  it('returns security-uniform copy for invalid credentials', () => {
    expect(userMessageForCode(ErrorCode.INVALID_CREDENTIALS)).toBe(
      'Email or password is incorrect.',
    );
  });

  it('maps session-expiring codes to the same copy', () => {
    expect(userMessageForCode(ErrorCode.INVALID_REFRESH_TOKEN)).toBe(
      userMessageForCode(ErrorCode.UNAUTHORIZED),
    );
  });

  it('falls back to a generic server message for unknown codes', () => {
    expect(userMessageForCode('NOPE')).toBe('Something went wrong on our end.');
  });

  it('maps every machine code to non-empty, non-leaky copy', () => {
    for (const code of Object.values(ErrorCode)) {
      const msg = userMessageForCode(code);
      expect(msg.length).toBeGreaterThan(0);
      // Copy must never echo the raw machine code at the user.
      expect(msg).not.toContain(code);
    }
  });
});

describe('error taxonomy treatment coverage', () => {
  // Every code resolves to exactly one of the four treatments the UI branches
  // on (DSC §11.1): soft AI notice, setup redirect, transient retry, or terminal.
  type Treatment = 'ai' | 'setup' | 'transient' | 'terminal';
  function treatment(code: ErrorCode | string, status: number): Treatment {
    const e = new ApiError({ code, message: code, status });
    if (e.isAiUnconfigured) return 'ai';
    if (e.isSetupIncomplete) return 'setup';
    if (e.isTransient) return 'transient';
    return 'terminal';
  }

  const cases: Array<[ErrorCode, number, Treatment]> = [
    [ErrorCode.AI_UNCONFIGURED, 503, 'ai'],
    [ErrorCode.SETUP_INCOMPLETE, 403, 'setup'],
    [ErrorCode.SERVICE_UNAVAILABLE, 503, 'transient'],
    [ErrorCode.UPSTREAM_ERROR, 502, 'transient'],
    [ErrorCode.INTERNAL_ERROR, 500, 'transient'],
    [ErrorCode.INVALID_CREDENTIALS, 401, 'terminal'],
    [ErrorCode.UNAUTHORIZED, 401, 'terminal'],
    [ErrorCode.FORBIDDEN, 403, 'terminal'],
    [ErrorCode.VALIDATION_ERROR, 400, 'terminal'],
    [ErrorCode.NOT_FOUND, 404, 'terminal'],
    [ErrorCode.CONFLICT, 409, 'terminal'],
    [ErrorCode.RATE_LIMITED, 429, 'terminal'],
    [ErrorCode.PAYLOAD_TOO_LARGE, 413, 'terminal'],
    [ErrorCode.NOT_IMPLEMENTED, 501, 'terminal'],
    [ErrorCode.NETWORK_ERROR, 0, 'terminal'],
  ];

  for (const [code, status, expected] of cases) {
    it(`routes ${code} → ${expected}`, () => {
      expect(treatment(code, status)).toBe(expected);
    });
  }

  it('never double-classifies AI_UNCONFIGURED as transient (503 trap)', () => {
    const e = new ApiError({ code: ErrorCode.AI_UNCONFIGURED, message: 'x', status: 503 });
    expect(e.isTransient).toBe(false);
    expect(e.isAiUnconfigured).toBe(true);
  });
});
