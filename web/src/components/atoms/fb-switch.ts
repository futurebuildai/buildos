import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';

/**
 * `fb-switch` — on/off toggle built on a native checkbox with `role="switch"`
 * semantics. Used for enable/disable affordances (e.g. integration cards). Emits
 * composed `change` with `{ checked }`.
 */
@customElement('fb-switch')
export class FbSwitch extends FBElement {
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
      .track {
        position: relative;
        width: 40px;
        height: 22px;
        flex: none;
        border-radius: var(--fb-radius-full);
        background: var(--fb-surface-3);
        border: 1px solid var(--md-sys-color-outline);
        transition: background var(--fb-motion-fast) var(--fb-ease-out);
      }
      .thumb {
        position: absolute;
        top: 2px;
        left: 2px;
        width: 16px;
        height: 16px;
        border-radius: 50%;
        background: var(--fb-text-secondary);
        transition:
          transform var(--fb-motion-fast) var(--fb-ease-out),
          background var(--fb-motion-fast) var(--fb-ease-out);
      }
      input:checked + .track {
        background: var(--fb-gable-green-dim);
        border-color: var(--fb-gable-green);
      }
      input:checked + .track .thumb {
        transform: translateX(18px);
        background: var(--fb-gable-green);
      }
      input:focus-visible + .track {
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 2px;
      }
      @media (prefers-reduced-motion: reduce) {
        .thumb,
        .track {
          transition: none;
        }
      }
    `,
  ];

  @property({ type: Boolean }) checked = false;
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: String }) name?: string;
  @property({ type: String }) label?: string;

  private onChange(e: Event): void {
    this.checked = (e.target as HTMLInputElement).checked;
    this.emit('change', { checked: this.checked });
  }

  override render(): TemplateResult {
    return html`
      <label class=${this.disabled ? 'disabled' : nothing}>
        <input
          type="checkbox"
          role="switch"
          .checked=${this.checked}
          name=${this.name ?? nothing}
          aria-label=${this.label ?? nothing}
          ?disabled=${this.disabled}
          @change=${this.onChange}
        />
        <span class="track"><span class="thumb"></span></span>
        <slot></slot>
      </label>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-switch': FbSwitch;
  }
}
