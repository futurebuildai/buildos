import { html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { classMap } from 'lit/directives/class-map.js';
import { FBBaseElement } from '../base/fb-element.js';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md' | 'lg';

/**
 * fb-button — Standard action button with GableLBM styling.
 *
 * @property variant - Visual style: primary | secondary | ghost | danger
 * @property size - Button size: sm | md | lg
 * @property disabled - Disable the button
 * @property loading - Show loading spinner
 * @fires fb-click - Emitted on click when not disabled/loading
 */
@customElement('fb-button')
export class FBButton extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: inline-block; }

      button {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: var(--fb-space-2);
        border: none;
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
        font-family: var(--fb-font-body);
        font-weight: 500;
        line-height: 1;
        white-space: nowrap;
        transition: transform var(--fb-transition-fast),
                    box-shadow var(--fb-transition-fast),
                    background var(--fb-transition-fast),
                    border-color var(--fb-transition-fast);
      }

      /* ── Sizes ── */
      .sm { padding: 6px 12px; font-size: var(--fb-text-sm); min-height: 32px; }
      .md { padding: 8px 16px; font-size: var(--fb-text-base); min-height: 40px; }
      .lg { padding: 12px 24px; font-size: var(--fb-text-lg); min-height: 48px; }

      /* ── Primary ── */
      .primary {
        background: linear-gradient(135deg, #00FFA3 0%, #00CC82 100%);
        color: #003822;
      }
      .primary:hover {
        box-shadow: var(--fb-shadow-glow);
        transform: translateY(-1px);
      }
      .primary:active {
        transform: scale(0.95);
        box-shadow: none;
      }

      /* ── Secondary ── */
      .secondary {
        background: transparent;
        color: var(--fb-text-primary);
        border: 1px solid var(--fb-border);
      }
      .secondary:hover {
        background: var(--fb-surface-hover);
        border-color: var(--fb-border-hover);
      }
      .secondary:active { transform: scale(0.95); }

      /* ── Ghost ── */
      .ghost {
        background: transparent;
        color: var(--fb-text-secondary);
      }
      .ghost:hover {
        background: var(--fb-surface-hover);
        color: var(--fb-text-primary);
      }
      .ghost:active { transform: scale(0.95); }

      /* ── Danger ── */
      .danger {
        background: transparent;
        color: var(--fb-safety-red);
        border: 1px solid rgba(244, 63, 94, 0.3);
      }
      .danger:hover {
        background: rgba(244, 63, 94, 0.1);
        box-shadow: 0 0 20px rgba(244, 63, 94, 0.15);
      }
      .danger:active {
        transform: scale(0.95);
        box-shadow: none;
      }

      /* ── Disabled ── */
      .is-disabled {
        opacity: 0.4;
        pointer-events: none;
        cursor: not-allowed;
      }

      /* ── Loading spinner ── */
      .spinner {
        width: 16px;
        height: 16px;
        border: 2px solid transparent;
        border-top-color: currentColor;
        border-radius: 50%;
        animation: spin 600ms linear infinite;
      }
      @keyframes spin {
        to { transform: rotate(360deg); }
      }

      @media (prefers-reduced-motion: reduce) {
        button:hover { transform: none; }
        button:active { transform: none; }
      }
    `,
  ];

  @property({ type: String }) variant: ButtonVariant = 'primary';
  @property({ type: String }) size: ButtonSize = 'md';
  @property({ type: Boolean }) disabled = false;
  @property({ type: Boolean }) loading = false;

  override render() {
    const classes = {
      [this.variant]: true,
      [this.size]: true,
      'is-disabled': this.disabled || this.loading,
    };

    return html`
      <button
        class=${classMap(classes)}
        ?disabled=${this.disabled || this.loading}
        @click=${this._handleClick}
        part="button"
      >
        ${this.loading ? html`<span class="spinner"></span>` : nothing}
        <slot></slot>
      </button>
    `;
  }

  private _handleClick(e: Event) {
    if (this.disabled || this.loading) {
      e.stopPropagation();
      return;
    }
    this.emitEvent('fb-click');
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-button': FBButton;
  }
}
