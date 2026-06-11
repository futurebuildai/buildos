import { html, svg, css, nothing, type TemplateResult, type SVGTemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import type { ProjectTask, TaskDependency } from '../../types/models.js';

const ROW_H = 28;
const PX_PER_DAY = 16;
const AXIS_H = 24;
const PAD = 8;
const DAY_MS = 86_400_000;
/** Fixed left gutter for task labels (WBS mono prefix + name), pixels. */
const LABEL_W = 220;
/** Bars/axis start after the label gutter. */
const ORIGIN_X = PAD + LABEL_W;

interface Placed {
  task: ProjectTask;
  esDay: number;
  efDay: number;
  lfDay: number;
  /** Row index in the placed set (for y-coord + dep-arrow lookup). */
  row: number;
}

function dayOf(date: string | undefined, origin: number): number | null {
  if (!date) return null;
  const t = new Date(date).getTime();
  if (Number.isNaN(t)) return null;
  return Math.round((t - origin) / DAY_MS);
}

/** Short month-day label for axis ticks, formatted from an absolute ms timestamp. */
const TICK_FMT = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  day: 'numeric',
  timeZone: 'UTC',
});
function tickLabel(ms: number): string {
  return TICK_FMT.format(new Date(ms));
}

/** Format an RFC3339 date for the accessible table (UTC, same as the axis); em-dash when absent. */
function fmtDateCell(iso: string | null | undefined): string {
  if (!iso) return '—';
  const ms = Date.parse(iso);
  return Number.isNaN(ms) ? iso : TICK_FMT.format(new Date(ms));
}

/**
 * `fb-gantt-chart` — the CPM schedule visualization (UX_CORE_SCREENS §2.2–§2.3,
 * DESIGN_SYSTEM §9.3). Renders one bar per task from `early_start → early_finish`
 * with a hollow float tail out to `late_finish`. Critical bars (`is_critical`)
 * are Gable Green; non-critical are Slate Steel with a Blueprint Blue outline.
 * Near-critical float tails (`0 < total_float ≤ near-critical`) are amber. A
 * fixed left gutter carries each task's WBS code (mono) + name; a dated axis
 * (mono ticks) anchors the timeline; finish-to-start dependency arrows connect
 * predecessor → successor bars. A today line and a project-end marker anchor
 * the axis.
 *
 * Dual representation (DSC §7.8): the SVG is `aria-hidden` and a parallel data
 * table carries the same schedule (names, dates, float, AND dependencies) for
 * assistive tech. The table rows are the canonical interactive surface —
 * keyboard-focusable and activating a `task-select` event. Mouse users may also
 * click a bar (pointer-only). Cascade slips (`slipped-ids`) pulse Safety Red,
 * suppressed under `prefers-reduced-motion`.
 */
