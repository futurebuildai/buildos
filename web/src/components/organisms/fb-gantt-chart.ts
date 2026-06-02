import { html, svg, css, nothing, type TemplateResult, type SVGTemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import type { ProjectTask } from '../../types/models.js';

const ROW_H = 28;
const PX_PER_DAY = 16;
const AXIS_H = 24;
const PAD = 8;
const DAY_MS = 86_400_000;

interface Placed {
  task: ProjectTask;
  esDay: number;
  efDay: number;
  lfDay: number;
}

function dayOf(date: string | undefined, origin: number): number | null {
  if (!date) return null;
  const t = new Date(date).getTime();
  if (Number.isNaN(t)) return null;
  return Math.round((t - origin) / DAY_MS);
}

/**
 * `fb-gantt-chart` — the CPM schedule visualization (UX_CORE_SCREENS §2.2–§2.3,
 * DESIGN_SYSTEM §9.3). Renders one bar per task from `early_start → early_finish`
 * with a hollow float tail out to `late_finish`. Critical bars (`is_critical`)
 * are Gable Green; non-critical are Slate Steel with a Blueprint Blue outline.
 * Near-critical float tails (`0 < total_float ≤ near-critical`) are amber. A today
 * line and a project-end marker anchor the axis.
 *
 * Dual representation (DSC §7.8): the SVG is `aria-hidden` and a parallel,
 * visually-hidden data table carries the same schedule for assistive tech. Cascade
 * slips (`slipped-ids`) pulse Safety Red, suppressed under `prefers-reduced-motion`.
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
      .axis {
        stroke: var(--fb-border, #3a3d44);
        stroke-width: 1;
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
    `,
  ];

  @property({ type: Array }) tasks: ProjectTask[] = [];
  /** Project end (RFC3339) for the end marker; ignored when zero-value. */
  @property({ type: String, attribute: 'project-end' }) projectEnd = '';
  /** Float ≤ this (and > 0) colors the tail amber (OQ-7, product default 2d). */
  @property({ type: Number, attribute: 'near-critical' }) nearCritical = 2;
  /** Task ids whose dates moved later in the last recalc (cascade pulse). */
  @property({ type: Array, attribute: 'slipped-ids' }) slippedIds: string[] = [];

  private place(origin: number): Placed[] {
    const placed: Placed[] = [];
    for (const task of this.tasks) {
      const esDay = dayOf(task.early_start, origin);
      const efDay = dayOf(task.early_finish, origin);
      if (esDay === null || efDay === null) continue;
      const lfDay = dayOf(task.late_finish, origin) ?? efDay;
      placed.push({ task, esDay, efDay, lfDay });
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

  private renderRow(p: Placed, i: number): SVGTemplateResult {
    const y = AXIS_H + i * ROW_H + 4;
    const h = ROW_H - 10;
    const x = PAD + p.esDay * PX_PER_DAY;
    const w = Math.max(PX_PER_DAY, (p.efDay - p.esDay) * PX_PER_DAY);
    const tailW = Math.max(0, (p.lfDay - p.efDay) * PX_PER_DAY);
    const float = p.task.total_float ?? 0;
    const near = float > 0 && float <= this.nearCritical;
    const slipped = this.slippedIds.includes(p.task.id);
    const cls = `bar ${p.task.is_critical ? 'critical' : 'normal'}${slipped ? ' slipped' : ''}`;
    // Each dynamic fragment is the sole child of a wrapped <g>: happy-dom drops a
    // child-binding that sits as a direct sibling of another element at a template
    // root, so the tail line is isolated in its own group to survive the commit.
    return svg`<g class="row">
      <text class="wbs" x=${PAD} y=${y + h - 1}>${p.task.wbs_code}</text>
      <g class="tail-host">${
        tailW > 0
          ? svg`<line class="tail ${near ? 'near' : ''}" x1=${x + w} y1=${y + h / 2} x2=${
              x + w + tailW
            } y2=${y + h / 2}></line>`
          : nothing
      }</g>
      <rect class=${cls} x=${x} y=${y} width=${w} height=${h} rx="3"></rect>
    </g>`;
  }

  /** Accessible parallel table (DSC §7.8) — the SVG is decorative for SR users. */
  private renderTable(): TemplateResult {
    return html`<table class="visually-hidden">
      <caption>
        Project schedule (critical path, early/late dates, and float per task)
      </caption>
      <thead>
        <tr>
          <th scope="col">WBS</th>
          <th scope="col">Task</th>
          <th scope="col">Early start</th>
          <th scope="col">Early finish</th>
          <th scope="col">Late finish</th>
          <th scope="col">Float (days)</th>
          <th scope="col">Critical</th>
        </tr>
      </thead>
      <tbody>
        ${this.tasks.map(
          (t) =>
            html`<tr>
              <td>${t.wbs_code}</td>
              <td>${t.name}</td>
              <td>${t.early_start ?? '—'}</td>
              <td>${t.early_finish ?? '—'}</td>
              <td>${t.late_finish ?? '—'}</td>
              <td>${t.total_float ?? '—'}</td>
              <td>${t.is_critical ? 'Yes' : 'No'}</td>
            </tr>`,
        )}
      </tbody>
    </table>`;
  }

  override render(): TemplateResult {
    const origin = this.origin();
    if (origin === null || this.tasks.length === 0) return this.renderTable();

    const placed = this.place(origin);
    const endDay = dayOf(this.projectEnd, origin);
    const todayDay = Math.round((Date.now() - origin) / DAY_MS);

    let maxDay = 1;
    for (const p of placed) maxDay = Math.max(maxDay, p.lfDay, p.efDay);
    if (endDay !== null) maxDay = Math.max(maxDay, endDay);
    maxDay = Math.max(maxDay, todayDay);

    const width = PAD * 2 + maxDay * PX_PER_DAY + 40;
    const height = AXIS_H + placed.length * ROW_H + PAD;
    const todayX = PAD + todayDay * PX_PER_DAY;
    const endX = endDay !== null ? PAD + endDay * PX_PER_DAY : null;

    const todayMarker =
      todayDay >= 0 && todayDay <= maxDay
        ? svg`<line class="today" x1=${todayX} y1=${AXIS_H} x2=${todayX} y2=${height}></line>
            <text class="marker-label" x=${todayX + 2} y=${AXIS_H - 4}>today</text>`
        : nothing;
    const endMarker =
      endX !== null
        ? svg`<line class="project-end" x1=${endX} y1=${AXIS_H} x2=${endX} y2=${height}></line>
            <text class="marker-label" x=${endX + 2} y=${AXIS_H - 4}>end</text>`
        : nothing;

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
      <line class="axis" x1=${PAD} y1=${AXIS_H} x2=${width - PAD} y2=${AXIS_H}></line>
      ${todayMarker}${endMarker}${placed.map((p, i) => this.renderRow(p, i))}
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
