import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ---------------------------------------------------------------------------
// PREVIEW-FIRST schedule apply/reject (Chunk 2b, ESC-AUX-01). The "Suggest
// adjustments" modal calls recommendAdjustments(dry_run) → renders per-row
// proposals → the user selects rows → applyAdjustments commits them. We mock
// the endpoints + the auth/capability stores (gated min-superintendent) so the
// page renders deterministically off in-test fixtures.
// ---------------------------------------------------------------------------
vi.mock('../src/api/endpoints/projects.js', () => ({
  listProjects: vi.fn(),
}));
vi.mock('../src/api/endpoints/schedule.js', () => ({
  getGantt: vi.fn(),
  recalculateSchedule: vi.fn(),
  recommendAdjustments: vi.fn(),
  applyAdjustments: vi.fn(),
}));

let aiConfiguredValue = true;
const markAiUnconfigured = vi.fn();
vi.mock('../src/state/capabilityStore.js', () => ({
  aiConfigured: { get: () => aiConfiguredValue },
  markAiUnconfigured: () => markAiUnconfigured(),
}));

let minRole: 'field_worker' | 'superintendent' | 'admin' | 'owner' = 'superintendent';
const ROLE_RANK = { field_worker: 0, superintendent: 1, admin: 2, owner: 3 } as const;
vi.mock('../src/state/authStore.js', () => ({
  hasRole: (...roles: string[]) => roles.includes(minRole),
  hasMinRole: (min: keyof typeof ROLE_RANK) => ROLE_RANK[minRole] >= ROLE_RANK[min],
}));

const navigate = vi.fn();
vi.mock('../src/router.js', () => ({
  navigate: (path: string) => navigate(path),
}));

import '../src/components/pages/fb-schedule-page.js';

import * as projectsApi from '../src/api/endpoints/projects.js';
import * as scheduleApi from '../src/api/endpoints/schedule.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import type {
  Project,
  ProjectTask,
  GanttView,
  ScheduleAdjustmentSet,
} from '../src/types/models.js';

// ----------------------------- harness helpers -----------------------------

