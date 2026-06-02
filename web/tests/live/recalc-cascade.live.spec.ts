import { test, expect, type Page } from '@playwright/test';

/**
 * Live journey: the CPM recalc → cascade-diff path (UX_CORE_SCREENS §2). Signs
 * in as the owner, opens the schedule workspace for the seeded project, and runs
 * Recalculate — proving the native-auth → physics-engine → persisted-schedule
 * round trip end to end against a live BuildOS backend.
 *
 * Seeded by scripts/e2e-backend.sh --seed-schedule: a single project ("E2E Tower")
 * with a linear finish-to-start task chain (Site Prep → Foundation → Framing) and
 * NO CPM results yet. So the page opens on the "not computed" empty state, and the
 * first Recalculate computes the schedule, draws the Gantt, reports
 * `recalculation_ms`, and — because the chain makes every task critical — surfaces
 * the "Critical path changed" cascade notice.
 *
 * Ordering: this runs in the same serial live config as onboarding.live.spec.ts,
 * AFTER it (file order: onboarding < recalc). The owner account it logs in with is
 * the one that spec's first-run claim creates and whose onboarding it completes;
 * the seeded project lives in that same fork-zero org.
 *
 * Contract (exported by the harness):
 *   E2E_OWNER_EMAIL              owner email to sign in with
 *   E2E_OWNER_PASSWORD           owner password
 *   E2E_SCHEDULE_PROJECT_NAME    name of the seeded project (assertion anchor)
 */

const EMAIL = process.env.E2E_OWNER_EMAIL ?? '';
const PASSWORD = process.env.E2E_OWNER_PASSWORD ?? '';
const PROJECT = process.env.E2E_SCHEDULE_PROJECT_NAME ?? '';

// Skip (don't fail) when the harness contract isn't present — keeps the spec a
// no-op under the backend-free `test:e2e` lane and only live under the harness
// with --seed-schedule.
test.skip(
  !EMAIL || !PASSWORD || !PROJECT,
  'live schedule contract (E2E_OWNER_EMAIL / E2E_OWNER_PASSWORD / E2E_SCHEDULE_PROJECT_NAME) not set',
);

/** Fill the native <input> inside an fb-* control (Playwright pierces shadow DOM). */
async function fillControl(page: Page, hostSelector: string, value: string): Promise<void> {
  await page.locator(`${hostSelector} input`).fill(value);
}

test.describe.serial('live recalc cascade', () => {
  test('sign in → schedule → recalculate computes + cascades', async ({ page }) => {
    // ---- Sign in as the owner created by the onboarding journey -----------
    await page.goto('/login');
    await expect(page.locator('fb-login-page')).toBeVisible();
    await fillControl(page, 'fb-input[name="email"]', EMAIL);
    await fillControl(page, 'fb-password-input[name="password"]', PASSWORD);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page).toHaveURL(/\/portfolio\/financials/, { timeout: 15_000 });

    // ---- Open the schedule workspace --------------------------------------
    // Navigate via the nav rail (client-side, intercepted by the SPA router) —
    // NOT page.goto, which would full-reload and drop the in-memory access token,
    // bouncing back to /login.
    await page.getByRole('link', { name: 'Schedule' }).click();
    await expect(page).toHaveURL(/\/command\/schedule/, { timeout: 15_000 });
    const pageEl = page.locator('fb-schedule-page');
    await expect(pageEl).toBeVisible();

    // The picker auto-selects the only project; confirm it's the seeded one.
    await expect(pageEl.locator('fb-select')).toContainText(PROJECT);

    // Seeded tasks carry no CPM results yet → "not computed" empty state.
    // (fb-state renders its heading as styled text, not a role=heading element.)
    await expect(page.getByText('Schedule not computed yet')).toBeVisible({ timeout: 15_000 });

    // ---- Recalculate: native auth → physics → persisted schedule ----------
    // Two Recalculate buttons can exist (the empty-state CTA + the toolbar);
    // either drives the same handler. Click the first visible one.
    await page.getByRole('button', { name: 'Recalculate' }).first().click();

    // The Gantt draws once the schedule is computed (empty state is replaced).
    await expect(page.locator('fb-gantt-chart')).toBeVisible({ timeout: 15_000 });

    // recalculation_ms is surfaced in the toolbar meta ("recomputed in {N}ms").
    await expect(page.getByText(/recomputed in \d+ms/)).toBeVisible();

    // The linear FS chain makes every task critical → critical_path_changed=true,
    // so the cascade notice renders.
    await expect(page.getByText(/Critical path changed/)).toBeVisible();
  });
});
