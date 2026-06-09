import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * Live authenticated a11y sweep for the Phase 3c admin-config screens
 * (PHASE_3C_ADMIN_UI §8: the two new routes are AUTHENTICATED, so the
 * backend-free `web/tests/e2e/a11y.spec.ts` PUBLIC_ROUTES sweep can't reach
 * them). This spec signs in as the owner, then CLIENT-SIDE navigates (nav-rail
 * links intercepted by the SPA router — never `page.goto`, which full-reloads
 * and drops the in-memory access token, bouncing back to /login) to:
 *
 *   /settings/agents      (fb-agents-page)
 *   /settings/connectors  (fb-connectors-page)
 *
 * On EACH route it runs AxeBuilder.withTags(['wcag2a','wcag2aa']).analyze() and
 * asserts zero violations (axe scans the rendered shadow DOM, so fb-* internals
 * are included), plus a Tab-reaches-a-named-control keyboard-reachability check
 * (piercing shadow roots like a11y.spec.ts). It additionally axes two transient
 * states the static route scan can't see:
 *
 *   - the OPEN Add-MCP modal (fb-modal focus-trap + form a11y)
 *   - a POPULATED foresight form-error state (an empty/invalid threshold + Save
 *     surfaces the fb-form summary `role=alert` + per-field error wiring)
 *
 * Like the other live specs it runs only under the live-backend harness
 * (playwright.live.config.ts) and is test.skip-guarded on the owner contract, so
 * the backend-free `npm run test:e2e` lane is a no-op.
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
 * The truly-focused leaf tag, piercing shadow roots (the focused control lives
 * inside an fb-* shadow DOM). Mirrors a11y.spec.ts.
 */
async function focusedLeafTag(page: Page): Promise<string> {
  return page.evaluate(() => {
    let el: Element | null = document.activeElement;
    while (el?.shadowRoot?.activeElement) el = el.shadowRoot.activeElement;
    return el?.tagName.toLowerCase() ?? 'body';
  });
}

test.describe.serial('live admin-config a11y', () => {
  test('agents + connectors routes are axe-clean and keyboard-reachable', async ({ page }) => {
    await signInAsOwner(page);

    // ---- /settings/agents (client-side nav via the rail, NOT page.goto) ----
    // The "AI Agents" link is owner/admin-gated; our owner qualifies.
    await page.getByRole('link', { name: 'AI Agents' }).click();
    await expect(page).toHaveURL(/\/settings\/agents/, { timeout: 15_000 });
    const agentsPage = page.locator('fb-agents-page');
    await expect(agentsPage).toBeVisible();
    // Wait for the loaded state: the three capability switches are rendered.
    await expect(page.getByRole('switch', { name: 'Enable Risk early-warning' })).toBeVisible({
      timeout: 15_000,
    });

    const agentsAxe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    expect(agentsAxe.violations).toEqual([]);

    // Keyboard reachability: Tab lands on a real, named control (not <body>).
    await page.keyboard.press('Tab');
    expect(await focusedLeafTag(page)).not.toBe('body');

    // ---- Populated foresight FORM-ERROR state (transient — axe it open) ----
    // Clear the schedule-float threshold to an invalid value and Save. The page
    // client-validates `Number.isInteger && >= 1` BEFORE any PUT and surfaces
    // the fb-form summary (role=alert) + per-field error — purely client-side,
    // so this works regardless of backend tuning state.
    const floatInput = page.locator('fb-input[name="schedule_float_days"] input');
    await floatInput.fill('');
    await page.getByRole('button', { name: 'Save thresholds' }).click();
    // The fb-form error summary appears (role=alert); wait for it before axing.
    await expect(page.getByRole('alert')).toBeVisible({ timeout: 10_000 });

    const foresightErrAxe = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa'])
      .analyze();
    expect(foresightErrAxe.violations).toEqual([]);

    // ---- /settings/connectors (client-side nav via the rail) ----
    await page.getByRole('link', { name: 'Connectors' }).click();
    await expect(page).toHaveURL(/\/settings\/connectors/, { timeout: 15_000 });
    const connectorsPage = page.locator('fb-connectors-page');
    await expect(connectorsPage).toBeVisible();
    // Wait for the loaded state: the Add-MCP primary button is rendered.
    await expect(page.getByRole('button', { name: 'Connect an external tool server' })).toBeVisible(
      { timeout: 15_000 },
    );

    const connectorsAxe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    expect(connectorsAxe.violations).toEqual([]);

    // Keyboard reachability on the second route too.
    await page.keyboard.press('Tab');
    expect(await focusedLeafTag(page)).not.toBe('body');

    // ---- OPEN Add-MCP modal state (transient — axe it open) ----
    // fb-modal is role=dialog + aria-modal with a focus trap; the open dialog's
    // form fields (Name/Web address) add a11y surface the static scan misses.
    await page.getByRole('button', { name: 'Connect an external tool server' }).click();
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 10_000 });

    const modalAxe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    expect(modalAxe.violations).toEqual([]);
  });
});
