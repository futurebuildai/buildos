import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Endpoint modules are mocked so pages never hit the network.
vi.mock('../src/api/endpoints/projects.js', () => ({
  listProjects: vi.fn(),
}));
vi.mock('../src/api/endpoints/schedule.js', () => ({
  getGantt: vi.fn(),
  recalculateSchedule: vi.fn(),
  recommendAdjustments: vi.fn(),
}));
vi.mock('../src/api/endpoints/procurement.js', () => ({
  listProcurement: vi.fn(),
  updateProcurementItem: vi.fn(),
  requestVendorReview: vi.fn(),
}));
vi.mock('../src/api/endpoints/feed.js', () => ({
  listFeed: vi.fn(),
  actionFeedCard: vi.fn(),
  dismissFeedCard: vi.fn(),
}));
vi.mock('../src/api/endpoints/briefing.js', () => ({
  getDailyBriefing: vi.fn(),
}));
vi.mock('../src/api/endpoints/audit.js', () => ({
  listAudit: vi.fn(),
}));

import '../src/components/molecules/fb-feed-list.js';
import '../src/components/organisms/fb-gantt-chart.js';
import '../src/components/pages/fb-schedule-page.js';
import '../src/components/pages/fb-procurement-page.js';
import '../src/components/pages/fb-briefing-page.js';
import '../src/components/pages/fb-activity-page.js';
import '../src/components/pages/fb-assistant-page.js';

import * as projectsApi from '../src/api/endpoints/projects.js';
import * as scheduleApi from '../src/api/endpoints/schedule.js';
import * as procurementApi from '../src/api/endpoints/procurement.js';
import * as feedApi from '../src/api/endpoints/feed.js';
import * as briefingApi from '../src/api/endpoints/briefing.js';
import * as auditApi from '../src/api/endpoints/audit.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import { markAiUnconfigured, clearCapabilities } from '../src/state/capabilityStore.js';
import type {
  Project,
  ProjectTask,
  GanttView,
  ProcurementItem,
  FeedCard,
  DailyBriefing,
  AuditEntry,
} from '../src/types/models.js';

