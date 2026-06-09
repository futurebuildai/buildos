import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { SignalWatcher } from '@lit-labs/signals';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-text.js';
import '../atoms/fb-button.js';
import '../atoms/fb-switch.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-input.js';
import '../molecules/fb-field.js';
import '../molecules/fb-form.js';
import '../organisms/fb-state.js';
import '../organisms/fb-confirm.js';
import { listAgents, setAgent, resetAgent } from '../../api/endpoints/admin.js';
import {
  type AgentCapability,
  type EffectiveAgentConfig,
  type ForesightConfig,
  readForesightConfig,
} from '../../types/models.js';
import { ApiError, ErrorCode } from '../../api/errors.js';
import { aiConfigured } from '../../state/capabilityStore.js';
import { hasRole } from '../../state/authStore.js';
import type { FbForm } from '../molecules/fb-form.js';

/**
 * Hand-authored, plain-language framing for each catalog capability. The backend
 * `description` is the raw catalog slug-sentence; the builder admin sees this
 * friendlier "what it does for you" copy instead (spec §4.1). The titles double
 * as the `fb-switch` accessible name ("Enable {title}").
 */
interface AgentCopy {
  title: string;
  blurb: string;
}
const AGENT_COPY: Record<AgentCapability, AgentCopy> = {
  delay_cascade: {
    title: 'Schedule-delay ripple',
    blurb:
      'When a task slips, automatically work out what else is affected — procurement, crews, budget — and post it to your feed.',
  },
  foresight: {
    title: 'Risk early-warning',
    blurb:
      'Keep an eye on every project and warn you about budget overruns, tight schedules, and at-risk material orders before they bite.',
  },
  experience: {
    title: 'AI assistant (chat)',
    blurb:
      'Let your team ask BuildOS questions in plain English and get answers grounded in your live data.',
  },
};

/** Render order — match the catalog ordering used elsewhere. */
const ORDER: AgentCapability[] = ['delay_cascade', 'foresight', 'experience'];

/** Human labels for the two foresight thresholds; also the fb-form summary keys. */
const FORESIGHT_LABELS = {
  schedule_float_days: 'Schedule float (days)',
  budget_burn_percent: 'Budget spent warning (%)',
} as const;

/**
 * `fb-agents-page` — Settings → AI Agents (`/settings/agents`, Phase 3c §4).
 *
 * Admin+ surface (route-gated; **NOT** plan-gated — ESC-002: the kill-switches
 * must reach admins on any tier) to enable/disable + tune the three catalog
 * capabilities. Three cards are rendered inline (no molecule — one consumer).
 *
 * Correctness contract (spec §4.3, rules 1-5):
 *  - The enable toggle and the foresight Save share ONE full-document PUT where an
 *    omitted/`{}` config RESETS tuning to the catalog default. So the toggle
 *    ALWAYS resends `config: savedConfig` — the last server-confirmed config read
 *    straight off the loaded agent — never live form inputs.
 *  - `fb-form` yields control values as STRINGS; foresight Save `Number()`-coerces
 *    and client-validates `Number.isInteger && >= 1` BEFORE the PUT.
 *  - After ANY toggle (success or failure) `await this.load()` in `finally` to
 *    re-derive the switch from server truth (`fb-switch` self-mutates + a
 *    same-value re-render is a Lit no-op).
 *  - VALIDATION_ERROR consumes `err.details[]` (wire field → human label → the
 *    right `fb-field`/`fb-form` error); never routed through `userMessageForCode`.
 */
