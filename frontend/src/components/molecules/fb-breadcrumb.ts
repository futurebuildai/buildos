import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export interface BreadcrumbItem {
  label: string;
  href: string;
}

/**
 * fb-breadcrumb — Breadcrumb navigation for drill-down views.
 *
 * @property items - Array of { label, href } in order from root to current
 * @fires fb-breadcrumb-nav - Emitted on breadcrumb click with { href }
 */
@customElement('fb-breadcrumb')
export class FBBreadcrumb extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .breadcrumb {
        display: flex;
        align-items: center;
        gap: var(--fb-space-1);
        flex-wrap: wrap;
      }

      .crumb {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        cursor: pointer;
        text-decoration: none;
        transition: color var(--fb-transition-fast);
        background: none;
        border: none;
        padding: 2px 4px;
        border-radius: 4px;
      }
      .crumb:hover {
        color: var(--fb-text-primary);
        background: rgba(255, 255, 255, 0.03);
      }

      .crumb.current {
        color: var(--fb-text-primary);
        cursor: default;
        font-weight: 500;
      }
      .crumb.current:hover { background: none; }

      .separator {
        color: var(--fb-text-muted);
        font-size: var(--fb-text-xs);
        user-select: none;
        display: flex;
        align-items: center;
      }
    `,
  ];

  @property({ type: Array }) items: BreadcrumbItem[] = [];

  override render() {
    return html`
      <nav class="breadcrumb" aria-label="Breadcrumb">
        ${this.items.map((item, i) => {
          const isLast = i === this.items.length - 1;
          return html`
            ${i > 0 ? html`
              <span class="separator" aria-hidden="true">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6-6-6z"/>
                </svg>
              </span>
            ` : ''}
            <button
              class="crumb ${isLast ? 'current' : ''}"
              @click=${() => !isLast && this._onNavigate(item.href)}
              ?aria-current=${isLast ? 'page' : false}
            >${item.label}</button>
          `;
        })}
      </nav>
    `;
  }

  private _onNavigate(href: string) {
    this.emitEvent('fb-breadcrumb-nav', { href });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-breadcrumb': FBBreadcrumb;
  }
}
