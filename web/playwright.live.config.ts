import { defineConfig, devices } from '@playwright/test';

/**
 * Live-backend Playwright config (Phase C verification / E2E lane).
 *
 * Unlike playwright.config.ts (which runs backend-free against the dev server
 * and asserts graceful degradation), this drives the real auth → setup wizard →
 * operate journeys against a LIVE BuildOS backend. The backend is stood up
 * out-of-band by scripts/e2e-backend.sh, which exports the test contract:
 *
 *   E2E_API_URL, E2E_BOOTSTRAP_TOKEN, E2E_OWNER_EMAIL, E2E_OWNER_PASSWORD
 *
 * We only manage the Vite dev server here; its /api proxy forwards to the Go
 * server on :8080 (vite.config.ts). The journeys are stateful and order-
 * dependent (the one-shot bootstrap claim must precede login), so they run
 * serially on a single worker with no retries — replaying a consumed claim is
 * meaningless.
 *
 * ORDERING IS LOAD-BEARING: onboarding.live.spec.ts performs the one-shot
 * owner CLAIM that creates the account every other spec signs in with.
 * Alphabetical file order put admin-config (and later feedback-widget)
 * BEFORE onboarding, so their sign-in hit "Email or password is incorrect"
 * — the long-red E2E lane. The two projects below make the dependency
 * explicit: `journey` (the claim + wizard) always runs first; `authed`
 * (everything else) only starts after it succeeds.
 */
export default defineConfig({
  testDir: './tests/live',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'journey',
      use: { ...devices['Desktop Chrome'] },
      testMatch: /onboarding\.live\.spec\.ts/,
    },
    {
      name: 'authed',
      use: { ...devices['Desktop Chrome'] },
      testIgnore: /onboarding\.live\.spec\.ts/,
      dependencies: ['journey'],
    },
  ],
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
