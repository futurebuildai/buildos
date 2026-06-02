import { test, expect, type Page } from '@playwright/test';

/**
 * Live journey: the BYOK → AI-on capability flip (UX_AUTH_ONBOARDING §7 +
 * FRONTEND_ARCHITECTURE §6.2). Signs in as the owner, opens Settings →
 * Integrations, and proves the end-to-end vault round trip:
 *
 *   set an Anthropic key  →  GET /api/v1/capabilities flips ai_configured=true
 *
 * The backend treats AI as "configured" purely by **active-credential presence**
 * (no upstream Anthropic call / validation — see internal/service/integrations.go
 * resolveActiveKey), so the flip is deterministic with a FAKE non-empty key. No
 * real Anthropic key (and no AI request) is needed to exercise the contract.
 *
 * Ordering: same serial live config as the other specs, AFTER onboarding
 * (file order: onboarding < recalc < settings-integrations). The owner account it
 * signs in with is the one the onboarding spec claims + completes.
 *
 * Cleanup: the journey toggles the key back off at the end so the org is left as
 * it was found — preserving serial isolation for any later spec.
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

test.describe.serial('live settings integrations', () => {
  test('set Anthropic key → /capabilities flips ai_configured=true', async ({ page }) => {
    // ---- Sign in as the owner created by the onboarding journey -----------
    await page.goto('/login');
    await expect(page.locator('fb-login-page')).toBeVisible();
    await fillControl(page, 'fb-input[name="email"]', EMAIL);
    await fillControl(page, 'fb-password-input[name="password"]', PASSWORD);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page).toHaveURL(/\/portfolio\/financials/, { timeout: 15_000 });

    // ---- Open Settings → Integrations -------------------------------------
    // Navigate via the nav rail (client-side, intercepted by the SPA router) —
    // NOT page.goto, which would full-reload and drop the in-memory access token,
    // bouncing back to /login. The Integrations link is owner-gated; our owner
    // qualifies. (This nav path works post-fb-app @navigate fix.)
    await page.getByRole('link', { name: 'Integrations' }).click();
    await expect(page).toHaveURL(/\/settings\/integrations/, { timeout: 15_000 });
    const pageEl = page.locator('fb-integrations-page');
    await expect(pageEl).toBeVisible();

    // No Anthropic key seeded → the "AI features are off" banner is visible.
    await expect(
      page.getByText('AI features are off until an Anthropic API key is added.'),
    ).toBeVisible({
      timeout: 15_000,
    });

    // ---- Set a (fake) Anthropic key → Save --------------------------------
    // The first fb-integration-card is Anthropic (PROVIDERS order). Its
    // fb-secret-input renders a type=password input in the unset state.
    const anthropicCard = pageEl.locator('fb-integration-card').first();
    await anthropicCard.locator('input[type="password"]').fill('sk-ant-e2e-fake-key-0000');

    // Arm the capabilities-response wait BEFORE clicking Save: onSave →
    // setCredential → load → refreshCapabilities() issues GET /capabilities.
    const capsAfterSet = page.waitForResponse(
      (r) => r.url().includes('/api/v1/capabilities') && r.request().method() === 'GET',
      { timeout: 15_000 },
    );
    await anthropicCard.getByRole('button', { name: 'Save' }).click();

    // End-to-end proof of the new endpoint: the live backend now reports AI on
    // (deterministic — driven by credential presence, no AI call).
    const capsResp = await capsAfterSet;
    const rawCaps = await capsResp.text();
    let capsBody: { data?: { ai_configured?: boolean } };
    try {
      capsBody = JSON.parse(rawCaps);
    } catch {
      throw new Error(
        `caps parse failed status=${capsResp.status()} url=${capsResp.url()} ct=${capsResp.headers()['content-type']} body=${JSON.stringify(rawCaps.slice(0, 300))}`,
      );
    }
    expect(capsBody.data?.ai_configured).toBe(true);

    // The UI confirms: success toast + the "off" banner is gone.
    await expect(page.getByText('Anthropic key saved.')).toBeVisible();
    await expect(
      page.getByText('AI features are off until an Anthropic API key is added.'),
    ).toHaveCount(0);

    // ---- Cleanup: toggle the key back off (restore the org) ---------------
    // Arm the next capabilities wait; toggling off → deleteCredential → load →
    // refreshCapabilities() → ai_configured flips back to false.
    const capsAfterDelete = page.waitForResponse(
      (r) => r.url().includes('/api/v1/capabilities') && r.request().method() === 'GET',
      { timeout: 15_000 },
    );
    // fb-switch is the visually-hidden-input toggle pattern: the real
    // <input role="switch"> is opacity:0/1px under the visible .track span,
    // which intercepts pointer events. force:true bypasses that overlay
    // actionability check (a real user clicks the track/label) and still
    // toggles the checkbox + fires its change handler.
    await anthropicCard.getByRole('switch', { name: 'Enable Anthropic' }).click({ force: true });
    const delResp = await capsAfterDelete;
    const delBody = await delResp.json();
    expect(delBody.data.ai_configured).toBe(false);

    // The banner returns now that no active key remains.
    await expect(
      page.getByText('AI features are off until an Anthropic API key is added.'),
    ).toBeVisible({ timeout: 15_000 });
  });
});
