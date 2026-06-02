import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';

/**
 * `fb-checkbox` — labelled checkbox atom. Wraps a visually-hidden native
 * `<input type=checkbox>` (keeps keyboard + form semantics) behind a styled box.
 * Emits composed `change` with `{ checked }`.
 */
@customElement('fb-checkbox')
export class FbCheckbox extends FBElement {
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
      .box {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 18px;
        height: 18px;
        flex: none;
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-xs);
        background: var(--fb-surface-1);
        color: var(--fb-deep-space);
        transition: background var(--fb-motion-fast) var(--fb-ease-out);
      }
      input:checked + .box {
        background: var(--fb-gable-green);
        border-color: var(--fb-gable-green);
      }
      input:focus-visible + .box {
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 2px;
      }
      fb-icon {
        opacity: 0;
      }
      input:checked + .box fb-icon {
        opacity: 1;
      }
    `,
  ];

  @property({ type: Boolean }) checked = false;
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: String }) name?: string;
  @property({ type: String }) value?: string;

  private onChange(e: Event): void {
    this.checked = (e.target as HTMLInputElement).checked;
    this.emit('change', { checked: this.checked });
  }

  override render(): TemplateResult {
    return html`
      <label class=${this.disabled ? 'disabled' : nothing}>
        <input
          type="checkbox"
          .checked=${this.checked}
          name=${this.name ?? nothing}
          value=${this.value ?? nothing}
          ?disabled=${this.disabled}
          @change=${this.onChange}
        />
        <span class="box"><fb-icon name="check" size="14"></fb-icon></span>
        <slot></slot>
      </label>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-checkbox': FbCheckbox;
  }
}
