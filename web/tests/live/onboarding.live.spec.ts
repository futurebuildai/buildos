import { test, expect, type Page } from '@playwright/test';

/**
 * Live journey: first-run owner claim → 6-step setup wizard → complete → land
 * on the portfolio, then sign in again as that owner. Runs against a live
 * BuildOS backend stood up by scripts/e2e-backend.sh (native auth, vault on),
 * with the Vite dev server proxying /api → :8080.
 *
 * Contract (exported by the harness):
 *   E2E_BOOTSTRAP_TOKEN  one-shot 43-char base64url first-owner claim token
 *   E2E_OWNER_EMAIL      owner email to create + sign in with
 *   E2E_OWNER_PASSWORD   owner password
 *
 * Stateful + order-dependent: the claim consumes the one-shot token and must
 * precede login, so the suite is serial (see playwright.live.config.ts).
 */

const TOKEN = process.env.E2E_BOOTSTRAP_TOKEN ?? '';
const EMAIL = process.env.E2E_OWNER_EMAIL ?? '';
const PASSWORD = process.env.E2E_OWNER_PASSWORD ?? '';

// Skip (don't fail) when the harness contract isn't present — keeps the spec a
// no-op under the backend-free `test:e2e` lane and only live under the harness.
test.skip(
  !TOKEN || !EMAIL || !PASSWORD,
  'live backend contract (E2E_BOOTSTRAP_TOKEN / E2E_OWNER_EMAIL / E2E_OWNER_PASSWORD) not set',
);

/** Fill the native <input> inside an fb-* control (Playwright pierces shadow DOM). */
async function fillControl(page: Page, hostSelector: string, value: string): Promise<void> {
  await page.locator(`${hostSelector} input`).fill(value);
}

test.describe.serial('live onboarding', () => {
  test('first-run claim → wizard → complete → portfolio', async ({ page }) => {
    // ---- B1: first-owner bootstrap claim --------------------------------
    await page.goto('/first-run');
    await expect(page.locator('fb-first-run-page')).toBeVisible();

    await fillControl(page, 'fb-secret-input[name="token"]', TOKEN);
    await fillControl(page, 'fb-input[name="display_name"]', 'E2E Owner');
    await fillControl(page, 'fb-input[name="email"]', EMAIL);
    await fillControl(page, 'fb-password-input[name="password"]', PASSWORD);
    await fillControl(page, 'fb-password-input[name="confirm"]', PASSWORD);
    await page.getByRole('button', { name: 'Create owner account' }).click();

    // Claim succeeds → owner's org is not yet onboarded → wizard.
    await expect(page).toHaveURL(/\/setup/, { timeout: 15_000 });
    await expect(page.getByRole('heading', { name: 'Company info' })).toBeVisible();

    // ---- Step 1: company info ------------------------------------------
    await fillControl(page, 'fb-input[name="legal_name"]', 'E2E Builders LLC');
    await page.getByRole('button', { name: 'Save & continue' }).click();
    await expect(page.getByRole('heading', { name: 'Trades' })).toBeVisible();

    // ---- Step 2: trades (per-row add) ----------------------------------
    await fillControl(page, 'fb-input[name="code"]', 'GC');
    await fillControl(page, 'fb-input[name="name"]', 'General');
    await page.getByRole('button', { name: 'Add trade' }).click();
    await expect(page.getByText('Added trade General.')).toBeVisible();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.getByRole('heading', { name: 'Cost codes' })).toBeVisible();

    // ---- Step 3: cost codes (CSI MasterFormat) -------------------------
    await fillControl(page, 'fb-input[name="code"]', '03-30-00');
    await fillControl(page, 'fb-input[name="division"]', '03');
    await fillControl(page, 'fb-input[name="name"]', 'Cast-in-Place Concrete');
    await page.getByRole('button', { name: 'Add cost code' }).click();
    await expect(page.getByText('Added cost code 03-30-00.')).toBeVisible();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.getByRole('heading', { name: 'Working calendar' })).toBeVisible();

    // ---- Step 4: working calendar (Mon–Fri / 8h defaults) --------------
    await page.getByRole('button', { name: 'Save calendar' }).click();
    await expect(page.getByText('Working calendar saved.')).toBeVisible();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.getByRole('heading', { name: 'Permit jurisdictions' })).toBeVisible();

    // ---- Step 5: jurisdictions (skippable) -----------------------------
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.getByRole('heading', { name: 'Review' })).toBeVisible();

    // ---- Step 6: review & complete -------------------------------------
    await page.getByRole('button', { name: 'Complete setup' }).click();

    // Owner lands on their role home (financials), and the SetupGate no longer
    // 403s operational routes — the page renders instead of redirecting to /setup.
    await expect(page).toHaveURL(/\/portfolio\/financials/, { timeout: 15_000 });
    await expect(page.locator('fb-financials-page')).toBeVisible();
  });

  test('sign in as the owner lands on the portfolio', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('fb-login-page')).toBeVisible();

    await fillControl(page, 'fb-input[name="email"]', EMAIL);
    await fillControl(page, 'fb-password-input[name="password"]', PASSWORD);
    await page.getByRole('button', { name: 'Sign in' }).click();

    await expect(page).toHaveURL(/\/portfolio\/financials/, { timeout: 15_000 });
    await expect(page.locator('fb-financials-page')).toBeVisible();
  });
});
