import { describe, it, expect } from 'vitest';
import {
  classifyField,
  FieldClass,
  maskValue,
  scrubString,
  scrubDeep,
  REDACTED,
} from '../src/obs/pii.js';
import { scrubEvent, beforeSend, type SentryEvent } from '../src/obs/sentry.js';

describe('classifyField', () => {
  it('marks people/contact fields Restricted', () => {
    for (const k of ['email', 'client_email', 'phone', 'display_name', 'full_name', 'address']) {
      expect(classifyField(k)).toBe(FieldClass.Restricted);
    }
  });

  it('marks GPS, IP, and credential fields Restricted', () => {
    for (const k of [
      'gps_lat',
      'gps_lng',
      'ip_address',
      'password',
      'access_token',
      'authorization',
    ]) {
      expect(classifyField(k)).toBe(FieldClass.Restricted);
    }
  });

  it('marks money/business-label fields Confidential', () => {
    for (const k of ['total_cents', 'amount', 'budget_cents', 'invoice_number', 'vendor']) {
      expect(classifyField(k)).toBe(FieldClass.Confidential);
    }
  });

  it('keeps triage IDs Internal even though they end in _id or contain name-ish bits', () => {
    for (const k of ['request_id', 'trace_id', 'org_id', 'resource_id', 'action']) {
      expect(classifyField(k)).toBe(FieldClass.Internal);
    }
  });

  it('defaults unknown fields to Public', () => {
    expect(classifyField('status')).toBe(FieldClass.Public);
    expect(classifyField('build_version')).toBe(FieldClass.Public);
  });
});

describe('maskValue', () => {
  it('keeps the first character and length, masking the rest', () => {
    expect(maskValue('Acme Corp')).toBe('A********');
    expect(maskValue('')).toBe('');
    expect(maskValue('x')).toBe('x');
  });
});

describe('scrubString', () => {
  it('redacts emails, IPv4 addresses, and bearer/JWT tokens', () => {
    expect(scrubString('contact a@b.com now')).toBe(`contact ${REDACTED} now`);
    expect(scrubString('from 192.168.0.42')).toBe(`from ${REDACTED}`);
    expect(scrubString('Authorization: Bearer abc.def.ghi')).toContain('Bearer [REDACTED]');
  });

  it('leaves clean text untouched', () => {
    expect(scrubString('schedule recalculated in 42ms')).toBe('schedule recalculated in 42ms');
  });
});

describe('scrubDeep', () => {
  it('redacts Restricted, masks Confidential, and keeps Internal/Public', () => {
    const out = scrubDeep({
      email: 'jane@site.com',
      total_cents: '125000',
      org_id: 'org-1',
      status: 'active',
    }) as Record<string, unknown>;
    expect(out.email).toBe(REDACTED);
    expect(out.total_cents).toBe('1*****');
    expect(out.org_id).toBe('org-1');
    expect(out.status).toBe('active');
  });

  it('recurses into nested objects and arrays', () => {
    const out = scrubDeep({
      user: { display_name: 'Jane Doe', id: 'u-1' },
      contacts: [{ phone: '555-1212' }],
    }) as { user: Record<string, unknown>; contacts: Array<Record<string, unknown>> };
    expect(out.user.display_name).toBe(REDACTED);
    expect(out.user.id).toBe('u-1');
    expect(out.contacts[0]!.phone).toBe(REDACTED);
  });

  it('guards against cyclic graphs', () => {
    const a: Record<string, unknown> = { status: 'ok' };
    a.self = a;
    expect(() => scrubDeep(a)).not.toThrow();
  });
});

describe('scrubEvent / beforeSend', () => {
  it('reduces the user object to a stable id', () => {
    const e: SentryEvent = {
      user: { id: 'u-1', email: 'jane@site.com', ip_address: '10.0.0.1', username: 'jane' },
    };
    scrubEvent(e);
    expect(e.user).toEqual({ id: 'u-1' });
  });

  it('drops request headers/cookies and scrubs request data', () => {
    const e: SentryEvent = {
      request: {
        headers: { Authorization: 'Bearer secret' },
        cookies: 'session=abc',
        data: { email: 'jane@site.com', org_id: 'org-1' },
      },
    };
    scrubEvent(e);
    expect(e.request!.headers).toBeUndefined();
    expect(e.request!.cookies).toBeUndefined();
    expect((e.request!.data as Record<string, unknown>).email).toBe(REDACTED);
    expect((e.request!.data as Record<string, unknown>).org_id).toBe('org-1');
  });

  it('scrubs free-text in messages, breadcrumbs, and exception values', () => {
    const e: SentryEvent = {
      message: 'login failed for jane@site.com',
      breadcrumbs: [{ message: 'POST /login from 192.168.1.5', data: { phone: '555-0000' } }],
      exception: { values: [{ value: 'token Bearer abc.def.ghi rejected' }] },
    };
    beforeSend(e);
    expect(e.message).not.toContain('jane@site.com');
    expect(e.breadcrumbs![0]!.message).not.toContain('192.168.1.5');
    expect((e.breadcrumbs![0]!.data as Record<string, unknown>).phone).toBe(REDACTED);
    expect(e.exception!.values![0]!.value).toContain('Bearer [REDACTED]');
  });

  it('returns the event (never null) so reports still send, just scrubbed', () => {
    expect(beforeSend({ message: 'boom' })).not.toBeNull();
  });
});