async function mount<T extends HTMLElement>(tag: string): Promise<T> {
  const el = document.createElement(tag) as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

async function flush(page: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

const sr = (page: HTMLElement): ShadowRoot => page.shadowRoot!;

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

const COMPUTED_GANTT: GanttView = {
  tasks: [TASK_A],
  critical_path: ['t-1'],
  project_end: '2026-03-12T00:00:00Z',
  dependencies: [],
};

/** A preview set: two proposed changes (one critical) + one advisory row. */
function previewSet(over: Partial<ScheduleAdjustmentSet> = {}): ScheduleAdjustmentSet {
  return {
    adjustments: [
      {
        task_id: 't-1',
        wbs_code: '1.0',
        name: 'Foundation',
        old_duration_days: 5,
        new_duration_days: 9,
        rationale: 'Extend **cure time** for the cold pour.',
        is_critical: true,
        proposed_change: true,
        applied: false,
      },
      {
        task_id: 't-2',
        wbs_code: '2.0',
        name: 'Framing',
        old_duration_days: 3,
        new_duration_days: 4,
        rationale: 'Add a buffer day.',
        is_critical: false,
        proposed_change: true,
        applied: false,
      },
      {
        task_id: 't-3',
        wbs_code: '3.0',
        name: 'Roofing',
        old_duration_days: 2,
        rationale: 'Monitor weather; no change needed yet.',
        is_critical: false,
        proposed_change: false,
        applied: false,
      },
    ],
    dry_run: true,
    proposed_changes: 2,
    advisory_count: 1,
    applied_deltas: 0,
    critical_recomputed: false,
    skipped_rationale_only: 1,
    ...over,
  };
}

function apiError(code: string, status = 400): ApiError {
  return new ApiError({ code, message: code, status });
}

/** Open the AI adjustments modal by clicking the "Suggest adjustments" toolbar button. */
async function openModal(page: HTMLElement): Promise<void> {
  const buttons = [...sr(page).querySelectorAll('fb-button')] as HTMLElement[];
  const suggest = buttons.find((b) => b.textContent?.includes('Suggest adjustments'));
  if (!suggest) throw new Error('no "Suggest adjustments" button');
  suggest.dispatchEvent(new MouseEvent('click', { bubbles: true, composed: true }));
  await flush(page);
}

const proposedRows = (page: HTMLElement): Element[] => [
  ...sr(page).querySelectorAll('section[aria-label="Proposed changes"] .adj'),
];
const advisorySection = (page: HTMLElement): Element | null =>
  sr(page).querySelector('section[aria-label="Advisory"]');

beforeEach(() => {
  vi.clearAllMocks();
  aiConfiguredValue = true;
  minRole = 'superintendent';
  vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
  vi.mocked(scheduleApi.getGantt).mockResolvedValue(COMPUTED_GANTT);
  vi.mocked(scheduleApi.recommendAdjustments).mockResolvedValue(previewSet());
  vi.mocked(scheduleApi.applyAdjustments).mockResolvedValue({
    applied_deltas: 1,
    critical_recomputed: true,
  });
});

afterEach(() => {
  document.body.innerHTML = '';
});

describe('fb-schedule-page — AI adjustments modal (PREVIEW-FIRST)', () => {
  it('suggest calls recommendAdjustments with dry_run=true and never auto-applies', async () => {
    const el = await mount('fb-schedule-page');
    await flush(el);
    await openModal(el);

    expect(scheduleApi.recommendAdjustments).toHaveBeenCalledWith('p-1', true);
    // Preview must not write anything.
    expect(scheduleApi.applyAdjustments).not.toHaveBeenCalled();
  });

  it('renders proposed rows with old→new delta + critical tag, advisory in a separate section', async () => {
    const el = await mount('fb-schedule-page');
    await flush(el);
    await openModal(el);

    const rows = proposedRows(el);
    expect(rows.length).toBe(2);
    // The first proposed row shows its identity + the old→new delta in mono.
    const firstText = rows[0]?.textContent ?? '';
    expect(firstText).toContain('1.0');
    expect(firstText).toContain('Foundation');
    expect(rows[0]?.querySelector('.adj-delta')?.textContent).toContain('5d → 9d');
    // Critical-path tag present on the critical proposed row.
    expect(rows[0]?.querySelector('fb-badge[status="critical"]')).not.toBeNull();
    // Header reflects proposed vs advisory, not "applied".
    const summary = sr(el).querySelector('.adj-summary')?.textContent ?? '';
    expect(summary).toContain('proposed change');
    expect(summary).toContain('advisory');
    expect(summary.toLowerCase()).not.toContain('applied');

    // Advisory section exists and carries NO apply control (no checkbox).
    const adv = advisorySection(el);
    expect(adv).not.toBeNull();
    expect(adv?.querySelector('fb-checkbox')).toBeNull();
    expect(adv?.textContent).toContain('Roofing');
  });

  it('rationale renders via fb-markdown (no literal asterisks)', async () => {
    const el = await mount('fb-schedule-page');
    await flush(el);
    await openModal(el);
    const rows = proposedRows(el);
    expect(rows[0]?.querySelector('fb-markdown')).not.toBeNull();
  });

  it('"Apply selected" commits only the selected rows with the right payload', async () => {
    const el = await mount('fb-schedule-page');
    await flush(el);
    await openModal(el);

    // All proposed rows are pre-selected; reject the second by unchecking it.
    const checkboxes = [
      ...sr(el).querySelectorAll('section[aria-label="Proposed changes"] fb-checkbox'),
    ] as HTMLElement[];
    expect(checkboxes.length).toBe(2);
    checkboxes[1]?.dispatchEvent(new CustomEvent('change', { detail: { checked: false } }));
    await flush(el);

    const footerButtons = [...sr(el).querySelectorAll('fb-button[slot="footer"]')] as HTMLElement[];
    const applySelected = footerButtons.find((b) => b.textContent?.includes('Apply selected'));
    expect(applySelected).not.toBeUndefined();
    applySelected!.dispatchEvent(new MouseEvent('click', { bubbles: true, composed: true }));
    await flush(el);

    expect(scheduleApi.applyAdjustments).toHaveBeenCalledTimes(1);
    expect(scheduleApi.applyAdjustments).toHaveBeenCalledWith('p-1', [
      { wbs_code: '1.0', new_duration_days: 9 },
    ]);
    // After a successful apply the Gantt refreshes (getGantt called again).
    expect(vi.mocked(scheduleApi.getGantt).mock.calls.length).toBeGreaterThanOrEqual(2);
    // Confirmation surfaced.
    expect(sr(el).querySelector('.banner.ok')?.textContent).toContain('Applied');
  });

  it('"Apply all" commits every proposed change', async () => {
    const el = await mount('fb-schedule-page');
    await flush(el);
    await openModal(el);

    const footerButtons = [...sr(el).querySelectorAll('fb-button[slot="footer"]')] as HTMLElement[];
    const applyAll = footerButtons.find((b) => b.textContent?.trim() === 'Apply all');
    expect(applyAll).not.toBeUndefined();
    applyAll!.dispatchEvent(new MouseEvent('click', { bubbles: true, composed: true }));
    await flush(el);

    expect(scheduleApi.applyAdjustments).toHaveBeenCalledWith('p-1', [
      { wbs_code: '1.0', new_duration_days: 9 },
      { wbs_code: '2.0', new_duration_days: 4 },
    ]);
  });

  it('surfaces an error when apply fails and does not close', async () => {
    vi.mocked(scheduleApi.applyAdjustments).mockRejectedValueOnce(
      apiError(ErrorCode.UPSTREAM_ERROR, 502),
    );
    const el = await mount('fb-schedule-page');
    await flush(el);
    await openModal(el);

    const footerButtons = [...sr(el).querySelectorAll('fb-button[slot="footer"]')] as HTMLElement[];
    const applySelected = footerButtons.find((b) => b.textContent?.includes('Apply selected'));
    applySelected!.dispatchEvent(new MouseEvent('click', { bubbles: true, composed: true }));
    await flush(el);

    expect(sr(el).querySelector('.toast.err')).not.toBeNull();
    // Modal stays open (still showing the proposed rows).
    expect(sr(el).querySelector('fb-modal')?.hasAttribute('open')).toBe(true);
  });

  it('shows the gated panel when AI is unconfigured (no endpoint call)', async () => {
    aiConfiguredValue = false;
    const el = await mount('fb-schedule-page');
    await flush(el);
    await openModal(el);

    expect(scheduleApi.recommendAdjustments).not.toHaveBeenCalled();
    expect(sr(el).querySelector('fb-state[mode="gated"]')).not.toBeNull();
  });
});
