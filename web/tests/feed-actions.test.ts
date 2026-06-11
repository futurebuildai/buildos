import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Endpoints + router are mocked so the list never hits the network and we can
// assert on navigation without a real history transition.
vi.mock('../src/api/endpoints/feed.js', () => ({
  listFeed: vi.fn(),
  actionFeedCard: vi.fn(),
  dismissFeedCard: vi.fn(),
}));
vi.mock('../src/router.js', () => ({
  navigate: vi.fn(),
}));

import '../src/components/molecules/fb-feed-list.js';

import * as feedApi from '../src/api/endpoints/feed.js';
import * as router from '../src/router.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import type { FeedCard } from '../src/types/models.js';

async function mount(tag: string): Promise<HTMLElement> {
  const el = document.createElement(tag);
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

async function flush(el: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

function apiError(code: string, status = 409): ApiError {
  return new ApiError({ code, message: code, status });
}

/** A delay_cascade review card (producer shape: agentic.go:307-325). */
const REVIEW_CARD: FeedCard = {
  id: 'f-rev',
  project_id: 'proj-77',
  card_type: 'delay_cascade',
  title: 'Framing slip cascades to drywall',
  body: 'The **framing** delay pushes drywall start by 4 days.\n\n- crew idle\n- inspection rebook',
  priority: 'critical',
  status: 'active',
  created_at: '2026-01-20T08:00:00Z',
  actions: [
    {
      label: 'Review impact',
      action_type: 'review_cascade_impact',
      payload: {
        module: 'schedule',
        severity: 'high',
        recommended_action: 'Rebook the drywall crew for the week of the 27th.',
      },
    },
  ],
};

/** A foresight risk on the budget module → financials route. */
const BUDGET_CARD: FeedCard = {
  ...REVIEW_CARD,
  id: 'f-bud',
  card_type: 'budget_burn',
  title: 'Budget burn accelerating',
  actions: [
    {
      label: 'Review impact',
      action_type: 'review_foresight_risk',
      payload: {
        risk_type: 'budget_burn',
        severity: 'critical',
        recommended_action: 'Re-baseline.',
      },
    },
  ],
};

/** A non-review action keeps the legacy optimistic POST /action path. */
const PLAIN_ACTION_CARD: FeedCard = {
  id: 'f-plain',
  card_type: 'progress',
  title: 'Confirm the slab pour',
  body: 'Tap to confirm.',
  priority: 'normal',
  status: 'active',
  created_at: '2026-01-20T08:00:00Z',
  actions: [{ label: 'Confirm', action_type: 'confirm_pour', payload: { ok: true } }],
};

/** A review card with no project_id → detail shows but hides "Go to project". */
function withoutProject(card: FeedCard): FeedCard {
  const clone: FeedCard = { ...card };
  delete clone.project_id;
  return clone;
}
const NO_PROJECT_CARD: FeedCard = { ...withoutProject(REVIEW_CARD), id: 'f-noproj' };

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(feedApi.listFeed).mockResolvedValue([]);
});

afterEach(() => {
  document.body.innerHTML = '';
});

type ListInternals = {
  detail: FeedCard | null;
  onAction(card: FeedCard, e: Event): Promise<void>;
  onMarkHandled(card: FeedCard): Promise<void>;
  onDismiss(card: FeedCard): Promise<void>;
  onGoToProject(card: FeedCard): void;
  updateComplete: Promise<unknown>;
};

function internals(el: HTMLElement): ListInternals {
  return el as unknown as ListInternals;
}

function actionEvent(actionId: string): CustomEvent {
  return new CustomEvent('action', { detail: { actionId } });
}

describe('fb-feed-list — review action opens a detail modal', () => {
  it('clicking a review_* action opens the modal and does NOT action or remove the card', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([REVIEW_CARD]);
    const el = await mount('fb-feed-list');
    await flush(el);

    await internals(el).onAction(REVIEW_CARD, actionEvent('0'));
    await flush(el);

    // Modal open, card still present, no /action POST yet.
    expect(el.shadowRoot!.querySelector('fb-modal')?.hasAttribute('open')).toBe(true);
    expect(internals(el).detail?.id).toBe('f-rev');
    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(1);
    expect(feedApi.actionFeedCard).not.toHaveBeenCalled();
  });

  it('renders the body via fb-markdown and the recommended_action from the payload', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([REVIEW_CARD]);
    const el = await mount('fb-feed-list');
    await flush(el);
    await internals(el).onAction(REVIEW_CARD, actionEvent('0'));
    await flush(el);

    const modal = el.shadowRoot!.querySelector('fb-modal')!;
    // Body goes through fb-markdown (the literal ** is parsed, not shown raw).
    expect(modal.querySelector('fb-markdown')).not.toBeNull();
    // Recommended action text is surfaced.
    expect(modal.textContent).toContain('Rebook the drywall crew');
    // Severity chip present.
    expect(modal.textContent).toContain('high');
  });
});

