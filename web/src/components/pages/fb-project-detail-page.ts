import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../molecules/fb-tab-bar.js';
import '../molecules/fb-breadcrumb.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-chip.js';
import '../organisms/fb-data-table.js';
import '../organisms/fb-state.js';
import { getProject, listProjectTasks, listProjectBudgets } from '../../api/endpoints/projects.js';
import type { Project, ProjectTask, ProjectBudget } from '../../types/models.js';
import { ApiError } from '../../api/errors.js';
import { navigate } from '../../router.js';
import type { Column, Row } from '../organisms/fb-data-table.js';
import type { BadgeStatus } from '../atoms/fb-badge.js';
import type { Tab } from '../molecules/fb-tab-bar.js';

type TabId = 'overview' | 'budget' | 'schedule' | 'team';

const TABS: Tab[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'budget', label: 'Budget' },
  { id: 'schedule', label: 'Schedule' },
  { id: 'team', label: 'Team' },
];

const BUDGET_COLUMNS: Column[] = [
  { key: 'phase_name', label: 'Phase' },
  { key: 'wbs_code', label: 'WBS' },
  {
    key: 'estimated_cost_cents',
    label: 'Estimated',
    type: 'money',
    currencyKey: 'estimated_cost_currency_code',
  },
  {
    key: 'committed_cost_cents',
    label: 'Committed',
    type: 'money',
    currencyKey: 'committed_cost_currency_code',
  },
  {
    key: 'actual_cost_cents',
    label: 'Actual',
    type: 'money',
    currencyKey: 'actual_cost_currency_code',
  },
];

const SCHEDULE_COLUMNS: Column[] = [
  { key: 'wbs_code', label: 'WBS' },
  { key: 'name', label: 'Task' },
  { key: 'duration_days', label: 'Days', type: 'number' },
  { key: 'flag', label: 'Critical path' },
  { key: 'status', label: 'Status', type: 'status' },
  { key: 'percent', label: 'Complete', align: 'right' },
];

function taskStatusBadge(status: string): BadgeStatus {
  switch (status) {
    case 'complete':
    case 'completed':
      return 'complete';
    case 'in_progress':
      return 'active';
    case 'blocked':
      return 'critical';
    default:
      return 'pending';
  }
}

/**
 * `fb-project-detail-page` — the per-project workspace (UX_CORE_SCREENS §1).
 * Tabs lazy-load their data: Overview is metadata only; Budget hits
 * `/budgets`, Schedule hits `/tasks`, Team derives crew from the loaded tasks.
 * Budget money is rendered per-phase with each row's own currency — there is no
 * cross-phase total because phases can carry different currencies.
 */
