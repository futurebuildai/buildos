import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * Phase F — route-by-route WCAG 2.1 AA sweep (FRONTEND_ARCHITECTURE §7, DSC §9).
 *
 * These are the routes reachable without a backend: the unauthenticated auth
 * surface (the app's silent-refresh fails → /login, and the rest render on the
 * minimal canvas). Authenticated org routes redirect to /login here; their axe
 * coverage runs under the Phase C/D/E backend harness (make db-up && migrate &&
 * go run ./cmd/server with a seeded session). Axe runs against the rendered
 * shadow DOM, so web-component internals are included in the scan.
 */
const PUBLIC_ROUTES = [
  { path: '/login', name: 'sign in' },
  { path: '/first-run', name: 'first-run bootstrap claim' },
  { path: '/forgot-password', name: 'forgot password' },
  { path: '/reset-password?token=sample-token', name: 'reset password' },
];

for (const route of PUBLIC_ROUTES) {
  test(`${route.name} (${route.path}) has no WCAG A/AA violations`, async ({ page }) => {
    await page.goto(route.path);
    await expect(page.locator('fb-app')).toBeVisible();

    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    expect(results.violations).toEqual([]);
  });
}

test('keyboard focus lands on an interactive control after the auth page loads', async ({
  page,
}) => {
  await page.goto('/login');
  await expect(page.locator('fb-app')).toBeVisible();
  // Tab into the page; the first stop must be a real focusable control, not the
  // body (proves a reachable keyboard path — DSC §9 keyboard-nav requirement).
  await page.keyboard.press('Tab');
  const activeTag = await page.evaluate(() => {
    let el: Element | null = document.activeElement;
    // Pierce shadow roots to find the truly-focused leaf.
    while (el?.shadowRoot?.activeElement) el = el.shadowRoot.activeElement;
    return el?.tagName.toLowerCase() ?? 'body';
  });
  expect(activeTag).not.toBe('body');
});
