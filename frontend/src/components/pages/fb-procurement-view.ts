import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  listProjects, listProcurement,
  type Project, type ProcurementItem, type ProcurementStatus, ApiError,
} from '../../state/api.js';
import { currentProject } from '../../state/store.js';
import { formatCents, type CurrencyCode } from '../../utils/currency.js';

type StatusFilter = ProcurementStatus | 'ALL';

@customElement('fb-procurement-view')
export class FBProcurementView extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _projects: Project[] = [];
  @state() private _selectedProject = '';
  @state() private _items: ProcurementItem[] = [];
  @state() private _filter: StatusFilter = 'ALL';

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host { display: block; padding: var(--fb-space-6); }

      .page-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--fb-space-6);
        flex-wrap: wrap;
        gap: var(--fb-space-4);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .controls {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
      }

      .project-select {
        background: var(--fb-surface);
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-2) var(--fb-space-4);
        color: var(--fb-text-primary);
        font-size: var(--fb-text-sm);
        font-family: var(--fb-font-body);
        min-width: 200px;
        cursor: pointer;
      }

      .project-select:focus { outline: none; border-color: var(--fb-gable-green); }
      .project-select option { background: var(--fb-surface); color: var(--fb-text-primary); }

      .filter-chips {
        display: flex;
        gap: var(--fb-space-2);
        margin-bottom: var(--fb-space-4);
      }

      .chip {
        padding: var(--fb-space-2) var(--fb-space-3);
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: 1px solid var(--fb-border);
        background: transparent;
        font-family: var(--fb-font-body);
      }

      .chip:hover { color: var(--fb-text-primary); border-color: var(--fb-border-hover); }
      .chip.active { background: var(--fb-gable-green); color: var(--fb-deep-space); border-color: var(--fb-gable-green); font-weight: 600; }

      .summary-cards {
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        gap: var(--fb-space-4);
        margin-bottom: var(--fb-space-6);
      }

      @media (max-width: 768px) { .summary-cards { grid-template-columns: repeat(2, 1fr); } }

      .summary-card {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-4);
      }

      .summary-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        font-variant-numeric: tabular-nums;
      }

      .summary-value.ok { color: var(--fb-gable-green); }
      .summary-value.warning { color: var(--fb-amber); }
      .summary-value.critical { color: var(--fb-safety-red); }
      .summary-value.ordered { color: var(--fb-blueprint-blue); }

      .summary-label {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-top: var(--fb-space-1);
      }

      .table-container {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        overflow-x: auto;
      }

      table { width: 100%; border-collapse: collapse; }

      th {
        text-align: left;
        padding: var(--fb-space-3) var(--fb-space-4);
        font-size: var(--fb-text-xs);
        font-weight: 600;
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.05em;
        border-bottom: 1px solid var(--fb-border);
        white-space: nowrap;
      }

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

      tr:hover td { background: rgba(255, 255, 255, 0.02); }
      tr.row-warning td { background: rgba(245, 158, 11, 0.05); }
      tr.row-critical td { background: rgba(244, 63, 94, 0.05); }

      .status-badge {
        display: inline-block;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
      }

      .status-badge.OK { background: rgba(0, 255, 163, 0.1); color: var(--fb-gable-green); }
      .status-badge.WARNING { background: rgba(245, 158, 11, 0.1); color: var(--fb-amber); }
      .status-badge.CRITICAL { background: rgba(244, 63, 94, 0.1); color: var(--fb-safety-red); }
      .status-badge.ORDERED { background: rgba(56, 189, 248, 0.1); color: var(--fb-blueprint-blue); }

      .error-banner {
        color: var(--fb-safety-red);
        padding: var(--fb-space-4);
        background: rgba(244, 63, 94, 0.1);
        border-radius: var(--fb-radius-md);
        border: 1px solid rgba(244, 63, 94, 0.2);
        margin-bottom: var(--fb-space-4);
        font-size: var(--fb-text-sm);
      }

      .loading-container { display: flex; align-items: center; justify-content: center; min-height: 300px; }
      .empty-state { text-align: center; padding: var(--fb-space-8); color: var(--fb-text-muted); font-size: var(--fb-text-sm); }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._loadProjects();
  }

  render() {
    if (this._loading && this._projects.length === 0) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    const filtered = this._getFilteredItems();
    const okCount = this._items.filter((i) => i.status === 'OK').length;
    const warnCount = this._items.filter((i) => i.status === 'WARNING').length;
    const critCount = this._items.filter((i) => i.status === 'CRITICAL').length;
    const orderedCount = this._items.filter((i) => i.status === 'ORDERED').length;

    return html`
      <div class="page-header">
        <h1>Procurement</h1>
        <div class="controls">
          <select class="project-select" @change=${this._onProjectChange} .value=${this._selectedProject}>
            <option value="">Select a project...</option>
            ${this._projects.map((p) => html`<option value=${p.id}>${p.name}</option>`)}
          </select>
        </div>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      <div class="summary-cards">
        <div class="summary-card">
          <div class="summary-value ok">${okCount}</div>
          <div class="summary-label">On Track</div>
        </div>
        <div class="summary-card">
          <div class="summary-value warning">${warnCount}</div>
          <div class="summary-label">Warning</div>
        </div>
        <div class="summary-card">
          <div class="summary-value critical">${critCount}</div>
          <div class="summary-label">Critical</div>
        </div>
        <div class="summary-card">
          <div class="summary-value ordered">${orderedCount}</div>
          <div class="summary-label">Ordered</div>
        </div>
      </div>

      <div class="filter-chips">
        ${(['ALL', 'OK', 'WARNING', 'CRITICAL', 'ORDERED'] as const).map(
          (f) => html`
            <button class="chip ${this._filter === f ? 'active' : ''}" @click=${() => { this._filter = f; }}>
              ${f}
            </button>
          `,
        )}
      </div>

      <div class="table-container">
        ${filtered.length === 0
          ? html`<div class="empty-state">${this._selectedProject ? 'No procurement items match the filter.' : 'Select a project to view procurement items.'}</div>`
          : html`
            <table>
              <thead>
                <tr>
                  <th>WBS</th>
                  <th>Item</th>
                  <th>Est. Cost</th>
                  <th>Lead Time</th>
                  <th>Buffer</th>
                  <th>Need By</th>
                  <th>Order By</th>
                  <th>PO #</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                ${filtered.map(
                  (item) => html`
                    <tr class="${item.status === 'WARNING' ? 'row-warning' : item.status === 'CRITICAL' ? 'row-critical' : ''}">
                      <td class="mono">${item.wbs_code}</td>
                      <td>${item.name}</td>
                      <td class="mono">${formatCents(item.estimated_cost_cents, item.estimated_cost_currency_code as CurrencyCode)}</td>
                      <td class="mono">${item.lead_time_days}d</td>
                      <td class="mono">${item.weather_buffer_days}d</td>
                      <td class="mono">${this._formatDate(item.need_by_date)}</td>
                      <td class="mono">${this._formatDate(item.must_order_date)}</td>
                      <td class="mono">${item.po_number ?? '\u2014'}</td>
                      <td><span class="status-badge ${item.status}">${item.status}</span></td>
                    </tr>
                  `,
                )}
              </tbody>
            </table>
          `}
      </div>
    `;
  }

  private _getFilteredItems(): ProcurementItem[] {
    if (this._filter === 'ALL') return this._items;
    return this._items.filter((i) => i.status === this._filter);
  }

  private _formatDate(dateStr: string): string {
    try {
      return new Date(dateStr).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    } catch {
      return dateStr;
    }
  }

  private _onProjectChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    this._selectedProject = select.value;
    if (this._selectedProject) {
      currentProject.set(this._selectedProject);
      this._loadItems();
    } else {
      this._items = [];
    }
  }

  private async _loadProjects() {
    try {
      const res = await listProjects({ status: 'active' });
      this._projects = res.projects;
      const cur = currentProject.get();
      if (cur && this._projects.some((p) => p.id === cur)) {
        this._selectedProject = cur;
        await this._loadItems();
      }
    } catch {
      this._error = 'Failed to load projects';
    } finally {
      this._loading = false;
    }
  }

  private async _loadItems() {
    if (!this._selectedProject) return;
    this._loading = true;
    this._error = '';
    try {
      const res = await listProcurement(this._selectedProject);
      this._items = res.items;
    } catch (err) {
      this._error = err instanceof ApiError ? `Failed to load procurement (${err.status})` : 'Failed to load procurement data';
      this.showToast(this._error, 'error');
    } finally {
      this._loading = false;
    }
  }
}
