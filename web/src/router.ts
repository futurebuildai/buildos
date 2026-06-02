/**
 * Thin client router over the History API (FRONTEND_ARCHITECTURE §2.1 — a custom
 * router avoids an extra dependency and gives full control over the RBAC/setup
 * guards). The route table mirrors DSC §1.3 with declarative role gates.
 *
 * Guards (DSC §1.3, FRONTEND_ARCHITECTURE §4.3):
 *  - Unauthenticated access to a protected route → /login.
 *  - Role/plan gate failure → /forbidden (the nav should already hide it).
 *  - A SETUP_INCOMPLETE ApiError from any call → redirectToSetup() (wired by the
 *    surfaces that make calls; the router exposes the helper + a setup shell).
 *
 * Page components are referenced by custom-element tag. Phase A registers a
 * placeholder for routes whose real page lands in later phases; the table itself
 * is the contract those phases fill in.
 */
import { signal } from '@lit-labs/signals';
import { isAuthenticated, hasMinRole, hasRole, isPro } from './state/authStore.js';
import type { Role } from './auth/jwt.js';

export type ShellKind = 'org' | 'auth' | 'setup';

export interface RouteGate {
  /** Minimum role (precedence owner>admin>super>field). */
  minRole?: Role;
  /** Exact role allow-list (overrides minRole when present). */
  roles?: Role[];
  /** Requires a pro/enterprise plan tier (AI surfaces). */
  requiresPro?: boolean;
}

export interface RouteDef {
  path: string;
  /** Custom-element tag rendered into the shell outlet. */
  tag: string;
  shell: ShellKind;
  /** Absent gate = public (auth screens) or any-authenticated (org screens). */
  gate?: RouteGate;
  /** Public routes skip the authentication check entirely. */
  public?: boolean;
  title: string;
}

export interface ResolvedRoute {
  def: RouteDef;
  params: Record<string, string>;
  path: string;
}

// ----------------------------- Route table (DSC §1.3) -----------------------------
export const routes: RouteDef[] = [
  // Auth (public, no shell chrome)
  { path: '/login', tag: 'fb-login-page', shell: 'auth', public: true, title: 'Sign in' },
  { path: '/first-run', tag: 'fb-first-run-page', shell: 'auth', public: true, title: 'First run' },
  {
    path: '/forgot-password',
    tag: 'fb-forgot-password-page',
    shell: 'auth',
    public: true,
    title: 'Reset password',
  },
  {
    path: '/reset-password',
    tag: 'fb-reset-password-page',
    shell: 'auth',
    public: true,
    title: 'Set new password',
  },

  // Setup wizard (authenticated, admin+, minimal shell)
  {
    path: '/setup',
    tag: 'fb-setup-page',
    shell: 'setup',
    gate: { minRole: 'admin' },
    title: 'Setup',
  },
  {
    path: '/setup/:step',
    tag: 'fb-setup-page',
    shell: 'setup',
    gate: { minRole: 'admin' },
    title: 'Setup',
  },

  // Portfolio workspace
  {
    path: '/portfolio/financials',
    tag: 'fb-financials-page',
    shell: 'org',
    gate: { minRole: 'superintendent' },
    title: 'Financials',
  },
  { path: '/portfolio/projects', tag: 'fb-projects-page', shell: 'org', title: 'Projects' },
  {
    path: '/portfolio/projects/:id',
    tag: 'fb-project-detail-page',
    shell: 'org',
    title: 'Project',
  },
  {
    path: '/portfolio/pipeline',
    tag: 'fb-pipeline-page',
    shell: 'org',
    gate: { minRole: 'superintendent' },
    title: 'Pipeline',
  },
  {
    path: '/portfolio/fleet',
    tag: 'fb-fleet-page',
    shell: 'org',
    gate: { minRole: 'superintendent' },
    title: 'Fleet',
  },
  {
    path: '/portfolio/hr',
    tag: 'fb-hr-page',
    shell: 'org',
    gate: { roles: ['owner', 'admin'] },
    title: 'HR & Certs',
  },

  // Command Center workspace
  {
    path: '/command/briefing',
    tag: 'fb-briefing-page',
    shell: 'org',
    gate: { minRole: 'superintendent' },
    title: 'Daily Briefing',
  },
  { path: '/command/schedule', tag: 'fb-schedule-page', shell: 'org', title: 'Schedule' },
  {
    path: '/command/procurement',
    tag: 'fb-procurement-page',
    shell: 'org',
    gate: { minRole: 'superintendent' },
    title: 'Procurement',
  },
  {
    path: '/command/assistant',
    tag: 'fb-assistant-page',
    shell: 'org',
    gate: { minRole: 'superintendent', requiresPro: true },
    title: 'AI Assistant',
  },

  // Activity / Settings / Profile
  {
    path: '/activity',
    tag: 'fb-activity-page',
    shell: 'org',
    gate: { roles: ['owner', 'admin'] },
    title: 'Activity',
  },
  {
    path: '/settings/org',
    tag: 'fb-settings-org-page',
    shell: 'org',
    gate: { roles: ['owner', 'admin'] },
    title: 'Organization',
  },
  {
    path: '/settings/integrations',
    tag: 'fb-integrations-page',
    shell: 'org',
    gate: { roles: ['owner'] },
    title: 'Integrations',
  },
  {
    path: '/settings/users',
    tag: 'fb-settings-users-page',
    shell: 'org',
    gate: { roles: ['owner', 'admin'] },
    title: 'Users & Roles',
  },
  {
    path: '/settings/notifications',
    tag: 'fb-settings-notifications-page',
    shell: 'org',
    title: 'Notifications',
  },
  { path: '/profile', tag: 'fb-profile-page', shell: 'org', title: 'Profile' },

  // field_worker landing (FRONTEND_ARCHITECTURE §1.1 — hard-block to "use the app")
  { path: '/use-the-app', tag: 'fb-use-mobile-page', shell: 'auth', title: 'Use the mobile app' },
  // Errors
  { path: '/forbidden', tag: 'fb-forbidden-page', shell: 'org', title: 'No access' },
  { path: '/not-found', tag: 'fb-not-found-page', shell: 'org', title: 'Not found' },
];

