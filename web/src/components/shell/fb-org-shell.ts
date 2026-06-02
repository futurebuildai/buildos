import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-top-bar.js';
import './fb-nav-rail.js';
import './fb-context-panel.js';
import './fb-sync-status.js';
import type { Workspace, Density } from './fb-top-bar.js';
import type { SyncState } from './fb-sync-status.js';
import type { Role, PlanTier } from '../../auth/jwt.js';

/**
 * `fb-org-shell` — the operational app shell (DSC §1.2). Composes the top bar,
 * the role-gated nav rail, the routed content (default slot), the on-demand
 * context panel (`context` slot), and the status strip (passive sync chip).
 *
 * Landmarks are explicit per DSC §9: `banner` (top bar), `navigation` (rail),
 * `main` (content), `complementary` (context panel), `contentinfo` (status strip).
 * The shell is property-driven so it can be unit-tested without the live auth
 * signals; the application root wires `role`/`plan`/`current`/`workspace`/etc.
 * from the stores and re-listens to the bubbled chrome events.
 */
@customElement('fb-org-shell')
export class FbOrgShell extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: grid;
        grid-template-rows: 56px 1fr auto;
        height: 100vh;
        background: var(--fb-deep-space);
        color: var(--fb-text-primary);
      }
      .top {
        grid-row: 1;
      }
      .middle {
        grid-row: 2;
        display: grid;
        grid-template-columns: 280px minmax(0, 1fr) auto;
        min-height: 0;
      }
      .nav {
        grid-column: 1;
        border-right: 1px solid var(--fb-glass-border);
        overflow: hidden;
      }
      main {
        grid-column: 2;
        overflow: auto;
        padding: var(--fb-spacing-lg);
      }
      .context {
        grid-column: 3;
      }
      .status {
        grid-row: 3;
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-md);
        min-height: 0;
        padding: 0 var(--fb-spacing-lg);
        border-top: 1px solid var(--fb-glass-border);
      }
      .status:empty {
        display: none;
      }
      /* Desktop: collapse the rail to icon-only. */
      @media (max-width: 1200px) {
        .middle {
          grid-template-columns: 72px minmax(0, 1fr) auto;
        }
      }
      /* Tablet and below: rail hidden (hamburger lives in the app, not the shell). */
      @media (max-width: 768px) {
        .middle {
          grid-template-columns: minmax(0, 1fr);
        }
        .nav,
        .context {
          display: none;
        }
      }
    `,
  ];

  /** Current user's role; named `user-role` to avoid the native ARIA `role`. */
  @property({ type: String, attribute: 'user-role' }) userRole: Role | null = null;
  @property({ type: String }) plan: PlanTier | null = null;
  /** Active route path — drives the nav rail's current item. */
  @property({ type: String }) current = '';
  @property({ type: String }) workspace: Workspace = 'portfolio';
  @property({ type: String }) density: Density = 'comfortable';
  @property({ type: Number }) notifications = 0;
  /** Collapse the nav rail to icon-only. */
  @property({ type: Boolean, attribute: 'rail-collapsed' }) railCollapsed = false;
  /** Open the right-hand context panel. */
  @property({ type: Boolean, attribute: 'context-open' }) contextOpen = false;
  @property({ type: String, attribute: 'context-heading' }) contextHeading = '';
  /** Passive sync state for the status strip. */
  @property({ type: String, attribute: 'sync-state' }) syncState: SyncState = 'idle';
  @property({ type: Number, attribute: 'sync-queued' }) syncQueued = 0;

  override render(): TemplateResult {
    return html`
      <fb-top-bar
        class="top"
        user-role=${this.userRole ?? nothing}
        workspace=${this.workspace}
        density=${this.density}
        notifications=${this.notifications}
      ></fb-top-bar>

      <div class="middle">
        <div class="nav">
          <fb-nav-rail
            user-role=${this.userRole ?? nothing}
            plan=${this.plan ?? nothing}
            current=${this.current}
            ?collapsed=${this.railCollapsed}
          ></fb-nav-rail>
        </div>

        <main role="main"><slot></slot></main>

        ${this.contextOpen
          ? html`<div class="context">
              <fb-context-panel open heading=${this.contextHeading}>
                <slot name="context"></slot>
              </fb-context-panel>
            </div>`
          : nothing}
      </div>

      <footer class="status" role="contentinfo">
        <fb-sync-status state=${this.syncState} queued=${this.syncQueued}></fb-sync-status>
        <slot name="status"></slot>
      </footer>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-org-shell': FbOrgShell;
  }
}
