import { html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

/**
 * fb-nav-item — Sidebar navigation item with icon, label, active state, badge count.
 *
 * @property icon - Icon name from fb-icon map
 * @property label - Navigation label text
 * @property active - Whether this item is currently active
 * @property badge - Badge count (0 = hidden)
 * @property href - Navigation target
 * @fires fb-nav - Emitted on click with { href }
 */
@customElement('fb-nav-item')
export class FBNavItem extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .nav-item {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
        padding: var(--fb-space-2) var(--fb-space-4);
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
        color: var(--fb-text-secondary);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        font-weight: 400;
        text-decoration: none;
        position: relative;
        transition: all var(--fb-transition-fast);
        min-height: 40px;
      }

      .nav-item:hover {
        background: var(--fb-surface-hover);
        color: var(--fb-text-primary);
      }

      .nav-item.active {
        color: var(--fb-gable-green);
        background: rgba(0, 255, 163, 0.05);
      }

      .nav-item.active::before {
        content: '';
        position: absolute;
        left: 0;
        top: 50%;
        transform: translateY(-50%);
        width: 3px;
        height: 60%;
        background: var(--fb-gable-green);
        border-radius: 0 3px 3px 0;
      }

      .label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

      .badge-count {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        font-variant-numeric: tabular-nums;
        background: rgba(0, 255, 163, 0.15);
        color: var(--fb-gable-green);
        padding: 2px 6px;
        border-radius: 9999px;
        min-width: 20px;
        text-align: center;
        line-height: 1.2;
      }
    `,
  ];

  @property({ type: String }) icon = '';
  @property({ type: String }) label = '';
  @property({ type: Boolean }) active = false;
  @property({ type: Number }) badge = 0;
  @property({ type: String }) href = '';

  override render() {
    return html`
      <a
        class="nav-item ${this.active ? 'active' : ''}"
        @click=${this._onClick}
        role="menuitem"
        tabindex="0"
        aria-current=${this.active ? 'page' : 'false'}
      >
        ${this.icon ? html`<fb-icon name=${this.icon} size="sm"></fb-icon>` : nothing}
        <span class="label">${this.label}</span>
        ${this.badge > 0 ? html`<span class="badge-count">${this.badge}</span>` : nothing}
      </a>
    `;
  }

  private _onClick(e: Event) {
    e.preventDefault();
    this.emitEvent('fb-nav', { href: this.href });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-nav-item': FBNavItem;
  }
}
