import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Endpoint modules are mocked so the schedule page never hits the network.
vi.mock('../src/api/endpoints/projects.js', () => ({
  listProjects: vi.fn(),
}));
vi.mock('../src/api/endpoints/schedule.js', () => ({
  getGantt: vi.fn(),
  recalculateSchedule: vi.fn(),
  recommendAdjustments: vi.fn(),
}));

import '../src/components/organisms/fb-gantt-chart.js';
import '../src/components/pages/fb-schedule-page.js';

import * as projectsApi from '../src/api/endpoints/projects.js';
import * as scheduleApi from '../src/api/endpoints/schedule.js';
import { clearCapabilities } from '../src/state/capabilityStore.js';
import type { Project, ProjectTask, GanttView, TaskDependency } from '../src/types/models.js';

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

const PROJECT: Project = {
  id: 'p-1',
  org_id: 'org-1',
  name: 'Maple Street Duplex',
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
  late_finish: '2026-03-12',
  total_float: 0,
  is_critical: true,
};
const TASK_C: ProjectTask = {
  ...TASK_A,
  id: 't-3',
  wbs_code: '3.0',
  name: 'Inspection',
  early_start: '2026-03-06',
  early_finish: '2026-03-09',
  late_finish: '2026-03-15',
  total_float: 3,
  is_critical: false,
};

const DEP_AB: TaskDependency = {
  id: 'd-1',
  project_id: 'p-1',
  predecessor_id: 't-1',
  successor_id: 't-2',
  dependency_type: 'FS',
  lag_days: 0,
};
const DEP_AC: TaskDependency = {
  id: 'd-2',
  project_id: 'p-1',
  predecessor_id: 't-1',
  successor_id: 't-3',
  dependency_type: 'FS',
  lag_days: 0,
};

const GANTT: GanttView = {
  tasks: [TASK_A, TASK_B, TASK_C],
  critical_path: ['t-1', 't-2'],
  project_end: '2026-03-12T00:00:00Z',
  dependencies: [DEP_AB, DEP_AC],
};

function setGantt(
  el: HTMLElement,
  tasks: ProjectTask[],
  deps: TaskDependency[],
  projectEnd = '2026-03-12T00:00:00Z',
): void {
  (el as unknown as { tasks: ProjectTask[] }).tasks = tasks;
  (el as unknown as { dependencies: TaskDependency[] }).dependencies = deps;
  el.setAttribute('project-end', projectEnd);
}

beforeEach(() => {
  vi.clearAllMocks();
  clearCapabilities();
});

afterEach(() => {
  document.body.innerHTML = '';
  window.history.replaceState({}, '', '/');
  clearCapabilities();
});