@customElement('fb-project-detail-page')
export class FbProjectDetailPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .tabpanel {
        padding-top: var(--fb-spacing-md);
      }
      dl.meta {
        display: grid;
        grid-template-columns: max-content 1fr;
        gap: var(--fb-spacing-sm) var(--fb-spacing-lg);
        margin: 0;
      }
      dl.meta dt {
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      dl.meta dd {
        margin: 0;
        color: var(--fb-text-primary);
      }
      .crew {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
      }
    `,
  ];

  /** Route param (`/portfolio/projects/:id`) — set as the `id` attribute by fb-app. */
  @property({ type: String, attribute: 'id' }) projectId = '';

  @state() private tab: TabId = 'overview';
  @state() private project: Project | null = null;
  @state() private tasks: ProjectTask[] | null = null;
  @state() private budgets: ProjectBudget[] | null = null;
  @state() private loading = true;
  @state() private errorCode: string | null = null;
  /** Per-tab in-flight/error tracking for the lazy data tabs. */
  @state() private tabLoading = false;
  @state() private tabError: string | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.loadProject();
  }

  private async loadProject(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.project = await getProject(this.projectId);
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private onTab(e: Event): void {
    const id = (e as CustomEvent<{ id: string }>).detail.id as TabId;
    this.tab = id;
    if (id === 'budget' && this.budgets === null) void this.loadBudgets();
    if ((id === 'schedule' || id === 'team') && this.tasks === null) void this.loadTasks();
  }

  private async loadTasks(): Promise<void> {
    this.tabLoading = true;
    this.tabError = null;
    try {
      this.tasks = await listProjectTasks(this.projectId);
    } catch (err) {
      this.tabError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.tabLoading = false;
    }
  }

  private async loadBudgets(): Promise<void> {
    this.tabLoading = true;
    this.tabError = null;
    try {
      this.budgets = await listProjectBudgets(this.projectId);
    } catch (err) {
      this.tabError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.tabLoading = false;
    }
  }

  private scheduleRows(): Row[] {
    return (this.tasks ?? []).map((t) => ({
      id: t.id,
      wbs_code: t.wbs_code,
      name: t.name,
      duration_days: t.duration_days,
      flag: t.is_critical ? 'Critical path' : '',
      status: taskStatusBadge(t.status),
      percent: `${t.percent_complete}%`,
    }));
  }

  private crewMembers(): string[] {
    const set = new Set<string>();
    for (const t of this.tasks ?? []) for (const c of t.assigned_crew ?? []) set.add(c);
    return [...set].sort();
  }

  private renderTabBody(): TemplateResult {
    if (this.tab === 'overview') return this.renderOverview();
    if (this.tabLoading)
      return html`<fb-state mode="loading" skeleton="table" rows="5"></fb-state>`;
    if (this.tabError)
      return html`<fb-state
        mode="error"
        error-code=${this.tabError}
        retryable
        @retry=${() => (this.tab === 'budget' ? void this.loadBudgets() : void this.loadTasks())}
      ></fb-state>`;
    if (this.tab === 'budget') return this.renderBudget();
    if (this.tab === 'schedule') return this.renderSchedule();
    return this.renderTeam();
  }

  private renderOverview(): TemplateResult {
    const p = this.project!;
    return html`<dl class="meta">
      <dt>Status</dt>
      <dd>${p.status}</dd>
      ${p.address
        ? html`<dt>Address</dt>
            <dd>${p.address}</dd>`
        : nothing}
      ${p.gsf
        ? html`<dt>Gross sq. ft.</dt>
            <dd>${p.gsf.toLocaleString()}</dd>`
        : nothing}
      ${p.permit_issued_date
        ? html`<dt>Permit issued</dt>
            <dd>${p.permit_issued_date}</dd>`
        : nothing}
      ${p.project_start_date
        ? html`<dt>Start date</dt>
            <dd>${p.project_start_date}</dd>`
        : nothing}
    </dl>`;
  }

  private renderBudget(): TemplateResult {
    if ((this.budgets ?? []).length === 0)
      return html`<fb-state
        mode="empty"
        icon="dollar"
        heading="No budget phases"
        message="Budget phases will appear here once they’re added."
      ></fb-state>`;
    return html`<fb-data-table
      caption="Budget by phase"
      .columns=${BUDGET_COLUMNS}
      .rows=${this.budgets as unknown as Row[]}
    ></fb-data-table>`;
  }

  private renderSchedule(): TemplateResult {
    if ((this.tasks ?? []).length === 0)
      return html`<fb-state
        mode="empty"
        icon="calendar"
        heading="No tasks scheduled"
        message="Schedule tasks will appear here once the project is planned."
      ></fb-state>`;
    return html`<fb-data-table
      caption="Project schedule"
      .columns=${SCHEDULE_COLUMNS}
      .rows=${this.scheduleRows()}
    ></fb-data-table>`;
  }

  private renderTeam(): TemplateResult {
    const crew = this.crewMembers();
    if (crew.length === 0)
      return html`<fb-state
        mode="empty"
        icon="users"
        heading="No crew assigned"
        message="Crew assignments come from scheduled tasks."
      ></fb-state>`;
    return html`<div class="crew">${crew.map((c) => html`<fb-chip>${c}</fb-chip>`)}</div>`;
  }

  override render(): TemplateResult {
    if (this.loading) return html`<fb-state mode="loading" skeleton="text" rows="4"></fb-state>`;
    if (this.errorCode)
      return html`<fb-state
        mode="error"
        error-code=${this.errorCode}
        retryable
        @retry=${() => void this.loadProject()}
      ></fb-state>`;

    const p = this.project!;
    return html`
      <div class="page">
        <fb-breadcrumb
          .crumbs=${[{ label: 'Projects', href: '/portfolio/projects' }, { label: p.name }]}
          @navigate=${(e: Event) => navigate((e as CustomEvent<{ href: string }>).detail.href)}
        ></fb-breadcrumb>
        <div class="page-head">
          <div>
            <h1 class="page-title">${p.name}</h1>
            ${p.address ? html`<p class="page-sub">${p.address}</p>` : nothing}
          </div>
        </div>
        <fb-tab-bar
          label="Project sections"
          .tabs=${TABS}
          active=${this.tab}
          @change=${this.onTab}
        ></fb-tab-bar>
        <div class="tabpanel" role="tabpanel">${this.renderTabBody()}</div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-project-detail-page': FbProjectDetailPage;
  }
}
