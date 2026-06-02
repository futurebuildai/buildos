import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Endpoint modules are mocked so pages never hit the network. Each Phase D page
// owns one or two endpoint modules; we mock all of them up-front.
vi.mock('../src/api/endpoints/projects.js', () => ({
  listProjects: vi.fn(),
  getProject: vi.fn(),
  listProjectTasks: vi.fn(),
  listProjectBudgets: vi.fn(),
  createProject: vi.fn(),
}));
vi.mock('../src/api/endpoints/financials.js', () => ({
  getFinancialsSummary: vi.fn(),
  getARAging: vi.fn(),
  getProjectFinancials: vi.fn(),
}));
vi.mock('../src/api/endpoints/fleet.js', () => ({
  listFleetAssets: vi.fn(),
  allocateAsset: vi.fn(),
}));
vi.mock('../src/api/endpoints/hr.js', () => ({
  listEmployees: vi.fn(),
  listCertifications: vi.fn(),
}));
vi.mock('../src/api/endpoints/pipeline.js', () => ({
  listProspects: vi.fn(),
  getProspect: vi.fn(),
  getPipelineAnalytics: vi.fn(),
  advanceProspect: vi.fn(),
  loseProspect: vi.fn(),
}));

import '../src/components/pages/fb-projects-page.js';
import '../src/components/pages/fb-project-detail-page.js';
import '../src/components/pages/fb-financials-page.js';
import '../src/components/pages/fb-fleet-page.js';
import '../src/components/pages/fb-hr-page.js';
import '../src/components/pages/fb-pipeline-page.js';

import * as projectsApi from '../src/api/endpoints/projects.js';
import * as financialsApi from '../src/api/endpoints/financials.js';
import * as fleetApi from '../src/api/endpoints/fleet.js';
import * as hrApi from '../src/api/endpoints/hr.js';
import * as pipelineApi from '../src/api/endpoints/pipeline.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import type {
  Project,
  FinancialsSummary,
  FleetAsset,
  Employee,
  Certification,
  Prospect,
  ProspectWithDetails,
  PipelineAnalyticsRow,
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

/** Flush the microtask queue (mocked async loads) then the Lit render. */
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

beforeEach(() => {
  vi.clearAllMocks();
  // Default every loader to an empty success so a bare mount lands on the
  // empty state rather than throwing.
  vi.mocked(projectsApi.listProjects).mockResolvedValue([]);
  vi.mocked(projectsApi.getProject).mockResolvedValue(PROJECT);
  vi.mocked(projectsApi.listProjectTasks).mockResolvedValue([]);
  vi.mocked(projectsApi.listProjectBudgets).mockResolvedValue([]);
  vi.mocked(financialsApi.getFinancialsSummary).mockResolvedValue({
    corporate_budgets: [],
    ar_aging: [],
  } satisfies FinancialsSummary);
  vi.mocked(financialsApi.getProjectFinancials).mockResolvedValue([]);
  vi.mocked(fleetApi.listFleetAssets).mockResolvedValue([]);
  vi.mocked(hrApi.listEmployees).mockResolvedValue([]);
  vi.mocked(hrApi.listCertifications).mockResolvedValue([]);
  vi.mocked(pipelineApi.listProspects).mockResolvedValue([]);
  vi.mocked(pipelineApi.getPipelineAnalytics).mockResolvedValue([]);
});

afterEach(() => {
  document.body.innerHTML = '';
  window.history.replaceState({}, '', '/');
});

describe('fb-projects-page', () => {
  it('renders the project grid after a successful load', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
    const el = await mount('fb-projects-page');
    await flush(el);
    const cards = el.shadowRoot!.querySelectorAll('a.project-card');
    expect(cards.length).toBe(1);
    expect(cards[0]!.getAttribute('href')).toBe('/portfolio/projects/p-1');
    expect(text(el, '.pc-name')).toBe('Maple Street Duplex');
  });

  it('shows the empty state when there are no projects', async () => {
    const el = await mount('fb-projects-page');
    await flush(el);
    const state = el.shadowRoot!.querySelector('fb-state');
    expect(state?.getAttribute('mode')).toBe('empty');
  });

  it('shows a retryable error state when the load fails', async () => {
    vi.mocked(projectsApi.listProjects).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mount('fb-projects-page');
    await flush(el);
    const state = el.shadowRoot!.querySelector('fb-state');
    expect(state?.getAttribute('mode')).toBe('error');
    expect(state?.getAttribute('error-code')).toBe(ErrorCode.SERVICE_UNAVAILABLE);
  });
});

