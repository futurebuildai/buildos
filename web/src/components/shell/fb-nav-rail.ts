import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../molecules/fb-nav-item.js';
import { roleAtLeast, roleIn, type Role, type PlanTier } from '../../auth/jwt.js';
import type { IconName } from '../atoms/icons.js';

/** Declarative gate mirroring the router (`router.ts` RouteGate / DSC §1.3). */
interface NavGate {
  minRole?: Role;
  roles?: Role[];
  requiresPro?: boolean;
}

interface NavItemDef {
  label: string;
  href: string;
  icon: IconName;
  gate?: NavGate;
}

interface NavGroupDef {
  label: string;
  items: NavItemDef[];
}

/**
 * The §1.3 nav model, derived directly from `internal/api/router.go` RBAC gates
 * and kept in lockstep with `router.ts`. Each item carries its declarative gate;
 * the rail must NOT render a section the role cannot reach (no dead-end 403s).
 */
const NAV_MODEL: NavGroupDef[] = [
  {
    label: 'Portfolio',
    items: [
      {
        label: 'Financials',
        href: '/portfolio/financials',
        icon: 'dollar',
        gate: { minRole: 'superintendent' },
      },
      { label: 'Projects', href: '/portfolio/projects', icon: 'folder' },
      {
        label: 'Pipeline',
        href: '/portfolio/pipeline',
        icon: 'trending-up',
        gate: { minRole: 'superintendent' },
      },
      {
        label: 'Fleet',
        href: '/portfolio/fleet',
        icon: 'truck',
        gate: { minRole: 'superintendent' },
      },
      {
        label: 'HR & Certs',
        href: '/portfolio/hr',
        icon: 'users',
        gate: { roles: ['owner', 'admin'] },
      },
    ],
  },
  {
    label: 'Command Center',
    items: [
      {
        label: 'Daily Briefing',
        href: '/command/briefing',
        icon: 'sun',
        gate: { minRole: 'superintendent' },
      },
      { label: 'Schedule', href: '/command/schedule', icon: 'calendar' },
      {
        label: 'Procurement',
        href: '/command/procurement',
        icon: 'package',
        gate: { minRole: 'superintendent' },
      },
      {
        // Role-gated only — NOT plan-gated (ESC-002: the pro gate was dropped;
        // mirrors the route gate in router.ts).
        label: 'AI Assistant',
        href: '/command/assistant',
        icon: 'sparkles',
        gate: { minRole: 'superintendent' },
      },
    ],
  },
  {
    label: 'Manage',
    items: [
      {
        label: 'Activity',
        href: '/activity',
        icon: 'history',
        gate: { roles: ['owner', 'admin'] },
      },
      {
        label: 'Organization',
        href: '/settings/org',
        icon: 'building',
        gate: { roles: ['owner', 'admin'] },
      },
      {
        label: 'Integrations',
        href: '/settings/integrations',
        icon: 'key',
        gate: { roles: ['owner'] },
      },
      {
        label: 'AI Agents',
        href: '/settings/agents',
        icon: 'sliders',
        gate: { roles: ['owner', 'admin'] },
      },
      {
        label: 'Connectors',
        href: '/settings/connectors',
        icon: 'command',
        gate: { roles: ['owner', 'admin'] },
      },
      {
        label: 'Users & Roles',
        href: '/settings/users',
        icon: 'users',
        gate: { roles: ['owner', 'admin'] },
      },
      { label: 'Notifications', href: '/settings/notifications', icon: 'bell' },
      { label: 'Profile', href: '/profile', icon: 'user' },
    ],
  },
];

/**
 * `fb-nav-rail` — the role-gated workspace navigation (DSC §4.2, §1.3).
 *
 * Role gating is declarative and lives in `NAV_MODEL` (one source, mirrors
 * `router.ts`). The rail evaluates each item's gate against the supplied
 * `role`/`plan` claims and omits anything the role can't reach — a whole group
 * disappears when none of its items are visible, so there are no dead-end 403s.
 * Items are presentational `fb-nav-item`s; their composed `navigate` event
 * bubbles through to the SPA router. `collapsed` drops to icon-only at the
 * Desktop breakpoint (labels hidden, titles preserved for hover/SR).
 *
 * Claims are passed in (not read from the store here) so the rail stays pure and
 * testable; `fb-org-shell` wires the live auth signals.
 */
@customElement('fb-nav-rail')
export class FbNavRail extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
        height: 100%;
      }
      nav {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-lg);
        padding: var(--fb-spacing-md);
        height: 100%;
        overflow-y: auto;
      }
      .group-label {
        padding: 0 var(--fb-spacing-md);
        font-size: var(--fb-text-label-sm);
        font-weight: 700;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        /* secondary, not muted: group labels are real text — muted
           (#5a5b66) is ~2.9:1 on the rail surface, below WCAG AA 4.5:1
           (caught by the live axe sweep once it could actually run). */
        color: var(--fb-text-secondary);
      }
      :host([collapsed]) .group-label {
        display: none;
      }
      ul {
        list-style: none;
        margin: var(--fb-spacing-xs) 0 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
      }
    `,
  ];

  /** Current user's role; `null` until the session is known. Named `user-role`
   * (not `role`) to avoid shadowing the host's native ARIA `role` attribute. */
  @property({ type: String, attribute: 'user-role' }) userRole: Role | null = null;
  /** Current plan tier; gates the pro-only AI surfaces. */
  @property({ type: String }) plan: PlanTier | null = null;
  /** Active route path — the matching item gets `aria-current="page"`. */
  @property({ type: String }) current = '';
  /** Icon-only rail (Desktop breakpoint). */
  @property({ type: Boolean, reflect: true }) collapsed = false;

  private canSee(gate: NavGate | undefined): boolean {
    if (!gate) return true;
    if (this.userRole === null) return false;
    if (gate.roles && !roleIn(this.userRole, gate.roles)) return false;
    if (gate.minRole && !roleAtLeast(this.userRole, gate.minRole)) return false;
    if (gate.requiresPro && this.plan !== 'pro' && this.plan !== 'enterprise') return false;
    return true;
  }

  override render(): TemplateResult {
    const groups = NAV_MODEL.map((g) => ({
      label: g.label,
      items: g.items.filter((it) => this.canSee(it.gate)),
    })).filter((g) => g.items.length > 0);

    return html`<nav aria-label="Primary">
      ${groups.map(
        (g) =>
          html`<div class="group">
            <div class="group-label">${g.label}</div>
            <ul>
              ${g.items.map(
                (it) =>
                  html`<li>
                    <fb-nav-item
                      icon=${it.icon}
                      label=${it.label}
                      href=${it.href}
                      ?active=${this.current === it.href}
                      ?collapsed=${this.collapsed}
                    ></fb-nav-item>
                  </li>`,
              )}
            </ul>
          </div>`,
      )}
      ${groups.length === 0
        ? html`<span class="visually-hidden">No sections available.</span>`
        : nothing}
    </nav>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-nav-rail': FbNavRail;
  }
}
