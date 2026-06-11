import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-feed-card.js';
import '../atoms/fb-button.js';
import '../atoms/fb-chip.js';
import '../atoms/fb-markdown.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-state.js';
import { listFeed, actionFeedCard, dismissFeedCard } from '../../api/endpoints/feed.js';
import { ApiError } from '../../api/errors.js';
import { navigate } from '../../router.js';
import type { FeedCard, FeedCardAction, FeedBackendPriority } from '../../types/models.js';
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

/**
 * Maps a card's `card_type` (set by both producers — `agentic.go:320`
 * "delay_cascade", `foresight.go:412` risk-type values) onto the in-app module
 * route. The deep-link routes to the right surface; preselecting the specific
 * project via `?project=<id>` is an optional enhancement to that page's loader
 * (flagged as a follow-up — routing alone is correct for v1).
 */
const CARD_TYPE_ROUTE: Record<string, string> = {
  schedule_slip: '/command/schedule',
  delay_cascade: '/command/schedule',
  budget_burn: '/portfolio/financials',
  procurement_criticality: '/command/procurement',
};

/** Human label for the module a `card_type` deep-links to. */
const CARD_TYPE_MODULE_LABEL: Record<string, string> = {
  schedule_slip: 'schedule',
  delay_cascade: 'schedule',
  budget_burn: 'financials',
  procurement_criticality: 'procurement',
};

/** A review-type action opens the detail modal instead of auto-acting. */
function isReviewAction(a: FeedCardAction): boolean {
  return typeof a.action_type === 'string' && a.action_type.startsWith('review_');
}

