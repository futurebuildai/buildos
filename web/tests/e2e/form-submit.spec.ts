import { test, expect } from '@playwright/test';

/**
 * Real-browser regression guard for the fb-form cross-shadow submit bridge.
 *
 * fb-input / fb-password-input / fb-secret-input and the submit fb-button each
 * render their native element inside their OWN shadow root, so none of them are
 * form-owned by the `<form>` in fb-form's shadow root. The browser therefore
 * fires neither a native `submit` on button click nor implicit submission on
 * Enter — across the shadow boundary the form is dead to native mechanics.
 * fb-form bridges it by listening for composed `click` / `keydown` events.
 *
 * happy-dom can't model that retargeting, so this lives here (Chromium). It
 * needs no backend: an empty /login submit surfaces the client-side validation
 * message ("Enter your email and password.") iff onSubmit actually ran.
 */
test.describe('fb-form cross-shadow submit', () => {
  test('clicking the submit button runs onSubmit', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('fb-login-page')).toBeVisible();

    await page.locator('fb-button[type="submit"] button').click();

    // .auth-error lives in the page's shadow root; pierce + assert via evaluate.
    await expect
      .poll(async () =>
        page
          .locator('fb-login-page')
          .evaluate((el) => el.shadowRoot?.querySelector('.auth-error')?.textContent ?? ''),
      )
      .toContain('Enter your email and password');
  });

  test('pressing Enter in a field runs onSubmit (implicit submission)', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('fb-login-page')).toBeVisible();

    await page.locator('fb-input[name="email"] input').click();
    await page.keyboard.press('Enter');

    await expect
      .poll(async () =>
        page
          .locator('fb-login-page')
          .evaluate((el) => el.shadowRoot?.querySelector('.auth-error')?.textContent ?? ''),
      )
      .toContain('Enter your email and password');
  });
});
