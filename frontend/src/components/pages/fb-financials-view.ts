import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  getFinancialSummary, getProjectFinancials,
  type FinancialSummary, type ProjectFinancial, ApiError,
} from '../../state/api.js';
import { currentOrg, currentCurrency } from '../../state/store.js';
import { formatCents, formatCentsCompact, type CurrencyCode } from '../../utils/currency.js';

type CurrencyFilter = 'USD' | 'CAD' | 'ALL';

@customElement('fb-financials-view')
export class FBFinancialsView extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _currency: CurrencyFilter = 'ALL';
  @state() private _summary: FinancialSummary | null = null;
  @state() private _projects: ProjectFinancial[] = [];
  @state() private _sortField: keyof ProjectFinancial = 'project_name';
  @state() private _sortDir: 'asc' | 'desc' = 'asc';

  private _unsubCurrency?: () => void;

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host { display: block; padding: var(--fb-space-6); }

      .page-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--fb-space-6);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .currency-tabs {
        display: flex;
        gap: var(--fb-space-1);
        background: var(--fb-glass-bg);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-1);
      }

      .currency-tab {
        padding: var(--fb-space-2) var(--fb-space-4);
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: none;
        background: transparent;
        font-family: var(--fb-font-mono);
      }

      .currency-tab:hover { color: var(--fb-text-primary); }

      .currency-tab.active {
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        font-weight: 600;
      }

      .budget-cards {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: var(--fb-space-4);
        margin-bottom: var(--fb-space-6);
      }

      @media (max-width: 768px) {
        .budget-cards { grid-template-columns: 1fr; }
      }

      .budget-card {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
      }

      .budget-card-label {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-bottom: var(--fb-space-2);
      }

      .budget-card-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-3xl);
        font-weight: 700;
        color: var(--fb-gable-green);
        font-variant-numeric: tabular-nums;
      }

      .budget-card-sub {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        margin-top: var(--fb-space-1);
      }

      .budget-card.committed .budget-card-value { color: var(--fb-blueprint-blue); }
      .budget-card.actual .budget-card-value { color: var(--fb-amber); }

      .sections-grid {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: var(--fb-space-6);
        margin-bottom: var(--fb-space-6);
      }

      @media (max-width: 1024px) {
        .sections-grid { grid-template-columns: 1fr; }
      }

      .section {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
      }

      .section-title {
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
      }

      .aging-grid {
        display: grid;
        grid-template-columns: repeat(5, 1fr);
        gap: var(--fb-space-3);
      }

      .aging-bucket {
        text-align: center;
      }

      .aging-label {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        margin-bottom: var(--fb-space-2);
      }

      .aging-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
        font-variant-numeric: tabular-nums;
      }

      .aging-bar {
        height: 4px;
        border-radius: 2px;
        background: var(--fb-surface);
        margin-top: var(--fb-space-2);
        overflow: hidden;
      }

      .aging-bar-fill {
        height: 100%;
        border-radius: 2px;
        transition: width var(--fb-transition-normal);
      }

      .aging-bar-fill.current { background: var(--fb-gable-green); }
      .aging-bar-fill.d30 { background: var(--fb-blueprint-blue); }
      .aging-bar-fill.d60 { background: var(--fb-amber); }
      .aging-bar-fill.d90 { background: #F97316; }
      .aging-bar-fill.over90 { background: var(--fb-safety-red); }

      /* Table */
      .table-container {
        overflow-x: auto;
      }

      table {
        width: 100%;
        border-collapse: collapse;
      }

      th {
        text-align: left;
        padding: var(--fb-space-3) var(--fb-space-4);
        font-size: var(--fb-text-xs);
        font-weight: 600;
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.05em;
        border-bottom: 1px solid var(--fb-border);
        cursor: pointer;
        user-select: none;
        white-space: nowrap;
      }

      th:hover { color: var(--fb-text-secondary); }
      th.sorted { color: var(--fb-gable-green); }

      td {
        padding: var(--fb-space-3) var(--fb-space-4);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-primary);
        border-bottom: 1px solid var(--fb-border);
        white-space: nowrap;
      }

      td.mono {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }

      td.positive { color: var(--fb-gable-green); }
      td.negative { color: var(--fb-safety-red); }

      tr:hover td { background: rgba(255, 255, 255, 0.02); }

      .error-banner {
        color: var(--fb-safety-red);
        padding: var(--fb-space-4);
        background: rgba(244, 63, 94, 0.1);
        border-radius: var(--fb-radius-md);
        border: 1px solid rgba(244, 63, 94, 0.2);
        margin-bottom: var(--fb-space-4);
        font-size: var(--fb-text-sm);
      }

      .loading-container {
        display: flex;
        align-items: center;
        justify-content: center;
        min-height: 300px;
      }

      .empty-state {
        text-align: center;
        padding: var(--fb-space-8);
        color: var(--fb-text-muted);
        font-size: var(--fb-text-sm);
      }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._currency = currentCurrency.get();
    this._unsubCurrency = currentCurrency.subscribe(() => {
      this._currency = currentCurrency.get();
      this._loadData();
    });
    this._loadData();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsubCurrency?.();
  }

  render() {
    if (this._loading) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    const budgets = this._summary?.corporate_budgets ?? [];
    const aging = this._summary?.ar_aging ?? [];

    // Aggregate budget totals by currency (or all)
    const totalEstimated = this._aggregateBudgetField(budgets, 'total_estimated_cents');
    const totalCommitted = this._aggregateBudgetField(budgets, 'total_committed_cents');
    const totalActual = this._aggregateBudgetField(budgets, 'total_actual_cents');

    const displayCurrency: CurrencyCode = this._currency === 'ALL' ? 'USD' : this._currency;

    return html`
      <div class="page-header">
        <h1>Financials</h1>
        <div class="currency-tabs">
          ${(['ALL', 'USD', 'CAD'] as const).map(
            (c) => html`
              <button
                class="currency-tab ${this._currency === c ? 'active' : ''}"
                @click=${() => this._setCurrency(c)}
              >${c}</button>
            `,
          )}
        </div>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      <div class="budget-cards">
        <div class="budget-card">
          <div class="budget-card-label">Total Estimated</div>
          <div class="budget-card-value">${this._currency === 'ALL' ? formatCentsCompact(totalEstimated, displayCurrency) : formatCents(totalEstimated, displayCurrency)}</div>
          <div class="budget-card-sub">${budgets.length} budget${budgets.length !== 1 ? 's' : ''}</div>
        </div>
        <div class="budget-card committed">
          <div class="budget-card-label">Total Committed</div>
          <div class="budget-card-value">${this._currency === 'ALL' ? formatCentsCompact(totalCommitted, displayCurrency) : formatCents(totalCommitted, displayCurrency)}</div>
          <div class="budget-card-sub">${totalEstimated > 0 ? Math.round((totalCommitted / totalEstimated) * 100) : 0}% of estimated</div>
        </div>
        <div class="budget-card actual">
          <div class="budget-card-label">Total Actual</div>
          <div class="budget-card-value">${this._currency === 'ALL' ? formatCentsCompact(totalActual, displayCurrency) : formatCents(totalActual, displayCurrency)}</div>
          <div class="budget-card-sub">${totalCommitted > 0 ? Math.round((totalActual / totalCommitted) * 100) : 0}% of committed</div>
        </div>
      </div>

      <div class="sections-grid">
        <div class="section">
          <h2 class="section-title">AR Aging</h2>
          ${aging.length === 0
            ? html`<div class="empty-state">No AR aging data available.</div>`
            : aging.map((snap) => this._renderAgingSnapshot(snap))}
        </div>
        <div class="section">
          <h2 class="section-title">Budget Breakdown</h2>
          ${budgets.length === 0
            ? html`<div class="empty-state">No budgets configured.</div>`
            : html`
              <div style="display: flex; flex-direction: column; gap: var(--fb-space-3);">
                ${budgets.map(
                  (b) => html`
                    <div style="display: flex; justify-content: space-between; align-items: center; padding: var(--fb-space-2) 0; border-bottom: 1px solid var(--fb-border);">
                      <span style="font-size: var(--fb-text-sm); color: var(--fb-text-primary);">Q${b.quarter} ${b.fiscal_year} (${b.currency_code})</span>
                      <span style="font-family: var(--fb-font-mono); font-size: var(--fb-text-sm); color: var(--fb-gable-green);">${formatCentsCompact(b.total_estimated_cents, b.currency_code as CurrencyCode)}</span>
                    </div>
                  `,
                )}
              </div>
            `}
        </div>
      </div>

      <div class="section" style="margin-bottom: var(--fb-space-6);">
        <h2 class="section-title">Project Financials</h2>
        ${this._projects.length === 0
          ? html`<div class="empty-state">No project financials available.</div>`
          : html`
            <div class="table-container">
              <table>
                <thead>
                  <tr>
                    <th class="${this._sortField === 'project_name' ? 'sorted' : ''}" @click=${() => this._toggleSort('project_name')}>
                      Project ${this._sortField === 'project_name' ? (this._sortDir === 'asc' ? '\u25B2' : '\u25BC') : ''}
                    </th>
                    <th>Currency</th>
                    <th class="${this._sortField === 'estimated_cost_cents' ? 'sorted' : ''}" @click=${() => this._toggleSort('estimated_cost_cents')}>
                      Estimated ${this._sortField === 'estimated_cost_cents' ? (this._sortDir === 'asc' ? '\u25B2' : '\u25BC') : ''}
                    </th>
                    <th class="${this._sortField === 'committed_cost_cents' ? 'sorted' : ''}" @click=${() => this._toggleSort('committed_cost_cents')}>
                      Committed ${this._sortField === 'committed_cost_cents' ? (this._sortDir === 'asc' ? '\u25B2' : '\u25BC') : ''}
                    </th>
                    <th class="${this._sortField === 'actual_cost_cents' ? 'sorted' : ''}" @click=${() => this._toggleSort('actual_cost_cents')}>
                      Actual ${this._sortField === 'actual_cost_cents' ? (this._sortDir === 'asc' ? '\u25B2' : '\u25BC') : ''}
                    </th>
                    <th class="${this._sortField === 'variance_cents' ? 'sorted' : ''}" @click=${() => this._toggleSort('variance_cents')}>
                      Variance ${this._sortField === 'variance_cents' ? (this._sortDir === 'asc' ? '\u25B2' : '\u25BC') : ''}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  ${this._getSortedProjects().map(
                    (p) => html`
                      <tr>
                        <td>${p.project_name}</td>
                        <td class="mono">${p.currency_code}</td>
                        <td class="mono">${formatCents(p.estimated_cost_cents, p.currency_code as CurrencyCode)}</td>
                        <td class="mono">${formatCents(p.committed_cost_cents, p.currency_code as CurrencyCode)}</td>
                        <td class="mono">${formatCents(p.actual_cost_cents, p.currency_code as CurrencyCode)}</td>
                        <td class="mono ${p.variance_cents >= 0 ? 'positive' : 'negative'}">${formatCents(p.variance_cents, p.currency_code as CurrencyCode)}</td>
                      </tr>
                    `,
                  )}
                </tbody>
              </table>
            </div>
          `}
      </div>
    `;
  }

  private _renderAgingSnapshot(snap: { currency_code: string; current_cents: number; days_30_cents: number; days_60_cents: number; days_90_cents: number; over_90_cents: number; total_cents: number }) {
    const total = snap.total_cents || 1;
    const cc = snap.currency_code as CurrencyCode;
    return html`
      <div style="margin-bottom: var(--fb-space-4);">
        <div style="font-size: var(--fb-text-xs); color: var(--fb-text-muted); margin-bottom: var(--fb-space-2);">${snap.currency_code}</div>
        <div class="aging-grid">
          <div class="aging-bucket">
            <div class="aging-label">Current</div>
            <div class="aging-value">${formatCentsCompact(snap.current_cents, cc)}</div>
            <div class="aging-bar"><div class="aging-bar-fill current" style="width: ${Math.round((snap.current_cents / total) * 100)}%"></div></div>
          </div>
          <div class="aging-bucket">
            <div class="aging-label">30 Days</div>
            <div class="aging-value">${formatCentsCompact(snap.days_30_cents, cc)}</div>
            <div class="aging-bar"><div class="aging-bar-fill d30" style="width: ${Math.round((snap.days_30_cents / total) * 100)}%"></div></div>
          </div>
          <div class="aging-bucket">
            <div class="aging-label">60 Days</div>
            <div class="aging-value">${formatCentsCompact(snap.days_60_cents, cc)}</div>
            <div class="aging-bar"><div class="aging-bar-fill d60" style="width: ${Math.round((snap.days_60_cents / total) * 100)}%"></div></div>
          </div>
          <div class="aging-bucket">
            <div class="aging-label">90 Days</div>
            <div class="aging-value">${formatCentsCompact(snap.days_90_cents, cc)}</div>
            <div class="aging-bar"><div class="aging-bar-fill d90" style="width: ${Math.round((snap.days_90_cents / total) * 100)}%"></div></div>
          </div>
          <div class="aging-bucket">
            <div class="aging-label">90+ Days</div>
            <div class="aging-value">${formatCentsCompact(snap.over_90_cents, cc)}</div>
            <div class="aging-bar"><div class="aging-bar-fill over90" style="width: ${Math.round((snap.over_90_cents / total) * 100)}%"></div></div>
          </div>
        </div>
      </div>
    `;
  }

  private _aggregateBudgetField(budgets: FinancialSummary['corporate_budgets'], field: 'total_estimated_cents' | 'total_committed_cents' | 'total_actual_cents'): number {
    if (this._currency === 'ALL') {
      // Sum all (display as USD for simplicity in compact view)
      return budgets.reduce((sum, b) => sum + b[field], 0);
    }
    return budgets.filter((b) => b.currency_code === this._currency).reduce((sum, b) => sum + b[field], 0);
  }

  private _setCurrency(currency: CurrencyFilter) {
    this._currency = currency;
    currentCurrency.set(currency);
  }

  private _toggleSort(field: keyof ProjectFinancial) {
    if (this._sortField === field) {
      this._sortDir = this._sortDir === 'asc' ? 'desc' : 'asc';
    } else {
      this._sortField = field;
      this._sortDir = 'asc';
    }
  }

  private _getSortedProjects(): ProjectFinancial[] {
    const sorted = [...this._projects];
    sorted.sort((a, b) => {
      const aVal = a[this._sortField];
      const bVal = b[this._sortField];
      if (typeof aVal === 'string' && typeof bVal === 'string') {
        return this._sortDir === 'asc' ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
      }
      if (typeof aVal === 'number' && typeof bVal === 'number') {
        return this._sortDir === 'asc' ? aVal - bVal : bVal - aVal;
      }
      return 0;
    });
    return sorted;
  }

  private async _loadData() {
    this._loading = true;
    this._error = '';
    const orgID = currentOrg.get();
    if (!orgID) {
      this._loading = false;
      this._error = 'No organization selected.';
      return;
    }

    try {
      const currencyParam = this._currency === 'ALL' ? undefined : this._currency;
      const [summaryRes, projectsRes] = await Promise.all([
        getFinancialSummary(orgID, currencyParam),
        getProjectFinancials(orgID, currencyParam),
      ]);
      this._summary = summaryRes;
      this._projects = projectsRes.projects;
    } catch (err) {
      if (err instanceof ApiError) {
        this._error = `Failed to load financials (${err.status})`;
      } else {
        this._error = 'Failed to load financial data';
      }
      this.showToast(this._error, 'error');
    } finally {
      this._loading = false;
    }
  }
}
