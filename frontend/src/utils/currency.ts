/**
 * Currency formatting utilities for the Composite Currency Pattern.
 * All monetary values are stored as BIGINT cents. This module handles
 * display formatting only — no arithmetic across currencies.
 *
 * Supported currencies: USD, CAD
 * Cross-currency arithmetic is FORBIDDEN.
 */

export type CurrencyCode = 'USD' | 'CAD';

const CURRENCY_SYMBOLS: Record<CurrencyCode, string> = {
  USD: '$',
  CAD: 'CA$',
};

const CURRENCY_LOCALES: Record<CurrencyCode, string> = {
  USD: 'en-US',
  CAD: 'en-CA',
};

/**
 * Format cents (BIGINT) into a display string.
 * Example: formatCents(1234567, 'USD') => "$12,345.67"
 */
export function formatCents(cents: number, currencyCode: CurrencyCode): string {
  const dollars = cents / 100;
  const locale = CURRENCY_LOCALES[currencyCode];
  const symbol = CURRENCY_SYMBOLS[currencyCode];
  const formatted = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(Math.abs(dollars));
  const sign = cents < 0 ? '-' : '';
  return `${sign}${symbol}${formatted}`;
}

/**
 * Format cents into a compact display (e.g., "$12.3K", "$1.2M").
 */
export function formatCentsCompact(cents: number, currencyCode: CurrencyCode): string {
  const dollars = Math.abs(cents / 100);
  const symbol = CURRENCY_SYMBOLS[currencyCode];
  const sign = cents < 0 ? '-' : '';

  if (dollars >= 1_000_000) {
    return `${sign}${symbol}${(dollars / 1_000_000).toFixed(1)}M`;
  }
  if (dollars >= 1_000) {
    return `${sign}${symbol}${(dollars / 1_000).toFixed(1)}K`;
  }
  return formatCents(cents, currencyCode);
}

/**
 * Format a cents value with explicit sign for variance display.
 * Positive values get "+" prefix, negative values get "-" prefix.
 */
export function formatCentsVariance(cents: number, currencyCode: CurrencyCode): string {
  const formatted = formatCents(Math.abs(cents), currencyCode);
  if (cents > 0) return `+${formatted}`;
  if (cents < 0) return `-${formatted}`;
  return formatted;
}

/**
 * Format a percentage value for display.
 * @param value - Percentage as a number (e.g., 12.5 for 12.5%)
 * @param signed - Whether to show +/- sign
 */
export function formatPercent(value: number, signed: boolean = false): string {
  const formatted = new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(Math.abs(value));

  if (signed) {
    if (value > 0) return `+${formatted}%`;
    if (value < 0) return `-${formatted}%`;
  }
  return `${formatted}%`;
}

/**
 * Get the currency symbol for a given code.
 */
export function currencySymbol(code: CurrencyCode): string {
  return CURRENCY_SYMBOLS[code];
}
