import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-select.js';
import '../atoms/fb-switch.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-state.js';
import '../organisms/fb-gantt-chart.js';
import {
  getGantt,
  recalculateSchedule,
  recommendAdjustments,
  applyAdjustments,
} from '../../api/endpoints/schedule.js';
import { listProjects } from '../../api/endpoints/projects.js';
import type {
  GanttView,
  ProjectTask,
  Project,
  ScheduleAdjustment,
  ScheduleAdjustmentSet,
  TaskDependency,
} from '../../types/models.js';
import '../atoms/fb-checkbox.js';
import '../atoms/fb-markdown.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { aiConfigured, markAiUnconfigured } from '../../state/capabilityStore.js';
import { hasMinRole, hasRole } from '../../state/authStore.js';
import { navigate } from '../../router.js';
import type { SelectOption } from '../atoms/fb-select.js';

/** AI drawer state — mirrors the briefing hero gating (§9). */
type AiState = 'idle' | 'loading' | 'ok' | 'gated' | 'transient';

/** A schedule is "never computed" when the critical path is empty (zero-value end). */
function neverComputed(g: GanttView | null): boolean {
  if (!g) return true;
  if (g.critical_path.length > 0) return false;
  return !g.project_end || g.project_end.startsWith('0001-01-01');
}

/**
 * `fb-schedule-page` — the CPM schedule workspace (UX_CORE_SCREENS §2–§3). Any
 * authenticated role may read; recalc and AI adjustments are min-superintendent
 * (router + button gates). The route carries no path param, so the page leads
 * with a project picker. The Gantt (`fb-gantt-chart`) is the centerpiece; a
 * "Critical path only" toggle filters non-critical bars. Recalc runs the physics
 * engine server-side, reports `recalculation_ms`, and diffs early-finish dates to
 * pulse the tasks that slipped (cascade diff). The AI "Suggest adjustments" drawer
 * (§3) is a post-hoc transparency view, gated on native-AI availability.
 */
