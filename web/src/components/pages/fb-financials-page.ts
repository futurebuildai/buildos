import { html, css, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../molecules/fb-tab-bar.js';
import '../molecules/fb-stat-card.js';
import '../atoms/fb-money.js';
import '../organisms/fb-data-table.js';
import '../organisms/fb-state.js';
import { getFinancialsSummary, getProjectFinancials } from '../../api/endpoints/financials.js';
import type { FinancialsSummary, ProjectFinancial } from '../../types/models.js';
import { ApiError } from '../../api/errors.js';
import { authClaims } from '../../state/authStore.js';
import type { Column, Row } from '../organisms/fb-data-table.js';
import type { Tab } from '../molecules/fb-tab-bar.js';

type TabId = 'summary' | 'aging' | 'projects';

const TABS: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'aging', label: 'AR Aging' },
  { id: 'projects', label: 'By Project' },
];

const AGING_COLUMNS: Column[] = [
  { key: 'currency_code', label: 'Currency' },
  { key: 'snapshot_date', label: 'As of', type: 'date' },
  { key: 'current_cents', label: 'Current', type: 'money' },
  { key: 'days_30_cents', label: '1–30 days', type: 'money' },
  { key: 'days_60_cents', label: '31–60 days', type: 'money' },
  { key: 'days_90_plus_cents', label: '60+ days', type: 'money' },
  { key: 'total_receivable_cents', label: 'Total', type: 'money' },
];

const PROJECT_COLUMNS: Column[] = [
  { key: 'project_name', label: 'Project' },
  { key: 'currency_code', label: 'Currency' },
  { key: 'total_estimated_cents', label: 'Estimated', type: 'money' },
  { key: 'total_committed_cents', label: 'Committed', type: 'money' },
  { key: 'total_actual_cents', label: 'Actual', type: 'money' },
  { key: 'phase_count', label: 'Phases', type: 'number' },
];

/**
 * `fb-financials-page` — corporate financials (UX_CORE_SCREENS §4). Read is
 * superintendent+ (router-gated). Every figure is grouped strictly by
 * `currency_code`: the backend never sums across currencies and neither does
 * this page (no grand total row anywhere). Summary + AR-aging arrive together
 * from `/financials/summary`; the By-Project rollup loads on demand.
 */
@customElement('fb-financials-page')
export class FbFinancialsPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .tabpanel {
        padding-top: var(--fb-spacing-md);
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-lg);
      }
    `,
  ];

  @state() private tab: TabId = 'summary';
  @state() private summary: FinancialsSummary | null = null;
  @state() private projects: ProjectFinancial[] | null = null;
  @state() private loading = true;
  @state() private errorCode: string | null = null;
  @state() private tabLoading = false;
  @state() private tabError: string | null = null;

  private get orgId(): string {
    return authClaims.get()?.orgId ?? '';
  }

  override connectedCallback(): void {
    super.connectedCallback();
    void this.loadSummary();
  }

  private async loadSummary(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.summary = await getFinancialsSummary(this.orgId);
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private async loadProjects(): Promise<void> {
    this.tabLoading = true;
    this.tabError = null;
    try {
      this.projects = await getProjectFinancials(this.orgId);
    } catch (err) {
      this.tabError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.tabLoading = false;
    }
  }

  private onTab(e: Event): void {
    const id = (e as CustomEvent<{ id: string }>).detail.id as TabId;
    this.tab = id;
    if (id === 'projects' && this.projects === null) void this.loadProjects();
  }

  private renderSummary(): TemplateResult {
    const budgets = this.summary?.corporate_budgets ?? [];
    if (budgets.length === 0)
      return html`<fb-state
        mode="empty"
        icon="dollar"
        heading="No budget rollups yet"
        message="Corporate budget figures appear here after the first rollup runs."
      ></fb-state>`;
    return html`<div class="summary-groups">
      ${budgets.map(
        (b) => html`
          <section class="currency-group">
            <span class="currency-label"
              ><fb-icon name="dollar" size="14"></fb-icon>${b.currency_code} · FY${b.fiscal_year}
              Q${b.quarter}</span
            >
            <div class="stat-row">
              <fb-stat-card heading="Estimated"
                ><fb-money
                  cents=${b.total_estimated_cents}
                  currency-code=${b.currency_code}
                ></fb-money
              ></fb-stat-card>
              <fb-stat-card heading="Committed"
                ><fb-money
                  cents=${b.total_committed_cents}
                  currency-code=${b.currency_code}
                ></fb-money
              ></fb-stat-card>
              <fb-stat-card heading="Actual"
                ><fb-money cents=${b.total_actual_cents} currency-code=${b.currency_code}></fb-money
              ></fb-stat-card>
              <fb-stat-card heading="Projects">${b.project_count}</fb-stat-card>
            </div>
          </section>
        `,
      )}
    </div>`;
  }

  private renderAging(): TemplateResult {
    const rows = (this.summary?.ar_aging ?? []) as unknown as Row[];
    if (rows.length === 0)
      return html`<fb-state
        mode="empty"
        icon="clock"
        heading="No receivables snapshot"
        message="AR aging appears once invoices are outstanding."
      ></fb-state>`;
    return html`<fb-data-table
      caption="Accounts receivable aging by currency"
      .columns=${AGING_COLUMNS}
      .rows=${rows}
    ></fb-data-table>`;
  }

  private renderProjects(): TemplateResult {
    if (this.tabLoading)
      return html`<fb-state mode="loading" skeleton="table" rows="5"></fb-state>`;
    if (this.tabError)
      return html`<fb-state
        mode="error"
        error-code=${this.tabError}
        retryable
        @retry=${() => void this.loadProjects()}
      ></fb-state>`;
    const rows = (this.projects ?? []) as unknown as Row[];
    if (rows.length === 0)
      return html`<fb-state
        mode="empty"
        icon="folder"
        heading="No project financials"
        message="Per-project cost rollups appear here once budgets are entered."
      ></fb-state>`;
    return html`<fb-data-table
      caption="Financials by project and currency"
      .columns=${PROJECT_COLUMNS}
      .rows=${rows}
      row-key="project_id"
    ></fb-data-table>`;
  }

  private renderBody(): TemplateResult {
    if (this.tab === 'summary') return this.renderSummary();
    if (this.tab === 'aging') return this.renderAging();
    return this.renderProjects();
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Financials</h1>
            <p class="page-sub">Budget, receivables, and cost rollups — grouped by currency.</p>
          </div>
        </div>

        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="3"></fb-state>`
          : this.errorCode
            ? html`<fb-state
                mode="error"
                error-code=${this.errorCode}
                retryable
                @retry=${() => void this.loadSummary()}
              ></fb-state>`
            : html`<fb-tab-bar
                  label="Financial views"
                  .tabs=${TABS}
                  active=${this.tab}
                  @change=${this.onTab}
                ></fb-tab-bar>
                <div class="tabpanel" role="tabpanel">${this.renderBody()}</div>`}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-financials-page': FbFinancialsPage;
  }
}
