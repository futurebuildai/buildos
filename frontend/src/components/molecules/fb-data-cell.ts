import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCents, type CurrencyCode } from '../../utils/currency.js';

export type DataCellType = 'text' | 'currency' | 'date' | 'status' | 'number' | 'percent';

/**
 * fb-data-cell — Table cell with type-aware formatting.
 *
 * Currency and number types use JetBrains Mono. Currency values are formatted
 * from cents using the formatCents utility.
 *
 * @property type - Cell data type
 * @property value - Raw value (string or number)
 * @property currencyCode - Currency code for currency type
 * @property statusVariant - Badge variant for status type
 */
@customElement('fb-data-cell')
export class FBDataCell extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: inline; }

      .cell { display: inline; }

      .currency, .number, .percent {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }

      .text {
        font-family: var(--fb-font-body);
      }

      .date {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
      }

      .positive { color: var(--fb-gable-green); }
      .negative { color: var(--fb-safety-red); }
    `,
  ];

  @property({ type: String }) type: DataCellType = 'text';
  @property({ type: String }) value: string | number = '';
  @property({ type: String }) currencyCode: CurrencyCode = 'USD';
  @property({ type: String }) statusVariant: string = 'neutral';

  override render() {
    switch (this.type) {
      case 'currency': {
        const cents = typeof this.value === 'string' ? parseInt(this.value, 10) : this.value;
        const numCents = isNaN(cents as number) ? 0 : (cents as number);
        const formatted = formatCents(numCents, this.currencyCode);
        const cls = numCents > 0 ? 'positive' : numCents < 0 ? 'negative' : '';
        return html`<span class="cell currency ${cls}">${formatted}</span>`;
      }
      case 'number': {
        const num = typeof this.value === 'string' ? parseFloat(this.value) : this.value;
        const formatted = new Intl.NumberFormat('en-US').format(num as number);
        return html`<span class="cell number">${formatted}</span>`;
      }
      case 'percent': {
        const pct = typeof this.value === 'string' ? parseFloat(this.value) : this.value;
        const pctNum = pct as number;
        const cls = pctNum > 0 ? 'positive' : pctNum < 0 ? 'negative' : '';
        return html`<span class="cell percent ${cls}">${pctNum > 0 ? '+' : ''}${pctNum.toFixed(1)}%</span>`;
      }
      case 'date': {
        const str = String(this.value);
        let display = str;
        try {
          const d = new Date(str);
          if (!isNaN(d.getTime())) {
            display = d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
          }
        } catch {
          // keep raw string
        }
        return html`<span class="cell date">${display}</span>`;
      }
      case 'status':
        return html`<fb-badge variant=${this.statusVariant}>${this.value}</fb-badge>`;
      default:
        return html`<span class="cell text">${this.value}</span>`;
    }
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-data-cell': FBDataCell;
  }
}
