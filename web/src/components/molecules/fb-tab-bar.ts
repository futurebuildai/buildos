import { html, css, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';

export interface Tab {
  id: string;
  label: string;
  disabled?: boolean;
}

/**
 * `fb-tab-bar` — horizontal tabs with full ARIA tablist semantics (DSC §7,
 * AG-05). Roving tabindex + arrow-key navigation; the active tab carries the
 * Gable-green underline. Emits composed `change` with `{ id }`. Panels are owned
 * by the caller (link `aria-controls`/`role="tabpanel"` at the call site).
 */
@customElement('fb-tab-bar')
export class FbTabBar extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      [role='tablist'] {
        display: flex;
        gap: var(--fb-spacing-xs);
        border-bottom: 1px solid var(--md-sys-color-outline);
      }
      button {
        position: relative;
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        font-weight: 600;
        color: var(--fb-text-secondary);
        background: transparent;
        border: none;
        border-bottom: 2px solid transparent;
        margin-bottom: -1px;
        cursor: pointer;
      }
      button[aria-selected='true'] {
        color: var(--fb-text-primary);
        border-bottom-color: var(--fb-gable-green);
      }
      button:hover:not(:disabled):not([aria-selected='true']) {
        color: var(--fb-text-primary);
      }
      button:disabled {
        opacity: 0.4;
        cursor: not-allowed;
      }
    `,
  ];

  @property({ type: Array }) tabs: Tab[] = [];
  @property({ type: String }) active = '';
  @property({ type: String }) label = 'Sections';

  private select(id: string): void {
    if (id === this.active) return;
    this.active = id;
    this.emit('change', { id });
  }

  private onKeydown(e: KeyboardEvent): void {
    const enabled = this.tabs.filter((t) => !t.disabled);
    const idx = enabled.findIndex((t) => t.id === this.active);
    let next = -1;
    if (e.key === 'ArrowRight') next = (idx + 1) % enabled.length;
    else if (e.key === 'ArrowLeft') next = (idx - 1 + enabled.length) % enabled.length;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = enabled.length - 1;
    if (next < 0) return;
    e.preventDefault();
    const target = enabled[next];
    if (!target) return;
    this.select(target.id);
    const btn = this.renderRoot.querySelector<HTMLButtonElement>(`#tab-${target.id}`);
    btn?.focus();
  }

  override render(): TemplateResult {
    return html`
      <div role="tablist" aria-label=${this.label} @keydown=${this.onKeydown}>
        ${this.tabs.map((t) => {
          const selected = t.id === this.active;
          return html`<button
            id="tab-${t.id}"
            role="tab"
            type="button"
            aria-selected=${selected ? 'true' : 'false'}
            tabindex=${selected ? '0' : '-1'}
            ?disabled=${t.disabled}
            @click=${() => this.select(t.id)}
          >
            ${t.label}
          </button>`;
        })}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-tab-bar': FbTabBar;
  }
}