async function mount<T extends HTMLElement>(
  tag: string,
  attrs: Record<string, string> = {},
): Promise<T> {
  const el = document.createElement(tag) as T;
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

async function flush(page: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

function text(page: HTMLElement, selector: string): string {
  return page.shadowRoot!.querySelector(selector)?.textContent?.trim() ?? '';
}

function apiError(code: string, status = 400): ApiError {
  return new ApiError({ code, message: code, status });
}

const PROJECT: Project = {
  id: 'p-1',
  org_id: 'org-1',
  name: 'Maple Street Duplex',
  address: '123 Maple St',
  project_start_date: '2026-03-01',
  status: 'active',
  created_at: '2026-01-01',
  updated_at: '2026-01-01',
};

const TASK_A: ProjectTask = {
  id: 't-1',
  project_id: 'p-1',
  wbs_code: '1.0',
  name: 'Foundation',
  duration_days: 5,
  early_start: '2026-03-01',
  early_finish: '2026-03-06',
  late_start: '2026-03-01',
  late_finish: '2026-03-06',
  total_float: 0,
  is_critical: true,
  status: 'pending',
  percent_complete: 0,
  created_at: '2026-01-01',
  updated_at: '2026-01-01',
};
const TASK_B: ProjectTask = {
  ...TASK_A,
  id: 't-2',
  wbs_code: '2.0',
  name: 'Framing',
  early_start: '2026-03-06',
  early_finish: '2026-03-12',
  late_finish: '2026-03-15',
  total_float: 3,
  is_critical: false,
};

const COMPUTED_GANTT: GanttView = {
  tasks: [TASK_A, TASK_B],
  critical_path: ['t-1'],
  project_end: '2026-03-12T00:00:00Z',
};
const EMPTY_GANTT: GanttView = {
  tasks: [],
  critical_path: [],
  project_end: '0001-01-01T00:00:00Z',
};

const PROC_ITEM: ProcurementItem = {
  id: 'pi-1',
  project_id: 'p-1',
  name: 'Rebar bundle',
  wbs_code: '1.0',
  estimated_cost_cents: '250000',
  estimated_cost_currency_code: 'USD',
  lead_time_days: 14,
  weather_buffer_days: 2,
  must_order_date: '2026-02-01',
  status: 'CRITICAL',
  status_changed_at: '2026-01-15',
  created_at: '2026-01-01',
  updated_at: '2026-01-10',
};

const FEED_CARD: FeedCard = {
  id: 'f-1',
  card_type: 'alert',
  title: 'Concrete pour at risk',
  body: 'Rain forecast collides with the slab pour window.',
  priority: 'critical',
  status: 'active',
  created_at: '2026-01-20T08:00:00Z',
};

beforeEach(() => {
  vi.clearAllMocks();
  clearCapabilities(); // reset AI to assume-on
  vi.mocked(projectsApi.listProjects).mockResolvedValue([]);
  vi.mocked(scheduleApi.getGantt).mockResolvedValue(EMPTY_GANTT);
  vi.mocked(procurementApi.listProcurement).mockResolvedValue([]);
  vi.mocked(feedApi.listFeed).mockResolvedValue([]);
  vi.mocked(auditApi.listAudit).mockResolvedValue([]);
  vi.mocked(briefingApi.getDailyBriefing).mockResolvedValue({
    reply: 'All quiet.',
    session_id: 's-1',
    task_count: 0,
    alert_count: 0,
  } satisfies DailyBriefing);
});

afterEach(() => {
  document.body.innerHTML = '';
  window.history.replaceState({}, '', '/');
  clearCapabilities();
});

describe('fb-gantt-chart', () => {
  it('renders the decorative SVG plus a parallel accessible table row per task', async () => {
    const el = await mount('fb-gantt-chart');
    (el as unknown as { tasks: ProjectTask[] }).tasks = [TASK_A, TASK_B];
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
    // The SVG is aria-hidden (dual representation, DSC §7.8); the accessible
    // table is the canonical data carrier and the layer assertions target —
    // happy-dom cannot render comment-marked dynamic content inside <svg>, so
    // SVG bar rendering is verified by the Playwright suite (Phase F).
    expect(el.shadowRoot!.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true');
    expect(el.shadowRoot!.querySelectorAll('table tbody tr').length).toBe(2);
  });

  it('marks critical tasks distinctly from float tasks in the accessible table', async () => {
    const el = await mount('fb-gantt-chart');
    (el as unknown as { tasks: ProjectTask[] }).tasks = [TASK_A, TASK_B];
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
    const rows = [...el.shadowRoot!.querySelectorAll('table tbody tr')];
    const criticalCol = rows.map((r) => {
      const cells = [...r.querySelectorAll('td')];
      return cells[cells.length - 1]?.textContent?.trim();
    });
    expect(criticalCol).toContain('Yes');
    expect(criticalCol).toContain('No');
  });

  it('falls back to the table only when no tasks have dates', async () => {
    const el = await mount('fb-gantt-chart');
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
    expect(el.shadowRoot!.querySelector('svg')).toBeNull();
    expect(el.shadowRoot!.querySelector('table')).not.toBeNull();
  });
});

describe('fb-feed-list', () => {
  it('renders a card per active feed item', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([FEED_CARD]);
    const el = await mount('fb-feed-list');
    await flush(el);
    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(1);
  });

  it('shows the all-caught-up empty state with no cards', async () => {
    const el = await mount('fb-feed-list');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-state')?.getAttribute('mode')).toBe('empty');
  });

  it('filters to the requested priorities', async () => {
    const normal: FeedCard = { ...FEED_CARD, id: 'f-2', priority: 'normal' };
    vi.mocked(feedApi.listFeed).mockResolvedValue([FEED_CARD, normal]);
    const el = await mount('fb-feed-list');
    (el as unknown as { priorities: string[] }).priorities = ['critical'];
    await flush(el);
    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(1);
  });

  it('dismisses optimistically and rolls back on failure', async () => {
    vi.mocked(feedApi.listFeed).mockResolvedValue([FEED_CARD]);
    vi.mocked(feedApi.dismissFeedCard).mockRejectedValueOnce(apiError(ErrorCode.CONFLICT, 409));
    const el = await mount('fb-feed-list');
    await flush(el);
    await (el as unknown as { onDismiss(c: FeedCard): Promise<void> }).onDismiss(FEED_CARD);
    await flush(el);
    // Rolled back: the card is still present after the failed dismiss.
    expect(el.shadowRoot!.querySelectorAll('fb-feed-card').length).toBe(1);
  });
});

describe('fb-procurement-page', () => {
  it('shows the no-projects empty state', async () => {
    const el = await mount('fb-procurement-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-state')?.getAttribute('mode')).toBe('empty');
  });

  it('loads items for the first project and groups by status', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
    vi.mocked(procurementApi.listProcurement).mockResolvedValue([PROC_ITEM]);
    const el = await mount('fb-procurement-page');
    await flush(el);
    expect(procurementApi.listProcurement).toHaveBeenCalledWith('p-1');
    expect(text(el, '.item-name')).toBe('Rebar bundle');
    // Per-currency subtotal section present (never a cross-currency grand total).
    expect(el.shadowRoot!.querySelector('.subtotals')).not.toBeNull();
  });

  it('shows a retryable error when items fail to load', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
    vi.mocked(procurementApi.listProcurement).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mount('fb-procurement-page');
    await flush(el);
    const state = el.shadowRoot!.querySelector('fb-state');
    expect(state?.getAttribute('mode')).toBe('error');
  });
});

