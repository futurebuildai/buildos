import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCents, formatCentsCompact, type CurrencyCode } from '../../utils/currency.js';

export interface AgingBucket {
  label: string;
  amountCents: number;
}

/**
 * fb-ar-aging-chart — Stacked horizontal bar chart for AR aging.
 *
 * Buckets: Current, 30-day, 60-day, 90+ day
 * Colors: Gable Green, Blueprint Blue, Amber, Safety Red
 *
 * @property buckets - Array of { label, amountCents }
 * @property currencyCode - Currency code (USD or CAD)
 */
@customElement('fb-ar-aging-chart')
export class FBArAgingChart extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .chart-container {
        padding: var(--fb-space-5);
      }

      .chart-title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        font-weight: 500;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
      }

      .bar-container {
        display: flex;
        height: 32px;
        border-radius: var(--fb-radius-sm);
        overflow: hidden;
        margin-bottom: var(--fb-space-4);
      }

      .bar-segment {
        height: 100%;
        min-width: 2px;
        transition: flex-grow var(--fb-transition-normal);
        position: relative;
      }
      .bar-segment:hover { opacity: 0.85; }

      .bar-current { background: var(--fb-gable-green); }
      .bar-30 { background: var(--fb-blueprint-blue); }
      .bar-60 { background: var(--fb-amber); }
      .bar-90 { background: var(--fb-safety-red); }

      .legend {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
        gap: var(--fb-space-3);
      }

      .legend-item {
        display: flex;
        flex-direction: column;
        gap: 2px;
      }

      .legend-header {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
      }

      .legend-dot {
        width: 10px;
        height: 10px;
        border-radius: 2px;
        flex-shrink: 0;
      }
      .dot-current { background: var(--fb-gable-green); }
      .dot-30 { background: var(--fb-blueprint-blue); }
      .dot-60 { background: var(--fb-amber); }
      .dot-90 { background: var(--fb-safety-red); }

      .legend-label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
      }

      .legend-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-base);
        font-weight: 500;
        font-variant-numeric: tabular-nums;
        color: var(--fb-text-primary);
        padding-left: 18px;
      }

      .legend-pct {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        padding-left: 18px;
      }

      .total-row {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding-top: var(--fb-space-3);
        border-top: 1px solid var(--fb-border);
        margin-top: var(--fb-space-3);
      }

      .total-label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
      }

      .total-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-lg);
        font-weight: 500;
        font-variant-numeric: tabular-nums;
        color: var(--fb-text-primary);
      }
    `,
  ];

  @property({ type: Array }) buckets: AgingBucket[] = [
    { label: 'Current', amountCents: 0 },
    { label: '30 Day', amountCents: 0 },
    { label: '60 Day', amountCents: 0 },
    { label: '90+ Day', amountCents: 0 },
  ];
  @property({ type: String }) currencyCode: CurrencyCode = 'USD';

  private _colorClasses = ['bar-current', 'bar-30', 'bar-60', 'bar-90'];
  private _dotClasses = ['dot-current', 'dot-30', 'dot-60', 'dot-90'];

  override render() {
    const total = this.buckets.reduce((sum, b) => sum + b.amountCents, 0);

    return html`
      <div class="chart-container glass-card">
        <div class="chart-title">Accounts Receivable Aging</div>

        <div class="bar-container">
          ${this.buckets.map((bucket, i) => {
            const pct = total > 0 ? (bucket.amountCents / total) * 100 : 0;
            return html`
              <div
                class="bar-segment ${this._colorClasses[i] ?? ''}"
                style="flex-grow: ${pct}"
                title="${bucket.label}: ${formatCents(bucket.amountCents, this.currencyCode)}"
              ></div>
            `;
          })}
        </div>

        <div class="legend">
          ${this.buckets.map((bucket, i) => {
            const pct = total > 0 ? ((bucket.amountCents / total) * 100).toFixed(1) : '0.0';
            return html`
              <div class="legend-item">
                <div class="legend-header">
                  <span class="legend-dot ${this._dotClasses[i] ?? ''}"></span>
                  <span class="legend-label">${bucket.label}</span>
                </div>
                <span class="legend-value">${formatCentsCompact(bucket.amountCents, this.currencyCode)}</span>
                <span class="legend-pct">${pct}%</span>
              </div>
            `;
          })}
        </div>

        <div class="total-row">
          <span class="total-label">Total Outstanding</span>
          <span class="total-value">${formatCents(total, this.currencyCode)}</span>
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-ar-aging-chart': FBArAgingChart;
  }
}
