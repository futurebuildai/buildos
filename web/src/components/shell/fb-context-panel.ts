import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';

/**
 * `fb-context-panel` — the right-hand on-demand panel (DSC §4.3). Hosts one of:
 * chat artifacts, record detail, or `fb-audit-trail`, supplied via the default
 * slot. `complementary` landmark, labelled by its heading. Dismissible (emits
 * `close`; Esc also closes). Slide-in uses Emphasized motion, suppressed under
 * reduced-motion. Renders nothing while closed (on-demand surface).
 */
@customElement('fb-context-panel')
export class FbContextPanel extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: contents;
      }
      aside {
        display: flex;
        flex-direction: column;
        width: var(--fb-context-width, 320px);
        height: 100%;
        background: var(--fb-surface-1);
        border-left: 1px solid var(--md-sys-color-outline);
        animation: slide-in var(--fb-motion-emphasized) var(--fb-ease-out);
      }
      @keyframes slide-in {
        from {
          transform: translateX(16px);
          opacity: 0;
        }
        to {
          transform: translateX(0);
          opacity: 1;
        }
      }
      @media (prefers-reduced-motion: reduce) {
        aside {
          animation: none;
        }
      }
      .head {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        border-bottom: 1px solid var(--fb-border);
      }
      .title {
        flex: 1;
        font-size: var(--fb-text-title-md);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .close {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 32px;
        height: 32px;
        color: var(--fb-text-secondary);
        background: none;
        border: none;
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
      }
      .close:hover {
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
      }
      .body {
        flex: 1;
        overflow-y: auto;
        padding: var(--fb-spacing-md);
      }
    `,
  ];

  @property({ type: Boolean, reflect: true }) open = false;
  @property({ type: String }) heading = '';

  private readonly titleId = `fb-ctx-title-${Math.random().toString(36).slice(2, 9)}`;

  override connectedCallback(): void {
    super.connectedCallback();
    this.addEventListener('keydown', this.onKeydown);
  }

  override disconnectedCallback(): void {
    this.removeEventListener('keydown', this.onKeydown);
    super.disconnectedCallback();
  }

  private readonly onKeydown = (e: KeyboardEvent): void => {
    if (e.key === 'Escape' && this.open) {
      e.preventDefault();
      this.emit('close');
    }
  };

  override render(): TemplateResult {
    if (!this.open) return html`${nothing}`;
    return html`<aside
      role="complementary"
      aria-labelledby=${this.heading ? this.titleId : nothing}
    >
      <div class="head">
        <span class="title" id=${this.titleId}>${this.heading}</span>
        <button
          class="close"
          type="button"
          aria-label="Close panel"
          @click=${() => this.emit('close')}
        >
          <fb-icon name="x" size="18"></fb-icon>
        </button>
      </div>
      <div class="body"><slot></slot></div>
    </aside>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-context-panel': FbContextPanel;
  }
}