describe('fb-schedule-page', () => {
  it('shows the never-computed empty state for a zero-value schedule', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
    vi.mocked(scheduleApi.getGantt).mockResolvedValue(EMPTY_GANTT);
    const el = await mount('fb-schedule-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-state')?.getAttribute('mode')).toBe('empty');
  });

  it('renders the gantt chart for a computed schedule', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
    vi.mocked(scheduleApi.getGantt).mockResolvedValue(COMPUTED_GANTT);
    const el = await mount('fb-schedule-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-gantt-chart')).not.toBeNull();
  });
});

describe('fb-briefing-page', () => {
  it('renders the AI hero and context chips when AI is on', async () => {
    const el = await mount('fb-briefing-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('.hero')).not.toBeNull();
    expect(el.shadowRoot!.querySelectorAll('fb-chip').length).toBe(2);
    // Briefing always renders the priority feed below the hero.
    expect(el.shadowRoot!.querySelector('fb-feed-list')).not.toBeNull();
  });

  it('renders the gated panel when AI is off, skipping the call', async () => {
    markAiUnconfigured();
    const el = await mount('fb-briefing-page');
    await flush(el);
    expect(briefingApi.getDailyBriefing).not.toHaveBeenCalled();
    const states = [...el.shadowRoot!.querySelectorAll('fb-state')];
    expect(states.some((s) => s.getAttribute('mode') === 'gated')).toBe(true);
  });

  it('degrades to the gated panel on a 503 soft-fail', async () => {
    vi.mocked(briefingApi.getDailyBriefing).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mount('fb-briefing-page');
    await flush(el);
    const states = [...el.shadowRoot!.querySelectorAll('fb-state')];
    expect(states.some((s) => s.getAttribute('mode') === 'gated')).toBe(true);
  });
});

describe('fb-activity-page', () => {
  it('degrades gracefully when the audit route is absent', async () => {
    vi.mocked(auditApi.listAudit).mockRejectedValueOnce(apiError(ErrorCode.NOT_FOUND, 404));
    const el = await mount('fb-activity-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-state')?.getAttribute('mode')).toBe('empty');
  });

  it('renders the audit trail when entries load', async () => {
    const entry: AuditEntry = {
      id: 'a-1',
      org_id: 'org-1',
      action: 'setup.trade.created',
      created_at: '2026-01-20T10:00:00Z',
    };
    vi.mocked(auditApi.listAudit).mockResolvedValue([entry]);
    const el = await mount('fb-activity-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-audit-trail')).not.toBeNull();
  });
});

describe('fb-assistant-page', () => {
  it('lists the live AI capabilities when AI is on', async () => {
    const el = await mount('fb-assistant-page');
    await flush(el);
    expect(el.shadowRoot!.querySelectorAll('.cap').length).toBe(2);
  });

  it('renders the gated panel when AI is off', async () => {
    markAiUnconfigured();
    const el = await mount('fb-assistant-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-state')?.getAttribute('mode')).toBe('gated');
  });
});
