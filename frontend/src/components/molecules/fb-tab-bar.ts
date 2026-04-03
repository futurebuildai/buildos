import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export interface TabItem {
  id: string;
  label: string;
}

/**
 * fb-tab-bar — Tab navigation with active indicator.
 *
 * Commonly used for currency toggles (USD/CAD/All) and view switching.
 *
 * @property tabs - Array of { id, label }
 * @property active - Currently active tab id
 * @fires fb-tab-change - Emitted on tab click with { id }
 */
@customElement('fb-tab-bar')
export class FBTabBar extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .tab-bar {
        display: flex;
        gap: 0;
        border-bottom: 1px solid var(--fb-border);
        overflow-x: auto;
      }

      .tab {
        display: flex;
        align-items: center;
        justify-content: center;
        padding: var(--fb-space-2) var(--fb-space-4);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
        background: none;
        border: none;
        cursor: pointer;
        position: relative;
        white-space: nowrap;
        transition: color var(--fb-transition-fast);
        min-height: 40px;
      }

      .tab:hover { color: var(--fb-text-primary); }

      .tab.active {
        color: var(--fb-gable-green);
      }

      .tab.active::after {
        content: '';
        position: absolute;
        bottom: -1px;
        left: 0;
        right: 0;
        height: 2px;
        background: var(--fb-gable-green);
        border-radius: 2px 2px 0 0;
      }
    `,
  ];

  @property({ type: Array }) tabs: TabItem[] = [];
  @property({ type: String }) active = '';

  override render() {
    return html`
      <div class="tab-bar" role="tablist">
        ${this.tabs.map(tab => html`
          <button
            class="tab ${tab.id === this.active ? 'active' : ''}"
            role="tab"
            aria-selected=${tab.id === this.active ? 'true' : 'false'}
            @click=${() => this._onTabClick(tab.id)}
          >
            ${tab.label}
          </button>
        `)}
      </div>
    `;
  }

  private _onTabClick(id: string) {
    this.active = id;
    this.emitEvent('fb-tab-change', { id });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-tab-bar': FBTabBar;
  }
}
