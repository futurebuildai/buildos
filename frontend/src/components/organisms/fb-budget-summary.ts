import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCents, formatCentsVariance, formatPercent, type CurrencyCode } from '../../utils/currency.js';

/**
 * fb-budget-summary — Three stat cards: Estimated, Committed, Actual.
 *
 * Calculates variance between estimated and actual, displays as percentage
 * with positive (under budget / green) or negative (over budget / red) coloring.
 *
 * @property estimatedCents - Estimated budget in cents
 * @property committedCents - Committed budget in cents
 * @property actualCents - Actual spend in cents
 * @property currencyCode - Currency code (USD or CAD)
 */
@customElement('fb-budget-summary')
export class FBBudgetSummary extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .summary-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
        gap: var(--fb-space-4);
      }
    `,
  ];

  @property({ type: Number }) estimatedCents = 0;
  @property({ type: Number }) committedCents = 0;
  @property({ type: Number }) actualCents = 0;
  @property({ type: String }) currencyCode: CurrencyCode = 'USD';

  private _getVariance(): { trend: string; direction: 'positive' | 'negative' | 'neutral'; description: string } {
    if (this.estimatedCents === 0) {
      return { trend: '', direction: 'neutral', description: '' };
    }

    const varianceCents = this.estimatedCents - this.actualCents;
    const variancePct = (varianceCents / this.estimatedCents) * 100;

    const trend = formatCentsVariance(varianceCents, this.currencyCode);
    const direction = varianceCents > 0 ? 'positive' as const : varianceCents < 0 ? 'negative' as const : 'neutral' as const;
    const description = varianceCents >= 0
      ? `${formatPercent(variancePct)} under budget`
      : `${formatPercent(Math.abs(variancePct))} over budget`;

    return { trend, direction, description };
  }

  override render() {
    const variance = this._getVariance();
    const committedPct = this.estimatedCents > 0
      ? formatPercent((this.committedCents / this.estimatedCents) * 100)
      : '0.0%';

    return html`
      <div class="summary-grid">
        <fb-stat-card
          label="Estimated"
          value=${formatCents(this.estimatedCents, this.currencyCode)}
          description="Total project budget"
        ></fb-stat-card>

        <fb-stat-card
          label="Committed"
          value=${formatCents(this.committedCents, this.currencyCode)}
          trend=${committedPct}
          trendDirection="neutral"
          description="of estimated budget"
        ></fb-stat-card>

        <fb-stat-card
          label="Actual"
          value=${formatCents(this.actualCents, this.currencyCode)}
          trend=${variance.trend}
          trendDirection=${variance.direction}
          description=${variance.description}
        ></fb-stat-card>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-budget-summary': FBBudgetSummary;
  }
}