describe('fb-gantt-chart — planning affordances', () => {
  it('renders each task NAME in the SVG (not just the a11y table)', async () => {
    const el = await mount('fb-gantt-chart');
    setGantt(el, [TASK_A, TASK_B], [DEP_AB]);
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;

    const nameNodes = [...el.shadowRoot!.querySelectorAll('svg text.task-name')];
    expect(nameNodes.length).toBe(2);
    const labels = nameNodes.map((n) => n.textContent ?? '');
    // textContent includes the <title> tooltip + the visible label.
    expect(labels.some((t) => t.includes('Foundation'))).toBe(true);
    expect(labels.some((t) => t.includes('Framing'))).toBe(true);
    // WBS mono prefix still rendered alongside the name.
    const wbs = [...el.shadowRoot!.querySelectorAll('svg text.wbs')].map(
      (n) => n.textContent ?? '',
    );
    expect(wbs.some((t) => t.includes('1.0'))).toBe(true);
  });

  it('draws a dated axis with mono tick labels', async () => {
    const el = await mount('fb-gantt-chart');
    setGantt(el, [TASK_A, TASK_B], [DEP_AB]);
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;

    const ticks = [...el.shadowRoot!.querySelectorAll('svg text.axis-tick')];
    expect(ticks.length).toBeGreaterThan(0);
    // The day-0 origin is 2026-03-01 → "Mar 1".
    const first = ticks[0]?.textContent ?? '';
    expect(first).toContain('Mar 1');
  });

  it('draws a dependency arrow path for an FS chain', async () => {
    const el = await mount('fb-gantt-chart');
    setGantt(el, [TASK_A, TASK_B], [DEP_AB]);
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;

    const deps = [...el.shadowRoot!.querySelectorAll('svg path.dep')];
    expect(deps.length).toBe(1);
    // Both endpoints critical → the link renders in the critical (green) style.
    expect(el.shadowRoot!.querySelector('svg path.dep.critical')).not.toBeNull();
    // An arrowhead marker is wired.
    expect(deps[0]?.getAttribute('marker-end')).toContain('arrow');
  });

  it('keeps critical-path bar styling intact (regression guard)', async () => {
    const el = await mount('fb-gantt-chart');
    setGantt(el, [TASK_A, TASK_C], [DEP_AC]);
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;

    const bars = [...el.shadowRoot!.querySelectorAll('svg rect.bar')];
    expect(bars.length).toBe(2);
    expect(el.shadowRoot!.querySelector('svg rect.bar.critical')).not.toBeNull();
    expect(el.shadowRoot!.querySelector('svg rect.bar.normal')).not.toBeNull();
  });

  it('keeps the SVG aria-hidden and the table as the AT surface, with deps in the table', async () => {
    const el = await mount('fb-gantt-chart');
    setGantt(el, [TASK_A, TASK_B], [DEP_AB]);
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;

    expect(el.shadowRoot!.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true');
    const rows = [...el.shadowRoot!.querySelectorAll('table tbody tr.task-row')];
    expect(rows.length).toBe(2);
    // The table now carries a "Predecessors" column; Framing has one (from t-1).
    const headers = [...el.shadowRoot!.querySelectorAll('table thead th')].map(
      (h) => h.textContent?.trim() ?? '',
    );
    expect(headers).toContain('Predecessors');
    const framingRow = rows.find((r) => r.textContent?.includes('Framing'));
    const cells = [...(framingRow?.querySelectorAll('td') ?? [])].map((c) => c.textContent?.trim());
    expect(cells).toContain('1'); // one predecessor
  });

  it('fires task-select with the task id when a table row is activated', async () => {
    const el = await mount('fb-gantt-chart');
    setGantt(el, [TASK_A, TASK_B], [DEP_AB]);
    await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;

    const events: string[] = [];
    el.addEventListener('task-select', (e) =>
      events.push((e as CustomEvent<{ id: string }>).detail.id),
    );

    const rows = [...el.shadowRoot!.querySelectorAll('table tbody tr.task-row')] as HTMLElement[];
    // Rows are keyboard-focusable buttons.
    expect(rows[0]?.getAttribute('tabindex')).toBe('0');
    expect(rows[0]?.getAttribute('role')).toBe('button');
    rows[0]?.click();
    expect(events).toEqual(['t-1']);

    // Enter key also activates.
    rows[1]?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    expect(events).toEqual(['t-1', 't-2']);
  });
});

describe('fb-schedule-page — click-to-inspect detail drawer', () => {
  it('opens a detail drawer with the task dates when a row fires task-select', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
    vi.mocked(scheduleApi.getGantt).mockResolvedValue(GANTT);
    const page = await mount('fb-schedule-page');
    await flush(page);

    // No drawer yet.
    expect(page.shadowRoot!.querySelector('fb-modal[open]')).toBeNull();

    // Activate a row on the embedded chart.
    const chart = page.shadowRoot!.querySelector('fb-gantt-chart') as HTMLElement;
    expect(chart).not.toBeNull();
    const row = chart.shadowRoot!.querySelector('table tbody tr.task-row') as HTMLElement;
    row.click();
    await flush(page);

    const modal = page.shadowRoot!.querySelector('fb-modal[open]');
    expect(modal).not.toBeNull();
    const heading = modal?.getAttribute('heading') ?? '';
    expect(heading).toContain('1.0');
    expect(heading).toContain('Foundation');
    // ES/EF/LS/LF dates are surfaced in the drawer body.
    const body = modal?.textContent ?? '';
    expect(body).toContain('2026-03-01'); // early start
    expect(body).toContain('2026-03-06'); // early/late finish
    expect(body).toContain('Critical path'); // critical badge
  });

  it('passes the dependency edges through to the chart', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
    vi.mocked(scheduleApi.getGantt).mockResolvedValue(GANTT);
    const page = await mount('fb-schedule-page');
    await flush(page);

    const chart = page.shadowRoot!.querySelector('fb-gantt-chart') as HTMLElement & {
      dependencies: TaskDependency[];
    };
    expect(chart.dependencies.length).toBe(2);
    // Two dependency arrows are drawn in the SVG.
    expect(chart.shadowRoot!.querySelectorAll('svg path.dep').length).toBe(2);
  });
});
