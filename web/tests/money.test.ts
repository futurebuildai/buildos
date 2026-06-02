import { describe, it, expect } from 'vitest';
import {
  formatCents,
  signOfCents,
  addMoney,
  sumByCurrency,
  dollarsToCents,
  centsToDollars,
  CrossCurrencyError,
  type Money,
} from '../src/money/money.js';

describe('formatCents', () => {
  it('groups thousands and keeps two fraction digits', () => {
    expect(formatCents('1234567', 'USD')).toBe('$12,345.67');
    expect(formatCents('5', 'USD')).toBe('$0.05');
    expect(formatCents('0', 'USD')).toBe('$0.00');
  });

  it('handles negatives and values beyond 2^53 exactly', () => {
    expect(formatCents('-1099', 'CAD')).toBe('-$10.99');
    // 90071992547409910 cents far exceeds Number.MAX_SAFE_INTEGER.
    expect(formatCents('90071992547409910', 'USD')).toBe('$900,719,925,474,099.10');
  });
});

describe('signOfCents', () => {
  it('returns -1, 0, 1', () => {
    expect(signOfCents('-1')).toBe(-1);
    expect(signOfCents('0')).toBe(0);
    expect(signOfCents('1')).toBe(1);
  });
});

describe('addMoney', () => {
  it('adds same-currency values', () => {
    const a: Money = { cents: '100', currencyCode: 'USD' };
    const b: Money = { cents: '250', currencyCode: 'USD' };
    expect(addMoney(a, b)).toEqual({ cents: '350', currencyCode: 'USD' });
  });

  it('throws CrossCurrencyError on mismatch (mirrors ErrCrossCurrency)', () => {
    const a: Money = { cents: '100', currencyCode: 'USD' };
    const b: Money = { cents: '100', currencyCode: 'CAD' };
    expect(() => addMoney(a, b)).toThrow(CrossCurrencyError);
  });
});

describe('sumByCurrency', () => {
  it('never produces a cross-currency grand total', () => {
    const out = sumByCurrency([
      { cents: '100', currencyCode: 'USD' },
      { cents: '200', currencyCode: 'CAD' },
      { cents: '50', currencyCode: 'USD' },
    ]);
    expect(out).toContainEqual({ cents: '150', currencyCode: 'USD' });
    expect(out).toContainEqual({ cents: '200', currencyCode: 'CAD' });
    expect(out).toHaveLength(2);
  });
});

describe('dollarsToCents', () => {
  it('parses via string math without float drift', () => {
    expect(dollarsToCents('1,234.5')).toBe('123450');
    expect(dollarsToCents('$12.345')).toBe('1234'); // truncates extra digit
    expect(dollarsToCents('0.10')).toBe('10');
    expect(dollarsToCents('-5')).toBe('-500');
  });

  it('returns null for empty-ish input', () => {
    expect(dollarsToCents('')).toBeNull();
    expect(dollarsToCents('.')).toBeNull();
    expect(dollarsToCents('-')).toBeNull();
  });
});

describe('centsToDollars', () => {
  it('round-trips to an editable decimal', () => {
    expect(centsToDollars('123456')).toBe('1234.56');
    expect(centsToDollars('5')).toBe('0.05');
    expect(centsToDollars('-1099')).toBe('-10.99');
  });
});
