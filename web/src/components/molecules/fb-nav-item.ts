import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import type { IconName } from '../atoms/icons.js';

/**
 * `fb-nav-item` — one entry in the nav rail (DSC §4.2, AG-05). Presentational:
 * the parent `fb-nav-rail` decides visibility from role/plan claims and passes
 * `active`/`collapsed` down. The active item gets the `.active-indicator` left-bar
 * (DESIGN_SYSTEM §8.1). Renders as a real link; clicks emit a composed `navigate`
 * so the SPA router intercepts. Optional `count` shows a notification pill.
 */
@customElement('fb-nav-item')
export class FbNavItem extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      a {
        position: relative;
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        color: var(--fb-text-secondary);
        text-decoration: none;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-body-md);
        font-weight: 600;
        white-space: nowrap;
      }
      a:hover {
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
      }
      :host([active]) a {
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
      }
      .label {
        flex: 1;
      }
      :host([collapsed]) .label {
        display: none;
      }
      .count {
        min-width: 18px;
        height: 18px;
        padding: 0 5px;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        font-size: var(--fb-text-label-sm);
        font-weight: 700;
        color: var(--fb-deep-space);
        background: var(--fb-gable-green);
        border-radius: var(--fb-radius-full);
      }
    `,
  ];

  @property({ type: String }) icon?: IconName;
  @property({ type: String }) label = '';
  @property({ type: String }) href = '#';
  @property({ type: Boolean, reflect: true }) active = false;
  @property({ type: Boolean, reflect: true }) collapsed = false;
  @property({ type: Number }) count?: number;

  private onClick(e: Event): void {
    e.preventDefault();
    this.emit('navigate', { href: this.href });
  }

  override render(): TemplateResult {
    return html`
      <a
        class=${this.active ? 'active-indicator' : ''}
        href=${this.href}
        aria-current=${this.active ? 'page' : nothing}
        title=${this.collapsed ? this.label : nothing}
        @click=${this.onClick}
      >
        ${this.icon ? html`<fb-icon name=${this.icon} size="20"></fb-icon>` : nothing}
        <span class="label">${this.label}</span>
        ${this.count
          ? html`<span class="count" aria-label="${this.count} new">${this.count}</span>`
          : nothing}
      </a>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-nav-item': FbNavItem;
  }
}