// ------------------------------- Matching --------------------------------
function matchPath(pattern: string, actual: string): Record<string, string> | null {
  const pSegs = pattern.split('/').filter(Boolean);
  const aSegs = actual.split('/').filter(Boolean);
  if (pSegs.length !== aSegs.length) return null;
  const params: Record<string, string> = {};
  for (let i = 0; i < pSegs.length; i++) {
    const p = pSegs[i] as string;
    const a = aSegs[i] as string;
    if (p.startsWith(':')) {
      params[p.slice(1)] = decodeURIComponent(a);
    } else if (p !== a) {
      return null;
    }
  }
  return params;
}

function findRoute(path: string): ResolvedRoute | null {
  // Static routes win over param routes; iterate static-first.
  const ordered = [...routes].sort(
    (a, b) => Number(a.path.includes(':')) - Number(b.path.includes(':')),
  );
  for (const def of ordered) {
    const params = matchPath(def.path, path);
    if (params) return { def, params, path };
  }
  return null;
}

// --------------------------- Guards & landing ----------------------------
function passesGate(gate: RouteGate | undefined): boolean {
  if (!gate) return true;
  if (gate.roles && !hasRole(...gate.roles)) return false;
  if (gate.minRole && !hasMinRole(gate.minRole)) return false;
  if (gate.requiresPro && !isPro()) return false;
  return true;
}

/** Role-based post-login landing (FRONTEND_ARCHITECTURE §1.1 / Phase C.2). */
export function landingPathForRole(role: Role | null): string {
  switch (role) {
    case 'owner':
    case 'admin':
      return '/portfolio/financials';
    case 'superintendent':
      return '/command/briefing';
    case 'field_worker':
      return '/use-the-app';
    default:
      return '/login';
  }
}

// ----------------------------- Router state ------------------------------
const currentSignal = signal<ResolvedRoute | null>(null);
export const currentRoute = currentSignal;

function resolve(path: string): ResolvedRoute {
  const matched = findRoute(path) ?? findRoute('/not-found')!;
  const def = matched.def;

  if (!def.public && !isAuthenticated.get()) {
    return findRoute('/login')!;
  }
  if (!passesGate(def.gate)) {
    return findRoute('/forbidden')!;
  }
  return matched;
}

export function navigate(path: string, opts: { replace?: boolean } = {}): void {
  if (opts.replace) {
    window.history.replaceState({}, '', path);
  } else {
    window.history.pushState({}, '', path);
  }
  currentSignal.set(resolve(window.location.pathname + window.location.search));
}

/** Redirect helper for SETUP_INCOMPLETE (called by surfaces on that ApiError). */
export function redirectToSetup(): void {
  navigate('/setup', { replace: true });
}

export function startRouter(): void {
  window.addEventListener('popstate', () => {
    currentSignal.set(resolve(window.location.pathname + window.location.search));
  });
  // Intercept same-origin link clicks for SPA navigation.
  document.addEventListener('click', (e) => {
    const anchor = (e.target as HTMLElement)?.closest?.('a[href]') as HTMLAnchorElement | null;
    if (!anchor) return;
    const href = anchor.getAttribute('href');
    if (!href || href.startsWith('http') || anchor.target === '_blank' || e.metaKey || e.ctrlKey)
      return;
    e.preventDefault();
    navigate(href);
  });
  currentSignal.set(resolve(window.location.pathname + window.location.search));
}
