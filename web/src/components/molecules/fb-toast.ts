import { html, css, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import type { IconName } from '../atoms/icons.js';

export type ToastTone = 'success' | 'error' | 'warning' | 'info';

const TONE_META: Record<ToastTone, { icon: IconName; color: string }> = {
  success: { icon: 'check-circle', color: 'var(--fb-gable-green)' },
  error: { icon: 'x-circle', color: 'var(--fb-safety-red)' },
  warning: { icon: 'alert-triangle', color: 'var(--fb-amber, #f59e0b)' },
  info: { icon: 'info', color: 'var(--fb-blueprint-blue, #38bdf8)' },
};

/**
 * `fb-toast` — a single transient notification (DSC §7, AG-05). Tone maps to a
 * color + icon (never color-only). Emits `dismiss` when the close button is hit;
 * the parent `fb-toaster` owns timing and stacking.
 */
@customElement('fb-toast')
export class FbToast extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .toast {
        display: flex;
        align-items: flex-start;
        gap: var(--fb-spacing-sm);
        min-width: 280px;
        max-width: 420px;
        padding: var(--fb-spacing-md);
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-left: 3px solid var(--tone-color);
        border-radius: var(--fb-radius-sm);
        box-shadow: var(--md-sys-elevation-2);
      }
      .icon {
        color: var(--tone-color);
        flex-shrink: 0;
      }
      .body {
        flex: 1;
        font-size: var(--fb-text-body-md);
      }
      .title {
        font-weight: 600;
      }
      .close {
        display: inline-flex;
        padding: 2px;
        color: var(--fb-text-secondary);
        background: transparent;
        border: none;
        border-radius: var(--fb-radius-xs);
        cursor: pointer;
      }
      .close:hover {
        color: var(--fb-text-primary);
      }
    `,
  ];

  @property({ type: String }) tone: ToastTone = 'info';
  @property({ type: String }) heading?: string;
  @property({ type: String }) message = '';

  private onClose(): void {
    this.emit('dismiss');
  }

  override render(): TemplateResult {
    const meta = TONE_META[this.tone];
    // Errors are assertive; the rest are polite (set on the toaster live region).
    return html`
      <div class="toast" style="--tone-color:${meta.color}">
        <fb-icon class="icon" name=${meta.icon} size="18"></fb-icon>
        <div class="body">
          ${this.heading ? html`<div class="title">${this.heading}</div>` : null}
          <div>${this.message}</div>
        </div>
        <button
          class="close"
          type="button"
          aria-label="Dismiss notification"
          @click=${this.onClose}
        >
          <fb-icon name="x" size="16"></fb-icon>
        </button>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-toast': FbToast;
  }
}
