import { html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import type { FeedPriority } from '../molecules/fb-feed-card.js';

export interface FeedItem {
  id: string;
  icon: string;
  title: string;
  body: string;
  priority: FeedPriority;
  timestamp: string;
}

const PRIORITY_ORDER: Record<FeedPriority, number> = {
  critical: 0,
  urgent: 1,
  normal: 2,
  low: 3,
};

/**
 * fb-feed-list — Scrollable feed card list with priority filtering.
 *
 * Cards are sorted by priority (critical > urgent > normal > low),
 * then by timestamp (newest first).
 *
 * @property items - Array of FeedItem objects
 * @fires fb-feed-action - Bubbled from individual feed cards
 * @fires fb-feed-dismiss - Bubbled when a card is dismissed
 */
@customElement('fb-feed-list')
export class FBFeedList extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .feed-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin-bottom: var(--fb-space-3);
      }

      .feed-title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-lg);
        font-weight: 500;
        color: var(--fb-text-primary);
      }

      .feed-count {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-muted);
      }

      .filters {
        display: flex;
        gap: var(--fb-space-2);
        margin-bottom: var(--fb-space-3);
        flex-wrap: wrap;
      }

      .feed-scroll {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-3);
        max-height: 600px;
        overflow-y: auto;
      }

      .empty-state {
        text-align: center;
        padding: var(--fb-space-8);
        color: var(--fb-text-muted);
        font-family: var(--fb-font-body);
      }
    `,
  ];

  @property({ type: Array }) items: FeedItem[] = [];

  @state() private _activeFilter: FeedPriority | 'all' = 'all';

  private _getFilteredItems(): FeedItem[] {
    let filtered = this.items;
    if (this._activeFilter !== 'all') {
      filtered = filtered.filter(item => item.priority === this._activeFilter);
    }
    return filtered.sort((a, b) => {
      const priorityDiff = PRIORITY_ORDER[a.priority] - PRIORITY_ORDER[b.priority];
      if (priorityDiff !== 0) return priorityDiff;
      return new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime();
    });
  }

  override render() {
    const filtered = this._getFilteredItems();
    const filters: Array<{ id: FeedPriority | 'all'; label: string }> = [
      { id: 'all', label: 'All' },
      { id: 'critical', label: 'Critical' },
      { id: 'urgent', label: 'Urgent' },
      { id: 'normal', label: 'Normal' },
      { id: 'low', label: 'Low' },
    ];

    return html`
      <div class="feed-header">
        <span class="feed-title">Action Feed</span>
        <span class="feed-count">${filtered.length} items</span>
      </div>

      <div class="filters">
        ${filters.map(f => html`
          <fb-chip
            label=${f.label}
            ?active=${this._activeFilter === f.id}
            @fb-chip-toggle=${() => { this._activeFilter = f.id; }}
          ></fb-chip>
        `)}
      </div>

      <div class="feed-scroll">
        ${filtered.length === 0
          ? html`<div class="empty-state">No items to display</div>`
          : filtered.map(item => html`
              <fb-feed-card
                icon=${item.icon}
                cardTitle=${item.title}
                body=${item.body}
                priority=${item.priority}
                timestamp=${item.timestamp}
                @fb-feed-dismiss=${() => this._onDismiss(item.id)}
              ></fb-feed-card>
            `)
        }
      </div>
    `;
  }

  private _onDismiss(id: string) {
    this.emitEvent('fb-feed-dismiss', { id });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-feed-list': FBFeedList;
  }
}