describe('fb-feed-list — deep-link routing', () => {
  it('"Go to project" navigates to /command/schedule for a delay_cascade card', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([REVIEW_CARD]);
    const el = await mount('fb-feed-list');
    await flush(el);
    await internals(el).onAction(REVIEW_CARD, actionEvent('0'));
    await flush(el);

    internals(el).onGoToProject(REVIEW_CARD);
    expect(router.navigate).toHaveBeenCalledWith('/command/schedule?project=proj-77');
  });

  it('routes a budget_burn card to /portfolio/financials (the real route)', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([BUDGET_CARD]);
    const el = await mount('fb-feed-list');
    await flush(el);
    await internals(el).onAction(BUDGET_CARD, actionEvent('0'));
    await flush(el);

    internals(el).onGoToProject(BUDGET_CARD);
    expect(router.navigate).toHaveBeenCalledWith('/portfolio/financials?project=proj-77');
  });

  it('hides "Go to project" when the card has no project_id', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([NO_PROJECT_CARD]);
    const el = await mount('fb-feed-list');
    await flush(el);
    await internals(el).onAction(NO_PROJECT_CARD, actionEvent('0'));
    await flush(el);

    const footer = el.shadowRoot!.querySelector('fb-modal')!.querySelector('[slot="footer"]')!;
    // Only "Mark handled" — no "Go to" button.
    expect(footer.textContent).toContain('Mark handled');
    expect(footer.textContent).not.toContain('Go to');
  });
});

describe('fb-feed-list — explicit acknowledge only', () => {
  it('"Mark handled" POSTs /action with the action_type + payload, then removes the card', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([REVIEW_CARD]);
    vi.mocked(feedApi.actionFeedCard).mockResolvedValue(undefined);
    const el = await mount('fb-feed-list');
    await flush(el);
    await internals(el).onAction(REVIEW_CARD, actionEvent('0'));
    await flush(el);

    await internals(el).onMarkHandled(REVIEW_CARD);
    await flush(el);

    expect(feedApi.actionFeedCard).toHaveBeenCalledTimes(1);
    expect(feedApi.actionFeedCard).toHaveBeenCalledWith('f-rev', {
      action_type: 'review_cascade_impact',
      payload: REVIEW_CARD.actions![0]!.payload,
    });
    // Card removed + modal closed.
    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(0);
    expect(internals(el).detail).toBeNull();
  });

  it('rolls back + reopens the modal if "Mark handled" fails', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([REVIEW_CARD]);
    vi.mocked(feedApi.actionFeedCard).mockRejectedValueOnce(apiError(ErrorCode.CONFLICT, 409));
    const el = await mount('fb-feed-list');
    await flush(el);
    await internals(el).onAction(REVIEW_CARD, actionEvent('0'));
    await flush(el);

    await internals(el).onMarkHandled(REVIEW_CARD);
    await flush(el);

    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(1);
    expect(internals(el).detail?.id).toBe('f-rev');
  });
});

describe('fb-feed-list — dismiss stays distinct from action', () => {
  it('the X calls dismissFeedCard (not actionFeedCard) and removes the card', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([REVIEW_CARD]);
    vi.mocked(feedApi.dismissFeedCard).mockResolvedValue(undefined);
    const el = await mount('fb-feed-list');
    await flush(el);

    await internals(el).onDismiss(REVIEW_CARD);
    await flush(el);

    expect(feedApi.dismissFeedCard).toHaveBeenCalledWith('f-rev');
    expect(feedApi.actionFeedCard).not.toHaveBeenCalled();
    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(0);
  });
});

describe('fb-feed-list — non-review actions keep the optimistic POST', () => {
  it('a plain action POSTs /action immediately and removes the card', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([PLAIN_ACTION_CARD]);
    vi.mocked(feedApi.actionFeedCard).mockResolvedValue(undefined);
    const el = await mount('fb-feed-list');
    await flush(el);

    await internals(el).onAction(PLAIN_ACTION_CARD, actionEvent('0'));
    await flush(el);

    expect(feedApi.actionFeedCard).toHaveBeenCalledWith('f-plain', {
      action_type: 'confirm_pour',
      payload: { ok: true },
    });
    // No modal opened for a non-review action.
    expect(internals(el).detail).toBeNull();
    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(0);
  });
});
