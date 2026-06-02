import { html, css, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-modal.js';
import './../atoms/fb-button.js';

/**
 * `fb-confirm` — a convenience confirmation dialog over `fb-modal` (DSC §7.14).
 * Destructive variants use `btn-destructive` styling and place Cancel first so
 * focus lands there, never on the destructive button (the user must explicitly
 * choose to confirm). Emits `confirm` / `cancel`; the parent owns `open`.
 */
@customElement('fb-confirm')
export class FbConfirm extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: contents;
      }
      .message {
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-md);
      }
    `,
  ];

  @property({ type: Boolean, reflect: true }) open = false;
  @property({ type: String }) heading = 'Are you sure?';
  @property({ type: String }) message = '';
  @property({ type: String, attribute: 'confirm-label' }) confirmLabel = 'Confirm';
  @property({ type: String, attribute: 'cancel-label' }) cancelLabel = 'Cancel';
  @property({ type: Boolean }) destructive = false;

  private onConfirm(): void {
    this.emit('confirm');
  }

  private onCancel(): void {
    this.emit('cancel');
  }

  override render(): TemplateResult {
    return html`
      <fb-modal
        ?open=${this.open}
        heading=${this.heading}
        ?dismissible=${true}
        @close=${this.onCancel}
      >
        ${this.message ? html`<p class="message">${this.message}</p>` : null}
        <div slot="footer">
          <fb-button variant="secondary" @click=${this.onCancel}>${this.cancelLabel}</fb-button>
          <fb-button
            variant=${this.destructive ? 'destructive' : 'primary'}
            @click=${this.onConfirm}
            >${this.confirmLabel}</fb-button
          >
        </div>
      </fb-modal>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-confirm': FbConfirm;
  }
}