@customElement('fb-agents-page')
export class FbAgentsPage extends SignalWatcher(FBElement) {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
        max-width: 720px;
      }
      .head {
        margin-bottom: var(--fb-spacing-lg);
      }
      .title {
        margin: 0 0 var(--fb-spacing-xs);
        font-size: var(--fb-text-headline-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .sub {
        margin: 0;
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      .grid {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-lg);
      }
      .card {
        padding: var(--fb-spacing-md) var(--fb-spacing-lg);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
      .card[aria-busy='true'] {
        opacity: 0.75;
      }
      .card-head {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--fb-spacing-md);
      }
      .card-title {
        margin: 0 0 var(--fb-spacing-xs);
        font-size: var(--fb-text-title-md);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .card-blurb {
        margin: 0;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .card-foot {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-md);
        margin-top: var(--fb-spacing-md);
      }
      .cross-note {
        margin: var(--fb-spacing-sm) 0 0;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .dep-row {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
        background: var(--fb-surface-2);
        border: 1px solid var(--fb-glass-border);
        border-radius: var(--fb-radius-sm);
      }
      .dep-row fb-icon {
        color: var(--fb-text-muted);
        flex: none;
      }
      .dep-row a {
        color: var(--fb-gable-green);
      }
      .form {
        margin-top: var(--fb-spacing-md);
        padding-top: var(--fb-spacing-md);
        border-top: 1px solid var(--fb-glass-border);
      }
      .fields {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
      }
      .preview {
        margin: var(--fb-spacing-sm) 0 var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
      }
      .num {
        max-width: 160px;
      }
      .banner {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        margin-bottom: var(--fb-spacing-lg);
        padding: var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-amber-warning, #f59e0b);
        background: color-mix(in srgb, var(--fb-amber-warning, #f59e0b) 12%, transparent);
        border: 1px solid var(--fb-amber-warning, #f59e0b);
        border-radius: var(--fb-radius-sm);
      }
      .banner fb-icon {
        flex: none;
      }
      .toast {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-bottom: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        border-radius: var(--fb-radius-sm);
      }
      .toast.ok {
        color: var(--fb-gable-green);
        background: color-mix(in srgb, var(--fb-gable-green) 12%, transparent);
        border: 1px solid var(--fb-gable-green);
      }
      .toast.err {
        color: var(--fb-safety-red);
        background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
        border: 1px solid var(--fb-safety-red);
      }
    `,
  ];

  @state() private agents: EffectiveAgentConfig[] = [];
  @state() private loading = true;
  @state() private loadError: ApiError | null = null;
  @state() private notice: { kind: 'ok' | 'err'; text: string } | null = null;
  /** Capabilities with a write in flight — serializes per-capability + drives aria-busy. */
  @state() private busy = new Set<AgentCapability>();
  /** The capability whose foresight reset confirm is open (only ever 'foresight'). */
  @state() private confirmReset: AgentCapability | null = null;
  /** Capability whose acted-on control should regain focus after a refetch. */
  private refocus: AgentCapability | null = null;
  /**
   * Per-field foresight errors (wire field → message), bound to each `fb-field`'s
   * `error` prop so the offending input gets `aria-invalid` + `aria-describedby`.
   * The fb-form summary (role=alert) announces; this lands the error ON the field.
   */
  @state() private foresightErrors: {
    schedule_float_days?: string;
    budget_burn_percent?: string;
  } = {};

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.loadError = null;
    try {
      this.agents = await listAgents();
    } catch (err) {
      this.loadError =
        err instanceof ApiError
          ? err
          : new ApiError({
              code: ErrorCode.UNKNOWN,
              message: 'load failed',
              status: 0,
            });
    } finally {
      this.loading = false;
    }
  }

  override updated(): void {
    // Restore focus to the acted-on card's switch after a full re-render — inline
    // card actions otherwise drop focus to <body> on refetch (WCAG 2.4.3).
    if (this.refocus && !this.loading) {
      const cap = this.refocus;
      this.refocus = null;
      // fb-switch has no focus delegation, so host.focus() is a no-op — reach the
      // inner native control (mirrors focusNameOrEndpoint in fb-connectors-page).
      const sw = this.renderRoot.querySelector(`fb-switch[data-cap='${cap}']`);
      const inner = sw?.shadowRoot?.querySelector<HTMLElement>('input');
      (inner ?? (sw as HTMLElement | null))?.focus();
    }
  }

  private agent(cap: AgentCapability): EffectiveAgentConfig | undefined {
    return this.agents.find((a) => a.capability === cap);
  }

  private setBusy(cap: AgentCapability, on: boolean): void {
    const next = new Set(this.busy);
    if (on) next.add(cap);
    else next.delete(cap);
    this.busy = next;
  }

  private foresightForm(): FbForm | null {
    return this.renderRoot.querySelector<FbForm>("fb-form[data-cap='foresight']");
  }

  // --------------------------- enable toggle ---------------------------

  private async onToggle(cap: AgentCapability, e: Event): Promise<void> {
    if (this.busy.has(cap)) return; // explicit double-submit gate (fb-switch has none)
    const enabled = (e as CustomEvent<{ checked: boolean }>).detail.checked;
    const current = this.agent(cap);
    this.notice = null;
    this.setBusy(cap, true);
    try {
      // CRITICAL: a full-document PUT — resend the LAST SERVER-CONFIRMED config so
      // the toggle never resets foresight tuning. delay_cascade/experience carry
      // `{}` (no tunable keys); their config round-trips as the empty object.
      await setAgent(cap, { enabled, config: current?.config ?? {} });
      const title = AGENT_COPY[cap].title;
      this.notice = {
        kind: 'ok',
        text: enabled ? `${title} is now on.` : `${title} is now off.`,
      };
    } catch (err) {
      this.notice = {
        kind: 'err',
        text:
          err instanceof ApiError && err.code === ErrorCode.VALIDATION_ERROR
            ? (this.firstDetailMessage(err) ?? `Couldn't update ${AGENT_COPY[cap].title}.`)
            : `Couldn't update ${AGENT_COPY[cap].title}.`,
      };
    } finally {
      this.refocus = cap;
      // CRITICAL: re-derive the switch from server truth after success OR failure.
      await this.load();
      this.setBusy(cap, false);
    }
  }

  // --------------------------- foresight save ---------------------------

  private async onForesightSave(e: Event): Promise<void> {
    const cap: AgentCapability = 'foresight';
    if (this.busy.has(cap)) return;
    const form = this.foresightForm();
    const values = (e as CustomEvent<{ values: Record<string, string> }>).detail.values;
    const current = this.agent(cap);
    if (!current) return;

    // fb-form yields STRINGS. Coerce + client-validate Number.isInteger && >= 1
    // BEFORE any PUT; empty/NaN/0 → fb-form.setErrors keyed by the HUMAN LABEL.
    const fieldErrors: Record<string, string> = {};
    const perField: { schedule_float_days?: string; budget_burn_percent?: string } = {};
    const float = Number(values['schedule_float_days']);
    const burn = Number(values['budget_burn_percent']);
    const INT_MSG = 'Enter a whole number of 1 or more.';
    if (!Number.isInteger(float) || float < 1) {
      fieldErrors[FORESIGHT_LABELS.schedule_float_days] = INT_MSG;
      perField.schedule_float_days = INT_MSG;
    }
    if (!Number.isInteger(burn) || burn < 1) {
      fieldErrors[FORESIGHT_LABELS.budget_burn_percent] = INT_MSG;
      perField.budget_burn_percent = INT_MSG;
    }
    if (Object.keys(fieldErrors).length > 0) {
      form?.setErrors(fieldErrors);
      this.foresightErrors = perField; // also land aria-invalid on the inputs
      return;
    }
    form?.setErrors({});
    this.foresightErrors = {};

    this.notice = null;
    this.setBusy(cap, true);
    const config: ForesightConfig = { schedule_float_days: float, budget_burn_percent: burn };
    try {
      // config sent as an OBJECT (never JSON.stringify'd); keep enabled authoritative.
      const saved = await setAgent(cap, { enabled: current.enabled, config });
      const eff = readForesightConfig(saved.config);
      this.foresightErrors = {};
      this.notice = {
        kind: 'ok',
        text: `Risk early-warning will now warn you at ${eff.budget_burn_percent}% budget and ${eff.schedule_float_days} days of slack.`,
      };
      await this.load();
    } catch (err) {
      if (err instanceof ApiError && err.code === ErrorCode.VALIDATION_ERROR) {
        // Map wire field → human label so aria-invalid lands on the right input.
        const mapped: Record<string, string> = {};
        const perField: { schedule_float_days?: string; budget_burn_percent?: string } = {};
        for (const d of err.details) {
          if (d.field === 'schedule_float_days') {
            mapped[FORESIGHT_LABELS.schedule_float_days] = d.reason;
            perField.schedule_float_days = d.reason;
          } else if (d.field === 'budget_burn_percent') {
            mapped[FORESIGHT_LABELS.budget_burn_percent] = d.reason;
            perField.budget_burn_percent = d.reason;
          } else {
            mapped[d.field] = d.reason;
          }
        }
        form?.setErrors(
          Object.keys(mapped).length > 0
            ? mapped
            : { [FORESIGHT_LABELS.budget_burn_percent]: 'Those thresholds were rejected.' },
        );
        this.foresightErrors =
          Object.keys(perField).length > 0
            ? perField
            : { budget_burn_percent: 'Those thresholds were rejected.' };
      } else {
        this.notice = { kind: 'err', text: 'Couldn’t save the warning thresholds.' };
      }
    } finally {
      this.setBusy(cap, false);
    }
  }

  // ------------------------------ reset ------------------------------

  private async onReset(cap: AgentCapability): Promise<void> {
    if (this.busy.has(cap)) return;
    this.notice = null;
    if (cap === 'foresight') this.foresightErrors = {};
    this.setBusy(cap, true);
    try {
      await resetAgent(cap);
      this.notice = { kind: 'ok', text: `${AGENT_COPY[cap].title} reset to standard settings.` };
    } catch (err) {
      this.notice = {
        kind: 'err',
        text:
          err instanceof ApiError && err.code === ErrorCode.NOT_FOUND
            ? `${AGENT_COPY[cap].title} reset to standard settings.`
            : `Couldn’t reset ${AGENT_COPY[cap].title}.`,
      };
    } finally {
      this.refocus = cap;
      await this.load();
      this.setBusy(cap, false);
    }
  }

  private onResetClick(cap: AgentCapability): void {
    // foresight carries tuned thresholds → confirm before discarding them.
    if (cap === 'foresight') {
      this.confirmReset = cap;
      return;
    }
    void this.onReset(cap);
  }

  private onConfirmReset(): void {
    const cap = this.confirmReset;
    this.confirmReset = null;
    if (cap) void this.onReset(cap);
  }

  /** First VALIDATION_ERROR detail reason, if any (for non-field toggle errors). */
  private firstDetailMessage(err: ApiError): string | undefined {
    return err.details[0]?.reason;
  }

  // ------------------------------ render ------------------------------

  private renderDependencyRow(): TemplateResult {
    // Persistent neutral dependency row (NOT an error) on every card — shown even
    // in assume-on/unknown so the AI-key dependency is never hidden (spec §4.4).
    const owner = hasRole('owner');
    return html`<p class="dep-row" role="note">
      <fb-icon name="key" size="14"></fb-icon>
      <span
        >Agents only run when an Anthropic key is set.${' '}
        ${owner
          ? html`<a href="/settings/integrations">Add an Anthropic key</a>.`
          : html`Ask your owner to add an Anthropic key.`}</span
      >
    </p>`;
  }

  private renderForesightForm(a: EffectiveAgentConfig): TemplateResult {
    const cfg = readForesightConfig(a.config);
    const cardBusy = this.busy.has('foresight');
    return html`
      <div class="form">
        <fb-form
          data-cap="foresight"
          summaryTitle="Please fix the warning thresholds"
          @submit=${(e: Event) => void this.onForesightSave(e)}
        >
          <div class="fields">
            <fb-field
              class="num"
              label=${FORESIGHT_LABELS.schedule_float_days}
              error=${this.foresightErrors.schedule_float_days ?? ''}
              hint="Warn me when a task has this many days or fewer of slack left — lower warns sooner."
            >
              <fb-input
                type="number"
                name="schedule_float_days"
                inputmode="numeric"
                .value=${String(cfg.schedule_float_days)}
                min=${1}
                step=${1}
                ?disabled=${cardBusy}
              ></fb-input>
            </fb-field>
            <fb-field
              class="num"
              label=${FORESIGHT_LABELS.budget_burn_percent}
              error=${this.foresightErrors.budget_burn_percent ?? ''}
              hint="Warn me once a project has spent this share of its budget (e.g. 80 = at 80% spent)."
            >
              <fb-input
                type="number"
                name="budget_burn_percent"
                inputmode="numeric"
                .value=${String(cfg.budget_burn_percent)}
                min=${1}
                step=${1}
                ?disabled=${cardBusy}
              ></fb-input>
            </fb-field>
          </div>
          <p class="preview">
            You’ll be warned at ${cfg.budget_burn_percent}% budget and ${cfg.schedule_float_days}
            days of slack.
          </p>
          <fb-button type="submit" variant="primary" size="sm" ?loading=${cardBusy}
            >Save thresholds</fb-button
          >
        </fb-form>
      </div>
    `;
  }

  private renderCard(cap: AgentCapability): TemplateResult {
    const a = this.agent(cap);
    if (!a) return html`${nothing}`;
    const copy = AGENT_COPY[cap];
    const cardBusy = this.busy.has(cap);
    const isOverride = a.source === 'override';
    return html`
      <section class="card" aria-busy=${cardBusy ? 'true' : nothing}>
        <div class="card-head">
          <div>
            <h2 class="card-title">${copy.title}</h2>
            <p class="card-blurb">${copy.blurb}</p>
          </div>
          <fb-switch
            data-cap=${cap}
            ?checked=${a.enabled}
            ?disabled=${cardBusy}
            label="Enable ${copy.title}"
            @change=${(e: Event) => void this.onToggle(cap, e)}
          ></fb-switch>
        </div>

        ${cap === 'experience'
          ? html`<p class="cross-note">
              Turning this off disables the AI Assistant chat for everyone in your company. (Chat
              may not be reachable on this deployment yet.)
            </p>`
          : nothing}
        ${cap === 'foresight' ? this.renderForesightForm(a) : nothing}

        <div class="card-foot">
          <fb-badge status=${isOverride ? 'active' : 'neutral'}>
            ${isOverride ? 'Your settings' : 'Standard settings'}
          </fb-badge>
          ${isOverride
            ? html`<fb-button
                variant="ghost"
                size="sm"
                ?disabled=${cardBusy}
                label="Reset ${cap} to default"
                @click=${() => this.onResetClick(cap)}
                >Reset to default</fb-button
              >`
            : nothing}
        </div>

        ${this.renderDependencyRow()}
      </section>
    `;
  }

  override render(): TemplateResult {
    if (this.loading) {
      return html`<fb-state mode="loading" skeleton="card" rows="3"></fb-state>`;
    }
    if (this.loadError) {
      return html`<fb-state
        mode="error"
        retryable
        error-code=${this.loadError.code}
        request-id=${this.loadError.requestId ?? nothing}
        @retry=${() => void this.load()}
      ></fb-state>`;
    }

    const aiOff = aiConfigured.get() === false;

    // Single root element: a bare notice expression directly preceding the card
    // grid is mis-rendered by happy-dom's part-marker handling (see
    // fb-integrations-page). Keep the child parts off the shadow-root top level.
    return html`
      <div class="page">
        <div class="head">
          <h1 class="title">AI Agents</h1>
          <p class="sub">
            Turn BuildOS’s built-in assistants on or off and tune how they warn you. These run on
            your own Anthropic key.
          </p>
        </div>

        ${aiOff
          ? html`<p class="banner" role="status">
              <fb-icon name="alert-triangle" size="16"></fb-icon>
              <span
                >No Anthropic key is set, so agents won’t run yet. You can still turn them on here —
                add a key on the Integrations page to activate them.</span
              >
            </p>`
          : nothing}
        ${this.notice
          ? html`<p
              class="toast ${this.notice.kind}"
              role=${this.notice.kind === 'err' ? 'alert' : 'status'}
            >
              <fb-icon
                name=${this.notice.kind === 'ok' ? 'check-circle' : 'alert-circle'}
                size="16"
              ></fb-icon>
              ${this.notice.text}
            </p>`
          : nothing}

        <div class="grid">${ORDER.map((cap) => this.renderCard(cap))}</div>

        <fb-confirm
          ?open=${this.confirmReset === 'foresight'}
          heading="Reset warning thresholds?"
          message="This restores the standard warning thresholds (80% budget, 2 days of slack) and discards your tuned values."
          confirm-label="Reset to standard"
          cancel-label="Keep mine"
          @confirm=${this.onConfirmReset}
          @cancel=${() => (this.confirmReset = null)}
        ></fb-confirm>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-agents-page': FbAgentsPage;
  }
}
