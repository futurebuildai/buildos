import { html, css, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { SignalWatcher } from '@lit-labs/signals';
import { FBElement } from '../base/fb-element.js';
import '../base/fb-placeholder.js';
import '../shell/fb-org-shell.js';
import '../organisms/fb-feedback-widget.js';
import type { Workspace, Density } from '../shell/fb-top-bar.js';
import { currentRoute, navigate, type ResolvedRoute } from '../../router.js';
import { authClaims, isAuthenticated } from '../../state/authStore.js';

/**
 * Root application element. Subscribes to the router's `currentRoute` signal and
 * renders the matched page inside the appropriate shell (auth / setup / org).
 *
 * Auth + setup routes render on a centered minimal canvas (no chrome). Org
 * routes render inside the real `fb-org-shell` (B5): top bar + role-gated nav
 * rail + content outlet + status strip. Chrome events (workspace switch, density
 * toggle, profile) bubble up here and are translated into navigation/state.
 */
@customElement('fb-app')
export class FbApp extends SignalWatcher(FBElement) {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
        min-height: 100vh;
      }
      /* auth + setup: centered minimal canvas */
      .centered {
        min-height: 100vh;
        display: grid;
        place-items: center;
        padding: var(--fb-spacing-lg);
      }
      .centered-inner {
        width: 100%;
        max-width: 420px;
      }
    `,
  ];

  @state() private density: Density = readStoredDensity();

  override willUpdate(): void {
    // Density tokens cascade from a [data-density] ancestor (DSC §2.3).
    this.setAttribute('data-density', this.density);
  }

  override render(): TemplateResult {
    const route = currentRoute.get();
    if (!route) return html`<div class="centered">Loading…</div>`;

    const page = this.renderPage(route);
    // Phase 0b: the floating feedback widget rides along on the authed ORG
    // shell only (fixed bottom-right, below toasts). Deliberately absent
    // from the auth/setup shells: pre-onboarding the SetupGate 403s
    // POST /api/v1/feedback (SETUP_INCOMPLETE), so rendering the button
    // there offers an affordance that can only fail.
    const feedback = isAuthenticated.get() ? html`<fb-feedback-widget></fb-feedback-widget>` : null;

    switch (route.def.shell) {
      case 'auth':
      case 'setup':
        return html`<div class="centered"><div class="centered-inner">${page}</div></div>`;
      case 'org':
      default:
        return html`${this.renderOrgShell(route, page)}${feedback}`;
    }
  }

  /** Renders the route's page element, falling back to fb-placeholder. */
  private renderPage(route: ResolvedRoute): TemplateResult {
    const tag = customElements.get(route.def.tag) ? route.def.tag : 'fb-placeholder';
    const el = document.createElement(tag);
    if (tag === 'fb-placeholder') {
      el.setAttribute('heading', route.def.title);
      el.setAttribute('route', route.def.tag);
    }
    for (const [k, v] of Object.entries(route.params)) {
      el.setAttribute(k, v);
    }
    return html`${el}`;
  }

  private renderOrgShell(route: ResolvedRoute, page: TemplateResult): TemplateResult {
    const claims = authClaims.get();
    const workspace: Workspace = route.path.startsWith('/command') ? 'command' : 'portfolio';
    return html`
      <fb-org-shell
        user-role=${claims?.role ?? ''}
        plan=${claims?.planTier ?? ''}
        current=${route.path}
        workspace=${workspace}
        density=${this.density}
        @navigate=${this.onNavigate}
        @workspace-change=${this.onWorkspaceChange}
        @density-change=${this.onDensityChange}
        @profile=${() => navigate('/profile')}
      >
        ${page}
      </fb-org-shell>
    `;
  }

  /**
   * Catches the composed `navigate` event bubbling out of the nav rail (and any
   * in-shell link molecule, e.g. breadcrumbs) and drives the SPA router. The
   * router's document-level click interceptor can't see these anchors — they
   * live inside `fb-nav-item`'s shadow root, so the click event is retargeted to
   * the host and `closest('a[href]')` misses it. `fb-nav-item` calls
   * `preventDefault` and emits this event instead; the shell is where it lands.
   */
  private onNavigate(e: Event): void {
    const href = (e as CustomEvent<{ href: string }>).detail?.href;
    if (href) navigate(href);
  }

  private onWorkspaceChange(e: Event): void {
    const ws = (e as CustomEvent<{ workspace: Workspace }>).detail.workspace;
    navigate(ws === 'command' ? '/command/briefing' : '/portfolio/projects');
  }

  private onDensityChange(e: Event): void {
    this.density = (e as CustomEvent<{ density: Density }>).detail.density;
    writeStoredDensity(this.density);
  }
}

const DENSITY_KEY = 'fb-density';

/** Reads the persisted density preference; defaults to comfortable. */
function readStoredDensity(): Density {
  try {
    const v = localStorage.getItem(DENSITY_KEY);
    return v === 'compact' || v === 'comfortable' ? v : 'comfortable';
  } catch {
    return 'comfortable';
  }
}

function writeStoredDensity(density: Density): void {
  try {
    localStorage.setItem(DENSITY_KEY, density);
  } catch {
    // Private-mode / storage-disabled: density just won't persist.
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-app': FbApp;
  }
}