describe('fb-project-detail-page', () => {
  it('loads the project by its id attribute and renders the overview', async () => {
    const el = await mount('fb-project-detail-page', { id: 'p-1' });
    await flush(el);
    expect(projectsApi.getProject).toHaveBeenCalledWith('p-1');
    expect(text(el, '.page-title')).toBe('Maple Street Duplex');
    // Tabs present; overview is the default and needs no extra fetch.
    expect(el.shadowRoot!.querySelector('fb-tab-bar')).not.toBeNull();
    expect(projectsApi.listProjectTasks).not.toHaveBeenCalled();
    expect(projectsApi.listProjectBudgets).not.toHaveBeenCalled();
  });

  it('lazy-loads budgets only when the Budget tab is selected', async () => {
    const el = await mount('fb-project-detail-page', { id: 'p-1' });
    await flush(el);
    const tabs = el.shadowRoot!.querySelector('fb-tab-bar')!;
    tabs.dispatchEvent(new CustomEvent('change', { detail: { id: 'budget' } }));
    await flush(el);
    expect(projectsApi.listProjectBudgets).toHaveBeenCalledWith('p-1');
  });
});

describe('fb-financials-page', () => {
  it('renders per-currency summary groups from the summary payload', async () => {
    const summary: FinancialsSummary = {
      corporate_budgets: [
        {
          id: 'cb-1',
          org_id: 'org-1',
          fiscal_year: 2026,
          quarter: 1,
          currency_code: 'USD',
          total_estimated_cents: '100000',
          total_committed_cents: '50000',
          total_actual_cents: '25000',
          project_count: 3,
          last_rollup_at: '2026-01-01',
          created_at: '2026-01-01',
          updated_at: '2026-01-01',
        },
      ],
      ar_aging: [],
    };
    vi.mocked(financialsApi.getFinancialsSummary).mockResolvedValue(summary);
    const el = await mount('fb-financials-page');
    await flush(el);
    expect(text(el, '.currency-label')).toContain('USD');
    expect(el.shadowRoot!.querySelectorAll('fb-stat-card').length).toBe(4);
  });

  it('lazy-loads the by-project rollup when that tab is opened', async () => {
    const el = await mount('fb-financials-page');
    await flush(el);
    expect(financialsApi.getProjectFinancials).not.toHaveBeenCalled();
    const tabs = el.shadowRoot!.querySelector('fb-tab-bar')!;
    tabs.dispatchEvent(new CustomEvent('change', { detail: { id: 'projects' } }));
    await flush(el);
    expect(financialsApi.getProjectFinancials).toHaveBeenCalled();
  });
});

describe('fb-fleet-page', () => {
  const ASSET: FleetAsset = {
    id: 'a-1',
    org_id: 'org-1',
    name: 'Excavator 320',
    asset_type: 'Heavy equipment',
    serial_number: 'SN-9',
    status: 'available',
    created_at: '2026-01-01',
  };

  it('renders asset cards with an enabled allocate button when available', async () => {
    vi.mocked(fleetApi.listFleetAssets).mockResolvedValue([ASSET]);
    const el = await mount('fb-fleet-page');
    await flush(el);
    expect(text(el, '.ac-name')).toBe('Excavator 320');
    const btn = el.shadowRoot!.querySelector('.ac-actions fb-button');
    expect(btn).not.toBeNull();
    expect(btn!.hasAttribute('disabled')).toBe(false);
  });

  it('surfaces a 409 overlap as an inline allocate error, not a toast', async () => {
    vi.mocked(fleetApi.listFleetAssets).mockResolvedValue([ASSET]);
    vi.mocked(fleetApi.allocateAsset).mockRejectedValueOnce(apiError(ErrorCode.CONFLICT, 409));
    const el = await mount('fb-fleet-page');
    await flush(el);
    // Open the allocate dialog and drive a submit with all fields set.
    const page = el as unknown as {
      allocating: FleetAsset | null;
      allocProjectId: string;
      allocStart: string;
      allocEnd: string;
      submitAllocate(): Promise<void>;
    };
    page.allocating = ASSET;
    page.allocProjectId = 'p-1';
    page.allocStart = '2026-03-01';
    page.allocEnd = '2026-03-10';
    await page.submitAllocate();
    await flush(el);
    expect(text(el, '.dialog-error')).toContain('already booked');
  });
});