@customElement('fb-schedule-page')
export class FbSchedulePage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .toolbar {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-md);
        flex-wrap: wrap;
      }
      .picker {
        max-width: 28rem;
        flex: 1 1 16rem;
      }
      .toolbar-spacer {
        flex: 1 1 auto;
      }
      .recalc-meta {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .gantt-wrap {
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
        overflow: hidden;
      }
      .legend {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-md);
        margin-top: var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .legend span {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
      }
      .swatch {
        width: 12px;
        height: 12px;
        border-radius: 2px;
        flex: none;
      }
      .swatch.critical {
        background: var(--fb-gable-green, #00ffa3);
      }
      .swatch.normal {
        background: var(--fb-slate-steel, #1e2029);
        border: 1px solid var(--fb-blueprint-blue, #38bdf8);
      }
      .swatch.float {
        background: transparent;
        border-top: 2px dashed var(--fb-amber, #f59e0b);
        border-radius: 0;
        height: 0;
      }
      /* AI adjustments drawer (PREVIEW-FIRST: proposed vs advisory sections) */
      .adj-list {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
      }
      .adj-section {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
      }
      .adj-section-head {
        margin: 0;
        font-size: var(--fb-text-body-sm);
        font-weight: 600;
        color: var(--fb-text-secondary);
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }
      .adj-rows {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
      }
      .adj {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        padding: var(--fb-spacing-sm) 0;
        border-bottom: 1px solid var(--fb-border);
      }
      .adj:last-child {
        border-bottom: none;
      }
      .adj-head {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        flex-wrap: wrap;
      }
      .adj-name {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .adj-delta {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
        margin-left: auto;
      }
      .adj-rationale {
        margin: 0;
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
        line-height: 1.5;
      }
      .adj-summary {
        margin: 0 0 var(--fb-spacing-xs) 0;
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      .banner.ok {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin: 0;
        color: var(--fb-gable-green, #00ffa3);
        font-size: var(--fb-text-body-sm);
      }
      /* Task-detail drawer */
      .detail {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
      }
      .detail-badges {
        display: flex;
        gap: var(--fb-spacing-sm);
        flex-wrap: wrap;
      }
      .detail-grid {
        display: grid;
        grid-template-columns: auto 1fr;
        gap: var(--fb-spacing-xs) var(--fb-spacing-md);
        margin: 0;
      }
      .detail-grid dt {
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      .detail-grid dd {
        margin: 0;
        color: var(--fb-text-primary);
        font-size: var(--fb-text-body-sm);
      }
      .detail-grid dd.mono {
        font-family: var(--fb-font-mono);
      }
      .swatch.dep {
        background: transparent;
        border-top: 2px solid var(--fb-blueprint-blue, #38bdf8);
        border-radius: 0;
        height: 0;
      }
    `,
  ];

  @state() private projects: Project[] = [];
  @state() private projectId = '';
  @state() private gantt: GanttView | null = null;
  @state() private loading = true;
  @state() private ganttLoading = false;
  @state() private errorCode: string | null = null;
  @state() private ganttError: string | null = null;

  @state() private criticalOnly = false;
  // Task-detail drawer (click-to-inspect a bar/row).
  @state() private selectedTaskId: string | null = null;
  @state() private recalcBusy = false;
  @state() private recalcMs: number | null = null;
  @state() private slippedIds: string[] = [];
  @state() private cascadeNotice: string | null = null;
  @state() private recalcError: string | null = null;

  // AI adjustments drawer (E2 / Chunk 2b — PREVIEW-FIRST: AI proposes, human commits).
  @state() private adjOpen = false;
  @state() private aiState: AiState = 'idle';
  @state() private adjustments: ScheduleAdjustmentSet | null = null;
  @state() private adjErrorRequestId: string | null = null;
  // Per-row apply selection, keyed by wbs_code (only proposed-change rows are selectable).
  @state() private selectedWbs: Set<string> = new Set();
  @state() private applying = false;
  @state() private applyError: string | null = null;
  // Set after a successful apply so the modal shows a confirmation.
  @state() private appliedCount: number | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.loadProjects();
  }

  private async loadProjects(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.projects = await listProjects();
      const first = this.projects[0];
      if (first) {
        this.projectId = first.id;
        void this.loadGantt();
      }
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private async loadGantt(): Promise<void> {
    if (!this.projectId) return;
    this.ganttLoading = true;
    this.ganttError = null;
    try {
      this.gantt = await getGantt(this.projectId);
    } catch (err) {
      this.ganttError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.ganttLoading = false;
    }
  }

  private onProject(e: Event): void {
    this.projectId = (e as CustomEvent<{ value: string }>).detail.value;
    // Reset per-project derived state.
    this.recalcMs = null;
    this.slippedIds = [];
    this.cascadeNotice = null;
    this.recalcError = null;
    void this.loadGantt();
  }

  private projectOptions(): SelectOption[] {
    return this.projects.map((p) => ({ value: p.id, label: p.name }));
  }

  /** Recalc: snapshot early-finish, run physics, reload, then pulse slipped tasks. */
  private async recalc(): Promise<void> {
    if (!this.projectId) return;
    this.recalcBusy = true;
    this.recalcError = null;
    this.cascadeNotice = null;
    const before = new Map<string, number>();
    for (const t of this.gantt?.tasks ?? []) {
      if (t.early_finish) before.set(t.id, new Date(t.early_finish).getTime());
    }
    try {
      const result = await recalculateSchedule(this.projectId);
      this.recalcMs = result.recalculation_ms;
      await this.loadGantt();
      const slipped: string[] = [];
      for (const t of this.gantt?.tasks ?? []) {
        const prev = before.get(t.id);
        const now = t.early_finish ? new Date(t.early_finish).getTime() : null;
        if (prev !== undefined && now !== null && now > prev) slipped.push(t.id);
      }
      this.slippedIds = slipped;
      if (result.cpm_result.critical_path_changed) {
        this.cascadeNotice =
          slipped.length > 0
            ? `Critical path changed — ${slipped.length} ${slipped.length === 1 ? 'task' : 'tasks'} slipped.`
            : 'Critical path changed.';
      } else if (slipped.length > 0) {
        this.cascadeNotice = `${slipped.length} ${slipped.length === 1 ? 'task' : 'tasks'} moved.`;
      }
    } catch (err) {
      this.recalcError =
        err instanceof ApiError ? userMessageForCode(err.code) : 'Could not recalculate.';
    } finally {
      this.recalcBusy = false;
    }
  }

  // ----------------- AI adjustments (PREVIEW-FIRST, Chunk 2b) -----------------
  // The "Suggest adjustments" flow is a DRY-RUN: the AI proposes per-row duration
  // changes that mutate nothing; the user selects rows and commits them via the
  // apply endpoint, which writes the durations + re-runs CPM.
  private openAdjustments(): void {
    this.adjOpen = true;
    this.adjustments = null;
    this.adjErrorRequestId = null;
    this.selectedWbs = new Set();
    this.applyError = null;
    this.appliedCount = null;
    if (!aiConfigured.get()) {
      this.aiState = 'gated';
      return;
    }
    void this.loadAdjustments();
  }

  private closeAdjustments(): void {
    this.adjOpen = false;
  }

  private async loadAdjustments(): Promise<void> {
    if (!this.projectId) return;
    this.aiState = 'loading';
    this.adjErrorRequestId = null;
    this.applyError = null;
    this.appliedCount = null;
    try {
      // dry_run=true — propose only, mutate nothing.
      const set = await recommendAdjustments(this.projectId, true);
      this.adjustments = set;
      // Pre-select every proposed change so "Apply all" is the default.
      this.selectedWbs = new Set(
        set.adjustments.filter((a) => a.proposed_change).map((a) => a.wbs_code),
      );
      this.aiState = 'ok';
    } catch (err) {
      if (err instanceof ApiError) {
        this.adjErrorRequestId = err.requestId ?? null;
        if (err.code === ErrorCode.SERVICE_UNAVAILABLE || err.isAiUnconfigured) {
          markAiUnconfigured();
          this.aiState = 'gated';
        } else {
          this.aiState = 'transient';
        }
      } else {
        this.aiState = 'transient';
      }
    }
  }

  /** Proposed-change rows (a real duration change the user can apply). */
  private proposedRows(): ScheduleAdjustment[] {
    return (this.adjustments?.adjustments ?? []).filter((a) => a.proposed_change);
  }

  /** Advisory / monitor-only rows (no apply control). */
  private advisoryRows(): ScheduleAdjustment[] {
    return (this.adjustments?.adjustments ?? []).filter((a) => !a.proposed_change);
  }

  private toggleRow(wbs: string, checked: boolean): void {
    const next = new Set(this.selectedWbs);
    if (checked) next.add(wbs);
    else next.delete(wbs);
    this.selectedWbs = next;
  }

  private selectAll(): void {
    this.selectedWbs = new Set(this.proposedRows().map((a) => a.wbs_code));
  }

  /** Commit the selected proposals via the apply endpoint, then refresh the board. */
  private async applySelected(): Promise<void> {
    if (!this.projectId || this.applying) return;
    const rows = this.proposedRows().filter(
      (a) => this.selectedWbs.has(a.wbs_code) && a.new_duration_days !== undefined,
    );
    if (rows.length === 0) return;
    this.applying = true;
    this.applyError = null;
    try {
      const result = await applyAdjustments(
        this.projectId,
        rows.map((a) => ({ wbs_code: a.wbs_code, new_duration_days: a.new_duration_days! })),
      );
      this.appliedCount = result.applied_deltas;
      // The server wrote durations + re-ran CPM; refresh the Gantt so the
      // schedule visibly recomputes.
      await this.loadGantt();
    } catch (err) {
      this.applyError =
        err instanceof ApiError ? userMessageForCode(err.code) : 'Could not apply adjustments.';
    } finally {
      this.applying = false;
    }
  }

  private async applyAll(): Promise<void> {
    this.selectAll();
    await this.applySelected();
  }

  // --------------------------- Task detail (click-to-inspect) ---------------------------
  private openTaskDetail(id: string): void {
    this.selectedTaskId = id;
  }

  private closeTaskDetail(): void {
    this.selectedTaskId = null;
  }

  private selectedTask(): ProjectTask | null {
    if (!this.selectedTaskId) return null;
    return this.gantt?.tasks.find((t) => t.id === this.selectedTaskId) ?? null;
  }

  // ------------------------------- Render -------------------------------
  private visibleTasks(): ProjectTask[] {
    const tasks = this.gantt?.tasks ?? [];
    return this.criticalOnly ? tasks.filter((t) => t.is_critical) : tasks;
  }

  /**
   * Dependencies whose BOTH endpoints survive the critical-path filter — so we
   * never draw an arrow to a hidden bar. With the filter off, all edges pass.
   */
  private visibleDependencies(): TaskDependency[] {
    const deps = this.gantt?.dependencies ?? [];
    if (!this.criticalOnly) return deps;
    const visible = new Set(this.visibleTasks().map((t) => t.id));
    return deps.filter((d) => visible.has(d.predecessor_id) && visible.has(d.successor_id));
  }

  private renderAdjBody(): TemplateResult {
    if (this.aiState === 'loading')
      return html`<fb-state mode="loading" skeleton="text" rows="4"></fb-state>`;

    if (this.aiState === 'gated')
      return html`<fb-state
        mode="gated"
        heading="AI adjustments are off"
        message="Add an Anthropic API key to enable AI schedule suggestions."
        ?can-configure=${hasRole('owner')}
        @configure=${() => navigate('/settings/integrations')}
      ></fb-state>`;

    if (this.aiState === 'transient')
      return html`<fb-state
        mode="error"
        error-code=${ErrorCode.UPSTREAM_ERROR}
        request-id=${this.adjErrorRequestId ?? nothing}
        retryable
        @retry=${() => void this.loadAdjustments()}
      ></fb-state>`;

    const set = this.adjustments;
    if (!set) return html`${nothing}`;
    if (set.adjustments.length === 0)
      return html`<fb-state
        mode="empty"
        icon="sparkles"
        heading="No adjustments suggested"
        message="The model reviewed the schedule and recommended no duration changes."
      ></fb-state>`;

    const proposed = this.proposedRows();
    const advisory = this.advisoryRows();
    return html`<div class="adj-list">
      <p class="adj-summary" role="status">
        <strong>${proposed.length}</strong>
        ${proposed.length === 1 ? 'proposed change' : 'proposed changes'} ·
        <strong>${advisory.length}</strong> advisory
      </p>

      ${this.appliedCount !== null
        ? html`<p class="banner ok" role="status">
            <fb-icon name="check-circle" size="16"></fb-icon>Applied ${this.appliedCount}
            ${this.appliedCount === 1 ? 'change' : 'changes'}; the schedule recomputed.
          </p>`
        : nothing}
      ${this.applyError
        ? html`<p class="toast err" role="alert">
            <fb-icon name="alert-circle" size="16"></fb-icon>${this.applyError}
          </p>`
        : nothing}
      ${proposed.length > 0
        ? html`<section aria-label="Proposed changes" class="adj-section">
            <h3 class="adj-section-head">Proposed changes</h3>
            <ul class="adj-rows">
              ${proposed.map((a) => this.renderProposedRow(a))}
            </ul>
          </section>`
        : nothing}
      ${advisory.length > 0
        ? html`<section aria-label="Advisory" class="adj-section">
            <h3 class="adj-section-head">Advisory (monitor only)</h3>
            <ul class="adj-rows">
              ${advisory.map(
                (a) =>
                  html`<li class="adj">
                    <div class="adj-head">
                      <fb-badge size="sm" status="neutral">Advisory</fb-badge>
                      <span class="adj-name">${a.wbs_code} · ${a.name}</span>
                      ${a.is_critical
                        ? html`<fb-badge size="sm" status="critical">Critical path</fb-badge>`
                        : nothing}
                    </div>
                    <fb-markdown class="adj-rationale" .source=${a.rationale}></fb-markdown>
                  </li>`,
              )}
            </ul>
          </section>`
        : nothing}
    </div>`;
  }

  /** One proposed-change row: identity, old→new delta, rationale, per-row apply toggle. */
  private renderProposedRow(a: ScheduleAdjustment): TemplateResult {
    const selected = this.selectedWbs.has(a.wbs_code);
    const label = `${a.wbs_code} ${a.name}, ${a.old_duration_days} to ${a.new_duration_days} days`;
    return html`<li class="adj">
      <div class="adj-head">
        <fb-checkbox
          .checked=${selected}
          ?disabled=${this.applying}
          aria-label=${`Apply: ${label}`}
          @change=${(e: Event) =>
            this.toggleRow(a.wbs_code, (e as CustomEvent<{ checked: boolean }>).detail.checked)}
        ></fb-checkbox>
        <span class="adj-name">${a.wbs_code} · ${a.name}</span>
        ${a.is_critical
          ? html`<fb-badge size="sm" status="critical">Critical path</fb-badge>`
          : nothing}
        <span
          class="adj-delta"
          aria-label=${`${a.old_duration_days} to ${a.new_duration_days} days`}
          >${a.old_duration_days}d → ${a.new_duration_days}d</span
        >
      </div>
      <fb-markdown class="adj-rationale" .source=${a.rationale}></fb-markdown>
    </li>`;
  }

  private renderDrawer(): TemplateResult {
    if (!this.adjOpen) return html`${nothing}`;
    const canApply = this.aiState === 'ok' && this.proposedRows().length > 0 && !this.applying;
    const selectedCount = this.proposedRows().filter((a) =>
      this.selectedWbs.has(a.wbs_code),
    ).length;
    return html`<fb-modal open heading="AI schedule adjustments" @close=${this.closeAdjustments}>
      ${this.renderAdjBody()}
      <fb-button slot="footer" variant="ghost" @click=${this.closeAdjustments}>Close</fb-button>
      ${canApply
        ? html`<fb-button
              slot="footer"
              variant="secondary"
              ?disabled=${this.applying}
              ?loading=${this.applying}
              @click=${() => void this.applyAll()}
              >Apply all</fb-button
            >
            <fb-button
              slot="footer"
              variant="primary"
              icon="check"
              ?disabled=${this.applying || selectedCount === 0}
              ?loading=${this.applying}
              @click=${() => void this.applySelected()}
              >Apply selected${selectedCount > 0 ? ` (${selectedCount})` : ''}</fb-button
            >`
        : nothing}
    </fb-modal>`;
  }

  /** Read-only task-detail drawer opened by clicking a Gantt bar/row. */
  private renderTaskDetail(): TemplateResult {
    const t = this.selectedTask();
    if (!t) return html`${nothing}`;
    const fmt = (d?: string): string => d ?? '—';
    const heading = `${t.wbs_code} · ${t.name}`;
    return html`<fb-modal open heading=${heading} @close=${this.closeTaskDetail}>
      <div class="detail">
        <div class="detail-badges">
          <fb-badge size="sm" status=${t.is_critical ? 'critical' : 'neutral'}>
            ${t.is_critical ? 'Critical path' : 'Has float'}
          </fb-badge>
          <fb-badge size="sm" status=${t.status === 'completed' ? 'complete' : 'neutral'}>
            ${t.status}
          </fb-badge>
        </div>
        <dl class="detail-grid">
          <dt>Early start</dt>
          <dd class="mono">${fmt(t.early_start)}</dd>
          <dt>Early finish</dt>
          <dd class="mono">${fmt(t.early_finish)}</dd>
          <dt>Late start</dt>
          <dd class="mono">${fmt(t.late_start)}</dd>
          <dt>Late finish</dt>
          <dd class="mono">${fmt(t.late_finish)}</dd>
          <dt>Total float</dt>
          <dd class="mono">${t.total_float ?? '—'}d</dd>
          <dt>Duration</dt>
          <dd class="mono">${t.duration_days}d</dd>
          <dt>Percent complete</dt>
          <dd class="mono">${t.percent_complete}%</dd>
          <dt>Assigned crew</dt>
          <dd>${t.assigned_crew && t.assigned_crew.length > 0 ? t.assigned_crew.length : '—'}</dd>
        </dl>
      </div>
      <fb-button slot="footer" variant="ghost" @click=${this.closeTaskDetail}>Close</fb-button>
    </fb-modal>`;
  }

  private renderChart(): TemplateResult {
    if (this.ganttLoading)
      return html`<fb-state mode="loading" skeleton="card" rows="6"></fb-state>`;
    if (this.ganttError)
      return html`<fb-state
        mode="error"
        error-code=${this.ganttError}
        retryable
        @retry=${() => void this.loadGantt()}
      ></fb-state>`;
    if (neverComputed(this.gantt))
      return html`<fb-state
        mode="empty"
        icon="calendar"
        heading="Schedule not computed yet"
        message="Recalculate to run the critical-path engine and draw the timeline."
      >
        ${hasMinRole('superintendent')
          ? html`<fb-button
              slot="action"
              variant="primary"
              icon="refresh"
              ?loading=${this.recalcBusy}
              @click=${() => void this.recalc()}
              >Recalculate</fb-button
            >`
          : nothing}
      </fb-state>`;

    return html`<div class="gantt-wrap">
      <fb-gantt-chart
        .tasks=${this.visibleTasks()}
        .dependencies=${this.visibleDependencies()}
        project-end=${this.gantt?.project_end ?? ''}
        .slippedIds=${this.slippedIds}
        @task-select=${(e: Event) =>
          this.openTaskDetail((e as CustomEvent<{ id: string }>).detail.id)}
      ></fb-gantt-chart>
      <div class="legend">
        <span><span class="swatch critical"></span>Critical path</span>
        <span><span class="swatch normal"></span>Has float</span>
        <span><span class="swatch float"></span>Near-critical float (≤2d)</span>
        <span><span class="swatch dep"></span>Dependency (finish→start)</span>
      </div>
    </div>`;
  }

  private renderWorkspace(): TemplateResult {
    const canRecalc = hasMinRole('superintendent');
    return html`<div class="workspace">
      <div class="toolbar">
        <div class="picker">
          <fb-select
            label="Project"
            .options=${this.projectOptions()}
            value=${this.projectId}
            @change=${this.onProject}
          ></fb-select>
        </div>
        <fb-switch
          label="Critical path only"
          .checked=${this.criticalOnly}
          @change=${(e: Event) =>
            (this.criticalOnly = (e as CustomEvent<{ checked: boolean }>).detail.checked)}
          >Critical path only</fb-switch
        >
        <div class="toolbar-spacer"></div>
        ${this.recalcMs !== null
          ? html`<span class="recalc-meta"
              ><fb-icon name="clock" size="14"></fb-icon>recomputed in ${this.recalcMs}ms</span
            >`
          : nothing}
        ${canRecalc
          ? html`<fb-button
                variant="secondary"
                icon="sparkles"
                @click=${() => this.openAdjustments()}
                >Suggest adjustments</fb-button
              >
              <fb-button
                variant="primary"
                icon="refresh"
                ?loading=${this.recalcBusy}
                @click=${() => void this.recalc()}
                >Recalculate</fb-button
              >`
          : nothing}
      </div>

      ${this.recalcError
        ? html`<p class="toast err" role="alert">
            <fb-icon name="alert-circle" size="16"></fb-icon>${this.recalcError}
          </p>`
        : nothing}
      ${this.cascadeNotice
        ? html`<p class="banner warn" role="status">
            <fb-icon name="alert-triangle" size="16"></fb-icon>${this.cascadeNotice}
          </p>`
        : nothing}
      ${this.renderChart()}
    </div>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Schedule</h1>
            <p class="page-sub">The critical-path timeline — recompute to cascade any slips.</p>
          </div>
        </div>

        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="6"></fb-state>`
          : this.errorCode
            ? html`<fb-state
                mode="error"
                error-code=${this.errorCode}
                retryable
                @retry=${() => void this.loadProjects()}
              ></fb-state>`
            : this.projects.length === 0
              ? html`<fb-state
                  mode="empty"
                  icon="folder"
                  heading="No projects yet"
                  message="Schedules are computed per project."
                ></fb-state>`
              : this.renderWorkspace()}
        ${this.renderDrawer()}${this.renderTaskDetail()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-schedule-page': FbSchedulePage;
  }
}
