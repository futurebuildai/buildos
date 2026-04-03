import { html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

interface NavSection {
  label: string;
  items: Array<{ icon: string; label: string; href: string; badge?: number }>;
}

const NAV_SECTIONS: NavSection[] = [
  {
    label: 'Overview',
    items: [
      { icon: 'dashboard', label: 'Dashboard', href: '/dashboard' },
      { icon: 'briefing', label: 'Briefing', href: '/briefing' },
    ],
  },
  {
    label: 'Operations',
    items: [
      { icon: 'schedule', label: 'Schedule', href: '/schedule' },
      { icon: 'pipeline', label: 'Pipeline', href: '/pipeline' },
      { icon: 'money', label: 'Financials', href: '/financials' },
      { icon: 'procurement', label: 'Procurement', href: '/procurement' },
    ],
  },
  {
    label: 'Resources',
    items: [
      { icon: 'fleet', label: 'Fleet', href: '/fleet' },
      { icon: 'people', label: 'HR', href: '/hr' },
    ],
  },
  {
    label: 'System',
    items: [
      { icon: 'settings', label: 'Settings', href: '/settings' },
    ],
  },
];

/**
 * fb-nav-sidebar — Glass-panel navigation sidebar.
 *
 * Contains logo, nav items for all pages, and collapse button.
 * Uses glass-panel styling with deep space background.
 *
 * @fires fb-nav - Bubbled from fb-nav-item with { href }
 * @fires fb-nav-collapse - Emitted when collapse button is clicked
 */
@customElement('fb-nav-sidebar')
export class FBNavSidebar extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host {
        display: flex;
        flex-direction: column;
        height: 100%;
        background: rgba(10, 11, 16, 0.8);
        backdrop-filter: blur(48px);
        -webkit-backdrop-filter: blur(48px);
        border-right: 1px solid var(--fb-border);
        width: 280px;
      }

      .logo {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
        padding: var(--fb-space-4) var(--fb-space-5);
        border-bottom: 1px solid var(--fb-border);
        flex-shrink: 0;
      }

      .logo-text {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-lg);
        font-weight: 700;
        color: var(--fb-text-primary);
      }
      .logo-text span { color: var(--fb-gable-green); }

      .nav-content {
        flex: 1;
        overflow-y: auto;
        padding: var(--fb-space-3) var(--fb-space-2);
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-1);
      }

      .section-label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-xs);
        font-weight: 500;
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.06em;
        padding: var(--fb-space-3) var(--fb-space-4) var(--fb-space-1);
      }

      .footer {
        flex-shrink: 0;
        padding: var(--fb-space-3) var(--fb-space-4);
        border-top: 1px solid var(--fb-border);
      }

      .collapse-btn {
        display: flex;
        align-items: center;
        justify-content: center;
        width: 100%;
        gap: var(--fb-space-2);
        background: none;
        border: none;
        color: var(--fb-text-muted);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        cursor: pointer;
        padding: var(--fb-space-2);
        border-radius: var(--fb-radius-sm);
        transition: all var(--fb-transition-fast);
      }
      .collapse-btn:hover { background: rgba(255,255,255,0.03); color: var(--fb-text-secondary); }
    `,
  ];

  @state() private _activePath = '/dashboard';

  override connectedCallback() {
    super.connectedCallback();
    this._activePath = window.location.hash.replace('#', '') || '/dashboard';
    window.addEventListener('hashchange', this._onHashChange);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('hashchange', this._onHashChange);
  }

  private _onHashChange = () => {
    this._activePath = window.location.hash.replace('#', '') || '/dashboard';
  };

  override render() {
    return html`
      <div class="logo">
        <svg width="28" height="28" viewBox="0 0 24 24" fill="#00FFA3">
          <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
        </svg>
        <span class="logo-text">Future<span>Build</span> OS</span>
      </div>

      <nav class="nav-content" role="navigation">
        ${NAV_SECTIONS.map(section => html`
          <span class="section-label">${section.label}</span>
          ${section.items.map(item => html`
            <fb-nav-item
              icon=${item.icon}
              label=${item.label}
              href=${item.href}
              ?active=${this._activePath === item.href}
              .badge=${item.badge ?? 0}
              @fb-nav=${(e: CustomEvent) => this._onNavigate(e)}
            ></fb-nav-item>
          `)}
        `)}
      </nav>

      <div class="footer">
        <button class="collapse-btn" @click=${this._onCollapse}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
            <path d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z"/>
          </svg>
          Collapse
        </button>
      </div>
    `;
  }

  private _onNavigate(e: CustomEvent) {
    const href = (e.detail as { href: string }).href;
    this._activePath = href;
    window.location.hash = href;
  }

  private _onCollapse() {
    this.emitEvent('fb-nav-collapse');
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-nav-sidebar': FBNavSidebar;
  }
}
