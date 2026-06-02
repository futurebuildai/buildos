import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-input.js';
import '../atoms/fb-select.js';
import '../atoms/fb-checkbox.js';
import '../atoms/fb-button.js';
import '../molecules/fb-form.js';
import '../molecules/fb-field.js';
import '../shell/fb-wizard-stepper.js';
import type { WizardStep } from '../shell/fb-wizard-stepper.js';
import * as setupApi from '../../api/endpoints/setup.js';
import type { SetupState } from '../../types/models.js';
import { navigate, landingPathForRole } from '../../router.js';
import { currentRole } from '../../state/authStore.js';
import { ApiError, userMessageForCode } from '../../api/errors.js';

type StepId = 'company' | 'trades' | 'codes' | 'calendar' | 'jurisdictions' | 'review';

const STEP_ORDER: StepId[] = ['company', 'trades', 'codes', 'calendar', 'jurisdictions', 'review'];
const STEP_LABELS: Record<StepId, string> = {
  company: 'Company info',
  trades: 'Trades',
  codes: 'Cost codes',
  calendar: 'Calendar',
  jurisdictions: 'Jurisdictions',
  review: 'Review',
};

const DAYS: { bit: number; label: string }[] = [
  { bit: 0, label: 'Mon' },
  { bit: 1, label: 'Tue' },
  { bit: 2, label: 'Wed' },
  { bit: 3, label: 'Thu' },
  { bit: 4, label: 'Fri' },
  { bit: 5, label: 'Sat' },
  { bit: 6, label: 'Sun' },
];
const MON_FRI_MASK = 0b0011111; // bits 0..4

/**
 * `fb-setup-page` — the embedded onboarding wizard (UX_AUTH_ONBOARDING §6). Six
 * steps: company info → trades → cost codes → working calendar (+holidays) →
 * permit jurisdictions (skippable) → review & complete. Admin-gated by the
 * router and exempt from SetupGate so it's reachable while the org is not yet
 * onboarded.
 *
 * Resume is server-driven: the page (re)loads `GET /setup/state` on mount and
 * derives step completion from it, so a refresh or a return visit picks up where
 * the org left off. List steps batch one-row-per-call (createTrade/CostCode/…)
 * and surface per-row success/failure inline, mirroring the server's validation.
 * `Complete` enforces the same prereqs the backend does (company + ≥1 trade +
 * ≥1 cost code + a default calendar) and lands the owner on their role home.
 */