describe('fb-hr-page', () => {
  const EMP: Employee = {
    id: 'e-1',
    org_id: 'org-1',
    first_name: 'Dana',
    last_name: 'Cruz',
    role: 'Foreman',
    phone: '555-0100',
    hire_date: '2025-06-01',
    created_at: '2026-01-01',
  };

  it('renders employee cards', async () => {
    vi.mocked(hrApi.listEmployees).mockResolvedValue([EMP]);
    const el = await mount('fb-hr-page');
    await flush(el);
    expect(text(el, '.emp-name')).toContain('Dana');
    expect(text(el, '.emp-name')).toContain('Cruz');
  });

  it('loads certifications on demand when opening the dialog', async () => {
    const cert: Certification = {
      id: 'c-1',
      employee_id: 'e-1',
      cert_type: 'OSHA 30',
      expiry_date: '2030-01-01',
      status: 'active',
      created_at: '2026-01-01',
    };
    vi.mocked(hrApi.listEmployees).mockResolvedValue([EMP]);
    vi.mocked(hrApi.listCertifications).mockResolvedValue([cert]);
    const el = await mount('fb-hr-page');
    await flush(el);
    await (el as unknown as { openCerts(e: Employee): Promise<void> }).openCerts(EMP);
    await flush(el);
    expect(hrApi.listCertifications).toHaveBeenCalledWith('', 'e-1');
    expect(el.shadowRoot!.querySelector('fb-modal')).not.toBeNull();
    expect(text(el, '.cert-type')).toBe('OSHA 30');
  });
});

describe('fb-pipeline-page', () => {
  const PROSPECT: Prospect = {
    id: 'pr-1',
    org_id: 'org-1',
    name: 'Oak Ridge Reno',
    client_name: 'J. Smith',
    pipeline_stage: 'LEAD',
    probability_pct: 20,
    created_at: '2026-01-01',
    updated_at: '2026-01-01',
  };

  it('renders one Kanban column per board stage', async () => {
    vi.mocked(pipelineApi.listProspects).mockResolvedValue([PROSPECT]);
    const el = await mount('fb-pipeline-page');
    await flush(el);
    const cols = el.shadowRoot!.querySelectorAll('.col');
    expect(cols.length).toBe(6);
    expect(text(el, '.p-name')).toBe('Oak Ridge Reno');
  });

  it('renders per-currency analytics stat cards', async () => {
    const analytics: PipelineAnalyticsRow[] = [
      {
        currency_code: 'USD',
        total_estimated_cents: '500000',
        weighted_revenue_cents: '100000',
        prospect_count: 4,
      },
    ];
    vi.mocked(pipelineApi.getPipelineAnalytics).mockResolvedValue(analytics);
    const el = await mount('fb-pipeline-page');
    await flush(el);
    expect(text(el, '.currency-label')).toContain('USD');
    expect(el.shadowRoot!.querySelectorAll('.stat-row fb-stat-card').length).toBe(3);
  });

  it('opens the detail dialog on prospect click', async () => {
    const detail: ProspectWithDetails = {
      prospect: PROSPECT,
      estimates: [],
      permits: [],
    };
    vi.mocked(pipelineApi.listProspects).mockResolvedValue([PROSPECT]);
    vi.mocked(pipelineApi.getProspect).mockResolvedValue(detail);
    const el = await mount('fb-pipeline-page');
    await flush(el);
    el.shadowRoot!.querySelector<HTMLButtonElement>('button.prospect')!.click();
    await flush(el);
    expect(pipelineApi.getProspect).toHaveBeenCalledWith('', 'pr-1');
    expect(el.shadowRoot!.querySelector('fb-modal')).not.toBeNull();
  });
});
