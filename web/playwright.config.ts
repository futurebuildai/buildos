import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright E2E config. Axe accessibility assertions run per-route via
 * @axe-core/playwright (FRONTEND_ARCHITECTURE §7, Phase F). The web server is
 * the Vite dev server; E2E against a live backend is driven by the Phase C
 * verification harness (make db-up && migrate && go run ./cmd/server).
 */
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