/** Reads a string field off the opaque action payload, defaulting to ''. */
function payloadString(payload: Record<string, unknown> | undefined, key: string): string {
  const v = payload?.[key];
  return typeof v === 'string' ? v : '';
}

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
 * JSONB) to buttons, and dismisses/acts on real endpoints.
 *
 * Action semantics (AGENTIC_UX §2a) — review vs dismiss vs action are distinct:
 *  - A **`review_*`** action opens a detail modal (full body via `fb-markdown`,
 *    the payload's `recommended_action` + `severity`, and a "Go to project"
 *    deep-link). It is NOT optimistically removed and does NOT auto-POST
 *    `/action` — the card is only acknowledged ("Mark handled") once the user
 *    explicitly acts inside the modal.
 *  - Any **non-review** action POSTs `/feed/{id}/action` optimistically (with
 *    rollback on a 4xx), as before.
 *  - The **X** stays the "not relevant" path → `/feed/{id}/dismiss`.
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
      .modal-meta {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        margin-bottom: var(--fb-spacing-md);
      }
      .rec-label {
        font-size: var(--fb-text-body-sm);
        font-weight: 600;
        color: var(--fb-text-secondary);
        text-transform: uppercase;
        letter-spacing: 0.04em;
        margin: var(--fb-spacing-md) 0 var(--fb-spacing-xs);
      }
      .rec-body {
        color: var(--fb-text-primary);
        line-height: 1.6;
      }
      /* announced-but-invisible dismiss confirmation for SR users */
      .sr-only {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip: rect(0, 0, 0, 0);
        white-space: nowrap;
        border: 0;
      }
    `,
  ];

  /** When set, only these backend priorities are shown (Briefing → critical+urgent). */
  @property({ type: Array }) priorities?: FeedBackendPriority[];

  @state() private cards: FeedCard[] = [];
  @state() private loading = true;
  @state() private errorCode: string | null = null;
  /** The card whose review detail modal is open, or null when closed. */
  @state() private detail: FeedCard | null = null;
  /** Acknowledging the open detail card (POST /action in flight). */
  @state() private acting = false;
  /** Polite live-region announcement (dismiss/handled), cleared after read. */
  @state() private announcement = '';

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

  /**
   * Routes a card's action. A `review_*` action opens the detail modal (no
   * write, no removal). Any other action keeps the legacy optimistic POST.
   */
  private async onAction(card: FeedCard, e: Event): Promise<void> {
    const actionId = Number((e as CustomEvent<{ actionId: string }>).detail.actionId);
    const spec = card.actions?.[actionId];
    if (!spec) return;
    if (isReviewAction(spec)) {
      this.detail = card; // open detail; do NOT remove, do NOT POST yet
      return;
    }
    await this.postAction(card, spec);
  }

  /** Optimistic POST /action with rollback on 4xx; removes the card on success. */
  private async postAction(card: FeedCard, spec: FeedCardAction): Promise<void> {
    const prev = this.cards;
    this.cards = this.cards.filter((c) => c.id !== card.id);
    try {
      await actionFeedCard(card.id, {
        action_type: spec.action_type,
        ...(spec.payload ? { payload: spec.payload } : {}),
      });
      this.announce(`${card.title} marked handled.`);
    } catch {
      this.cards = prev; // rollback
    }
  }

  /** The review action spec for the open detail card (first `review_*`), if any. */
  private reviewSpec(card: FeedCard): FeedCardAction | undefined {
    return (card.actions ?? []).find(isReviewAction);
  }

  /** Deep-link route for the open detail card, or null when unmapped/no project. */
  private detailRoute(card: FeedCard): string | null {
    if (!card.project_id) return null;
    return CARD_TYPE_ROUTE[card.card_type] ?? null;
  }

  private closeDetail(): void {
    this.detail = null;
    this.acting = false;
  }

  private onGoToProject(card: FeedCard): void {
    const route = this.detailRoute(card);
    if (!route) return;
    // Routing alone is correct; preselecting the project via the query param is
    // a no-op on pages that ignore it (graceful enhancement, AGENTIC_UX §2a).
    navigate(`${route}?project=${encodeURIComponent(card.project_id!)}`);
    this.closeDetail();
  }

  /**
   * "Mark handled" — only NOW do we POST /action (terminal, irreversible) and
   * remove the card. Optimistic with rollback on failure.
   */
  private async onMarkHandled(card: FeedCard): Promise<void> {
    const spec = this.reviewSpec(card);
    if (!spec) return;
    this.acting = true;
    const prev = this.cards;
    this.cards = this.cards.filter((c) => c.id !== card.id);
    this.detail = null;
    try {
      await actionFeedCard(card.id, {
        action_type: spec.action_type,
        ...(spec.payload ? { payload: spec.payload } : {}),
      });
      this.announce(`${card.title} marked handled.`);
    } catch {
      this.cards = prev; // rollback
      this.detail = card; // reopen so the user can retry
    } finally {
      this.acting = false;
    }
  }

  private async onDismiss(card: FeedCard): Promise<void> {
    const prev = this.cards;
    this.cards = this.cards.filter((c) => c.id !== card.id);
    try {
      await dismissFeedCard(card.id);
      this.announce(`${card.title} dismissed.`);
    } catch {
      this.cards = prev; // rollback
    }
  }

  /** Push a polite live-region message, then clear it so re-announcing works. */
  private announce(msg: string): void {
    this.announcement = msg;
    window.setTimeout(() => {
      this.announcement = '';
    }, 1500);
  }

  private renderDetail(): TemplateResult {
    const card = this.detail;
    if (!card) return html`${nothing}`;
    const spec = this.reviewSpec(card);
    const payload = spec?.payload;
    const recommended = payloadString(payload, 'recommended_action');
    const severity = payloadString(payload, 'severity');
    const route = this.detailRoute(card);
    const moduleLabel = CARD_TYPE_MODULE_LABEL[card.card_type] ?? 'project';
    return html`<fb-modal open heading=${card.title} @close=${this.closeDetail}>
      <div class="modal-meta">
        ${severity ? html`<fb-chip>Severity: ${severity}</fb-chip>` : nothing}
      </div>
      ${card.body ? html`<fb-markdown .source=${card.body}></fb-markdown>` : nothing}
      ${recommended
        ? html`<p class="rec-label">Recommended action</p>
            <p class="rec-body">${recommended}</p>`
        : nothing}
      <div slot="footer">
        ${route
          ? html`<fb-button
              variant="secondary"
              icon="chevron-right"
              @click=${() => this.onGoToProject(card)}
              >Go to ${moduleLabel}</fb-button
            >`
          : nothing}
        <fb-button
          variant="primary"
          icon="check"
          ?loading=${this.acting}
          @click=${() => void this.onMarkHandled(card)}
          >Mark handled</fb-button
        >
      </div>
    </fb-modal>`;
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
      return html`${this.liveRegion()}<fb-state
          mode="empty"
          icon="check-circle"
          heading="You're all caught up"
          message="New alerts and notifications will appear here."
        ></fb-state>`;
    return html`${this.liveRegion()}
      <div class="list">
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
      </div>
      ${this.renderDetail()}`;
  }

  private liveRegion(): TemplateResult {
    return html`<div class="sr-only" role="status" aria-live="polite">${this.announcement}</div>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-feed-list': FbFeedList;
  }
}
