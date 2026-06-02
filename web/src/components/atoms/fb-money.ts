import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { formatCents, signOfCents, type CurrencyCode } from '../../money/money.js';

/**
 * `fb-money` — displays a Composite Currency Pattern value (DSC §6.1,
 * API_CONTRACT §2.4). Integer cents come in as a STRING (BigInt-safe; never a JS
 * `number` that could exceed 2^53) paired with a `currency-code`. Always renders
 * in JetBrains Mono with `tabular-nums` so columns align.
 *
 * `variance` colors the sign via the §2.1 variance tokens; because color is never
 * the sole signal, variance mode also prefixes an explicit `+`/`−`. The currency
 * code is appended only when `show-code` is set (USD and CAD share the `$` glyph,
 * so callers disambiguate where it matters). This atom never sums values — mixing
 * currencies is a list-level concern handled by `sumByCurrency`.
 */
@customElement('fb-money')
export class FbMoney extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline;
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
        white-space: nowrap;
      }
      .code {
        margin-inline-start: var(--fb-spacing-xs, 0.25rem);
        color: var(--fb-text-secondary);
        font-size: 0.85em;
      }
      :host([variance]) .amount.positive {
        color: var(--fb-variance-positive);
      }
      :host([variance]) .amount.negative {
        color: var(--fb-variance-negative);
      }
    `,
  ];

  /** Integer cents. STRING is preferred; bigint accepted. Never a float. */
  @property({ type: String }) cents: string | bigint = '0';
  @property({ type: String, attribute: 'currency-code' }) currencyCode: CurrencyCode = 'USD';
  /** Color + sign-prefix positive/negative (budget variance, AR deltas). */
  @property({ type: Boolean, reflect: true }) variance = false;
  /** Append the ISO code (e.g. "USD") for USD/CAD disambiguation. */
  @property({ type: Boolean, attribute: 'show-code' }) showCode = false;

  override render(): TemplateResult {
    let formatted: string;
    try {
      formatted = formatCents(this.cents, this.currencyCode);
    } catch {
      // Bad input must never crash a screen — render the em-dash placeholder.
      return html`<span aria-hidden="true">—</span>`;
    }

    const sign = this.variance ? signOfCents(this.cents) : 0;
    const classes = sign > 0 ? 'amount positive' : sign < 0 ? 'amount negative' : 'amount';
    // In variance mode, prefix an explicit sign so color is never the only cue.
    const display = this.variance && sign > 0 ? `+${formatted}` : formatted;

    return html`<span class=${classes}>${display}</span>${this.showCode
        ? html`<span class="code">${this.currencyCode}</span>`
        : nothing}`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-money': FbMoney;
  }
}
