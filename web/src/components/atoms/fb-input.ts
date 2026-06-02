import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';

export type InputType = 'text' | 'email' | 'number' | 'date' | 'tel' | 'url' | 'search';

/**
 * `fb-input` — single-line text/number/date input atom (DSC §3, AG-05).
 *
 * Renders a bare control; visible labels, hints, and error text are the job of
 * `fb-field` (DSC §7.3), which wires `aria-describedby`/`aria-invalid` here via
 * the `describedby`/`invalid` props. Emits composed `input` and `change` events
 * carrying `{ value }`. Numeric content uses mono tabular figures.
 */
@customElement('fb-input')
export class FbInput extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      input {
        width: 100%;
        min-height: var(--fb-density-control);
        padding: 0 var(--fb-spacing-md);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        color: var(--fb-text-primary);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
      }
      input::placeholder {
        color: var(--fb-text-muted);
      }
      input:hover:not(:disabled) {
        border-color: var(--fb-text-secondary);
      }
      input:focus-visible {
        border-color: var(--fb-gable-green);
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 1px;
      }
      input:disabled {
        opacity: 0.5;
        cursor: not-allowed;
      }
      input[aria-invalid='true'] {
        border-color: var(--fb-safety-red);
      }
      input[type='number'],
      :host([mono]) input {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }
    `,
  ];

  @property({ type: String }) type: InputType = 'text';
  @property({ type: String }) value = '';
  @property({ type: String }) name?: string;
  @property({ type: String }) placeholder?: string;
  @property({ type: String }) autocomplete?: string;
  @property({ type: String }) inputmode?: string;
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: Boolean }) required = false;
  @property({ type: Boolean, reflect: true }) invalid = false;
  @property({ type: Boolean, reflect: true }) mono = false;
  /** Accessible name when no visible `<label>` is wired (else prefer fb-field). */
  @property({ type: String }) label?: string;
  /** id(s) of describing hint/error nodes (set by fb-field). */
  @property({ type: String }) describedby?: string;
  @property({ type: Number }) min?: number;
  @property({ type: Number }) max?: number;
  @property({ type: Number }) step?: number;
  @property({ type: Number }) maxlength?: number;

  private onInput(e: Event): void {
    // The inner native input fires a composed `input` that would otherwise
    // escape this shadow root alongside our curated `{ value }` event, hitting
    // host `@input` listeners twice (the native one carries detail=0, clobbering
    // any value a consumer reads off detail). Stop it at the boundary; re-emit ours.
    e.stopPropagation();
    this.value = (e.target as HTMLInputElement).value;
    this.emit('input', { value: this.value });
  }

  private onChange(e: Event): void {
    e.stopPropagation();
    this.emit('change', { value: this.value });
  }

  override render(): TemplateResult {
    return html`
      <input
        .value=${this.value}
        type=${this.type}
        name=${this.name ?? nothing}
        placeholder=${this.placeholder ?? nothing}
        autocomplete=${this.autocomplete ?? nothing}
        inputmode=${this.inputmode ?? nothing}
        ?disabled=${this.disabled}
        ?required=${this.required}
        aria-invalid=${this.invalid ? 'true' : nothing}
        aria-label=${this.label ?? nothing}
        aria-describedby=${this.describedby ?? nothing}
        min=${this.min ?? nothing}
        max=${this.max ?? nothing}
        step=${this.step ?? nothing}
        maxlength=${this.maxlength ?? nothing}
        @input=${this.onInput}
        @change=${this.onChange}
      />
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-input': FbInput;
  }
}
