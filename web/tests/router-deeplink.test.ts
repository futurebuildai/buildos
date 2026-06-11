import { describe, it, expect } from 'vitest';
import { findRoute } from '../src/router.js';

// Regression: feed-card deep-links carry a ?project=<id> query. matchPath must
// strip the query (and hash) before segment-matching, or the link falls through
// to /not-found. (Adversarial review CRITICAL finding.)
describe('router findRoute — query/hash stripping on deep-links', () => {
  it('resolves a path with a query string to the real route, not /not-found', () => {
    const r = findRoute('/portfolio/financials?project=abc-123');
    expect(r).not.toBeNull();
    expect(r?.def.tag).toBe('fb-financials-page');
  });

  it('resolves a path with a hash fragment', () => {
    const r = findRoute('/command/schedule#section');
    expect(r).not.toBeNull();
    expect(r?.def.tag).toBe('fb-schedule-page');
  });

  it('still resolves a clean path (no regression)', () => {
    expect(findRoute('/portfolio/financials')?.def.tag).toBe('fb-financials-page');
  });

  it('a genuinely-unknown path still resolves to null (→ caller maps to /not-found)', () => {
    expect(findRoute('/command/financials')).toBeNull();
  });
});
