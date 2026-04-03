import { html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

/**
 * fb-input — Text input with label, error state, prefix/suffix slots.
 *
 * @property label - Input label text
 * @property type - Input type (text, number, date, email)
 * @property value - Current value
 * @property placeholder - Placeholder text
 * @property error - Error message (shows error state when non-empty)
 * @property disabled - Disable the input
 * @fires fb-input - Emitted on input with { value }
 * @fires fb-change - Emitted on change with { value }
 */
@customElement('fb-input')
export class FBInput extends FBBaseElement {
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

      .input-wrapper {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
        background: var(--fb-slate-steel);
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-sm);
        padding: 0 var(--fb-space-3);
        transition: border-color var(--fb-transition-fast);
      }

      .input-wrapper:focus-within {
        border-color: var(--fb-gable-green);
        box-shadow: 0 0 0 1px var(--fb-gable-green);
      }

      .input-wrapper.has-error {
        border-color: var(--fb-safety-red);
      }
      .input-wrapper.has-error:focus-within {
        box-shadow: 0 0 0 1px var(--fb-safety-red);
      }

      .input-wrapper.is-disabled {
        opacity: 0.4;
        pointer-events: none;
      }

      input {
        flex: 1;
        background: transparent;
        border: none;
        outline: none;
        color: var(--fb-text-primary);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        padding: var(--fb-space-2) 0;
        min-height: 40px;
        width: 100%;
      }

      input::placeholder {
        color: var(--fb-text-muted);
      }

      input[type="number"] {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }

      .error-msg {
        font-size: var(--fb-text-xs);
        color: var(--fb-safety-red);
        display: flex;
        align-items: center;
        gap: 4px;
      }

      ::slotted([slot="prefix"]) {
        color: var(--fb-text-muted);
        flex-shrink: 0;
      }
      ::slotted([slot="suffix"]) {
        color: var(--fb-text-muted);
        flex-shrink: 0;
      }
    `,
  ];

  @property({ type: String }) label = '';
  @property({ type: String }) type: 'text' | 'number' | 'date' | 'email' = 'text';
  @property({ type: String }) value = '';
  @property({ type: String }) placeholder = '';
  @property({ type: String }) error = '';
  @property({ type: Boolean }) disabled = false;

  override render() {
    const wrapperClass = `input-wrapper${this.error ? ' has-error' : ''}${this.disabled ? ' is-disabled' : ''}`;

    return html`
      <div class="field">
        ${this.label ? html`<label>${this.label}</label>` : nothing}
        <div class=${wrapperClass}>
          <slot name="prefix"></slot>
          <input
            .type=${this.type}
            .value=${this.value}
            .placeholder=${this.placeholder}
            ?disabled=${this.disabled}
            @input=${this._onInput}
            @change=${this._onChange}
          />
          <slot name="suffix"></slot>
        </div>
        ${this.error ? html`<span class="error-msg">${this.error}</span>` : nothing}
      </div>
    `;
  }

  private _onInput(e: Event) {
    const input = e.target as HTMLInputElement;
    this.value = input.value;
    this.emitEvent('fb-input', { value: this.value });
  }

  private _onChange(e: Event) {
    const input = e.target as HTMLInputElement;
    this.value = input.value;
    this.emitEvent('fb-change', { value: this.value });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-input': FBInput;
  }
}
