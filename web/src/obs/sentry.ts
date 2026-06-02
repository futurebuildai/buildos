/**
 * Frontend error tracking (TECH_STACK "Error Tracking: Sentry"; Phase F).
 *
 * BuildOS ships a single build across all forks, so the Sentry SDK is loaded by
 * the host page (CDN/loader) only when a deployment opts in — there is no npm
 * dependency here. This module owns the one thing that must never be skipped:
 * the PII-scrubbing `beforeSend`. `initObservability` configures the SDK with it
 * when present, and is a safe no-op otherwise. `scrubEvent` is exported pure so
 * it is unit-testable without the SDK.
 */
import { scrubDeep, scrubString } from './pii.js';

/** The subset of the Sentry event envelope we touch (kept structural, no dep). */
export interface SentryEvent {
  message?: string;
  request?: { headers?: Record<string, unknown>; cookies?: unknown; data?: unknown };
  user?: { id?: string; email?: string; username?: string; ip_address?: string } & Record<
    string,
    unknown
  >;
  extra?: Record<string, unknown>;
  contexts?: Record<string, unknown>;
  breadcrumbs?: Array<{ message?: string; data?: unknown } & Record<string, unknown>>;
  exception?: { values?: Array<{ value?: string } & Record<string, unknown>> };
  [key: string]: unknown;
}

interface SentryLike {
  init(options: { dsn: string; beforeSend: (event: SentryEvent) => SentryEvent | null }): void;
}

/**
 * Scrubs an outbound Sentry event in place-safe fashion (returns the same ref).
 * Restricted-class data is redacted; Confidential is masked; Internal IDs
 * (`request_id`, `trace_id`, `org_id`) survive so crashes stay triageable.
 */
export function scrubEvent(event: SentryEvent): SentryEvent {
  // The user object is the densest PII: keep only a stable id, drop the rest.
  if (event.user) {
    event.user = event.user.id ? { id: event.user.id } : {};
  }

  if (event.request) {
    delete event.request.headers;
    delete event.request.cookies;
    if (event.request.data !== undefined) {
      event.request.data = scrubDeep(event.request.data);
    }
  }

  if (event.extra) event.extra = scrubDeep(event.extra) as Record<string, unknown>;
  if (event.contexts) event.contexts = scrubDeep(event.contexts) as Record<string, unknown>;

  if (event.breadcrumbs) {
    event.breadcrumbs = event.breadcrumbs.map((b) => {
      if (typeof b.message === 'string') b.message = scrubString(b.message);
      if (b.data !== undefined) b.data = scrubDeep(b.data);
      return b;
    });
  }

  if (typeof event.message === 'string') event.message = scrubString(event.message);

  if (event.exception?.values) {
    for (const v of event.exception.values) {
      if (typeof v.value === 'string') v.value = scrubString(v.value);
    }
  }

  return event;
}

/** The configured `beforeSend`. Exported so the host page can pass it directly. */
export function beforeSend(event: SentryEvent): SentryEvent | null {
  return scrubEvent(event);
}

/**
 * Wires `beforeSend` into the Sentry SDK if the host page loaded it and a DSN is
 * configured (`VITE_SENTRY_DSN`). No-op (and never throws) otherwise — empty
 * config means no telemetry, matching the backend's turn-on-when-configured
 * posture.
 */
export function initObservability(): void {
  const dsn = import.meta.env.VITE_SENTRY_DSN;
  if (!dsn) return;
  const sdk = (globalThis as { Sentry?: SentryLike }).Sentry;
  if (!sdk?.init) return;
  try {
    sdk.init({ dsn, beforeSend });
  } catch {
    // Telemetry must never break boot.
  }
}
