import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';

/**
 * `fb-radio` — single radio option. Group multiple by sharing `name`; the native
 * input handles roving selection + arrow-key nav within a group. Emits composed
 * `change` with `{ value }` when selected.
 */
@customElement('fb-radio')
export class FbRadio extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-block;
      }
      label {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        cursor: pointer;
        font-size: var(--fb-text-body-md);
        color: var(--fb-text-primary);
      }
      label.disabled {
        cursor: not-allowed;
        opacity: 0.5;
      }
      input {
        position: absolute;
        opacity: 0;
        width: 1px;
        height: 1px;
      }
      .ring {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 18px;
        height: 18px;
        flex: none;
        border: 1px solid var(--md-sys-color-outline);
        border-radius: 50%;
        background: var(--fb-surface-1);
      }
      .ring::after {
        content: '';
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: var(--fb-gable-green);
        transform: scale(0);
        transition: transform var(--fb-motion-fast) var(--fb-ease-out);
      }
      input:checked + .ring {
        border-color: var(--fb-gable-green);
      }
      input:checked + .ring::after {
        transform: scale(1);
      }
      input:focus-visible + .ring {
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 2px;
      }
      @media (prefers-reduced-motion: reduce) {
        .ring::after {
          transition: none;
        }
      }
    `,
  ];

  @property({ type: Boolean }) checked = false;
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: String }) name?: string;
  @property({ type: String }) value = '';

  private onChange(): void {
    this.checked = true;
    this.emit('change', { value: this.value });
  }

  override render(): TemplateResult {
    return html`
      <label class=${this.disabled ? 'disabled' : nothing}>
        <input
          type="radio"
          .checked=${this.checked}
          name=${this.name ?? nothing}
          value=${this.value}
          ?disabled=${this.disabled}
          @change=${this.onChange}
        />
        <span class="ring"></span>
        <slot></slot>
      </label>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-radio': FbRadio;
  }
}
