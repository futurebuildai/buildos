import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';
import type { IconName } from './icons.js';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'destructive';
export type ButtonSize = 'sm' | 'md';

/**
 * `fb-button` — the primary action atom (DSC §3, DESIGN_SYSTEM §9).
 *
 * Wraps a native `<button>` so it is keyboard- and form-correct. Variants map to
 * the FBElement button bases (primary glow, destructive red). Honors density via
 * `--fb-density-control`. While `loading`, the button is disabled and shows a
 * spinner; the label stays for layout stability and is announced via `aria-busy`.
 */
@customElement('fb-button')
export class FbButton extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-block;
      }
      button {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: var(--fb-spacing-sm);
        min-height: var(--fb-density-control);
        padding: 0 var(--fb-spacing-md);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        font-weight: 600;
        line-height: 1;
        border-radius: var(--fb-radius-sm);
        border: 1px solid transparent;
        cursor: pointer;
        white-space: nowrap;
        color: var(--fb-text-primary);
        background: transparent;
      }
      button:disabled {
        cursor: not-allowed;
        opacity: 0.5;
      }
      :host([size='sm']) button {
        min-height: 28px;
        padding: 0 var(--fb-spacing-sm);
        font-size: var(--fb-text-body-sm);
      }
      :host([full]) {
        display: block;
      }
      :host([full]) button {
        width: 100%;
      }

      /* Variants */
      .primary {
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
      }
      .secondary {
        background: var(--fb-surface-2);
        border-color: var(--md-sys-color-outline);
        color: var(--fb-text-primary);
      }
      .secondary:hover:not(:disabled) {
        background: var(--fb-surface-3);
      }
      .ghost {
        background: transparent;
        color: var(--fb-text-secondary);
      }
      .ghost:hover:not(:disabled) {
        background: var(--fb-surface-2);
        color: var(--fb-text-primary);
      }
      .destructive {
        background: transparent;
        color: var(--fb-safety-red);
      }

      .spin {
        animation: fb-btn-spin 0.8s linear infinite;
      }
      @keyframes fb-btn-spin {
        to {
          transform: rotate(360deg);
        }
      }
      @media (prefers-reduced-motion: reduce) {
        .spin {
          animation-duration: 1.5s;
        }
      }
    `,
  ];

  @property({ type: String, reflect: true }) variant: ButtonVariant = 'primary';
  @property({ type: String, reflect: true }) size: ButtonSize = 'md';
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: Boolean, reflect: true }) loading = false;
  @property({ type: Boolean, reflect: true }) full = false;
  /** Native button type so it behaves correctly inside `fb-form`. */
  @property({ type: String }) type: 'button' | 'submit' | 'reset' = 'button';
  /** Optional leading icon. */
  @property({ type: String }) icon?: IconName;
  /** Accessible label override for icon-only buttons. */
  @property({ type: String }) label?: string;

  private readonly variantClass: Record<ButtonVariant, string> = {
    primary: 'primary btn-primary',
    secondary: 'secondary',
    ghost: 'ghost',
    destructive: 'destructive btn-destructive',
  };

  override render(): TemplateResult {
    const isDisabled = this.disabled || this.loading;
    return html`
      <button
        class=${this.variantClass[this.variant]}
        type=${this.type}
        ?disabled=${isDisabled}
        aria-busy=${this.loading ? 'true' : nothing}
        aria-label=${this.label ?? nothing}
      >
        ${this.loading
          ? html`<fb-icon
              class="spin"
              name="spinner"
              size=${this.size === 'sm' ? 14 : 16}
            ></fb-icon>`
          : this.icon
            ? html`<fb-icon name=${this.icon} size=${this.size === 'sm' ? 14 : 16}></fb-icon>`
            : nothing}
        <slot></slot>
      </button>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-button': FbButton;
  }
}
