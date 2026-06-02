import { describe, it, expect, beforeEach } from 'vitest';
import '../src/components/app/fb-app.js';

/**
 * Phase F: the density preference (DSC §2.3) must survive a reload. fb-app reads
 * it from localStorage at construction and mirrors it onto `data-density`, the
 * ancestor the density tokens cascade from. With no route resolved yet the app
 * renders its loading canvas, which is enough to assert the attribute wiring.
 */
async function mountApp(): Promise<HTMLElement> {
  const el = document.createElement('fb-app');
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

describe('fb-app density persistence', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    localStorage.clear();
  });

  it('defaults to comfortable when nothing is stored', async () => {
    const el = await mountApp();
    expect(el.getAttribute('data-density')).toBe('comfortable');
  });

  it('restores a persisted compact preference on boot', async () => {
    localStorage.setItem('fb-density', 'compact');
    const el = await mountApp();
    expect(el.getAttribute('data-density')).toBe('compact');
  });

  it('ignores a corrupt stored value and falls back to comfortable', async () => {
    localStorage.setItem('fb-density', 'banana');
    const el = await mountApp();
    expect(el.getAttribute('data-density')).toBe('comfortable');
  });
});
