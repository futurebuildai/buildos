import { test, expect } from '@playwright/test';

/**
 * Real-browser regression guard for the fb-* control event boundary.
 *
 * Each fb-input / fb-password-input / fb-secret-input renders its native control
 * inside its OWN shadow root. The native `input` event is `composed: true`, so it
 * escapes that shadow root — landing on host `@input` listeners ALONGSIDE the
 * curated `{ value }` CustomEvent the control re-emits. The native one carries a
 * numeric `detail` (UIEvent.detail = 0), so a consumer reading `e.detail.value`
 * off it gets `undefined` and clobbers the value it just captured. This silently
 * broke the setup wizard's per-row binding (`@input=${bind('code')}`).
 *
 * The controls now `stopPropagation()` the native `input` at the boundary, so a
 * host listener sees exactly ONE event and its detail is always the curated
 * `{ value }`. happy-dom doesn't model composed-event retargeting, so this lives
 * here (Chromium) and needs no backend.
 */
test.describe('fb-input event boundary', () => {
  test('typing fires exactly one curated input event per keystroke', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('fb-login-page')).toBeVisible();

    // Instrument the email fb-input host: record every `input` event's detail.
    await page.locator('fb-input[name="email"]').evaluate((host) => {
      const w = window as unknown as { __fbInputDetails: unknown[] };
      w.__fbInputDetails = [];
      host.addEventListener('input', (e) => {
        w.__fbInputDetails.push((e as CustomEvent).detail);
      });
    });

    await page.locator('fb-input[name="email"] input').fill('ab');

    const details = await page.evaluate(
      () => (window as unknown as { __fbInputDetails: unknown[] }).__fbInputDetails,
    );

    // `.fill('ab')` triggers one composite input event in Chromium. The native
    // (composed) duplicate is stopped at the boundary, so we see one event and its
    // detail is the curated `{ value }` shape — never a bare number.
    expect(details).toHaveLength(1);
    expect(details[0]).toEqual({ value: 'ab' });
  });
});
