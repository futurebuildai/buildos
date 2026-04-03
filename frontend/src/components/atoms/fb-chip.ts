import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

/**
 * fb-chip — Filter chip with active/inactive states.
 *
 * @property label - Chip text
 * @property active - Whether the chip is selected/active
 * @property removable - Show remove button
 * @fires fb-chip-toggle - Emitted on click with { active }
 * @fires fb-chip-remove - Emitted on remove button click
 */
@customElement('fb-chip')
export class FBChip extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: inline-flex; }

      .chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 4px 12px;
        border-radius: 9999px;
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        cursor: pointer;
        border: 1px solid var(--fb-border);
        background: transparent;
        color: var(--fb-text-secondary);
        transition: all var(--fb-transition-fast);
        white-space: nowrap;
        line-height: 1.5;
      }

      .chip:hover {
        border-color: var(--fb-border-hover);
        color: var(--fb-text-primary);
      }

      .chip.active {
        background: rgba(0, 255, 163, 0.12);
        border-color: rgba(0, 255, 163, 0.3);
        color: var(--fb-gable-green);
      }

      .remove-btn {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 16px;
        height: 16px;
        border: none;
        background: rgba(255, 255, 255, 0.1);
        border-radius: 50%;
        cursor: pointer;
        color: inherit;
        padding: 0;
        font-size: 12px;
        line-height: 1;
      }
      .remove-btn:hover {
        background: rgba(255, 255, 255, 0.2);
      }
    `,
  ];

  @property({ type: String }) label = '';
  @property({ type: Boolean }) active = false;
  @property({ type: Boolean }) removable = false;

  override render() {
    return html`
      <span
        class="chip ${this.active ? 'active' : ''}"
        @click=${this._onToggle}
        role="button"
        tabindex="0"
        aria-pressed=${this.active ? 'true' : 'false'}
      >
        ${this.label}
        ${this.removable ? html`
          <button class="remove-btn" @click=${this._onRemove} aria-label="Remove">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
              <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12 19 6.41z"/>
            </svg>
          </button>
        ` : ''}
      </span>
    `;
  }

  private _onToggle() {
    this.active = !this.active;
    this.emitEvent('fb-chip-toggle', { active: this.active });
  }

  private _onRemove(e: Event) {
    e.stopPropagation();
    this.emitEvent('fb-chip-remove');
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-chip': FBChip;
  }
}
