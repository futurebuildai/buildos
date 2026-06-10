# BuildOS Web Console

The operator-facing web console for BuildOS — a Lit + TypeScript single-page app
served same-origin behind the Go backend in production. Owners, admins, and
superintendents run portfolio, scheduling (CPM Gantt), financials, procurement,
fleet, HR, and onboarding from here. The field surfaces live in `mobile/` (Flutter).

Design specs (binding): `.agents/handoff/frontend/*.md` and root `CLAUDE.md`.

## Stack

- **Vite** + **Lit 3** + **TypeScript** (strict, `noUncheckedIndexedAccess`,
  `exactOptionalPropertyTypes`).
- **State:** `@lit-labs/signals`.
- **Styling:** Vanilla CSS + CSS custom-property tokens (`src/styles/variables.css`).
  Dark-only (GableLBM Industrial Dark). No Tailwind, no UI framework.
- **API:** native `fetch` with a single-flight 401→refresh→retry interceptor
  (`src/api/client.ts`). Tokens are in-memory only (see `src/api/tokens.ts`).
- **Test:** Vitest (happy-dom) + `@open-wc/testing` for components; Playwright +
  `@axe-core/playwright` for E2E/a11y.

## Layout

```
src/
  api/         fetch client, error taxonomy, token store, typed endpoints
  auth/        JWT decode + role precedence helpers
  state/       Lit Signals stores (auth, capability)
  components/  base / atoms / molecules / organisms / pages / app
  styles/      variables.css (design tokens)
  router.ts    History-API router with role + setup gates
  main.ts      bootstrap
eslint-rules/  fb/composite-currency custom rule
tests/         unit (*.test.ts) + e2e (tests/e2e/*.spec.ts)
```

## Commands

```bash
npm install          # first time
npm run dev          # Vite dev server on :5173, proxies /api -> :8080
npm run build        # tsc --noEmit + vite build
npm run typecheck    # tsc --noEmit
npm run lint         # eslint + prettier --check
npm run lint:fix     # eslint --fix + prettier --write
npm test             # vitest run (unit)
npm run test:e2e     # playwright (needs backend; see plan Phase C verification)
```

## Backend dev loop

The dev server proxies `/api` to `http://localhost:8080`. Run the backend with
the dev auth header bypass for fast role switching:

```bash
make db-up && make migrate
DEV_AUTH_MODE=header go run ./cmd/server
```

## Production serving (same-origin)

The production image builds this app in a `node:20-alpine` Docker stage and the
Go server serves `web/dist` itself — same origin as the API, no separate static
host. `WEB_DIST_DIR` (baked into the image as `/var/lib/buildos/web`) points the
server at the bundle; `internal/api/spa.go` owns the serving contract: SPA
fallback to `index.html` for client-routed paths, year-long immutable cache for
hashed `/assets/*`, `no-cache` + `Content-Security-Policy` on the HTML, and
JSON 404s for `/api/*` misses. Local rigs leave `WEB_DIST_DIR` unset and use the
Vite proxy above.

Two production-serving caveats: source maps are emitted `hidden` and **stripped
from the image** (they embed the full TypeScript source; see the Dockerfile
webbuilder stage), and the server-stamped CSP (`script-src 'self'`,
`connect-src 'self'`) blocks the opt-in browser-Sentry hook (`src/obs/sentry.ts`)
unless a fork extends `spaCSP` in `internal/api/spa.go` with its Sentry origins.

## Conventions

- **Money:** integer `*Cents` (string) + sibling `*CurrencyCode`. Never a float
  `number`. Never sum across currencies. Enforced by `fb/composite-currency`
  ESLint rule + CI.
- **Components:** custom elements prefixed `fb-`, extending `FBElement`.
- **A11y:** status is never conveyed by color alone; axe runs on every route in CI.
