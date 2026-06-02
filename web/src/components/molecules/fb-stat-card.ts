import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import type { IconName } from '../atoms/icons.js';

/**
 * `fb-stat-card` — a single KPI tile (DSC §7, AG-05). Used across financials,
 * portfolio, and briefing. The value is slotted so callers can drop in an
 * `fb-money` (mono/tabular, per-currency) rather than a formatted string. An
 * optional delta line carries a direction arrow + sign so trend is never
 * color-only (WCAG 1.4.1).
 */
@customElement('fb-stat-card')
export class FbStatCard extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .card {
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
      .head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-label-lg);
      }
      .value {
        margin-top: var(--fb-spacing-xs);
        font-size: var(--fb-text-title-lg);
        font-weight: 700;
        color: var(--fb-text-primary);
      }
      .delta {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-xs);
        font-size: var(--fb-text-body-sm);
        font-family: var(--fb-font-mono);
      }
      .delta.up {
        color: var(--fb-variance-positive);
      }
      .delta.down {
        color: var(--fb-variance-negative);
      }
    `,
  ];

  @property({ type: String }) heading = '';
  @property({ type: String }) icon?: IconName;
  /** Signed percentage/number for the trend line; omit to hide the delta. */
  @property({ type: Number }) delta?: number;
  @property({ type: String }) deltaLabel?: string;

  override render(): TemplateResult {
    const dir = this.delta === undefined ? 0 : this.delta > 0 ? 1 : this.delta < 0 ? -1 : 0;
    const cls = dir > 0 ? 'delta up' : dir < 0 ? 'delta down' : 'delta';
    const arrow = dir > 0 ? '▲' : dir < 0 ? '▼' : '◆';
    return html`
      <div class="card">
        <div class="head">
          <span>${this.heading}</span>
          ${this.icon ? html`<fb-icon name=${this.icon} size="16"></fb-icon>` : nothing}
        </div>
        <div class="value"><slot></slot></div>
        ${this.delta !== undefined
          ? html`<div class=${cls}>
              <span aria-hidden="true">${arrow}</span>
              <span
                >${this.delta > 0 ? '+' : ''}${this.delta}%${this.deltaLabel
                  ? html` ${this.deltaLabel}`
                  : nothing}</span
              >
            </div>`
          : nothing}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-stat-card': FbStatCard;
  }
}
