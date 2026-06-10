import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * Live authenticated a11y sweep for the Phase 0b feedback widget
 * (fb-feedback-widget). The widget mounts only on the AUTHENTICATED org shell
 * (fb-app renders it when isAuthenticated), so the backend-free
 * `web/tests/e2e/a11y.spec.ts` PUBLIC_ROUTES sweep can never reach it. This
 * spec signs in as the owner and — on the org-shell landing route — runs
 * AxeBuilder.withTags(['wcag2a','wcag2aa']).analyze() asserting zero
 * violations (axe scans the rendered shadow DOM, so fb-* internals are
 * included) across the widget's three states the static route scan can't see:
 *
 *   - panel CLOSED (just the floating trigger)
 *   - panel OPEN (the form: category select + message textarea + send)
 *   - the VALIDATION-ERROR state (empty message + Send → aria-invalid +
 *     aria-describedby error wiring, purely client-side)
 *
 * Plus a keyboard-reachability check: a bounded Tab sweep must reach the
 * floating trigger (piercing shadow roots like a11y.spec.ts).
 *
 * Navigation rule (mirrors admin-config.live.spec.ts): after sign-in, only
 * CLIENT-SIDE navigation — never `page.goto`, which full-reloads and drops
 * the in-memory access token, bouncing back to /login. This spec stays on the
 * sign-in landing route, so no further navigation is needed.
 *
 * Like the other live specs it runs only under the live-backend harness
 * (playwright.live.config.ts) and is test.skip-guarded on the owner contract,
 * so the backend-free `npm run test:e2e` lane is a no-op.
 *
 * Contract (exported by the harness):
 *   E2E_OWNER_EMAIL     owner email to sign in with
 *   E2E_OWNER_PASSWORD  owner password
 */

const EMAIL = process.env.E2E_OWNER_EMAIL ?? '';
const PASSWORD = process.env.E2E_OWNER_PASSWORD ?? '';

// Skip (don't fail) when the harness contract isn't present — keeps the spec a
// no-op under the backend-free `test:e2e` lane and only live under the harness.
test.skip(
  !EMAIL || !PASSWORD,
  'live owner contract (E2E_OWNER_EMAIL / E2E_OWNER_PASSWORD) not set',
);

/** Fill the native <input> inside an fb-* control (Playwright pierces shadow DOM). */
async function fillControl(page: Page, hostSelector: string, value: string): Promise<void> {
  await page.locator(`${hostSelector} input`).fill(value);
}

/** Sign in as the owner created by the onboarding journey. */
async function signInAsOwner(page: Page): Promise<void> {
  await page.goto('/login');
  await expect(page.locator('fb-login-page')).toBeVisible();
  await fillControl(page, 'fb-input[name="email"]', EMAIL);
  await fillControl(page, 'fb-password-input[name="password"]', PASSWORD);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/portfolio\/financials/, { timeout: 15_000 });
}

/**
 * The truly-focused leaf's accessible label, piercing shadow roots (the
 * trigger lives inside fb-feedback-widget's shadow DOM). Mirrors the
 * focused-leaf helper in admin-config.live.spec.ts / a11y.spec.ts.
 */
async function focusedLeafLabel(page: Page): Promise<string> {
  return page.evaluate(() => {
    let el: Element | null = document.activeElement;
    while (el?.shadowRoot?.activeElement) el = el.shadowRoot.activeElement;
    return el?.getAttribute('aria-label') ?? '';
  });
}

test.describe.serial('live feedback-widget a11y', () => {
  test('floating widget is axe-clean closed, open, and in the error state', async ({ page }) => {
    await signInAsOwner(page);

    // The floating trigger rides along on the authed org shell.
    const widget = page.locator('fb-feedback-widget');
    await expect(widget).toBeVisible();
    const trigger = page.getByRole('button', { name: 'Send feedback' });
    await expect(trigger).toBeVisible();

    // ---- Panel CLOSED: the landing route + trigger ----
    const closedAxe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    expect(closedAxe.violations).toEqual([]);

    // Keyboard reachability: a bounded Tab sweep must land on the trigger
    // (its aria-label is unique while the panel is closed). The widget is the
    // last element in fb-app's org-shell template, so the sweep walks the
    // whole shell first.
    let reachedTrigger = false;
    for (let i = 0; i < 100; i++) {
      await page.keyboard.press('Tab');
      if ((await focusedLeafLabel(page)) === 'Send feedback') {
        reachedTrigger = true;
        break;
      }
    }
    expect(reachedTrigger).toBe(true);

    // ---- Panel OPEN (form state — transient, axe it open) ----
    await trigger.click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByLabel('Message')).toBeVisible();

    const openAxe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    expect(openAxe.violations).toEqual([]);

    // ---- VALIDATION-ERROR state (transient — axe it open) ----
    // Submitting with an empty message is client-validated BEFORE any POST
    // (so this works regardless of backend state) and surfaces the
    // aria-invalid + aria-describedby error wiring.
    await dialog.getByRole('button', { name: 'Send feedback' }).click();
    await expect(page.getByText('Enter a message before sending.')).toBeVisible();

    const errorAxe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    expect(errorAxe.violations).toEqual([]);
  });
});
