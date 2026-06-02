import { html, css, nothing, type TemplateResult, type PropertyValues } from 'lit';
import { customElement, property, query } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';

const FOCUSABLE =
  'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';

/**
 * `fb-modal` — accessible dialog (DSC §7.14). `role="dialog"` + `aria-modal`,
 * labelled by its title. Traps Tab focus inside the panel, closes on `Esc` and
 * backdrop click (when `dismissible`), and restores focus to the invoker on
 * close. Slots: default (body) and `footer` (actions).
 *
 * Emits `close` whenever the dialog requests dismissal; the parent owns `open`.
 */
@customElement('fb-modal')
export class FbModal extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: contents;
      }
      .backdrop {
        position: fixed;
        inset: 0;
        z-index: var(--fb-z-modal-backdrop);
        background: rgba(0, 0, 0, 0.6);
        display: flex;
        align-items: center;
        justify-content: center;
        padding: var(--fb-spacing-lg);
      }
      .panel {
        z-index: var(--fb-z-modal);
        width: 100%;
        max-width: 520px;
        max-height: 85vh;
        display: flex;
        flex-direction: column;
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
        box-shadow: var(--md-sys-elevation-3);
      }
      .head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        border-bottom: 1px solid var(--fb-border);
      }
      .title {
        font-size: var(--fb-text-title-md);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .close {
        display: inline-flex;
        padding: 4px;
        color: var(--fb-text-secondary);
        background: transparent;
        border: none;
        border-radius: var(--fb-radius-xs);
        cursor: pointer;
      }
      .close:hover {
        color: var(--fb-text-primary);
      }
      .body {
        padding: var(--fb-spacing-md);
        overflow: auto;
        color: var(--fb-text-primary);
      }
      .foot {
        display: flex;
        justify-content: flex-end;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        border-top: 1px solid var(--fb-border);
      }
    `,
  ];

  @property({ type: Boolean, reflect: true }) open = false;
  @property({ type: String }) heading = '';
  /** Allow Esc + backdrop click to request close. */
  @property({ type: Boolean }) dismissible = true;

  @query('.panel') private panel?: HTMLElement;

  private invoker: HTMLElement | null = null;
  private readonly titleId = `fb-modal-title-${Math.random().toString(36).slice(2, 9)}`;

  override connectedCallback(): void {
    super.connectedCallback();
    this.addEventListener('keydown', this.onKeydown);
  }

  override disconnectedCallback(): void {
    super.disconnectedCallback();
    this.removeEventListener('keydown', this.onKeydown);
  }

  override updated(changed: PropertyValues<this>): void {
    if (changed.has('open')) {
      if (this.open) {
        this.invoker = (this.getRootNode() as Document).activeElement as HTMLElement | null;
        void this.updateComplete.then(() => this.focusFirst());
      } else if (changed.get('open')) {
        // Restore focus to whatever opened the dialog.
        this.invoker?.focus?.();
        this.invoker = null;
      }
    }
  }

  /**
   * Collects every focusable inside the dialog: the shadow-DOM chrome (close
   * button) plus any slotted light-DOM controls (body fields, footer actions).
   * Slotted content lives in the host's light DOM, so a `panel.querySelector`
   * alone would miss it — we union both trees in visual order (chrome first).
   */
  protected focusables(): HTMLElement[] {
    const shadow = this.renderRoot.querySelectorAll<HTMLElement>(FOCUSABLE);
    const light = this.querySelectorAll<HTMLElement>(FOCUSABLE);
    return [...shadow, ...light];
  }

  /** Focus the first focusable in the dialog, falling back to the panel itself. */
  protected focusFirst(): void {
    const [first] = this.focusables();
    (first ?? this.panel)?.focus();
  }

  private requestClose(): void {
    if (this.dismissible) this.emit('close');
  }

  private readonly onKeydown = (e: KeyboardEvent): void => {
    if (!this.open) return;
    if (e.key === 'Escape') {
      e.stopPropagation();
      this.requestClose();
      return;
    }
    if (e.key !== 'Tab' || !this.panel) return;
    const items = this.focusables();
    if (items.length === 0) {
      e.preventDefault();
      this.panel.focus();
      return;
    }
    const firstItem = items[0]!;
    const lastItem = items[items.length - 1]!;
    const active = (this.getRootNode() as ShadowRoot | Document).activeElement;
    if (e.shiftKey && active === firstItem) {
      e.preventDefault();
      lastItem.focus();
    } else if (!e.shiftKey && active === lastItem) {
      e.preventDefault();
      firstItem.focus();
    }
  };

  private onBackdrop(e: MouseEvent): void {
    if (e.target === e.currentTarget) this.requestClose();
  }

  override render(): TemplateResult {
    if (!this.open) return html`${nothing}`;
    return html`
      <div class="backdrop" @click=${this.onBackdrop}>
        <div
          class="panel"
          role="dialog"
          aria-modal="true"
          aria-labelledby=${this.titleId}
          tabindex="-1"
        >
          <div class="head">
            <span class="title" id=${this.titleId}>${this.heading}</span>
            ${this.dismissible
              ? html`<button
                  class="close"
                  type="button"
                  aria-label="Close dialog"
                  @click=${this.requestClose}
                >
                  <fb-icon name="x" size="18"></fb-icon>
                </button>`
              : nothing}
          </div>
          <div class="body"><slot></slot></div>
          <div class="foot"><slot name="footer"></slot></div>
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-modal': FbModal;
  }
}
