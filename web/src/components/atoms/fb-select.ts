import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

/**
 * `fb-select` — native `<select>` wrapper (DSC §3, AG-05). Native is intentional:
 * it gives correct keyboard handling, type-ahead, and mobile pickers for free.
 * Emits a composed `change` event with `{ value }`. Pair with `fb-field` for the
 * visible label + error wiring.
 */
@customElement('fb-select')
export class FbSelect extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .wrap {
        position: relative;
        display: block;
      }
      select {
        appearance: none;
        width: 100%;
        min-height: var(--fb-density-control);
        padding: 0 calc(var(--fb-spacing-md) + 18px) 0 var(--fb-spacing-md);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        color: var(--fb-text-primary);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
      }
      select:focus-visible {
        border-color: var(--fb-gable-green);
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 1px;
      }
      select:disabled {
        opacity: 0.5;
        cursor: not-allowed;
      }
      select[aria-invalid='true'] {
        border-color: var(--fb-safety-red);
      }
      fb-icon {
        position: absolute;
        right: var(--fb-spacing-sm);
        top: 50%;
        transform: translateY(-50%);
        color: var(--fb-text-secondary);
        pointer-events: none;
      }
    `,
  ];

  @property({ type: Array }) options: SelectOption[] = [];
  @property({ type: String }) value = '';
  @property({ type: String }) name?: string;
  @property({ type: String }) placeholder?: string;
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: Boolean }) required = false;
  @property({ type: Boolean, reflect: true }) invalid = false;
  @property({ type: String }) label?: string;
  @property({ type: String }) describedby?: string;

  private onChange(e: Event): void {
    this.value = (e.target as HTMLSelectElement).value;
    this.emit('change', { value: this.value });
  }

  override render(): TemplateResult {
    return html`
      <div class="wrap">
        <select
          .value=${this.value}
          name=${this.name ?? nothing}
          ?disabled=${this.disabled}
          ?required=${this.required}
          aria-invalid=${this.invalid ? 'true' : nothing}
          aria-label=${this.label ?? nothing}
          aria-describedby=${this.describedby ?? nothing}
          @change=${this.onChange}
        >
          ${this.placeholder
            ? html`<option value="" disabled ?selected=${!this.value}>${this.placeholder}</option>`
            : nothing}
          ${this.options.map(
            (o) =>
              html`<option
                value=${o.value}
                ?disabled=${o.disabled}
                ?selected=${o.value === this.value}
              >
                ${o.label}
              </option>`,
          )}
        </select>
        <fb-icon name="chevron-down" size="16"></fb-icon>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-select': FbSelect;
  }
}
