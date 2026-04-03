import { html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

/**
 * fb-select — Dropdown select with GableLBM styling.
 *
 * @property label - Select label text
 * @property options - Array of { value, label, disabled? }
 * @property value - Currently selected value
 * @property placeholder - Placeholder text for unselected state
 * @property disabled - Disable the select
 * @fires fb-change - Emitted on selection change with { value }
 */
@customElement('fb-select')
export class FBSelect extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .field { display: flex; flex-direction: column; gap: var(--fb-space-1); }

      label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
      }

      .select-wrapper {
        position: relative;
        display: flex;
        align-items: center;
      }

      select {
        width: 100%;
        appearance: none;
        -webkit-appearance: none;
        background: var(--fb-slate-steel);
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-sm);
        padding: var(--fb-space-2) var(--fb-space-8) var(--fb-space-2) var(--fb-space-3);
        color: var(--fb-text-primary);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        min-height: 40px;
        cursor: pointer;
        transition: border-color var(--fb-transition-fast);
        outline: none;
      }

      select:focus {
        border-color: var(--fb-gable-green);
        box-shadow: 0 0 0 1px var(--fb-gable-green);
      }

      select:disabled {
        opacity: 0.4;
        cursor: not-allowed;
      }

      option {
        background: var(--fb-slate-steel);
        color: var(--fb-text-primary);
      }

      .chevron {
        position: absolute;
        right: 12px;
        pointer-events: none;
        color: var(--fb-text-muted);
      }
    `,
  ];

  @property({ type: String }) label = '';
  @property({ type: Array }) options: SelectOption[] = [];
  @property({ type: String }) value = '';
  @property({ type: String }) placeholder = 'Select...';
  @property({ type: Boolean }) disabled = false;

  override render() {
    return html`
      <div class="field">
        ${this.label ? html`<label>${this.label}</label>` : nothing}
        <div class="select-wrapper">
          <select
            .value=${this.value}
            ?disabled=${this.disabled}
            @change=${this._onChange}
          >
            ${this.placeholder ? html`<option value="" disabled ?selected=${!this.value}>${this.placeholder}</option>` : nothing}
            ${this.options.map(opt => html`
              <option
                .value=${opt.value}
                ?selected=${opt.value === this.value}
                ?disabled=${opt.disabled ?? false}
              >${opt.label}</option>
            `)}
          </select>
          <span class="chevron">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M7.41 8.59L12 13.17l4.59-4.58L18 10l-6 6-6-6 1.41-1.41z"/>
            </svg>
          </span>
        </div>
      </div>
    `;
  }

  private _onChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    this.value = select.value;
    this.emitEvent('fb-change', { value: this.value });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-select': FBSelect;
  }
}
