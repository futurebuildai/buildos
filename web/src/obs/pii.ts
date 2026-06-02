/**
 * Frontend PII scrubbing — the browser-side mirror of the backend `internal/pii`
 * taxonomy (CLAUDE.md "PII handling"). Used by the Sentry `beforeSend` hook
 * (FRONTEND_ARCHITECTURE §7, Phase F) so no Restricted-class data leaves the
 * device in crash reports. Four classifications:
 *
 *   - Public        org names, UUIDs, build versions      → kept
 *   - Internal      request_id, trace_id, org_id, action  → kept (triage needs them)
 *   - Confidential  vendor/invoice/project names, *_cents → length-preserved mask
 *   - Restricted    emails, phones, names, GPS, IPs, sub  → full redaction
 *
 * Restricted values are NOT length-preserved (defends against length
 * fingerprinting); Confidential values keep length so amounts/labels stay
 * recognizable in shape without disclosing content.
 */

export const FieldClass = {
  Public: 'public',
  Internal: 'internal',
  Confidential: 'confidential',
  Restricted: 'restricted',
} as const;
export type FieldClass = (typeof FieldClass)[keyof typeof FieldClass];

export const REDACTED = '[REDACTED]';

/** Field-name fragments that mark a value Restricted (full redaction). */
const RESTRICTED = [
  'email',
  'phone',
  'name', // display_name, first_name, full_name, client_name, contact name…
  'password',
  'secret',
  'token',
  'authorization',
  'cookie',
  'ssn',
  'gps',
  'lat',
  'lng',
  'latitude',
  'longitude',
  'ip_address',
  'ipaddr',
  'subject',
  'address',
];

/** Field-name fragments that mark a value Confidential (length-preserved mask). */
const CONFIDENTIAL = [
  'cents',
  'amount',
  'cost',
  'price',
  'total',
  'budget',
  'balance',
  'revenue',
  'expense',
  'fee',
  'payment',
  'invoice',
  'vendor',
];

/** Field-name fragments that are explicitly Internal (kept clear for triage). */
const INTERNAL = [
  'request_id',
  'requestid',
  'trace_id',
  'traceid',
  'span_id',
  'spanid',
  'org_id',
  'orgid',
  'resource_id',
  'resourceid',
  'event_type',
  'action',
];

function matches(haystack: string, needles: string[]): boolean {
  return needles.some((n) => haystack.includes(n));
}

/** Classifies a field by its (case-insensitive) name. Most-sensitive wins. */
export function classifyField(name: string): FieldClass {
  const k = name.toLowerCase();
  // Internal IDs are checked first so `org_id`/`resource_id` aren't pulled into
  // Restricted by the bare `name`/`id` fragments.
  if (matches(k, INTERNAL)) return FieldClass.Internal;
  if (matches(k, RESTRICTED)) return FieldClass.Restricted;
  if (matches(k, CONFIDENTIAL)) return FieldClass.Confidential;
  return FieldClass.Public;
}

/** Length-preserving mask: first character kept, the rest replaced with `*`. */
export function maskValue(value: string): string {
  if (value.length === 0) return value;
  return value[0] + '*'.repeat(Math.max(0, value.length - 1));
}

const EMAIL_RE = /[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi;
const IPV4_RE = /\b(?:\d{1,3}\.){3}\d{1,3}\b/g;
// Bearer tokens / JWTs — three base64url segments, or an explicit Bearer prefix.
const BEARER_RE = /\bBearer\s+[A-Za-z0-9._-]+/gi;
const JWT_RE = /\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/g;

/** Redacts Restricted patterns (emails, IPs, bearer/JWT tokens) inside free text. */
export function scrubString(input: string): string {
  return input
    .replace(BEARER_RE, 'Bearer [REDACTED]')
    .replace(JWT_RE, REDACTED)
    .replace(EMAIL_RE, REDACTED)
    .replace(IPV4_RE, REDACTED);
}

function scrubByClass(cls: FieldClass, value: unknown): unknown {
  if (cls === FieldClass.Restricted) return REDACTED;
  if (cls === FieldClass.Confidential) {
    return typeof value === 'string' ? maskValue(value) : REDACTED;
  }
  // Public / Internal: keep the value, but still sweep free text for embedded
  // Restricted patterns (e.g. an error message that happens to contain an email).
  return typeof value === 'string' ? scrubString(value) : value;
}

/**
 * Deep-scrubs arbitrary data by field name. `keyHint` carries the classification
 * of the parent key into nested scalars (so an array of emails under an `emails`
 * key is redacted element-wise). Cyclic graphs are guarded with a seen-set.
 */
export function scrubDeep(value: unknown, keyHint?: string, seen = new WeakSet<object>()): unknown {
  if (value === null || value === undefined) return value;

  if (Array.isArray(value)) {
    if (seen.has(value)) return REDACTED;
    seen.add(value);
    return value.map((v) => scrubDeep(v, keyHint, seen));
  }

  if (typeof value === 'object') {
    if (seen.has(value)) return REDACTED;
    seen.add(value);
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      const cls = classifyField(k);
      if (cls === FieldClass.Restricted) {
        out[k] = REDACTED;
      } else if (cls === FieldClass.Confidential && typeof v !== 'object') {
        out[k] = scrubByClass(cls, v);
      } else {
        out[k] = scrubDeep(v, k, seen);
      }
    }
    return out;
  }

  // Scalars: classify by the inherited key hint.
  if (keyHint) return scrubByClass(classifyField(keyHint), value);
  return typeof value === 'string' ? scrubString(value) : value;
}
