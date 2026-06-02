import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { dollarsToCents, centsToDollars, type CurrencyCode } from '../../money/money.js';
import './fb-icon.js';

const CURRENCIES: CurrencyCode[] = ['USD', 'CAD'];

/**
 * `fb-money-input` — captures dollars in the UI, emits integer **cents** + a
 * currency code (DSC §6.2). Parsing is pure string math (`dollarsToCents`), never
 * a `parseFloat` round-trip, so cent values never drift. The emitted detail field
 * is named `cents` (string) so the composite-currency ESLint rule is satisfied at
 * call sites that bind it to a `*Cents` property.
 *
 * Emits a composed `change` with `{ cents, currencyCode }`; `cents` is `null` for
 * empty input so callers can distinguish "blank" from "zero".
 */
@customElement('fb-money-input')
export class FbMoneyInput extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .row {
        display: flex;
        align-items: stretch;
      }
      .symbol {
        display: inline-flex;
        align-items: center;
        padding: 0 var(--fb-spacing-sm);
        color: var(--fb-text-secondary);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-right: none;
        border-radius: var(--fb-radius-sm) 0 0 var(--fb-radius-sm);
        font-family: var(--fb-font-mono);
      }
      input {
        flex: 1;
        min-width: 0;
        min-height: var(--fb-density-control);
        padding: 0 var(--fb-spacing-md);
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
        font-size: var(--fb-text-body-md);
        text-align: right;
        color: var(--fb-text-primary);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
      }
      input:focus-visible {
        border-color: var(--fb-gable-green);
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 1px;
        position: relative;
        z-index: 1;
      }
      input[aria-invalid='true'] {
        border-color: var(--fb-safety-red);
      }
      select {
        appearance: none;
        padding: 0 var(--fb-spacing-sm);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-left: none;
        border-radius: 0 var(--fb-radius-sm) var(--fb-radius-sm) 0;
        cursor: pointer;
      }
      select:focus-visible {
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 1px;
        position: relative;
        z-index: 1;
      }
      :host([disabled]) {
        opacity: 0.5;
        pointer-events: none;
      }
    `,
  ];

  /** Integer cents (string). Bind from a `*Cents` model field. */
  @property({ type: String }) cents: string | null = null;
  @property({ type: String, attribute: 'currency-code' }) currencyCode: CurrencyCode = 'USD';
  @property({ type: String }) name?: string;
  @property({ type: String }) placeholder = '0.00';
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: Boolean }) required = false;
  @property({ type: Boolean, reflect: true }) invalid = false;
  @property({ type: String }) label?: string;
  @property({ type: String }) describedby?: string;
  /** Hide the currency selector when the currency is fixed by context. */
  @property({ type: Boolean, attribute: 'lock-currency' }) lockCurrency = false;

  /** The editable dollar string mirrors `cents` but is only re-synced on blur. */
  @state() private draft = '';

  override willUpdate(changed: Map<string, unknown>): void {
    // Sync the visible draft from `cents` only when it changes externally and the
    // field isn't being actively edited (draft empty means "show the model").
    if (changed.has('cents') && this.draft === '') {
      this.draft = this.cents == null ? '' : centsToDollars(this.cents);
    }
  }

  private onInput(e: Event): void {
    this.draft = (e.target as HTMLInputElement).value;
  }

  private onBlur(): void {
    const parsed = dollarsToCents(this.draft);
    this.cents = parsed;
    // Re-render the canonical 2dp form so "12.5" shows back as "12.50".
    this.draft = parsed == null ? '' : centsToDollars(parsed);
    this.emit('change', { cents: this.cents, currencyCode: this.currencyCode });
  }

  private onCurrencyChange(e: Event): void {
    this.currencyCode = (e.target as HTMLSelectElement).value as CurrencyCode;
    this.emit('change', { cents: this.cents, currencyCode: this.currencyCode });
  }

  override render(): TemplateResult {
    return html`
      <div class="row">
        <span class="symbol" aria-hidden="true">$</span>
        <input
          inputmode="decimal"
          .value=${this.draft}
          name=${this.name ?? nothing}
          placeholder=${this.placeholder}
          ?disabled=${this.disabled}
          ?required=${this.required}
          aria-invalid=${this.invalid ? 'true' : nothing}
          aria-label=${this.label ?? nothing}
          aria-describedby=${this.describedby ?? nothing}
          @input=${this.onInput}
          @blur=${this.onBlur}
        />
        ${this.lockCurrency
          ? nothing
          : html`<select
              aria-label="Currency"
              ?disabled=${this.disabled}
              @change=${this.onCurrencyChange}
            >
              ${CURRENCIES.map(
                (c) => html`<option value=${c} ?selected=${c === this.currencyCode}>${c}</option>`,
              )}
            </select>`}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-money-input': FbMoneyInput;
  }
}
