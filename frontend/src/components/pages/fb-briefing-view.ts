import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  listFeed, actionFeed, dismissFeed,
  type FeedCard, type FeedPriority, ApiError,
} from '../../state/api.js';

type PriorityFilter = FeedPriority | 'all';

@customElement('fb-briefing-view')
export class FBBriefingView extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _cards: FeedCard[] = [];
  @state() private _filter: PriorityFilter = 'all';
  @state() private _actioningCard: string | null = null;

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host { display: block; padding: var(--fb-space-6); }

      .page-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--fb-space-6);
        flex-wrap: wrap;
        gap: var(--fb-space-4);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .filter-chips {
        display: flex;
        gap: var(--fb-space-2);
      }

      .chip {
        padding: var(--fb-space-2) var(--fb-space-3);
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: 1px solid var(--fb-border);
        background: transparent;
        font-family: var(--fb-font-body);
      }

      .chip:hover { color: var(--fb-text-primary); border-color: var(--fb-border-hover); }

      .chip.active {
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        border-color: var(--fb-gable-green);
        font-weight: 600;
      }

      .chip.critical.active { background: var(--fb-safety-red); border-color: var(--fb-safety-red); color: #fff; }
      .chip.urgent.active { background: var(--fb-amber); border-color: var(--fb-amber); color: var(--fb-deep-space); }

      .card-count {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-muted);
        margin-bottom: var(--fb-space-4);
      }

      .card-count span {
        font-family: var(--fb-font-mono);
        color: var(--fb-gable-green);
      }

      .feed-list {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-4);
      }

      .feed-card {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        transition: border-color var(--fb-transition-fast);
      }

      .feed-card:hover { border-color: var(--fb-border-hover); }

      .feed-card.critical { border-left: 3px solid var(--fb-safety-red); }
      .feed-card.urgent { border-left: 3px solid var(--fb-amber); }
      .feed-card.normal { border-left: 3px solid var(--fb-blueprint-blue); }
      .feed-card.low { border-left: 3px solid var(--fb-text-muted); }

      .card-header {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        margin-bottom: var(--fb-space-3);
      }

      .card-title {
        font-size: var(--fb-text-base);
        font-weight: 600;
        color: var(--fb-text-primary);
      }

      .card-meta {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
      }

      .card-type {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        background: var(--fb-surface);
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        text-transform: uppercase;
        letter-spacing: 0.05em;
      }

      .card-priority {
        font-size: var(--fb-text-xs);
        font-weight: 600;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
      }

      .card-priority.critical { background: rgba(244, 63, 94, 0.1); color: var(--fb-safety-red); }
      .card-priority.urgent { background: rgba(245, 158, 11, 0.1); color: var(--fb-amber); }
      .card-priority.normal { background: rgba(56, 189, 248, 0.1); color: var(--fb-blueprint-blue); }
      .card-priority.low { background: rgba(100, 116, 139, 0.1); color: var(--fb-text-muted); }

      .card-body {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        line-height: 1.5;
        margin-bottom: var(--fb-space-4);
      }

      .card-footer {
        display: flex;
        justify-content: space-between;
        align-items: center;
      }

      .card-actions {
        display: flex;
        gap: var(--fb-space-2);
      }

      .action-btn {
        padding: var(--fb-space-2) var(--fb-space-3);
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: 1px solid var(--fb-gable-green);
        background: rgba(0, 255, 163, 0.1);
        color: var(--fb-gable-green);
        font-family: var(--fb-font-body);
      }

      .action-btn:hover { background: var(--fb-gable-green); color: var(--fb-deep-space); }
      .action-btn:disabled { opacity: 0.5; cursor: not-allowed; }

      .dismiss-btn {
        padding: var(--fb-space-2) var(--fb-space-3);
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 500;
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: 1px solid var(--fb-border);
        background: transparent;
        color: var(--fb-text-muted);
        font-family: var(--fb-font-body);
      }

      .dismiss-btn:hover { color: var(--fb-text-secondary); border-color: var(--fb-border-hover); }

      .card-time {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
      }

      .error-banner {
        color: var(--fb-safety-red);
        padding: var(--fb-space-4);
        background: rgba(244, 63, 94, 0.1);
        border-radius: var(--fb-radius-md);
        border: 1px solid rgba(244, 63, 94, 0.2);
        margin-bottom: var(--fb-space-4);
        font-size: var(--fb-text-sm);
      }

      .loading-container {
        display: flex;
        align-items: center;
        justify-content: center;
        min-height: 300px;
      }

      .empty-state {
        text-align: center;
        padding: var(--fb-space-12);
        color: var(--fb-text-muted);
      }

      .empty-state h3 {
        font-size: var(--fb-text-lg);
        margin-bottom: var(--fb-space-2);
      }

      .empty-state p {
        font-size: var(--fb-text-sm);
      }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._loadCards();
  }

  render() {
    if (this._loading) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    const filteredCards = this._getFilteredCards();

    return html`
      <div class="page-header">
        <h1>Daily Briefing</h1>
        <div class="filter-chips">
          ${(['all', 'critical', 'urgent', 'normal'] as const).map(
            (f) => html`
              <button
                class="chip ${f !== 'all' ? f : ''} ${this._filter === f ? 'active' : ''}"
                @click=${() => this._setFilter(f)}
              >${f === 'all' ? 'All' : f.charAt(0).toUpperCase() + f.slice(1)}</button>
            `,
          )}
        </div>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      <div class="card-count">
        Showing <span>${filteredCards.length}</span> of <span>${this._cards.length}</span> cards
      </div>

      ${filteredCards.length === 0
        ? html`
          <div class="empty-state">
            <h3>No feed cards</h3>
            <p>${this._filter === 'all' ? 'All caught up! No active briefing items.' : `No ${this._filter} priority items.`}</p>
          </div>
        `
        : html`
          <div class="feed-list">
            ${filteredCards.map((card) => this._renderCard(card))}
          </div>
        `}
    `;
  }

  private _renderCard(card: FeedCard) {
    const isActioning = this._actioningCard === card.id;
    return html`
      <div class="feed-card ${card.priority}">
        <div class="card-header">
          <div class="card-title">${card.title}</div>
          <div class="card-meta">
            <span class="card-type">${card.card_type.replace(/_/g, ' ')}</span>
            <span class="card-priority ${card.priority}">${card.priority}</span>
          </div>
        </div>
        <div class="card-body">${card.body}</div>
        <div class="card-footer">
          <div class="card-actions">
            ${card.actions.map(
              (action) => html`
                <button
                  class="action-btn"
                  ?disabled=${isActioning}
                  @click=${() => this._handleAction(card.id, action.action_type, action.payload)}
                >${action.label}</button>
              `,
            )}
            <button
              class="dismiss-btn"
              ?disabled=${isActioning}
              @click=${() => this._handleDismiss(card.id)}
            >Dismiss</button>
          </div>
          <div class="card-time">${this._formatTime(card.created_at)}</div>
        </div>
      </div>
    `;
  }

  private _getFilteredCards(): FeedCard[] {
    if (this._filter === 'all') return this._cards;
    return this._cards.filter((c) => c.priority === this._filter);
  }

  private _setFilter(f: PriorityFilter) {
    this._filter = f;
  }

  private _formatTime(isoString: string): string {
    try {
      const d = new Date(isoString);
      const now = new Date();
      const diffMs = now.getTime() - d.getTime();
      const diffMins = Math.floor(diffMs / 60000);
      if (diffMins < 60) return `${diffMins}m ago`;
      const diffHours = Math.floor(diffMins / 60);
      if (diffHours < 24) return `${diffHours}h ago`;
      return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    } catch {
      return isoString;
    }
  }

  private async _handleAction(cardID: string, actionType: string, payload: Record<string, unknown>) {
    this._actioningCard = cardID;
    try {
      await actionFeed(cardID, { action_type: actionType, payload });
      this._cards = this._cards.filter((c) => c.id !== cardID);
      this.showToast('Action completed', 'success');
    } catch (err) {
      const msg = err instanceof ApiError ? `Action failed (${err.status})` : 'Action failed';
      this.showToast(msg, 'error');
    } finally {
      this._actioningCard = null;
    }
  }

  private async _handleDismiss(cardID: string) {
    this._actioningCard = cardID;
    try {
      await dismissFeed(cardID);
      this._cards = this._cards.filter((c) => c.id !== cardID);
      this.showToast('Card dismissed', 'info');
    } catch (err) {
      const msg = err instanceof ApiError ? `Dismiss failed (${err.status})` : 'Dismiss failed';
      this.showToast(msg, 'error');
    } finally {
      this._actioningCard = null;
    }
  }

  private async _loadCards() {
    this._loading = true;
    this._error = '';
    try {
      const res = await listFeed({ status: 'active' });
      this._cards = res.cards;
    } catch (err) {
      this._error = err instanceof ApiError ? `Failed to load feed (${err.status})` : 'Failed to load feed';
      this.showToast(this._error, 'error');
    } finally {
      this._loading = false;
    }
  }
}