@customElement('fb-gantt-chart')
export class FbGanttChart extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
        overflow-x: auto;
      }
      .bar {
        rx: 3;
        cursor: pointer;
      }
      .bar.critical {
        fill: var(--fb-gable-green, #00ffa3);
      }
      .bar.normal {
        fill: var(--fb-slate-steel, #1e2029);
        stroke: var(--fb-blueprint-blue, #38bdf8);
        stroke-width: 1;
      }
      .bar.slipped {
        animation: slip-pulse 1.2s ease-in-out 2;
      }
      .tail {
        fill: none;
        stroke-dasharray: 3 2;
        stroke: var(--fb-border, #3a3d44);
      }
      .tail.near {
        stroke: var(--fb-amber, #f59e0b);
      }
      .wbs {
        fill: var(--fb-text-secondary, #9aa0a6);
        font-family: var(--fb-font-mono, monospace);
        font-size: 11px;
      }
      .task-name {
        fill: var(--fb-text-secondary, #9aa0a6);
        font-size: 12px;
      }
      .task-name.critical {
        fill: var(--fb-text-primary, #f0f0f5);
      }
      .axis {
        stroke: var(--fb-border, #3a3d44);
        stroke-width: 1;
      }
      .gridline {
        stroke: var(--fb-border, #3a3d44);
        stroke-width: 1;
        opacity: 0.4;
      }
      .axis-tick {
        fill: var(--fb-text-muted, #6b7077);
        font-family: var(--fb-font-mono, monospace);
        font-size: 10px;
      }
      .today {
        stroke: var(--fb-blueprint-blue, #38bdf8);
        stroke-width: 1.5;
        stroke-dasharray: 4 3;
      }
      .project-end {
        stroke: var(--fb-gable-green, #00ffa3);
        stroke-width: 1.5;
      }
      .marker-label {
        fill: var(--fb-text-muted, #6b7077);
        font-family: var(--fb-font-mono, monospace);
        font-size: 10px;
      }
      .dep {
        fill: none;
        stroke: var(--fb-border, #3a3d44);
        stroke-width: 1.25;
      }
      .dep.blue {
        stroke: var(--fb-blueprint-blue, #38bdf8);
        opacity: 0.55;
      }
      .dep.critical {
        stroke: var(--fb-gable-green, #00ffa3);
      }
      @keyframes slip-pulse {
        0%,
        100% {
          fill: var(--fb-gable-green, #00ffa3);
        }
        50% {
          fill: var(--fb-safety-red, #f43f5e);
        }
      }
      @media (prefers-reduced-motion: reduce) {
        .bar.slipped {
          animation: none;
          stroke: var(--fb-safety-red, #f43f5e);
          stroke-width: 2;
        }
      }
      /* The accessible table is the canonical interactive surface. */
      table.schedule {
        border-collapse: collapse;
        width: 100%;
      }
      tr.task-row {
        cursor: pointer;
      }
      tr.task-row:focus-visible {
        outline: 2px solid var(--fb-gable-green, #00ffa3);
        outline-offset: -2px;
      }
    `,
  ];

  @property({ type: Array }) tasks: ProjectTask[] = [];
  /** Dependency edges (predecessor→successor) for drawing arrows. */
  @property({ type: Array }) dependencies: TaskDependency[] = [];
  /** Project end (RFC3339) for the end marker; ignored when zero-value. */
  @property({ type: String, attribute: 'project-end' }) projectEnd = '';
  /** Float ≤ this (and > 0) colors the tail amber (OQ-7, product default 2d). */
  @property({ type: Number, attribute: 'near-critical' }) nearCritical = 2;
  /** Task ids whose dates moved later in the last recalc (cascade pulse). */
  @property({ type: Array, attribute: 'slipped-ids' }) slippedIds: string[] = [];

  private place(origin: number): Placed[] {
    const placed: Placed[] = [];
    let row = 0;
    for (const task of this.tasks) {
      const esDay = dayOf(task.early_start, origin);
      const efDay = dayOf(task.early_finish, origin);
      if (esDay === null || efDay === null) continue;
      const lfDay = dayOf(task.late_finish, origin) ?? efDay;
      placed.push({ task, esDay, efDay, lfDay, row });
      row++;
    }
    return placed;
  }

  /** Earliest dated milestone across the task set, used as the day-0 origin. */
  private origin(): number | null {
    let min = Infinity;
    for (const t of this.tasks) {
      for (const d of [t.early_start, t.late_start]) {
        if (!d) continue;
        const ms = new Date(d).getTime();
        if (!Number.isNaN(ms)) min = Math.min(min, ms);
      }
    }
    return min === Infinity ? null : min;
  }

  /** Dispatch the canonical selection event (table row or bar click). */
  private selectTask(id: string): void {
    this.dispatchEvent(
      new CustomEvent('task-select', { detail: { id }, bubbles: true, composed: true }),
    );
  }

  private renderRow(p: Placed): SVGTemplateResult {
    const y = AXIS_H + p.row * ROW_H + 4;
    const h = ROW_H - 10;
    const x = ORIGIN_X + p.esDay * PX_PER_DAY;
    const w = Math.max(PX_PER_DAY, (p.efDay - p.esDay) * PX_PER_DAY);
    const tailW = Math.max(0, (p.lfDay - p.efDay) * PX_PER_DAY);
    const float = p.task.total_float ?? 0;
    const near = float > 0 && float <= this.nearCritical;
    const slipped = this.slippedIds.includes(p.task.id);
    const critical = p.task.is_critical;
    const cls = `bar ${critical ? 'critical' : 'normal'}${slipped ? ' slipped' : ''}`;
    const labelY = y + h - 1;
    // Each dynamic fragment is the sole child of a wrapped <g>: happy-dom drops a
    // child-binding that sits as a direct sibling of another element at a template
    // root, so the tail line is isolated in its own group to survive the commit.
    return svg`<g class="row">
      <text class="wbs" x=${PAD} y=${labelY}><title>${p.task.name}</title>${p.task.wbs_code}</text>
      <text class="task-name ${critical ? 'critical' : ''}" x=${PAD + 44} y=${labelY}><title>${
        p.task.name
      }</title>${p.task.name}</text>
      <g class="tail-host">${
        tailW > 0
          ? svg`<line class="tail ${near ? 'near' : ''}" x1=${x + w} y1=${y + h / 2} x2=${
              x + w + tailW
            } y2=${y + h / 2}></line>`
          : nothing
      }</g>
      <rect
        class=${cls}
        x=${x}
        y=${y}
        width=${w}
        height=${h}
        rx="3"
        @click=${() => this.selectTask(p.task.id)}
      ></rect>
    </g>`;
  }

  /**
   * Orthogonal elbow arrow predecessor finish-edge → successor start-edge (FS).
   * Honors `lag_days` (the successor already starts later in CPM, so the arrow
   * simply lands on the successor's placed start). Critical-chain links (both
   * endpoints critical) render Gable Green; others a muted blue. Skips edges
   * whose endpoint is filtered out of the placed set.
   */
  private renderDep(dep: TaskDependency, byId: Map<string, Placed>): SVGTemplateResult | symbol {
    const pred = byId.get(dep.predecessor_id);
    const succ = byId.get(dep.successor_id);
    if (!pred || !succ) return nothing;

    const barH = ROW_H - 10;
    const predY = AXIS_H + pred.row * ROW_H + 4 + barH / 2;
    const succY = AXIS_H + succ.row * ROW_H + 4 + barH / 2;
    const predEndX = ORIGIN_X + pred.efDay * PX_PER_DAY;
    const succStartX = ORIGIN_X + succ.esDay * PX_PER_DAY;

    // Elbow: out from predecessor finish, a small stub, then drop/rise to the
    // successor row, then in to the successor start edge.
    const stub = Math.min(10, Math.max(4, (succStartX - predEndX) / 2));
    const midX = Math.max(predEndX + stub, succStartX - stub);
    const d = `M ${predEndX} ${predY} H ${midX} V ${succY} H ${succStartX}`;
    const critical = pred.task.is_critical && succ.task.is_critical;
    const cls = `dep ${critical ? 'critical' : 'blue'}`;
    return svg`<path
      class=${cls}
      d=${d}
      marker-end=${critical ? 'url(#arrow-critical)' : 'url(#arrow-muted)'}
    ></path>`;
  }

  /** Accessible parallel table (DSC §7.8) — the SVG is decorative for SR users. */
  private renderTable(): TemplateResult {
    const depCount = new Map<string, number>();
    for (const d of this.dependencies) {
      depCount.set(d.successor_id, (depCount.get(d.successor_id) ?? 0) + 1);
    }
    return html`<table class="schedule">
      <caption class="visually-hidden">
        Project schedule (critical path, early/late dates, float, and dependency count per task).
        Activate a row to inspect the task.
      </caption>
      <thead>
        <tr>
          <th scope="col">WBS</th>
          <th scope="col">Task</th>
          <th scope="col">Early start</th>
          <th scope="col">Early finish</th>
          <th scope="col">Late finish</th>
          <th scope="col">Float (days)</th>
          <th scope="col">Predecessors</th>
          <th scope="col">Critical</th>
        </tr>
      </thead>
      <tbody>
        ${this.tasks.map(
          (t) =>
            html`<tr
              class="task-row"
              role="button"
              tabindex="0"
              aria-label=${`Inspect task ${t.wbs_code} ${t.name}`}
              @click=${() => this.selectTask(t.id)}
              @keydown=${(e: KeyboardEvent) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  this.selectTask(t.id);
                }
              }}
            >
              <td>${t.wbs_code}</td>
              <td>${t.name}</td>
              <td>${fmtDateCell(t.early_start)}</td>
              <td>${fmtDateCell(t.early_finish)}</td>
              <td>${fmtDateCell(t.late_finish)}</td>
              <td>${t.total_float ?? '—'}</td>
              <td>${depCount.get(t.id) ?? 0}</td>
              <td>${t.is_critical ? 'Yes' : 'No'}</td>
            </tr>`,
        )}
      </tbody>
    </table>`;
  }

  /** Pick a tick interval (days) and emit dated gridlines + mono labels. */
  private renderAxis(origin: number, maxDay: number, height: number): SVGTemplateResult {
    const step = maxDay <= 14 ? 1 : 7;
    const ticks: SVGTemplateResult[] = [];
    for (let day = 0; day <= maxDay; day += step) {
      const x = ORIGIN_X + day * PX_PER_DAY;
      const label = tickLabel(origin + day * DAY_MS);
      ticks.push(
        svg`<g class="tick-host">
          <line class="gridline" x1=${x} y1=${AXIS_H} x2=${x} y2=${height}></line>
          <text class="axis-tick" x=${x + 2} y=${AXIS_H - 12}>${label}</text>
        </g>`,
      );
    }
    return svg`<g class="axis-ticks">${ticks}</g>`;
  }

  override render(): TemplateResult {
    const origin = this.origin();
    if (origin === null || this.tasks.length === 0) return this.renderTable();

    const placed = this.place(origin);
    const byId = new Map<string, Placed>();
    for (const p of placed) byId.set(p.task.id, p);

    const endDay = dayOf(this.projectEnd, origin);
    const todayDay = Math.round((Date.now() - origin) / DAY_MS);

    let maxDay = 1;
    for (const p of placed) maxDay = Math.max(maxDay, p.lfDay, p.efDay);
    if (endDay !== null) maxDay = Math.max(maxDay, endDay);
    maxDay = Math.max(maxDay, todayDay);

    const width = ORIGIN_X + maxDay * PX_PER_DAY + PAD + 40;
    const height = AXIS_H + placed.length * ROW_H + PAD;
    const todayX = ORIGIN_X + todayDay * PX_PER_DAY;
    const endX = endDay !== null ? ORIGIN_X + endDay * PX_PER_DAY : null;

    const todayMarker =
      todayDay >= 0 && todayDay <= maxDay
        ? svg`<g class="today-host"><line class="today" x1=${todayX} y1=${AXIS_H} x2=${todayX} y2=${height}></line>
            <text class="marker-label" x=${todayX + 2} y=${AXIS_H - 4}>today</text></g>`
        : nothing;
    const endMarker =
      endX !== null
        ? svg`<g class="end-host"><line class="project-end" x1=${endX} y1=${AXIS_H} x2=${endX} y2=${height}></line>
            <text class="marker-label" x=${endX + 2} y=${AXIS_H - 4}>end</text></g>`
        : nothing;

    const deps = this.dependencies.map((d) => this.renderDep(d, byId));

    // The decorative SVG lives in its own nested template embedded as a single
    // child-part of `.host`. This isolates a happy-dom quirk (it strips Lit's
    // comment part-markers inside an <svg>, so dynamic bars don't commit and
    // their content would otherwise leak into the next sibling part) — keeping
    // the parallel accessible table, the canonical AT representation, clean.
    const chart = html`<svg
      width=${width}
      height=${height}
      viewBox="0 0 ${width} ${height}"
      role="img"
      aria-hidden="true"
    >
      <defs>
        <marker
          id="arrow-muted"
          viewBox="0 0 8 8"
          refX="6"
          refY="4"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M0,0 L8,4 L0,8 z" fill="var(--fb-blueprint-blue, #38bdf8)"></path>
        </marker>
        <marker
          id="arrow-critical"
          viewBox="0 0 8 8"
          refX="6"
          refY="4"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M0,0 L8,4 L0,8 z" fill="var(--fb-gable-green, #00ffa3)"></path>
        </marker>
      </defs>
      <line class="axis" x1=${ORIGIN_X} y1=${AXIS_H} x2=${width - PAD} y2=${AXIS_H}></line>
      ${this.renderAxis(origin, maxDay, height)}${todayMarker}${endMarker}
      <g class="deps">${deps}</g>
      ${placed.map((p) => this.renderRow(p))}
    </svg>`;
    return html`
      <div class="host">${chart}</div>
      <div class="table-host">${this.renderTable()}</div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-gantt-chart': FbGanttChart;
  }
}
