import { html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

/**
 * fb-stat-card — KPI card with label, value, trend indicator, and description.
 *
 * Value is displayed in JetBrains Mono. Trend indicator shows positive (green)
 * or negative (red) change.
 *
 * @property label - KPI label (e.g., "Estimated Budget")
 * @property value - KPI value string (pre-formatted, e.g., "$1,234,567")
 * @property trend - Trend string (e.g., "+2.3%", "-$4,500")
 * @property trendDirection - Positive or negative trend for coloring
 * @property description - Additional context text
 */
@customElement('fb-stat-card')
export class FBStatCard extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .card {
        padding: var(--fb-space-5);
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
      }

      .label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }

      .value-row {
        display: flex;
        align-items: baseline;
        gap: var(--fb-space-3);
      }

      .value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-3xl);
        font-weight: 500;
        font-variant-numeric: tabular-nums;
        color: var(--fb-text-primary);
        line-height: 1.2;
      }

      .trend {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        font-variant-numeric: tabular-nums;
      }
      .trend.positive { color: var(--fb-gable-green); }
      .trend.negative { color: var(--fb-safety-red); }
      .trend.neutral { color: var(--fb-text-muted); }

      .description {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        line-height: 1.4;
      }
    `,
  ];

  @property({ type: String }) label = '';
  @property({ type: String }) value = '';
  @property({ type: String }) trend = '';
  @property({ type: String }) trendDirection: 'positive' | 'negative' | 'neutral' = 'neutral';
  @property({ type: String }) description = '';

  override render() {
    return html`
      <div class="card glass-card">
        <span class="label">${this.label}</span>
        <div class="value-row">
          <span class="value">${this.value}</span>
          ${this.trend
            ? html`<span class="trend ${this.trendDirection}">${this.trend}</span>`
            : nothing
          }
        </div>
        ${this.description
          ? html`<span class="description">${this.description}</span>`
          : nothing
        }
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-stat-card': FBStatCard;
  }
}
