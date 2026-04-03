import { html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

/**
 * fb-org-shell — Top-level app shell: sidebar + main content area + top bar.
 *
 * Responsive layout:
 * - Desktop: 280px sidebar + flex content
 * - Tablet: collapsible sidebar (hamburger)
 * - Mobile: full-width content with bottom nav
 */
@customElement('fb-org-shell')
export class FBOrgShell extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host {
        display: block;
        height: 100vh;
        width: 100%;
        overflow: hidden;
        background: var(--fb-deep-space);
      }

      .shell {
        display: flex;
        height: 100%;
      }

      .sidebar {
        width: 280px;
        flex-shrink: 0;
        height: 100%;
        overflow: hidden;
        transition: width var(--fb-transition-normal), transform var(--fb-transition-normal);
      }
      .sidebar.collapsed {
        width: 0;
        transform: translateX(-280px);
      }

      .main-area {
        flex: 1;
        display: flex;
        flex-direction: column;
        min-width: 0;
        height: 100%;
      }

      .top-bar {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: var(--fb-space-3) var(--fb-space-6);
        border-bottom: 1px solid var(--fb-border);
        background: var(--fb-slate-steel);
        flex-shrink: 0;
        min-height: 56px;
        gap: var(--fb-space-4);
      }

      .top-bar-left {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
      }

      .menu-btn {
        display: none;
        background: none;
        border: none;
        color: var(--fb-text-secondary);
        cursor: pointer;
        padding: 6px;
        border-radius: var(--fb-radius-sm);
      }
      .menu-btn:hover { background: rgba(255,255,255,0.05); color: var(--fb-text-primary); }

      .top-bar-right {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
      }

      .content {
        flex: 1;
        overflow-y: auto;
        padding: var(--fb-space-6);
      }

      .overlay {
        display: none;
        position: fixed;
        inset: 0;
        background: rgba(0,0,0,0.5);
        z-index: 10;
      }

      @media (max-width: 1200px) {
        .sidebar {
          position: fixed;
          z-index: 20;
          top: 0;
          left: 0;
          transform: translateX(-280px);
        }
        .sidebar.open {
          transform: translateX(0);
        }
        .sidebar.collapsed { transform: translateX(-280px); }
        .menu-btn { display: flex; }
        .overlay.visible { display: block; }
      }

      @media (max-width: 768px) {
        .content { padding: var(--fb-space-4); }
        .top-bar { padding: var(--fb-space-2) var(--fb-space-4); }
      }
    `,
  ];

  @state() private _sidebarOpen = true;
  @state() private _isMobile = false;

  override connectedCallback() {
    super.connectedCallback();
    this._checkViewport();
    window.addEventListener('resize', this._checkViewport);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('resize', this._checkViewport);
  }

  private _checkViewport = () => {
    this._isMobile = window.innerWidth <= 1200;
    if (this._isMobile) {
      this._sidebarOpen = false;
    }
  };

  override render() {
    const sidebarClass = this._isMobile
      ? `sidebar ${this._sidebarOpen ? 'open' : ''}`
      : `sidebar ${this._sidebarOpen ? '' : 'collapsed'}`;

    return html`
      <div class="shell">
        <div class=${sidebarClass}>
          <fb-nav-sidebar @fb-nav-collapse=${this._toggleSidebar}></fb-nav-sidebar>
        </div>

        <div class="overlay ${this._isMobile && this._sidebarOpen ? 'visible' : ''}"
          @click=${() => { this._sidebarOpen = false; }}>
        </div>

        <div class="main-area">
          <div class="top-bar">
            <div class="top-bar-left">
              <button class="menu-btn" @click=${this._toggleSidebar} aria-label="Toggle menu">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M3 18h18v-2H3v2zm0-5h18v-2H3v2zm0-7v2h18V6H3z"/>
                </svg>
              </button>
              <slot name="breadcrumb"></slot>
            </div>
            <div class="top-bar-right">
              <slot name="top-bar-actions"></slot>
              <fb-avatar name="User" size="sm"></fb-avatar>
            </div>
          </div>
          <div class="content">
            <slot></slot>
          </div>
        </div>
      </div>
    `;
  }

  private _toggleSidebar() {
    this._sidebarOpen = !this._sidebarOpen;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-org-shell': FBOrgShell;
  }
}
