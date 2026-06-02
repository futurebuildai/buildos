/**
 * Wire-normalization helpers.
 *
 * The Go backend serializes monetary `int64` cents as bare JSON numbers
 * (e.g. `amount_cents: 123456`), but the frontend's Composite Currency
 * convention keeps cents as STRINGS end-to-end so the BigInt-based money
 * helpers (`money/money.ts`) and `fb-money` never risk a 2^53 overflow.
 * `normalizeCents` reconciles the two at the API boundary: it walks a parsed
 * response and converts every `*_cents` numeric leaf into its string form,
 * leaving everything else untouched. Applied once per endpoint so the rest of
 * the app only ever sees string cents.
 */
export function normalizeCents<T>(value: T): T {
  if (Array.isArray(value)) {
    return value.map((v) => normalizeCents(v)) as unknown as T;
  }
  if (value !== null && typeof value === 'object') {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) {
      out[k] = /_cents$/.test(k) && typeof v === 'number' ? String(v) : normalizeCents(v);
    }
    return out as T;
  }
  return value;
}
