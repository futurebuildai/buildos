import { html, css } from 'lit';
import { customElement, property, state, query } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

/**
 * fb-search-bar — Search input with icon, clear button, debounced input event.
 *
 * @property placeholder - Placeholder text
 * @property value - Current search value
 * @property debounce - Debounce delay in ms (default: 300)
 * @fires fb-search - Emitted after debounce with { value }
 */
@customElement('fb-search-bar')
export class FBSearchBar extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .search {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
        background: var(--fb-slate-steel);
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-sm);
        padding: 0 var(--fb-space-3);
        transition: border-color var(--fb-transition-fast);
      }

      .search:focus-within {
        border-color: var(--fb-gable-green);
      }

      .icon {
        color: var(--fb-text-muted);
        flex-shrink: 0;
        display: flex;
        align-items: center;
      }

      input {
        flex: 1;
        background: transparent;
        border: none;
        outline: none;
        color: var(--fb-text-primary);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        padding: var(--fb-space-2) 0;
        min-height: 36px;
      }
      input::placeholder { color: var(--fb-text-muted); }

      .clear-btn {
        display: flex;
        align-items: center;
        justify-content: center;
        background: none;
        border: none;
        color: var(--fb-text-muted);
        cursor: pointer;
        padding: 4px;
        border-radius: 4px;
        flex-shrink: 0;
      }
      .clear-btn:hover { color: var(--fb-text-primary); background: rgba(255,255,255,0.05); }
    `,
  ];

  @property({ type: String }) placeholder = 'Search...';
  @property({ type: String }) value = '';
  @property({ type: Number }) debounce = 300;

  @state() private _hasValue = false;
  @query('input') private _input!: HTMLInputElement;

  private _timer: ReturnType<typeof setTimeout> | null = null;

  override render() {
    return html`
      <div class="search">
        <span class="icon">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
            <path d="M15.5 14h-.79l-.28-.27A6.471 6.471 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/>
          </svg>
        </span>
        <input
          .value=${this.value}
          .placeholder=${this.placeholder}
          @input=${this._onInput}
          aria-label=${this.placeholder}
        />
        ${this._hasValue ? html`
          <button class="clear-btn" @click=${this._onClear} aria-label="Clear search">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
            </svg>
          </button>
        ` : ''}
      </div>
    `;
  }

  private _onInput(e: Event) {
    const input = e.target as HTMLInputElement;
    this.value = input.value;
    this._hasValue = this.value.length > 0;

    if (this._timer !== null) clearTimeout(this._timer);
    this._timer = setTimeout(() => {
      this.emitEvent('fb-search', { value: this.value });
    }, this.debounce);
  }

  private _onClear() {
    this.value = '';
    this._hasValue = false;
    if (this._input) this._input.value = '';
    if (this._timer !== null) clearTimeout(this._timer);
    this.emitEvent('fb-search', { value: '' });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-search-bar': FBSearchBar;
  }
}
