import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export type BadgeVariant = 'success' | 'warning' | 'error' | 'info' | 'neutral';

/**
 * fb-badge — Status badge with semantic colors.
 *
 * @property variant - Semantic variant: success | warning | error | info | neutral
 * @property size - Display size: sm | md
 */
@customElement('fb-badge')
export class FBBadge extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: inline-flex; }

      .badge {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        border-radius: 9999px;
        font-family: var(--fb-font-body);
        font-weight: 500;
        white-space: nowrap;
        line-height: 1;
      }

      .sm { padding: 2px 8px; font-size: var(--fb-text-xs); }
      .md { padding: 4px 12px; font-size: var(--fb-text-sm); }

      .dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        flex-shrink: 0;
      }

      .success { background: rgba(0, 255, 163, 0.12); color: var(--fb-gable-green); }
      .success .dot { background: var(--fb-gable-green); }

      .warning { background: rgba(245, 158, 11, 0.12); color: var(--fb-amber); }
      .warning .dot { background: var(--fb-amber); }

      .error { background: rgba(244, 63, 94, 0.12); color: var(--fb-safety-red); }
      .error .dot { background: var(--fb-safety-red); }

      .info { background: rgba(56, 189, 248, 0.12); color: var(--fb-blueprint-blue); }
      .info .dot { background: var(--fb-blueprint-blue); }

      .neutral { background: rgba(139, 141, 152, 0.12); color: var(--fb-text-secondary); }
      .neutral .dot { background: var(--fb-text-secondary); }
    `,
  ];

  @property({ type: String }) variant: BadgeVariant = 'neutral';
  @property({ type: String }) size: 'sm' | 'md' = 'sm';

  override render() {
    return html`
      <span class="badge ${this.variant} ${this.size}">
        <span class="dot"></span>
        <slot></slot>
      </span>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-badge': FBBadge;
  }
}
