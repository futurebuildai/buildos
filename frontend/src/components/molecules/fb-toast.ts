import { html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export type ToastVariant = 'success' | 'error' | 'warning' | 'info';

/**
 * fb-toast — Notification toast with auto-dismiss.
 *
 * @property message - Toast message text
 * @property variant - Severity: success | error | warning | info
 * @property duration - Auto-dismiss delay in ms (0 = no auto-dismiss)
 * @fires fb-toast-dismiss - Emitted when toast is dismissed
 */
@customElement('fb-toast')
export class FBToast extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .toast {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
        padding: var(--fb-space-3) var(--fb-space-4);
        border-radius: var(--fb-radius-md);
        background: rgba(22, 24, 33, 0.95);
        backdrop-filter: blur(24px);
        -webkit-backdrop-filter: blur(24px);
        border: 1px solid var(--fb-border);
        box-shadow: var(--fb-shadow-lg);
        min-width: 280px;
        max-width: 480px;
        animation: slideIn 300ms cubic-bezier(0.2, 0, 0, 1);
      }

      .toast.leaving {
        animation: slideOut 200ms ease-out forwards;
      }

      .icon {
        flex-shrink: 0;
        display: flex;
        align-items: center;
      }
      .toast.success .icon { color: var(--fb-gable-green); }
      .toast.error .icon { color: var(--fb-safety-red); }
      .toast.warning .icon { color: var(--fb-amber); }
      .toast.info .icon { color: var(--fb-blueprint-blue); }

      .message {
        flex: 1;
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-primary);
        line-height: 1.4;
      }

      .close-btn {
        flex-shrink: 0;
        background: none;
        border: none;
        color: var(--fb-text-muted);
        cursor: pointer;
        padding: 4px;
        border-radius: 4px;
        display: flex;
      }
      .close-btn:hover { color: var(--fb-text-primary); }

      @keyframes slideIn {
        from { transform: translateX(100%); opacity: 0; }
        to { transform: translateX(0); opacity: 1; }
      }
      @keyframes slideOut {
        from { transform: translateX(0); opacity: 1; }
        to { transform: translateX(100%); opacity: 0; }
      }

      @media (prefers-reduced-motion: reduce) {
        .toast { animation: none; }
        .toast.leaving { animation: none; opacity: 0; }
      }
    `,
  ];

  @property({ type: String }) message = '';
  @property({ type: String }) variant: ToastVariant = 'info';
  @property({ type: Number }) duration = 5000;

  @state() private _leaving = false;

  private _timer: ReturnType<typeof setTimeout> | null = null;

  private _getIcon(): string {
    switch (this.variant) {
      case 'success': return 'M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z';
      case 'error': return 'M12 2C6.47 2 2 6.47 2 12s4.47 10 10 10 10-4.47 10-10S17.53 2 12 2zm5 13.59L15.59 17 12 13.41 8.41 17 7 15.59 10.59 12 7 8.41 8.41 7 12 10.59 15.59 7 17 8.41 13.41 12 17 15.59z';
      case 'warning': return 'M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z';
      case 'info': return 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z';
    }
  }

  override connectedCallback() {
    super.connectedCallback();
    if (this.duration > 0) {
      this._timer = setTimeout(() => this._dismiss(), this.duration);
    }
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    if (this._timer !== null) clearTimeout(this._timer);
  }

  override render() {
    return html`
      <div class="toast ${this.variant} ${this._leaving ? 'leaving' : ''}" role="alert">
        <span class="icon">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d=${this._getIcon()} />
          </svg>
        </span>
        <span class="message">${this.message}</span>
        <button class="close-btn" @click=${this._dismiss} aria-label="Dismiss">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
            <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
          </svg>
        </button>
      </div>
    `;
  }

  private _dismiss() {
    this._leaving = true;
    setTimeout(() => {
      this.emitEvent('fb-toast-dismiss');
    }, 200);
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-toast': FBToast;
  }
}
