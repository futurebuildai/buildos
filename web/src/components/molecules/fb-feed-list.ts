import { html, css, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-feed-card.js';
import '../organisms/fb-state.js';
import { listFeed, actionFeedCard, dismissFeedCard } from '../../api/endpoints/feed.js';
import { ApiError } from '../../api/errors.js';
import type { FeedCard, FeedBackendPriority } from '../../types/models.js';
import type { FeedPriority, FeedAction } from './fb-feed-card.js';

/** Backend 4-tier priority → fb-feed-card's 3-tier visual scale (§6.2). */
const UI_PRIORITY: Record<FeedBackendPriority, FeedPriority> = {
  critical: 'critical',
  urgent: 'warning',
  normal: 'info',
  low: 'info',
};

/** Descending-urgency sort weight (critical first, then created_at desc). */
const PRIORITY_RANK: Record<FeedBackendPriority, number> = {
  critical: 0,
  urgent: 1,
  normal: 2,
  low: 3,
};

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '';
  const secs = Math.round((Date.now() - then) / 1000);
  if (secs < 60) return 'just now';
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.round(hrs / 24)}d ago`;
}

/**
 * `fb-feed-list` — the prioritized notification list (UX_CORE_SCREENS §6),
 * reused by the Feed surface and the Daily Briefing (§10). Sorts critical →
 * urgent → normal → low then newest-first, maps each card's `actions[]` (opaque
 * JSONB, OQ-11) to buttons, and dismisses/acts **optimistically** with rollback
 * on a 4xx. An optional `priorities` filter lets the Briefing show only
 * critical+urgent without a second endpoint shape.
 */
@customElement('fb-feed-list')
export class FbFeedList extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .list {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
      }
    `,
  ];

  /** When set, only these backend priorities are shown (Briefing → critical+urgent). */
  @property({ type: Array }) priorities?: FeedBackendPriority[];

  @state() private cards: FeedCard[] = [];
  @state() private loading = true;
  @state() private errorCode: string | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  async load(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.cards = await listFeed({ status: 'active' });
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private visible(): FeedCard[] {
    const filtered = this.priorities
      ? this.cards.filter((c) => this.priorities!.includes(c.priority))
      : this.cards;
    return [...filtered].sort((a, b) => {
      const r = PRIORITY_RANK[a.priority] - PRIORITY_RANK[b.priority];
      if (r !== 0) return r;
      return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
    });
  }

  private cardActions(card: FeedCard): FeedAction[] {
    return (card.actions ?? []).map((a, i) => ({ id: String(i), label: a.label }));
  }

  /** Optimistic remove with rollback on 4xx. */
  private async onAction(card: FeedCard, e: Event): Promise<void> {
    const actionId = Number((e as CustomEvent<{ actionId: string }>).detail.actionId);
    const spec = card.actions?.[actionId];
    if (!spec) return;
    const prev = this.cards;
    this.cards = this.cards.filter((c) => c.id !== card.id);
    try {
      await actionFeedCard(card.id, {
        action_type: spec.action_type,
        ...(spec.payload ? { payload: spec.payload } : {}),
      });
    } catch {
      this.cards = prev; // rollback
    }
  }

  private async onDismiss(card: FeedCard): Promise<void> {
    const prev = this.cards;
    this.cards = this.cards.filter((c) => c.id !== card.id);
    try {
      await dismissFeedCard(card.id);
    } catch {
      this.cards = prev; // rollback
    }
  }

  override render(): TemplateResult {
    if (this.loading) return html`<fb-state mode="loading" skeleton="card" rows="4"></fb-state>`;
    if (this.errorCode)
      return html`<fb-state
        mode="error"
        error-code=${this.errorCode}
        retryable
        @retry=${() => void this.load()}
      ></fb-state>`;
    const cards = this.visible();
    if (cards.length === 0)
      return html`<fb-state
        mode="empty"
        icon="check-circle"
        heading="You're all caught up"
        message="New alerts and notifications will appear here."
      ></fb-state>`;
    return html`<div class="list">
      ${cards.map(
        (c) =>
          html`<fb-feed-card
            priority=${UI_PRIORITY[c.priority]}
            heading=${c.title}
            message=${c.body}
            timestamp=${relativeTime(c.created_at)}
            .actions=${this.cardActions(c)}
            dismissible
            @action=${(e: Event) => void this.onAction(c, e)}
            @dismiss=${() => void this.onDismiss(c)}
          ></fb-feed-card>`,
      )}
    </div>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-feed-list': FbFeedList;
  }
}
