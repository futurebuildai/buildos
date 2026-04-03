import { html, css, nothing } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export type FeedPriority = 'critical' | 'urgent' | 'normal' | 'low';

/**
 * fb-feed-card — Action card from AI agents with priority indicator.
 *
 * @property icon - Icon name for the card
 * @property cardTitle - Card title text
 * @property body - Card body text
 * @property priority - Priority level: critical | urgent | normal | low
 * @property timestamp - ISO timestamp string
 * @property dismissable - Whether the card can be dismissed
 * @fires fb-feed-action - Emitted when primary action is clicked
 * @fires fb-feed-dismiss - Emitted when dismiss is clicked
 */
@customElement('fb-feed-card')
export class FBFeedCard extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .card {
        padding: var(--fb-space-4);
        display: flex;
        gap: var(--fb-space-3);
        position: relative;
        transition: border-color var(--fb-transition-fast);
      }

      .card.critical { border-left: 3px solid var(--fb-safety-red); }
      .card.urgent { border-left: 3px solid var(--fb-amber); }
      .card.normal { border-left: 3px solid var(--fb-blueprint-blue); }
      .card.low { border-left: 3px solid var(--fb-text-muted); }

      .icon-col {
        display: flex;
        flex-shrink: 0;
        align-items: flex-start;
        padding-top: 2px;
        color: var(--fb-text-secondary);
      }

      .content {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
        min-width: 0;
      }

      .header {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--fb-space-2);
      }

      .title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        font-weight: 500;
        color: var(--fb-text-primary);
        line-height: 1.4;
      }

      .time {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        white-space: nowrap;
        flex-shrink: 0;
      }

      .body {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        line-height: 1.5;
      }

      .actions {
        display: flex;
        gap: var(--fb-space-2);
        margin-top: var(--fb-space-1);
      }

      .dismiss-btn {
        position: absolute;
        top: 8px;
        right: 8px;
        background: none;
        border: none;
        color: var(--fb-text-muted);
        cursor: pointer;
        padding: 4px;
        border-radius: 4px;
        display: flex;
        align-items: center;
        justify-content: center;
      }
      .dismiss-btn:hover { color: var(--fb-text-primary); background: rgba(255,255,255,0.05); }
    `,
  ];

  @property({ type: String }) icon = 'briefing';
  @property({ type: String }) cardTitle = '';
  @property({ type: String }) body = '';
  @property({ type: String }) priority: FeedPriority = 'normal';
  @property({ type: String }) timestamp = '';
  @property({ type: Boolean }) dismissable = true;

  private _formatTime(): string {
    if (!this.timestamp) return '';
    try {
      const date = new Date(this.timestamp);
      const now = new Date();
      const diffMs = now.getTime() - date.getTime();
      const diffMins = Math.floor(diffMs / 60000);

      if (diffMins < 1) return 'just now';
      if (diffMins < 60) return `${diffMins}m ago`;
      const diffHours = Math.floor(diffMins / 60);
      if (diffHours < 24) return `${diffHours}h ago`;
      const diffDays = Math.floor(diffHours / 24);
      return `${diffDays}d ago`;
    } catch {
      return '';
    }
  }

  override render() {
    return html`
      <div class="card glass-card ${this.priority}">
        <div class="icon-col">
          <fb-icon name=${this.icon} size="sm"></fb-icon>
        </div>
        <div class="content">
          <div class="header">
            <span class="title">${this.cardTitle}</span>
            <span class="time">${this._formatTime()}</span>
          </div>
          ${this.body ? html`<div class="body">${this.body}</div>` : nothing}
          <div class="actions">
            <slot name="actions"></slot>
          </div>
        </div>
        ${this.dismissable ? html`
          <button class="dismiss-btn" @click=${this._onDismiss} aria-label="Dismiss">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
            </svg>
          </button>
        ` : nothing}
      </div>
    `;
  }

  private _onDismiss() {
    this.emitEvent('fb-feed-dismiss');
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-feed-card': FBFeedCard;
  }
}
