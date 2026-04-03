import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  listProjects, getGantt, listTasks, recalculateSchedule,
  type Project, type TaskSchedule, type GanttData, ApiError,
} from '../../state/api.js';
import { currentProject } from '../../state/store.js';

@customElement('fb-schedule-view')
export class FBScheduleView extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _projects: Project[] = [];
  @state() private _selectedProject = '';
  @state() private _gantt: GanttData | null = null;
  @state() private _tasks: TaskSchedule[] = [];
  @state() private _recalculating = false;
  @state() private _recalcMs: number | null = null;
  @state() private _sortField: keyof TaskSchedule = 'wbs_code';
  @state() private _sortDir: 'asc' | 'desc' = 'asc';

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

      .project-select:focus {
        outline: none;
        border-color: var(--fb-gable-green);
      }

      .project-select option {
        background: var(--fb-surface);
        color: var(--fb-text-primary);
      }

      .recalc-btn {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
        padding: var(--fb-space-2) var(--fb-space-4);
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        border: none;
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 600;
        cursor: pointer;
        transition: opacity var(--fb-transition-fast);
        font-family: var(--fb-font-body);
      }

      .recalc-btn:hover { opacity: 0.9; }
      .recalc-btn:disabled { opacity: 0.5; cursor: not-allowed; }

      .recalc-time {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        font-variant-numeric: tabular-nums;
      }

      .gantt-container {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        margin-bottom: var(--fb-space-6);
        min-height: 200px;
      }

      .gantt-placeholder {
        text-align: center;
        color: var(--fb-text-muted);
        font-size: var(--fb-text-sm);
        padding: var(--fb-space-8);
      }

      .gantt-bars {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
      }

      .gantt-row {
        display: grid;
        grid-template-columns: 160px 1fr;
        gap: var(--fb-space-3);
        align-items: center;
      }

      .gantt-label {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }

      .gantt-label .wbs {
        font-family: var(--fb-font-mono);
        color: var(--fb-text-muted);
        margin-right: var(--fb-space-2);
      }

      .gantt-bar-track {
        height: 24px;
        background: var(--fb-surface);
        border-radius: var(--fb-radius-sm);
        position: relative;
        overflow: hidden;
      }

      .gantt-bar {
        position: absolute;
        top: 2px;
        bottom: 2px;
        border-radius: 3px;
        transition: width var(--fb-transition-normal);
        min-width: 4px;
      }

      .gantt-bar.critical { background: var(--fb-gable-green); }
      .gantt-bar.normal { background: var(--fb-blueprint-blue); opacity: 0.7; }
      .gantt-bar.completed { background: var(--fb-text-muted); opacity: 0.5; }

      .gantt-bar-progress {
        position: absolute;
        top: 0;
        bottom: 0;
        left: 0;
        background: rgba(255, 255, 255, 0.2);
        border-radius: 3px;
      }

      .section-title {
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
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

      tr:hover td { background: rgba(255, 255, 255, 0.02); }

      .badge {
        display: inline-block;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
      }

      .badge.critical {
        background: rgba(0, 255, 163, 0.1);
        color: var(--fb-gable-green);
      }

      .badge.pending { background: rgba(148, 163, 184, 0.1); color: var(--fb-text-secondary); }
      .badge.in_progress { background: rgba(56, 189, 248, 0.1); color: var(--fb-blueprint-blue); }
      .badge.completed { background: rgba(0, 255, 163, 0.1); color: var(--fb-gable-green); }

      .progress-bar {
        width: 60px;
        height: 6px;
        background: var(--fb-surface);
        border-radius: 3px;
        overflow: hidden;
        display: inline-block;
        vertical-align: middle;
        margin-right: var(--fb-space-2);
      }

      .progress-fill {
        height: 100%;
        border-radius: 3px;
        background: var(--fb-gable-green);
        transition: width var(--fb-transition-normal);
      }

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
    this._loadProjects();
  }

  render() {
    if (this._loading && this._projects.length === 0) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    return html`
      <div class="page-header">
        <h1>Schedule</h1>
        <div class="controls">
          <select class="project-select" @change=${this._onProjectChange} .value=${this._selectedProject}>
            <option value="">Select a project...</option>
            ${this._projects.map((p) => html`<option value=${p.id}>${p.name}</option>`)}
          </select>
          <button
            class="recalc-btn"
            ?disabled=${!this._selectedProject || this._recalculating}
            @click=${this._onRecalculate}
          >
            ${this._recalculating ? 'Calculating...' : 'Recalculate CPM'}
          </button>
          ${this._recalcMs !== null
            ? html`<span class="recalc-time">${this._recalcMs}ms</span>`
            : nothing}
        </div>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      <div class="gantt-container">
        <h2 class="section-title">Gantt Chart</h2>
        ${!this._selectedProject
          ? html`<div class="gantt-placeholder">Select a project to view the Gantt chart.</div>`
          : this._loading
            ? html`<div class="gantt-placeholder"><fb-spinner></fb-spinner></div>`
            : this._gantt === null || this._gantt.tasks.length === 0
              ? html`<div class="gantt-placeholder">No schedule data. Run a CPM recalculation.</div>`
              : this._renderGantt()}
      </div>

      <div class="table-container">
        <h2 class="section-title">Task List</h2>
        ${this._tasks.length === 0
          ? html`<div class="empty-state">${this._selectedProject ? 'No tasks found.' : 'Select a project to view tasks.'}</div>`
          : html`
            <table>
              <thead>
                <tr>
                  <th class="${this._sortField === 'wbs_code' ? 'sorted' : ''}" @click=${() => this._toggleSort('wbs_code')}>WBS</th>
                  <th class="${this._sortField === 'name' ? 'sorted' : ''}" @click=${() => this._toggleSort('name')}>Task</th>
                  <th class="${this._sortField === 'duration_days' ? 'sorted' : ''}" @click=${() => this._toggleSort('duration_days')}>Duration</th>
                  <th class="${this._sortField === 'early_start' ? 'sorted' : ''}" @click=${() => this._toggleSort('early_start')}>Start</th>
                  <th class="${this._sortField === 'early_finish' ? 'sorted' : ''}" @click=${() => this._toggleSort('early_finish')}>Finish</th>
                  <th class="${this._sortField === 'total_float' ? 'sorted' : ''}" @click=${() => this._toggleSort('total_float')}>Float</th>
                  <th>Progress</th>
                  <th>Status</th>
                  <th>Critical</th>
                </tr>
              </thead>
              <tbody>
                ${this._getSortedTasks().map(
                  (t) => html`
                    <tr>
                      <td class="mono">${t.wbs_code}</td>
                      <td>${t.name}</td>
                      <td class="mono">${t.duration_days}d</td>
                      <td class="mono">${this._formatDate(t.early_start)}</td>
                      <td class="mono">${this._formatDate(t.early_finish)}</td>
                      <td class="mono">${t.total_float}d</td>
                      <td>
                        <div class="progress-bar">
                          <div class="progress-fill" style="width: ${t.percent_complete}%"></div>
                        </div>
                        <span class="mono" style="font-size: var(--fb-text-xs);">${t.percent_complete}%</span>
                      </td>
                      <td><span class="badge ${t.status}">${t.status.replace('_', ' ')}</span></td>
                      <td>${t.is_critical ? html`<span class="badge critical">CRITICAL</span>` : '\u2014'}</td>
                    </tr>
                  `,
                )}
              </tbody>
            </table>
          `}
      </div>
    `;
  }

  private _renderGantt() {
    if (!this._gantt) return nothing;

    const tasks = this._gantt.tasks;
    if (tasks.length === 0) return html`<div class="gantt-placeholder">No tasks.</div>`;

    // Calculate date range for proportional bar positioning
    const allStarts = tasks.map((t) => new Date(t.early_start).getTime());
    const allEnds = tasks.map((t) => new Date(t.early_finish).getTime());
    const minDate = Math.min(...allStarts);
    const maxDate = Math.max(...allEnds);
    const range = maxDate - minDate || 1;

    return html`
      <div class="gantt-bars">
        ${tasks.slice(0, 20).map((t) => {
          const start = new Date(t.early_start).getTime();
          const end = new Date(t.early_finish).getTime();
          const leftPct = ((start - minDate) / range) * 100;
          const widthPct = Math.max(((end - start) / range) * 100, 1);
          const barClass = t.status === 'completed' ? 'completed' : t.is_critical ? 'critical' : 'normal';

          return html`
            <div class="gantt-row">
              <div class="gantt-label">
                <span class="wbs">${t.wbs_code}</span>${t.name}
              </div>
              <div class="gantt-bar-track">
                <div
                  class="gantt-bar ${barClass}"
                  style="left: ${leftPct}%; width: ${widthPct}%;"
                >
                  ${t.percent_complete > 0
                    ? html`<div class="gantt-bar-progress" style="width: ${t.percent_complete}%"></div>`
                    : nothing}
                </div>
              </div>
            </div>
          `;
        })}
      </div>
    `;
  }

  private _formatDate(isoString: string): string {
    try {
      const d = new Date(isoString);
      return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    } catch {
      return isoString;
    }
  }

  private _toggleSort(field: keyof TaskSchedule) {
    if (this._sortField === field) {
      this._sortDir = this._sortDir === 'asc' ? 'desc' : 'asc';
    } else {
      this._sortField = field;
      this._sortDir = 'asc';
    }
  }

  private _getSortedTasks(): TaskSchedule[] {
    const sorted = [...this._tasks];
    sorted.sort((a, b) => {
      const aVal = a[this._sortField];
      const bVal = b[this._sortField];
      if (typeof aVal === 'string' && typeof bVal === 'string') {
        return this._sortDir === 'asc' ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
      }
      if (typeof aVal === 'number' && typeof bVal === 'number') {
        return this._sortDir === 'asc' ? aVal - bVal : bVal - aVal;
      }
      if (typeof aVal === 'boolean' && typeof bVal === 'boolean') {
        return this._sortDir === 'asc' ? (aVal === bVal ? 0 : aVal ? 1 : -1) : (aVal === bVal ? 0 : aVal ? -1 : 1);
      }
      return 0;
    });
    return sorted;
  }

  private _onProjectChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    this._selectedProject = select.value;
    if (this._selectedProject) {
      currentProject.set(this._selectedProject);
      this._loadScheduleData();
    } else {
      this._gantt = null;
      this._tasks = [];
    }
  }

  private async _onRecalculate() {
    if (!this._selectedProject) return;
    this._recalculating = true;
    this._recalcMs = null;
    this._error = '';

    try {
      const result = await recalculateSchedule(this._selectedProject);
      this._recalcMs = result.recalculation_ms;
      this.showToast(`CPM recalculated in ${result.recalculation_ms}ms`, 'success');
      await this._loadScheduleData();
    } catch (err) {
      if (err instanceof ApiError) {
        this._error = `Recalculation failed (${err.status})`;
      } else {
        this._error = 'Recalculation failed';
      }
      this.showToast(this._error, 'error');
    } finally {
      this._recalculating = false;
    }
  }

  private async _loadProjects() {
    try {
      const res = await listProjects({ status: 'active' });
      this._projects = res.projects;
      const cur = currentProject.get();
      if (cur && this._projects.some((p) => p.id === cur)) {
        this._selectedProject = cur;
        await this._loadScheduleData();
      }
    } catch (err) {
      this._error = 'Failed to load projects';
    } finally {
      this._loading = false;
    }
  }

  private async _loadScheduleData() {
    if (!this._selectedProject) return;
    this._loading = true;
    this._error = '';

    try {
      const [ganttRes, tasksRes] = await Promise.all([
        getGantt(this._selectedProject),
        listTasks(this._selectedProject),
      ]);
      this._gantt = ganttRes;
      this._tasks = tasksRes.tasks;
    } catch (err) {
      if (err instanceof ApiError) {
        this._error = `Failed to load schedule data (${err.status})`;
      } else {
        this._error = 'Failed to load schedule data';
      }
    } finally {
      this._loading = false;
    }
  }
}