@customElement('fb-setup-page')
export class FbSetupPage extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .wrap {
        display: grid;
        grid-template-columns: 240px 1fr;
        gap: var(--fb-spacing-xl);
        max-width: 920px;
        margin: 0 auto;
        padding: var(--fb-spacing-lg);
      }
      @media (max-width: 720px) {
        .wrap {
          grid-template-columns: 1fr;
        }
      }
      .panel {
        min-width: 0;
      }
      h1 {
        margin: 0 0 var(--fb-spacing-xs);
        font-size: var(--fb-text-headline-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .step-sub {
        margin: 0 0 var(--fb-spacing-lg);
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      .fields {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
        margin-bottom: var(--fb-spacing-lg);
      }
      .row {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: var(--fb-spacing-md);
      }
      .days {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) 0;
      }
      .list {
        margin: 0 0 var(--fb-spacing-lg);
        padding: 0;
        list-style: none;
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
      }
      .list li {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--fb-glass-border);
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-body-sm);
      }
      .list .code {
        font-family: var(--fb-font-mono);
        color: var(--fb-text-secondary);
      }
      .empty {
        margin: 0 0 var(--fb-spacing-md);
        color: var(--fb-text-muted);
        font-size: var(--fb-text-body-sm);
      }
      .adder {
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-2);
        border: 1px solid var(--fb-glass-border);
        border-radius: var(--fb-radius-md);
        margin-bottom: var(--fb-spacing-lg);
      }
      .actions {
        display: flex;
        justify-content: space-between;
        gap: var(--fb-spacing-md);
        margin-top: var(--fb-spacing-lg);
      }
      .notice {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-bottom: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        border-radius: var(--fb-radius-sm);
      }
      .notice.ok {
        color: var(--fb-gable-green);
        background: color-mix(in srgb, var(--fb-gable-green) 12%, transparent);
        border: 1px solid var(--fb-gable-green);
      }
      .notice.err {
        color: var(--fb-safety-red);
        background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
        border: 1px solid var(--fb-safety-red);
      }
      .review dl {
        display: grid;
        grid-template-columns: auto 1fr;
        gap: var(--fb-spacing-xs) var(--fb-spacing-md);
        margin: 0 0 var(--fb-spacing-lg);
      }
      .review dt {
        color: var(--fb-text-secondary);
      }
      .review dd {
        margin: 0;
        color: var(--fb-text-primary);
      }
      .loading {
        padding: var(--fb-spacing-2xl);
        text-align: center;
        color: var(--fb-text-secondary);
      }
    `,
  ];

  /** Route param `:step`; empty for `/setup` → defaults to the first step. */
  @property({ type: String }) step = '';

  @state() private data: SetupState | null = null;
  @state() private loading = true;
  @state() private busy = false;
  @state() private notice: { kind: 'ok' | 'err'; text: string } | null = null;

  // Local add-row form state (not persisted until the row's Add call succeeds).
  @state() private form: Record<string, string> = {};
  @state() private rowDefault = false;
  @state() private dayMask = MON_FRI_MASK;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private get active(): StepId {
    return (STEP_ORDER as string[]).includes(this.step) ? (this.step as StepId) : 'company';
  }

  private async load(): Promise<void> {
    this.loading = true;
    try {
      this.data = await setupApi.getSetupState();
    } catch (err) {
      // A brand-new owner may have no state yet; treat as an empty wizard.
      this.notice =
        err instanceof ApiError && err.status >= 500
          ? { kind: 'err', text: userMessageForCode(err.code) }
          : null;
      this.data = null;
    } finally {
      this.loading = false;
    }
  }

  // ----- completion derivation (drives the stepper + review prereqs) -----
  private done(step: StepId): boolean {
    const d = this.data;
    if (!d) return false;
    switch (step) {
      case 'company':
        return !!d.company_profile?.legal_name;
      case 'trades':
        return d.trades.length > 0;
      case 'codes':
        return d.cost_codes.length > 0;
      case 'calendar':
        return !!d.default_calendar;
      case 'jurisdictions':
        return d.permit_jurisdictions.length > 0;
      case 'review':
        return !!d.onboarding_complete;
    }
  }

  private get prereqsMet(): boolean {
    return (
      this.done('company') && this.done('trades') && this.done('codes') && this.done('calendar')
    );
  }

  private stepperSteps(): WizardStep[] {
    return STEP_ORDER.map((id) => {
      let stState: WizardStep['state'];
      if (id === this.active) stState = 'current';
      else if (this.done(id)) stState = 'done';
      else if (id === 'review' && !this.prereqsMet) stState = 'blocked';
      else stState = 'upcoming';
      return { id, label: STEP_LABELS[id], state: stState };
    });
  }

  private goto(step: StepId): void {
    this.notice = null;
    this.form = {};
    this.rowDefault = false;
    navigate(`/setup/${step}`);
  }

  private nextOf(step: StepId): StepId {
    const i = STEP_ORDER.indexOf(step);
    return STEP_ORDER[Math.min(i + 1, STEP_ORDER.length - 1)] as StepId;
  }
  private prevOf(step: StepId): StepId {
    const i = STEP_ORDER.indexOf(step);
    return STEP_ORDER[Math.max(i - 1, 0)] as StepId;
  }

  private bind(name: string) {
    return (e: Event) => {
      this.form = { ...this.form, [name]: (e as CustomEvent<{ value: string }>).detail.value };
    };
  }

  // ------------------------------- mutations -------------------------------
  private async onCompanySubmit(e: CustomEvent<{ values: Record<string, string> }>): Promise<void> {
    const v = e.detail.values;
    const legalName = (v.legal_name ?? '').trim();
    if (!legalName) {
      this.notice = { kind: 'err', text: 'A legal company name is required.' };
      return;
    }
    this.busy = true;
    this.notice = null;
    try {
      await setupApi.updateCompanyInfo({
        legal_name: legalName,
        ...(v.company_type ? { company_type: v.company_type } : {}),
        ...(v.region ? { region: v.region.trim() } : {}),
        ...(v.address ? { address: v.address.trim() } : {}),
        ...(v.ein ? { ein: v.ein.trim() } : {}),
      });
      await this.load();
      this.goto('trades');
    } catch (err) {
      this.notice = { kind: 'err', text: this.errText(err) };
    } finally {
      this.busy = false;
    }
  }

  private async addTrade(): Promise<void> {
    const code = (this.form.code ?? '').trim();
    const name = (this.form.name ?? '').trim();
    if (!code || !name) {
      this.notice = { kind: 'err', text: 'A trade needs both a code and a name.' };
      return;
    }
    this.busy = true;
    try {
      await setupApi.createTrade({
        code,
        name,
        is_default: this.rowDefault,
        ...(this.form.description ? { description: this.form.description.trim() } : {}),
      });
      this.notice = { kind: 'ok', text: `Added trade ${name}.` };
      this.form = {};
      this.rowDefault = false;
      await this.load();
    } catch (err) {
      this.notice = { kind: 'err', text: this.errText(err) };
    } finally {
      this.busy = false;
    }
  }

  private async addCostCode(): Promise<void> {
    const code = (this.form.code ?? '').trim();
    const name = (this.form.name ?? '').trim();
    const division = (this.form.division ?? '').trim();
    if (!code || !name || !division) {
      this.notice = { kind: 'err', text: 'A cost code needs a code, name, and division.' };
      return;
    }
    this.busy = true;
    try {
      await setupApi.createCostCode({
        code,
        name,
        division,
        is_default: this.rowDefault,
        ...(this.form.parent_code ? { parent_code: this.form.parent_code.trim() } : {}),
      });
      this.notice = { kind: 'ok', text: `Added cost code ${code}.` };
      this.form = {};
      this.rowDefault = false;
      await this.load();
    } catch (err) {
      this.notice = { kind: 'err', text: this.errText(err) };
    } finally {
      this.busy = false;
    }
  }

  private toggleDay(bit: number, checked: boolean): void {
    this.dayMask = checked ? this.dayMask | (1 << bit) : this.dayMask & ~(1 << bit);
  }

  private async createCalendar(): Promise<void> {
    const name = (this.form.name ?? '').trim() || 'Standard';
    const tz =
      (this.form.timezone ?? '').trim() ||
      Intl.DateTimeFormat().resolvedOptions().timeZone ||
      'UTC';
    const hours = Number(this.form.hours ?? '8');
    const minutes = Number.isFinite(hours) && hours > 0 ? Math.round(hours * 60) : 480;
    if (this.dayMask === 0) {
      this.notice = { kind: 'err', text: 'Pick at least one working day.' };
      return;
    }
    this.busy = true;
    try {
      await setupApi.createCalendar({
        name,
        timezone: tz,
        working_days_mask: this.dayMask,
        daily_work_minutes: minutes,
        is_default: true,
      });
      this.notice = { kind: 'ok', text: 'Working calendar saved.' };
      this.form = {};
      await this.load();
    } catch (err) {
      this.notice = { kind: 'err', text: this.errText(err) };
    } finally {
      this.busy = false;
    }
  }

  private async addHoliday(): Promise<void> {
    const calId = this.data?.default_calendar?.id;
    const date = (this.form.holiday_date ?? '').trim();
    const hname = (this.form.holiday_name ?? '').trim();
    if (!calId || !date || !hname) {
      this.notice = { kind: 'err', text: 'A holiday needs a date and a name.' };
      return;
    }
    this.busy = true;
    try {
      await setupApi.addHoliday(calId, { holiday_date: date, name: hname });
      this.notice = { kind: 'ok', text: `Added holiday ${hname}.` };
      this.form = { ...this.form, holiday_date: '', holiday_name: '' };
      await this.load();
    } catch (err) {
      this.notice = { kind: 'err', text: this.errText(err) };
    } finally {
      this.busy = false;
    }
  }

  private async addJurisdiction(): Promise<void> {
    const name = (this.form.name ?? '').trim();
    if (!name) {
      this.notice = { kind: 'err', text: 'A jurisdiction needs a name.' };
      return;
    }
    this.busy = true;
    try {
      await setupApi.addJurisdiction({
        name,
        ...(this.form.region ? { region: this.form.region.trim() } : {}),
        ...(this.form.notes ? { notes: this.form.notes.trim() } : {}),
      });
      this.notice = { kind: 'ok', text: `Added ${name}.` };
      this.form = {};
      await this.load();
    } catch (err) {
      this.notice = { kind: 'err', text: this.errText(err) };
    } finally {
      this.busy = false;
    }
  }

  private async complete(): Promise<void> {
    this.busy = true;
    this.notice = null;
    try {
      await setupApi.completeSetup();
      navigate(landingPathForRole(currentRole.get()), { replace: true });
    } catch (err) {
      this.notice = { kind: 'err', text: this.errText(err) };
    } finally {
      this.busy = false;
    }
  }

  private errText(err: unknown): string {
    if (err instanceof ApiError) {
      if (err.details.length) return err.details.map((d) => `${d.field}: ${d.reason}`).join('; ');
      return userMessageForCode(err.code);
    }
    return 'Something went wrong on our end.';
  }

  // -------------------------------- render --------------------------------
  override render(): TemplateResult {
    if (this.loading) return html`<p class="loading" role="status">Loading setup…</p>`;
    return html`
      <div class="wrap">
        <fb-wizard-stepper
          class="panel"
          .steps=${this.stepperSteps()}
          @step=${(e: CustomEvent<{ id: string }>) => this.goto(e.detail.id as StepId)}
        ></fb-wizard-stepper>
        <section class="panel" aria-live="polite">${this.renderStep()}</section>
      </div>
    `;
  }

  private renderNotice(): TemplateResult | typeof nothing {
    if (!this.notice) return nothing;
    return html`<p
      class="notice ${this.notice.kind}"
      role=${this.notice.kind === 'err' ? 'alert' : 'status'}
    >
      <fb-icon
        name=${this.notice.kind === 'ok' ? 'check-circle' : 'alert-circle'}
        size="16"
      ></fb-icon>
      ${this.notice.text}
    </p>`;
  }

  private renderStep(): TemplateResult {
    switch (this.active) {
      case 'company':
        return this.renderCompany();
      case 'trades':
        return this.renderTrades();
      case 'codes':
        return this.renderCodes();
      case 'calendar':
        return this.renderCalendar();
      case 'jurisdictions':
        return this.renderJurisdictions();
      case 'review':
        return this.renderReview();
    }
  }

  private renderCompany(): TemplateResult {
    const cp = this.data?.company_profile;
    // Wrapped in a single root <div> on purpose: a bare root-level child
    // expression (renderNotice) directly preceding the form is mis-rendered by
    // happy-dom's part markers, which corrupts the following fb-select `.options`
    // binding (it arrives as "" and throws). Keeping one container element avoids
    // top-level sibling child parts. See fb-integrations-page for the same note.
    return html`
      <div class="step">
        <h1>Company info</h1>
        <p class="step-sub">Tell us about the company this BuildOS runs.</p>
        ${this.renderNotice()}
        <fb-form @submit=${this.onCompanySubmit}>
          <div class="fields">
            <fb-field label="Legal company name" required fieldId="su-legal">
              <fb-input name="legal_name" value=${cp?.legal_name ?? ''} required></fb-input>
            </fb-field>
            <fb-field label="Company type" fieldId="su-type">
              <fb-select
                name="company_type"
                value=${cp?.company_type ?? ''}
                placeholder="Select a type"
                .options=${[
                  { value: 'llc', label: 'LLC' },
                  { value: 'corporation', label: 'Corporation' },
                  { value: 'sole_proprietor', label: 'Sole proprietor' },
                  { value: 'partnership', label: 'Partnership' },
                ]}
              ></fb-select>
            </fb-field>
            <fb-field label="Region / state" fieldId="su-region">
              <fb-input name="region" value=${cp?.region ?? ''}></fb-input>
            </fb-field>
            <fb-field label="Address" fieldId="su-address">
              <fb-input name="address" value=${cp?.address ?? ''}></fb-input>
            </fb-field>
            <fb-field label="EIN" hint="Optional tax id." fieldId="su-ein">
              <fb-input name="ein" value=${cp?.ein ?? ''}></fb-input>
            </fb-field>
          </div>
          <div class="actions">
            <span></span>
            <fb-button type="submit" variant="primary" ?loading=${this.busy}
              >Save & continue</fb-button
            >
          </div>
        </fb-form>
      </div>
    `;
  }

  private renderTrades(): TemplateResult {
    const trades = this.data?.trades ?? [];
    return html`
      <h1>Trades</h1>
      <p class="step-sub">Add the trade categories your crews work in.</p>
      ${this.renderNotice()}
      ${trades.length
        ? html`<ul class="list">
            ${trades.map(
              (t) =>
                html`<li>
                  <span class="code">${t.code}</span>${t.name}${t.is_default
                    ? html` · <span class="code">default</span>`
                    : nothing}
                </li>`,
            )}
          </ul>`
        : html`<p class="empty">No trades yet. Add at least one to continue.</p>`}

      <div class="adder">
        <div class="row">
          <fb-field label="Code" fieldId="tr-code">
            <fb-input
              name="code"
              .value=${this.form.code ?? ''}
              @input=${this.bind('code')}
            ></fb-input>
          </fb-field>
          <fb-field label="Name" fieldId="tr-name">
            <fb-input
              name="name"
              .value=${this.form.name ?? ''}
              @input=${this.bind('name')}
            ></fb-input>
          </fb-field>
        </div>
        <fb-field label="Description" fieldId="tr-desc">
          <fb-input
            name="description"
            .value=${this.form.description ?? ''}
            @input=${this.bind('description')}
          ></fb-input>
        </fb-field>
        <div style="margin-top: var(--fb-spacing-sm)">
          <fb-checkbox
            ?checked=${this.rowDefault}
            @change=${(e: CustomEvent<{ checked: boolean }>) =>
              (this.rowDefault = e.detail.checked)}
            >Default trade</fb-checkbox
          >
        </div>
        <div style="margin-top: var(--fb-spacing-md)">
          <fb-button variant="secondary" icon="plus" ?loading=${this.busy} @click=${this.addTrade}
            >Add trade</fb-button
          >
        </div>
      </div>

      ${this.renderNav('trades', this.done('trades'))}
    `;
  }

  private renderCodes(): TemplateResult {
    const codes = this.data?.cost_codes ?? [];
    return html`
      <h1>Cost codes</h1>
      <p class="step-sub">
        Add CSI MasterFormat cost codes (e.g. 03-30-00 Cast-in-Place Concrete).
      </p>
      ${this.renderNotice()}
      ${codes.length
        ? html`<ul class="list">
            ${codes.map(
              (c) =>
                html`<li>
                  <span class="code">${c.code}</span>${c.name} ·
                  <span class="code">${c.division}</span>
                </li>`,
            )}
          </ul>`
        : html`<p class="empty">No cost codes yet. Add at least one to continue.</p>`}

      <div class="adder">
        <div class="row">
          <fb-field label="Code" hint="CSI format, e.g. 03-30-00" fieldId="cc-code">
            <fb-input
              name="code"
              .value=${this.form.code ?? ''}
              @input=${this.bind('code')}
            ></fb-input>
          </fb-field>
          <fb-field label="Division" hint="e.g. 03 Concrete" fieldId="cc-div">
            <fb-input
              name="division"
              .value=${this.form.division ?? ''}
              @input=${this.bind('division')}
            ></fb-input>
          </fb-field>
        </div>
        <fb-field label="Name" fieldId="cc-name">
          <fb-input
            name="name"
            .value=${this.form.name ?? ''}
            @input=${this.bind('name')}
          ></fb-input>
        </fb-field>
        <fb-field label="Parent code" fieldId="cc-parent">
          <fb-input
            name="parent_code"
            .value=${this.form.parent_code ?? ''}
            @input=${this.bind('parent_code')}
          ></fb-input>
        </fb-field>
        <div style="margin-top: var(--fb-spacing-sm)">
          <fb-checkbox
            ?checked=${this.rowDefault}
            @change=${(e: CustomEvent<{ checked: boolean }>) =>
              (this.rowDefault = e.detail.checked)}
            >Default cost code</fb-checkbox
          >
        </div>
        <div style="margin-top: var(--fb-spacing-md)">
          <fb-button
            variant="secondary"
            icon="plus"
            ?loading=${this.busy}
            @click=${this.addCostCode}
            >Add cost code</fb-button
          >
        </div>
      </div>

      ${this.renderNav('codes', this.done('codes'))}
    `;
  }

  private renderCalendar(): TemplateResult {
    const cal = this.data?.default_calendar;
    const holidays = this.data?.default_holidays ?? [];
    return html`
      <h1>Working calendar</h1>
      <p class="step-sub">Set the default schedule the CPM engine plans against.</p>
      ${this.renderNotice()}
      ${cal
        ? html`<ul class="list">
            <li>
              <span class="code">${cal.name}</span>${cal.timezone} ·
              ${cal.daily_work_minutes / 60}h/day
            </li>
          </ul>`
        : html`<div class="adder">
            <fb-field label="Calendar name" fieldId="cal-name">
              <fb-input
                name="name"
                .value=${this.form.name ?? ''}
                @input=${this.bind('name')}
              ></fb-input>
            </fb-field>
            <div class="row">
              <fb-field label="Timezone" fieldId="cal-tz">
                <fb-input
                  name="timezone"
                  .value=${this.form.timezone ??
                  Intl.DateTimeFormat().resolvedOptions().timeZone ??
                  ''}
                  @input=${this.bind('timezone')}
                ></fb-input>
              </fb-field>
              <fb-field label="Hours per day" fieldId="cal-hours">
                <fb-input
                  name="hours"
                  type="number"
                  .value=${this.form.hours ?? '8'}
                  @input=${this.bind('hours')}
                ></fb-input>
              </fb-field>
            </div>
            <fb-field label="Working days" fieldId="cal-days">
              <div class="days" role="group" aria-label="Working days">
                ${DAYS.map(
                  (d) =>
                    html`<fb-checkbox
                      ?checked=${(this.dayMask & (1 << d.bit)) !== 0}
                      @change=${(e: CustomEvent<{ checked: boolean }>) =>
                        this.toggleDay(d.bit, e.detail.checked)}
                      >${d.label}</fb-checkbox
                    >`,
                )}
              </div>
            </fb-field>
            <div style="margin-top: var(--fb-spacing-md)">
              <fb-button
                variant="secondary"
                icon="plus"
                ?loading=${this.busy}
                @click=${this.createCalendar}
                >Save calendar</fb-button
              >
            </div>
          </div>`}
      ${cal
        ? html`<h2 style="font-size: var(--fb-text-title-md)">Holidays</h2>
            ${holidays.length
              ? html`<ul class="list">
                  ${holidays.map(
                    (h) =>
                      html`<li>
                        <span class="code">${h.holiday_date.slice(0, 10)}</span>${h.name}
                      </li>`,
                  )}
                </ul>`
              : html`<p class="empty">No holidays added (optional).</p>`}
            <div class="adder">
              <div class="row">
                <fb-field label="Date" fieldId="hol-date">
                  <fb-input
                    name="holiday_date"
                    type="date"
                    .value=${this.form.holiday_date ?? ''}
                    @input=${this.bind('holiday_date')}
                  ></fb-input>
                </fb-field>
                <fb-field label="Name" fieldId="hol-name">
                  <fb-input
                    name="holiday_name"
                    .value=${this.form.holiday_name ?? ''}
                    @input=${this.bind('holiday_name')}
                  ></fb-input>
                </fb-field>
              </div>
              <div style="margin-top: var(--fb-spacing-md)">
                <fb-button
                  variant="ghost"
                  icon="plus"
                  ?loading=${this.busy}
                  @click=${this.addHoliday}
                  >Add holiday</fb-button
                >
              </div>
            </div>`
        : nothing}
      ${this.renderNav('calendar', this.done('calendar'))}
    `;
  }

  private renderJurisdictions(): TemplateResult {
    const js = this.data?.permit_jurisdictions ?? [];
    return html`
      <h1>Permit jurisdictions</h1>
      <p class="step-sub">
        Optional — add the authorities you pull permits from. You can skip this.
      </p>
      ${this.renderNotice()}
      ${js.length
        ? html`<ul class="list">
            ${js.map(
              (j) =>
                html`<li>
                  ${j.name}${j.region ? html` · <span class="code">${j.region}</span>` : nothing}
                </li>`,
            )}
          </ul>`
        : html`<p class="empty">None added.</p>`}

      <div class="adder">
        <div class="row">
          <fb-field label="Name" fieldId="ju-name">
            <fb-input
              name="name"
              .value=${this.form.name ?? ''}
              @input=${this.bind('name')}
            ></fb-input>
          </fb-field>
          <fb-field label="Region" fieldId="ju-region">
            <fb-input
              name="region"
              .value=${this.form.region ?? ''}
              @input=${this.bind('region')}
            ></fb-input>
          </fb-field>
        </div>
        <fb-field label="Notes" fieldId="ju-notes">
          <fb-input
            name="notes"
            .value=${this.form.notes ?? ''}
            @input=${this.bind('notes')}
          ></fb-input>
        </fb-field>
        <div style="margin-top: var(--fb-spacing-md)">
          <fb-button
            variant="secondary"
            icon="plus"
            ?loading=${this.busy}
            @click=${this.addJurisdiction}
            >Add jurisdiction</fb-button
          >
        </div>
      </div>

      <div class="actions">
        <fb-button variant="ghost" @click=${() => this.goto(this.prevOf('jurisdictions'))}
          >Back</fb-button
        >
        <fb-button variant="primary" @click=${() => this.goto('review')}>Continue</fb-button>
      </div>
    `;
  }

  private renderReview(): TemplateResult {
    const d = this.data;
    return html`
      <h1>Review &amp; finish</h1>
      <p class="step-sub">Confirm the essentials, then complete setup.</p>
      ${this.renderNotice()}
      <div class="review">
        <dl>
          <dt>Company</dt>
          <dd>${d?.company_profile?.legal_name ?? '—'}</dd>
          <dt>Trades</dt>
          <dd>${d?.trades.length ?? 0}</dd>
          <dt>Cost codes</dt>
          <dd>${d?.cost_codes.length ?? 0}</dd>
          <dt>Calendar</dt>
          <dd>${d?.default_calendar?.name ?? '—'}</dd>
          <dt>Jurisdictions</dt>
          <dd>${d?.permit_jurisdictions.length ?? 0}</dd>
        </dl>
      </div>
      ${this.prereqsMet
        ? nothing
        : html`<p class="notice err" role="alert">
            <fb-icon name="alert-triangle" size="16"></fb-icon>
            Finish company info, at least one trade, one cost code, and a working calendar first.
          </p>`}
      <div class="actions">
        <fb-button variant="ghost" @click=${() => this.goto(this.prevOf('review'))}>Back</fb-button>
        <fb-button
          variant="primary"
          ?disabled=${!this.prereqsMet}
          ?loading=${this.busy}
          @click=${this.complete}
          >Complete setup</fb-button
        >
      </div>
    `;
  }

  private renderNav(step: StepId, canAdvance: boolean): TemplateResult {
    const isFirst = STEP_ORDER.indexOf(step) === 0;
    return html`<div class="actions">
      ${isFirst
        ? html`<span></span>`
        : html`<fb-button variant="ghost" @click=${() => this.goto(this.prevOf(step))}
            >Back</fb-button
          >`}
      <fb-button
        variant="primary"
        ?disabled=${!canAdvance}
        @click=${() => this.goto(this.nextOf(step))}
        >Continue</fb-button
      >
    </div>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-setup-page': FbSetupPage;
  }
}
