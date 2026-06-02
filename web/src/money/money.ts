/**
 * Composite Currency Pattern helpers (CLAUDE.md / TECH_STACK §Constraints /
 * API_CONTRACT §2.4). Money is integer cents kept as a STRING (BIGINT can exceed
 * 2^53), paired with a `currency_code`. All math here is BigInt — never
 * `parseFloat`/`Number` arithmetic, which would drift. Mirrors the backend
 * `internal/currency` package, including its `ErrCrossCurrency` sentinel.
 */

export type CurrencyCode = 'USD' | 'CAD';

/** Thrown when callers attempt to combine values of different currencies. */
export class CrossCurrencyError extends Error {
  constructor(a: string, b: string) {
    super(`refusing to combine ${a} with ${b} (ErrCrossCurrency)`);
    this.name = 'CrossCurrencyError';
  }
}

/** A monetary value: integer cents (string) + ISO currency code. */
export interface Money {
  cents: string;
  currencyCode: CurrencyCode;
}

const CURRENCY_FRACTION = 2; // USD + CAD both minor-unit ×100.

function toBigInt(cents: string | bigint): bigint {
  if (typeof cents === 'bigint') return cents;
  const trimmed = cents.trim();
  if (!/^-?\d+$/.test(trimmed)) {
    throw new Error(`invalid cents string: ${JSON.stringify(cents)}`);
  }
  return BigInt(trimmed);
}

/** Groups an unsigned integer-digit string with thousands separators. */
function groupThousands(intDigits: string): string {
  return intDigits.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

/**
 * Formats integer cents as a display string, e.g. "1234567" + USD → "$12,345.67".
 * Uses pure string/BigInt math so values beyond 2^53 cents format exactly. The
 * currency code is NOT appended here — callers (e.g. `fb-money`) decide whether to
 * show it for disambiguation (USD vs CAD share the `$` glyph).
 */
export function formatCents(cents: string | bigint, _currencyCode: CurrencyCode): string {
  const value = toBigInt(cents);
  const negative = value < 0n;
  const digits = (negative ? -value : value).toString().padStart(CURRENCY_FRACTION + 1, '0');
  const intPart = digits.slice(0, digits.length - CURRENCY_FRACTION);
  const fracPart = digits.slice(digits.length - CURRENCY_FRACTION);
  return `${negative ? '-' : ''}$${groupThousands(intPart)}.${fracPart}`;
}

/** Sign of a cents value: -1, 0, or 1. */
export function signOfCents(cents: string | bigint): -1 | 0 | 1 {
  const v = toBigInt(cents);
  return v < 0n ? -1 : v > 0n ? 1 : 0;
}

/** Adds two same-currency values; throws CrossCurrencyError on mismatch. */
export function addMoney(a: Money, b: Money): Money {
  if (a.currencyCode !== b.currencyCode)
    throw new CrossCurrencyError(a.currencyCode, b.currencyCode);
  return {
    cents: (toBigInt(a.cents) + toBigInt(b.cents)).toString(),
    currencyCode: a.currencyCode,
  };
}

/**
 * Sums a list of values, grouped by currency. Never produces a cross-currency
 * grand total — returns one subtotal per distinct `currency_code` (DSC §6.1).
 */
export function sumByCurrency(values: readonly Money[]): Money[] {
  const totals = new Map<CurrencyCode, bigint>();
  for (const v of values) {
    totals.set(v.currencyCode, (totals.get(v.currencyCode) ?? 0n) + toBigInt(v.cents));
  }
  return [...totals.entries()].map(([currencyCode, cents]) => ({
    cents: cents.toString(),
    currencyCode,
  }));
}

/**
 * Parses a user-entered dollar string ("1,234.5", "$12.345") into integer cents,
 * via string math only (no float round-trip). Extra fraction digits are
 * truncated to the currency's 2 minor digits. Returns null for empty input.
 */
export function dollarsToCents(input: string): string | null {
  const cleaned = input.replace(/[^0-9.-]/g, '');
  if (cleaned === '' || cleaned === '-' || cleaned === '.') return null;
  const negative = cleaned.startsWith('-');
  const unsigned = cleaned.replace(/-/g, '');
  const [whole = '0', frac = ''] = unsigned.split('.');
  const fracPadded = (frac + '00').slice(0, CURRENCY_FRACTION);
  const centsDigits = `${whole}${fracPadded}`.replace(/^0+(?=\d)/, '');
  const value = BigInt(centsDigits === '' ? '0' : centsDigits);
  return (negative && value !== 0n ? -value : value).toString();
}

/** Renders integer cents back to an editable decimal dollar string ("1234.56"). */
export function centsToDollars(cents: string | bigint): string {
  const value = toBigInt(cents);
  const negative = value < 0n;
  const digits = (negative ? -value : value).toString().padStart(CURRENCY_FRACTION + 1, '0');
  const intPart = digits.slice(0, digits.length - CURRENCY_FRACTION);
  const fracPart = digits.slice(digits.length - CURRENCY_FRACTION);
  return `${negative ? '-' : ''}${intPart}.${fracPart}`;
}
