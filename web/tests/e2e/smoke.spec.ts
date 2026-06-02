import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * Phase A smoke: the app boots, mounts <fb-app>, and renders an accessible
 * shell even with no backend reachable (silent refresh fails → /login). Real
 * route-by-route axe coverage and journey E2E land in Phase C/F once the auth
 * pages are implemented.
 */
test('app boots and is accessible at the root', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('fb-app')).toBeVisible();

  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
  expect(results.violations).toEqual([]);
});
