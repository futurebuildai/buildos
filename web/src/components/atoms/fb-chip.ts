import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';

/**
 * `fb-chip` — compact filter/selection chip (DSC §3, AG-05). Two modes:
 *  - filter (`selectable`): toggles `selected`, emits `fb-chip-toggle` `{ selected }`,
 *    rendered as a `role="button"` with `aria-pressed`.
 *  - removable (`removable`): shows an × that emits `fb-chip-remove`.
 * Plain (neither) is a static label tag.
 */
@customElement('fb-chip')
export class FbChip extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-flex;
      }
      .chip {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        padding: 3px var(--fb-spacing-sm);
        border-radius: var(--fb-radius-full);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
        background: var(--fb-surface-2);
        border: 1px solid var(--fb-border);
        white-space: nowrap;
      }
      .chip.selectable {
        cursor: pointer;
      }
      .chip.selectable:hover {
        color: var(--fb-text-primary);
        border-color: var(--md-sys-color-outline);
      }
      .chip[aria-pressed='true'] {
        color: var(--fb-gable-green);
        background: color-mix(in srgb, var(--fb-gable-green) 12%, transparent);
        border-color: color-mix(in srgb, var(--fb-gable-green) 40%, transparent);
      }
      .chip:focus-visible {
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 2px;
      }
      .remove {
        display: inline-flex;
        border: 0;
        background: transparent;
        color: inherit;
        cursor: pointer;
        padding: 0;
        margin-left: 2px;
      }
    `,
  ];

  @property({ type: Boolean, reflect: true }) selectable = false;
  @property({ type: Boolean, reflect: true }) selected = false;
  @property({ type: Boolean, reflect: true }) removable = false;
  @property({ type: String }) value?: string;
  /** Accessible label for the remove button. */
  @property({ type: String }) removeLabel = 'Remove';

  private toggle(): void {
    if (!this.selectable) return;
    this.selected = !this.selected;
    this.emit('fb-chip-toggle', { selected: this.selected, value: this.value });
  }

  private onKey(e: KeyboardEvent): void {
    if (!this.selectable) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      this.toggle();
    }
  }

  private onRemove(e: Event): void {
    e.stopPropagation();
    this.emit('fb-chip-remove', { value: this.value });
  }

  override render(): TemplateResult {
    return html`
      <span
        class="chip ${this.selectable ? 'selectable' : ''}"
        role=${this.selectable ? 'button' : nothing}
        tabindex=${this.selectable ? '0' : nothing}
        aria-pressed=${this.selectable ? (this.selected ? 'true' : 'false') : nothing}
        @click=${this.toggle}
        @keydown=${this.onKey}
      >
        <slot></slot>
        ${this.removable
          ? html`<button
              class="remove"
              type="button"
              aria-label=${this.removeLabel}
              @click=${this.onRemove}
            >
              <fb-icon name="x" size="12"></fb-icon>
            </button>`
          : nothing}
      </span>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-chip': FbChip;
  }
}
