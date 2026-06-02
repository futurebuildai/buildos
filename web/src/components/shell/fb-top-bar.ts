import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import { roleAtLeast, type Role } from '../../auth/jwt.js';

export type Workspace = 'portfolio' | 'command';
export type Density = 'comfortable' | 'compact';

/**
 * `fb-top-bar` — the glass app-bar (DSC §4.1). Hosts the brand mark, a workspace
 * switcher (Portfolio ⇄ Command Center), the ⌘K command-palette trigger, a
 * density toggle, a notifications button, and the profile button.
 *
 * The Command Center workspace is hidden when the role can't reach any of its
 * sections (field_worker, per §1.3) — the switcher never offers a dead end.
 * `banner` landmark. Emits: `workspace-change` ({ workspace }),
 * `command-palette`, `density-change` ({ density }), `notifications`, `profile`.
 */
@customElement('fb-top-bar')
export class FbTopBar extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      header {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-lg);
        height: 56px;
        padding: 0 var(--fb-spacing-lg);
        border-bottom: 1px solid var(--fb-glass-border);
      }
      .brand {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        font-weight: 700;
        font-size: var(--fb-text-title-md);
        color: var(--fb-text-primary);
        white-space: nowrap;
      }
      .brand fb-icon {
        color: var(--fb-gable-green);
      }
      .switcher {
        display: inline-flex;
        background: var(--fb-surface-2);
        border-radius: var(--fb-radius-full);
        padding: 2px;
      }
      .switcher button {
        font: inherit;
        font-size: var(--fb-text-label-lg);
        font-weight: 600;
        color: var(--fb-text-secondary);
        background: none;
        border: none;
        padding: 6px var(--fb-spacing-md);
        border-radius: var(--fb-radius-full);
        cursor: pointer;
      }
      .switcher button[aria-pressed='true'] {
        background: var(--fb-surface-1);
        color: var(--fb-text-primary);
      }
      .spacer {
        flex: 1;
      }
      .actions {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
      }
      .iconbtn {
        position: relative;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 40px;
        height: 40px;
        color: var(--fb-text-secondary);
        background: none;
        border: none;
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
      }
      .iconbtn:hover {
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
      }
      .cmdk {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        font: inherit;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
        padding: 6px var(--fb-spacing-md);
        cursor: pointer;
      }
      .cmdk kbd {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-label-sm);
        color: var(--fb-text-secondary);
      }
      .badge {
        position: absolute;
        top: 4px;
        right: 4px;
        min-width: 16px;
        height: 16px;
        padding: 0 4px;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        font-size: 10px;
        font-weight: 700;
        color: var(--fb-deep-space);
        background: var(--fb-safety-red);
        border-radius: var(--fb-radius-full);
      }
    `,
  ];

  /** Current user's role; named `user-role` to avoid the native ARIA `role`. */
  @property({ type: String, attribute: 'user-role' }) userRole: Role | null = null;
  @property({ type: String }) workspace: Workspace = 'portfolio';
  @property({ type: String }) density: Density = 'comfortable';
  /** Unread notification count; shows a red pill when > 0. */
  @property({ type: Number }) notifications = 0;

  private get canCommand(): boolean {
    // Command Center requires at least superintendent (§1.3); field_worker hidden.
    return this.userRole !== null && roleAtLeast(this.userRole, 'superintendent');
  }

  private switchTo(ws: Workspace): void {
    if (ws !== this.workspace) this.emit('workspace-change', { workspace: ws });
  }

  private toggleDensity(): void {
    const next: Density = this.density === 'comfortable' ? 'compact' : 'comfortable';
    this.emit('density-change', { density: next });
  }

  override render(): TemplateResult {
    return html`<header role="banner">
      <span class="brand">
        <fb-icon name="hexagon" size="22" label="BuildOS"></fb-icon>
        BuildOS
      </span>

      ${this.canCommand
        ? html`<div class="switcher" role="group" aria-label="Workspace">
            <button
              type="button"
              aria-pressed=${this.workspace === 'portfolio'}
              @click=${() => this.switchTo('portfolio')}
            >
              Portfolio
            </button>
            <button
              type="button"
              aria-pressed=${this.workspace === 'command'}
              @click=${() => this.switchTo('command')}
            >
              Command Center
            </button>
          </div>`
        : nothing}

      <div class="spacer"></div>

      <div class="actions">
        <button class="cmdk" type="button" @click=${() => this.emit('command-palette')}>
          <fb-icon name="search" size="16"></fb-icon>
          Search
          <kbd>⌘K</kbd>
        </button>
        <button
          class="iconbtn"
          type="button"
          aria-pressed=${this.density === 'compact'}
          aria-label=${this.density === 'compact'
            ? 'Switch to comfortable density'
            : 'Switch to compact density'}
          @click=${this.toggleDensity}
        >
          <fb-icon name="sliders" size="20"></fb-icon>
        </button>
        <button
          class="iconbtn"
          type="button"
          aria-label=${this.notifications > 0
            ? `Notifications, ${this.notifications} unread`
            : 'Notifications'}
          @click=${() => this.emit('notifications')}
        >
          <fb-icon name="bell" size="20"></fb-icon>
          ${this.notifications > 0
            ? html`<span class="badge"
                >${this.notifications > 99 ? '99+' : this.notifications}</span
              >`
            : nothing}
        </button>
        <button
          class="iconbtn"
          type="button"
          aria-label="Profile and account"
          @click=${() => this.emit('profile')}
        >
          <fb-icon name="user" size="20"></fb-icon>
        </button>
      </div>
    </header>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-top-bar': FbTopBar;
  }
}
