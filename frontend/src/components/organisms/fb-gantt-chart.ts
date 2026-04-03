import { html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export interface GanttTask {
  id: string;
  name: string;
  startDate: string;   // ISO date
  endDate: string;      // ISO date
  isCritical: boolean;
  floatDays: number;
  progress: number;     // 0-100
  wbsCode?: string;
}

/**
 * fb-gantt-chart — CPM Gantt chart visualization.
 *
 * Horizontal bars per task:
 * - Critical path: Gable Green
 * - Non-critical: Blueprint Blue
 * - Float: transparent extension
 * - Today line: vertical red line
 *
 * @property tasks - Array of GanttTask objects
 * @property projectStart - ISO date string for project start
 * @property projectEnd - ISO date string for project end
 */
@customElement('fb-gantt-chart')
export class FBGanttChart extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .gantt-container {
        border-radius: var(--fb-radius-md);
        border: 1px solid var(--fb-border);
        overflow: hidden;
      }

      .gantt-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: var(--fb-space-3) var(--fb-space-4);
        background: var(--fb-slate-steel);
        border-bottom: 1px solid var(--fb-border);
      }

      .gantt-title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        font-weight: 500;
        color: var(--fb-text-primary);
      }

      .legend {
        display: flex;
        gap: var(--fb-space-4);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
      }
      .legend-item {
        display: flex;
        align-items: center;
        gap: 4px;
      }
      .legend-dot {
        width: 10px;
        height: 10px;
        border-radius: 2px;
      }
      .legend-critical { background: var(--fb-gable-green); }
      .legend-noncritical { background: var(--fb-blueprint-blue); }
      .legend-float { background: rgba(56, 189, 248, 0.2); }

      .gantt-scroll {
        overflow-x: auto;
        overflow-y: auto;
        max-height: 500px;
      }

      .gantt-body {
        display: flex;
        min-width: max-content;
      }

      .task-labels {
        flex-shrink: 0;
        width: 200px;
        background: var(--fb-slate-steel);
        border-right: 1px solid var(--fb-border);
      }

      .task-label {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
        padding: var(--fb-space-2) var(--fb-space-3);
        height: 36px;
        border-bottom: 1px solid var(--fb-border);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-primary);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }
      .task-label .wbs {
        font-family: var(--fb-font-mono);
        color: var(--fb-text-muted);
        font-size: 10px;
      }

      .timeline {
        flex: 1;
        position: relative;
        min-width: 600px;
      }

      .date-axis {
        display: flex;
        height: 28px;
        border-bottom: 1px solid var(--fb-border);
        background: var(--fb-slate-steel);
      }
      .date-tick {
        flex: 1;
        display: flex;
        align-items: center;
        justify-content: center;
        font-family: var(--fb-font-mono);
        font-size: 10px;
        color: var(--fb-text-muted);
        border-right: 1px solid var(--fb-border);
      }

      .task-row {
        position: relative;
        height: 36px;
        border-bottom: 1px solid var(--fb-border);
      }

      .task-bar {
        position: absolute;
        top: 8px;
        height: 20px;
        border-radius: 4px;
        display: flex;
        align-items: center;
        transition: opacity var(--fb-transition-fast);
      }
      .task-bar:hover { opacity: 0.85; }

      .bar-critical { background: var(--fb-gable-green); }
      .bar-noncritical { background: var(--fb-blueprint-blue); }

      .bar-float {
        position: absolute;
        top: 8px;
        height: 20px;
        background: rgba(56, 189, 248, 0.15);
        border: 1px dashed rgba(56, 189, 248, 0.3);
        border-radius: 4px;
      }

      .bar-progress {
        height: 100%;
        background: rgba(0, 0, 0, 0.2);
        border-radius: 4px;
      }

      .today-line {
        position: absolute;
        top: 0;
        bottom: 0;
        width: 2px;
        background: var(--fb-safety-red);
        z-index: 5;
      }
      .today-label {
        position: absolute;
        top: 2px;
        font-family: var(--fb-font-mono);
        font-size: 9px;
        color: var(--fb-safety-red);
        white-space: nowrap;
        transform: translateX(4px);
      }

      .empty {
        text-align: center;
        padding: var(--fb-space-8);
        color: var(--fb-text-muted);
      }
    `,
  ];

  @property({ type: Array }) tasks: GanttTask[] = [];
  @property({ type: String }) projectStart = '';
  @property({ type: String }) projectEnd = '';

  @state() private _dateRange: Date[] = [];

  override willUpdate() {
    this._computeDateRange();
  }

  private _computeDateRange() {
    if (!this.projectStart || !this.projectEnd) {
      if (this.tasks.length === 0) {
        this._dateRange = [];
        return;
      }
      const dates = this.tasks.flatMap(t => [new Date(t.startDate), new Date(t.endDate)]);
      const min = new Date(Math.min(...dates.map(d => d.getTime())));
      const max = new Date(Math.max(...dates.map(d => d.getTime())));
      this._dateRange = this._generateWeeks(min, max);
    } else {
      this._dateRange = this._generateWeeks(new Date(this.projectStart), new Date(this.projectEnd));
    }
  }

  private _generateWeeks(start: Date, end: Date): Date[] {
    const weeks: Date[] = [];
    const d = new Date(start);
    d.setDate(d.getDate() - d.getDay()); // start at Sunday
    while (d <= end) {
      weeks.push(new Date(d));
      d.setDate(d.getDate() + 7);
    }
    weeks.push(new Date(d)); // one extra
    return weeks;
  }

  private _getPosition(dateStr: string): number {
    if (this._dateRange.length < 2) return 0;
    const date = new Date(dateStr);
    const start = this._dateRange[0]!.getTime();
    const end = this._dateRange[this._dateRange.length - 1]!.getTime();
    const total = end - start;
    if (total === 0) return 0;
    return ((date.getTime() - start) / total) * 100;
  }

  private _formatWeek(d: Date): string {
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  }

  override render() {
    if (this.tasks.length === 0) {
      return html`
        <div class="gantt-container">
          <div class="gantt-header">
            <span class="gantt-title">CPM Schedule</span>
          </div>
          <div class="empty">No tasks scheduled</div>
        </div>
      `;
    }

    const todayPos = this._getPosition(new Date().toISOString());

    return html`
      <div class="gantt-container">
        <div class="gantt-header">
          <span class="gantt-title">CPM Schedule</span>
          <div class="legend">
            <span class="legend-item"><span class="legend-dot legend-critical"></span> Critical Path</span>
            <span class="legend-item"><span class="legend-dot legend-noncritical"></span> Non-Critical</span>
            <span class="legend-item"><span class="legend-dot legend-float"></span> Float</span>
          </div>
        </div>

        <div class="gantt-scroll">
          <div class="gantt-body">
            <div class="task-labels">
              <div class="task-label" style="height:28px; font-weight: 500; color: var(--fb-text-muted);">Task</div>
              ${this.tasks.map(task => html`
                <div class="task-label" title=${task.name}>
                  ${task.wbsCode ? html`<span class="wbs">${task.wbsCode}</span>` : nothing}
                  ${task.name}
                </div>
              `)}
            </div>

            <div class="timeline">
              <div class="date-axis">
                ${this._dateRange.map(d => html`
                  <div class="date-tick">${this._formatWeek(d)}</div>
                `)}
              </div>

              ${this.tasks.map(task => {
                const left = this._getPosition(task.startDate);
                const right = this._getPosition(task.endDate);
                const width = Math.max(right - left, 0.5);
                const floatEnd = task.floatDays > 0
                  ? this._getPosition(new Date(new Date(task.endDate).getTime() + task.floatDays * 86400000).toISOString())
                  : 0;
                const floatWidth = floatEnd > 0 ? floatEnd - right : 0;

                return html`
                  <div class="task-row">
                    <div
                      class="task-bar ${task.isCritical ? 'bar-critical' : 'bar-noncritical'}"
                      style="left: ${left}%; width: ${width}%;"
                      title="${task.name}: ${task.progress}% complete"
                    >
                      ${task.progress > 0 ? html`
                        <div class="bar-progress" style="width: ${task.progress}%;"></div>
                      ` : nothing}
                    </div>
                    ${floatWidth > 0 ? html`
                      <div class="bar-float" style="left: ${right}%; width: ${floatWidth}%;"></div>
                    ` : nothing}
                  </div>
                `;
              })}

              ${todayPos > 0 && todayPos < 100 ? html`
                <div class="today-line" style="left: ${todayPos}%;">
                  <span class="today-label">Today</span>
                </div>
              ` : nothing}
            </div>
          </div>
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-gantt-chart': FBGanttChart;
  }
}
