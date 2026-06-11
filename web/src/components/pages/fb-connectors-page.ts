import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state, query } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-button.js';
import '../molecules/fb-field.js';
import '../molecules/fb-form.js';
import '../molecules/fb-connector-card.js';
import '../organisms/fb-state.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-confirm.js';
import {
  listConnectors,
  setConnector,
  refreshConnector,
  deleteConnector,
} from '../../api/endpoints/admin.js';
import {
  listIntegrations,
  setCredential,
  deleteCredential,
} from '../../api/endpoints/integrations.js';
import type { EffectiveConnector, IntegrationCredential } from '../../types/models.js';
import { ApiError, ErrorCode } from '../../api/errors.js';
import type { FbConnectorCard, CredState } from '../molecules/fb-connector-card.js';
import type { FbForm } from '../molecules/fb-form.js';
import type { FbInput } from '../atoms/fb-input.js';

/** The vault provider id for an MCP instance's bearer token. */
const credProvider = (name: string): string => `connector:${name}`;

/** Connector instance name shape (mirror the backend regex). */
const NAME_RE = /^[a-z0-9][a-z0-9_-]{1,40}$/;

/** Human labels keyed to the Add/Edit form field `name`s (for fb-form.setErrors). */
const NAME_LABEL = 'Name';
const ENDPOINT_LABEL = 'Web address';

/**
 * `fb-connectors-page` — Settings → Connectors (Phase 3c §5). The admin surface
 * for the connector registry: enable the built-in `reference` connector and
 * create / configure / refresh / credential / delete MCP server instances —
 * "Claude for Small Business inside the ERP", from the UI, no curl.
 *
 * Loads GET /admin/connectors (primary) AND GET /api/v1/integrations (secondary;
 * partial-load tolerant — a failed integrations fetch renders credential state
 * as "unknown", never "no token"). The page owns every async call + all state;
 * `fb-connector-card` only emits intents.
 *
 * Route gating is admin+ — NOT plan-gated (ESC-002): the connector kill-switches
 * must reach admins regardless of plan tier. See router.ts.
 */
@customElement('fb-connectors-page')
export class FbConnectorsPage extends FBElement {
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
        max-width: 60ch;
      }
      .toolbar {
        display: flex;
        justify-content: flex-end;
        margin-bottom: var(--fb-spacing-md);
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
        color: var(--fb-safety-red-text);
        background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
        border: 1px solid var(--fb-safety-red);
      }
      /* Same red treatment as .notice.err but lives INSIDE the modal flow (no
         bottom margin — the .modal-form gap handles spacing). */
      .modal-error {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin: 0;
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-safety-red-text);
        background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
        border: 1px solid var(--fb-safety-red);
        border-radius: var(--fb-radius-sm);
      }
      .group-label {
        margin: var(--fb-spacing-lg) 0 var(--fb-spacing-sm);
        font-size: var(--fb-text-label-sm);
        font-weight: 600;
        letter-spacing: 0.04em;
        text-transform: uppercase;
        color: var(--fb-text-secondary);
      }
      .grid {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
      }
      .modal-form {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
      }
      .modal-actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--fb-spacing-sm);
        margin-top: var(--fb-spacing-xs);
      }
    `,
  ];

  @state() private connectors: EffectiveConnector[] = [];
  @state() private integrations: IntegrationCredential[] = [];
  /** True when the secondary integrations fetch failed → credential = "unknown". */
  @state() private integrationsUnknown = false;
  @state() private loading = true;
  @state() private loadError: ApiError | null = null;
  @state() private notice: { kind: 'ok' | 'err'; text: string } | null = null;

  /** Per-connector in-flight gate (name → busy). */
  @state() private busy = new Set<string>();
  /** Per-connector persistent error copy (name → message), e.g. a refresh 502. */
  @state() private cardErrors = new Map<string, string>();
  /** Card to refocus after a refetch re-renders the grid (keyboard focus, WCAG 2.4.3). */
  private refocusCard: string | null = null;

  // ---- Add / Edit MCP modal ----
  @state() private modalOpen = false;
  /** When editing, the locked connector name; null when adding a new instance. */
  @state() private editName: string | null = null;
  @state() private editEndpoint = '';
  /** Editing preserves the connector's CURRENT enabled state — never force it on. */
  @state() private editEnabled = true;
  /** Controlled value for the Add-flow Name field so it resets on reopen (not stale). */
  @state() private addName = '';
  /** Non-400 PUT failure shown INSIDE the modal (a page notice renders behind the backdrop). */
  @state() private modalError = '';
  @state() private modalBusy = false;

  // ---- Delete confirm ----
  @state() private confirmName: string | null = null;
  /** Whether the to-be-deleted connector currently has a saved bearer token. */
  @state() private confirmHasCred = false;

  @query('fb-form') private modalForm?: FbForm;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  override updated(): void {
    // Restore focus to the acted-on card after a refetch re-renders the grid —
    // otherwise inline card actions drop focus to <body> (WCAG 2.4.3).
    // fb-connector-card delegates focus to its first control, so focusing the
    // host host lands focus inside that card (not the page top / body).
    if (this.refocusCard && !this.loading && !this.modalOpen) {
      const name = this.refocusCard;
      this.refocusCard = null;
      this.renderRoot
        .querySelector<HTMLElement>(`fb-connector-card[data-connector='${name}']`)
        ?.focus?.();
    }
  }

  /** Primary + secondary fetch. Connectors block the page; integrations degrade. */
  private async load(): Promise<void> {
    this.loading = true;
    this.loadError = null;
    try {
      this.connectors = await listConnectors();
    } catch (err) {
      this.loadError =
        err instanceof ApiError
          ? err
          : new ApiError({ code: ErrorCode.UNKNOWN, message: 'load failed', status: 0 });
      this.loading = false;
      return;
    }
    // Secondary: a failure here must not block the page — mark cred state unknown.
    try {
      this.integrations = await listIntegrations();
      this.integrationsUnknown = false;
    } catch {
      this.integrations = [];
      this.integrationsUnknown = true;
    }
    this.loading = false;
  }

  // ----------------------------- Derived state -----------------------------

  private get builtins(): EffectiveConnector[] {
    return this.connectors.filter((c) => c.kind === 'builtin');
  }

  private get mcps(): EffectiveConnector[] {
    return this.connectors.filter((c) => c.kind === 'mcp');
  }

  /** Credential presence for an MCP, filtered on is_active. Orthogonal to enabled. */
  private credStateFor(name: string): CredState {
    if (this.integrationsUnknown) return 'unknown';
    const row = this.integrations.find((c) => c.provider === credProvider(name) && c.is_active);
    return row ? 'set' : 'none';
  }

  private credLast4For(name: string): string | undefined {
    return this.integrations.find((c) => c.provider === credProvider(name) && c.is_active)?.last4;
  }

  private isBusy(name: string): boolean {
    return this.busy.has(name);
  }

  private setBusy(name: string, on: boolean): void {
    const next = new Set(this.busy);
    if (on) next.add(name);
    else next.delete(name);
    this.busy = next;
  }

  private setCardError(name: string, msg: string | null): void {
    const next = new Map(this.cardErrors);
    if (msg) next.set(name, msg);
    else next.delete(name);
    this.cardErrors = next;
  }

  // ------------------------------- Intents -------------------------------

  /**
   * Toggle. Built-in default-OFF → DELETE drops the override row (turning off).
   * Built-in ON or any MCP → setConnector. Enabling an MCP auto-fires refresh
   * (PUT does not run tools/list). `await this.load()` in `finally` re-derives
   * `enabled` from server truth (fb-switch self-mutates + Lit no-op on re-render).
   */
  private async onToggle(c: EffectiveConnector, enabled: boolean): Promise<void> {
    if (this.isBusy(c.connector)) return;
    this.notice = null;
    this.setBusy(c.connector, true);
    let enabledMcp = false;
    try {
      if (c.kind === 'builtin' && !enabled) {
        // Drop the override row rather than persist {enabled:false} on a default-OFF.
        await deleteConnector(c.connector);
        this.notice = { kind: 'ok', text: `Turned off the ${c.connector} connector.` };
      } else if (c.kind === 'builtin') {
        await setConnector(c.connector, { enabled: true });
        this.notice = { kind: 'ok', text: `Turned on the ${c.connector} connector.` };
      } else {
        // MCP: preserve the endpoint config snapshot on the full-document PUT.
        await setConnector(c.connector, {
          enabled,
          kind: 'mcp',
          config: { endpoint: c.endpoint ?? '' },
        });
        enabledMcp = enabled;
        this.notice = {
          kind: 'ok',
          text: enabled ? `Turned on ${c.connector}.` : `Turned off ${c.connector}.`,
        };
      }
    } catch (err) {
      this.notice = { kind: 'err', text: this.connectorErr(err, c.connector) };
    } finally {
      this.setBusy(c.connector, false);
      this.refocusCard = c.connector;
      await this.load();
    }
    // After a successful enable of an MCP, auto-run tools/list (PUT doesn't).
    if (enabledMcp) await this.onRefresh(c.connector);
  }

  /** Refresh tools (MCP). A 502 UPSTREAM_ERROR becomes a persistent card error. */
  private async onRefresh(name: string): Promise<void> {
    if (this.isBusy(name)) return;
    // Clear any stale page notice (e.g. a green "Turned on" from the enable that
    // triggered this auto-refresh) so it can't contradict a refresh card-error.
    this.notice = null;
    this.setBusy(name, true);
    this.setCardError(name, null);
    try {
      const res = await refreshConnector(name);
      const noun = res.tools_count === 1 ? 'tool' : 'tools';
      this.notice = { kind: 'ok', text: `${name}: found ${res.tools_count} ${noun}.` };
    } catch (err) {
      if (err instanceof ApiError && err.code === ErrorCode.UPSTREAM_ERROR) {
        const endpoint =
          this.connectors.find((c) => c.connector === name)?.endpoint ?? 'the server';
        this.setCardError(
          name,
          `Last refresh failed — couldn’t reach ${endpoint}. Check the address and try again.`,
        );
      } else {
        this.notice = { kind: 'err', text: this.connectorErr(err, name) };
      }
    } finally {
      this.setBusy(name, false);
      this.refocusCard = name;
      await this.load();
    }
  }

  /** Save the MCP bearer token to the vault. `.submit()` clears the DOM in finally. */
  private async onSaveCredential(
    card: FbConnectorCard,
    name: string,
    value: string,
  ): Promise<void> {
    if (!value || this.isBusy(name)) return;
    if (this.modalBusy) return;
    this.notice = null;
    this.setBusy(name, true);
    try {
      await setCredential(credProvider(name), { label: `${name} access token`, key: value });
      this.notice = { kind: 'ok', text: `Saved the access token for ${name}.` };
    } catch (err) {
      this.notice = { kind: 'err', text: this.connectorErr(err, name) };
    } finally {
      // Wipe plaintext from the DOM (success AND error), then refresh last4.
      card.clearCredential();
      this.setBusy(name, false);
      this.refocusCard = name;
      try {
        this.integrations = await listIntegrations();
        this.integrationsUnknown = false;
      } catch {
        this.integrationsUnknown = true;
      }
    }
  }

  /** Open the delete confirm; note whether an orphan vault credential exists. */
  private onRequestDelete(name: string): void {
    if (this.isBusy(name)) return;
    this.confirmName = name;
    this.confirmHasCred = this.credStateFor(name) === 'set';
  }

  private onCancelDelete(): void {
    this.confirmName = null;
    this.confirmHasCred = false;
    this.focusAddButton();
  }

  /**
   * Delete the connector. A 404 (name neither built-in nor instance) is treated
   * as success (idempotent). Always offers to remove the orphan vault credential.
   * Focus moves to a stable target (the Add button) after the refetch.
   */
  private async onConfirmDelete(): Promise<void> {
    const name = this.confirmName;
    if (!name || this.isBusy(name)) return;
    const alsoDeleteCred = this.confirmHasCred;
    this.confirmName = null;
    this.confirmHasCred = false;
    this.notice = null;
    this.setBusy(name, true);
    this.setCardError(name, null);
    try {
      if (alsoDeleteCred) {
        try {
          await deleteCredential(credProvider(name));
        } catch (err) {
          // The connector delete is the primary action; a 404 on the cred is fine.
          if (!(err instanceof ApiError && err.code === ErrorCode.NOT_FOUND)) throw err;
        }
      }
      await deleteConnector(name);
      this.notice = { kind: 'ok', text: `Removed ${name}.` };
    } catch (err) {
      if (err instanceof ApiError && err.code === ErrorCode.NOT_FOUND) {
        this.notice = { kind: 'ok', text: `Removed ${name}.` };
      } else {
        this.notice = { kind: 'err', text: this.connectorErr(err, name) };
      }
    } finally {
      this.setBusy(name, false);
      await this.load();
      this.focusAddButton();
    }
  }

  /**
   * Edit-endpoint affordance: re-PUT path. Reuse the modal, name locked. Capture
   * the connector's CURRENT enabled state so the full-document PUT preserves it —
   * editing a disabled instance's address must NOT silently re-enable it.
   */
  private onEditEndpoint(name: string, enabled: boolean, endpoint: string): void {
    if (this.isBusy(name)) return;
    this.editName = name;
    this.editEnabled = enabled;
    this.editEndpoint = endpoint;
    this.modalError = '';
    this.modalOpen = true;
    this.modalForm?.setErrors({});
    void this.focusNameOrEndpoint();
  }

  // ------------------------------ Add / Edit ------------------------------

  private openAdd(): void {
    this.editName = null;
    this.editEnabled = true; // a new instance is created enabled
    this.addName = '';
    this.editEndpoint = '';
    this.modalError = '';
    this.modalOpen = true;
    this.modalForm?.setErrors({});
    void this.focusNameOrEndpoint();
  }

  private closeModal(): void {
    this.modalOpen = false;
    this.editName = null;
    this.editEnabled = true;
    this.addName = '';
    this.editEndpoint = '';
    this.modalError = '';
    this.modalForm?.setErrors({});
  }

  /**
   * Submit the Add/Edit form. Client-validate the name (when adding) BEFORE the
   * PUT. On 400 → stay in the modal, surface details[] inline. On success →
   * close + `await load()` FIRST (so the card renders), THEN a card-scoped
   * refresh (mirror the auto-refresh on enable).
   */
  private async onModalSubmit(e: Event): Promise<void> {
    if (this.modalBusy) return;
    const values = (e as CustomEvent<{ values: Record<string, string> }>).detail.values;
    const editing = this.editName !== null;
    const name = (editing ? this.editName! : (values['name'] ?? '')).trim().toLowerCase();
    const endpoint = (values['endpoint'] ?? '').trim();

    const fieldErrors: Record<string, string> = {};
    if (!editing && !NAME_RE.test(name)) {
      fieldErrors[NAME_LABEL] = 'Use lowercase letters, numbers, dashes; 2–41 characters.';
    }
    if (!/^https:\/\//i.test(endpoint)) {
      fieldErrors[ENDPOINT_LABEL] = 'Must start with https://';
    }
    if (Object.keys(fieldErrors).length) {
      this.modalForm?.setErrors(fieldErrors);
      return;
    }

    // Editing preserves the connector's current enabled state; a new instance is
    // created enabled. NEVER force enabled=true on edit (the full-document PUT
    // would silently re-enable a deliberately-disabled connector).
    const enabled = editing ? this.editEnabled : true;

    this.modalBusy = true;
    this.modalError = '';
    this.modalForm?.setErrors({});
    try {
      await setConnector(name, { enabled, kind: 'mcp', config: { endpoint } });
    } catch (err) {
      // 400 → stay in the modal, map details[] to the human labels.
      if (
        err instanceof ApiError &&
        err.code === ErrorCode.VALIDATION_ERROR &&
        err.details.length
      ) {
        this.modalForm?.setErrors(this.mapDetails(err.details));
      } else if (err instanceof ApiError && err.code === ErrorCode.VALIDATION_ERROR) {
        this.modalForm?.setErrors({
          [ENDPOINT_LABEL]: err.message || 'That didn’t work — check the address.',
        });
      } else {
        // Non-400 (502/500/network): surface INSIDE the modal — a page notice
        // renders behind the modal backdrop and would be invisible.
        this.modalError = this.connectorErr(err, name);
      }
      this.modalBusy = false;
      return;
    }
    this.modalBusy = false;
    this.closeModal();
    // Reload FIRST so the new/updated card is in the DOM, THEN (only when the
    // connector is enabled) refresh its tools — never dial a disabled instance.
    this.refocusCard = name;
    await this.load();
    if (enabled) await this.onRefresh(name);
  }

  private mapDetails(details: { field: string; reason: string }[]): Record<string, string> {
    const out: Record<string, string> = {};
    for (const d of details) {
      const f = d.field.toLowerCase();
      if (f.includes('name') || f.includes('connector')) out[NAME_LABEL] = d.reason;
      else if (f.includes('endpoint') || f.includes('url') || f.includes('config'))
        out[ENDPOINT_LABEL] = d.reason;
      else out[ENDPOINT_LABEL] = d.reason;
    }
    return out;
  }

  /** UPSTREAM_ERROR / VALIDATION_ERROR get explicit copy; never userMessageForCode. */
  private connectorErr(err: unknown, name: string): string {
    if (err instanceof ApiError) {
      if (err.code === ErrorCode.UPSTREAM_ERROR) {
        return `Couldn’t reach ${name}. Check the address and try again.`;
      }
      if (err.code === ErrorCode.VALIDATION_ERROR && err.details.length) {
        return err.details.map((d) => d.reason).join(' ');
      }
      if (err.code === ErrorCode.FORBIDDEN) {
        return 'You don’t have access to manage connectors. Ask an owner.';
      }
      if (err.code === ErrorCode.NOT_FOUND) {
        return `${name} no longer exists.`;
      }
    }
    return `Something went wrong with ${name}. Try again.`;
  }

  // ------------------------------- Focus -------------------------------

  private focusAddButton(): void {
    void this.updateComplete.then(() => {
      // fb-button has no focus delegation, so host.focus() is a no-op — reach the
      // inner native <button> (mirrors focusNameOrEndpoint).
      const host = this.renderRoot.querySelector('fb-button.add');
      const inner = host?.shadowRoot?.querySelector<HTMLElement>('button');
      (inner ?? (host as HTMLElement | null))?.focus?.();
    });
  }

  private async focusNameOrEndpoint(): Promise<void> {
    await this.updateComplete;
    // fb-modal focuses its Close button on open (microtask). Defer one frame so
    // we land focus on the field AFTER the modal settles, per spec §5.1.
    const place = (): void => {
      // Focus the Name field when adding; the (only editable) Endpoint when editing.
      // fb-input has no focus delegation, so reach its inner native <input>.
      const sel = this.editName === null ? 'fb-input[name="name"]' : 'fb-input[name="endpoint"]';
      const host = this.renderRoot.querySelector<FbInput>(sel);
      const inner = host?.shadowRoot?.querySelector<HTMLInputElement>('input');
      (inner ?? host)?.focus?.();
    };
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(place);
    else place();
  }

  // ------------------------------- Render -------------------------------

  override render(): TemplateResult {
    if (this.loading) {
      return html`<fb-state mode="loading" skeleton="card" rows="2"></fb-state>`;
    }
    if (this.loadError) {
      return html`<fb-state
        mode="error"
        heading="Couldn’t load connectors"
        error-code=${this.loadError.code}
        request-id=${this.loadError.requestId ?? nothing}
        retryable
        @retry=${() => void this.load()}
      ></fb-state>`;
    }

    const noticeRole = this.notice?.kind === 'err' ? 'alert' : 'status';
    return html`
      <div class="page">
        <div class="head">
          <h1 class="title">Connectors</h1>
          <p class="sub">
            Connectors let BuildOS’s AI use tools from another service you run. You’ll need the
            server’s web address (https) from whoever set it up.
          </p>
        </div>

        ${this.notice
          ? html`<p class="notice ${this.notice.kind}" role=${noticeRole}>
              <fb-icon
                name=${this.notice.kind === 'ok' ? 'check-circle' : 'alert-circle'}
                size="16"
              ></fb-icon>
              ${this.notice.text}
            </p>`
          : nothing}

        <div class="toolbar">
          <fb-button class="add" variant="primary" icon="plus" @click=${this.openAdd}
            >Connect an external tool server</fb-button
          >
        </div>

        ${this.renderBuiltins()} ${this.renderMcps()} ${this.renderModal()} ${this.renderConfirm()}
      </div>
    `;
  }

  private renderBuiltins(): TemplateResult {
    const items = this.builtins;
    if (!items.length) return html`${nothing}`;
    return html`
      <p class="group-label">Built in</p>
      <div class="grid">
        ${items.map(
          (c) =>
            html`<fb-connector-card
              data-connector=${c.connector}
              .connector=${c}
              ?busy=${this.isBusy(c.connector)}
              @toggle=${(e: Event) =>
                void this.onToggle(c, (e as CustomEvent<{ enabled: boolean }>).detail.enabled)}
            ></fb-connector-card>`,
        )}
      </div>
    `;
  }

  private renderMcps(): TemplateResult {
    const items = this.mcps;
    // NOTE: wrap the whole section in one static container and keep every `${}`
    // nested inside a static element (the grid div / the empty-slot div). A bare
    // child-expression sibling at a fragment's top level is silently dropped by
    // happy-dom's part-marker handling in component tests (same footgun the
    // fb-integrations-page comment calls out) — renderBuiltins is immune only
    // because its single `${}` lives inside <div class="grid">.
    return html`
      <section class="mcp-section">
        <p class="group-label">External tool servers</p>
        <div class="grid">
          ${items.map((c) => {
            const refreshError = this.cardErrors.get(c.connector);
            return html`<fb-connector-card
              data-connector=${c.connector}
              .connector=${c}
              cred-state=${this.credStateFor(c.connector)}
              cred-last4=${this.credLast4For(c.connector) ?? nothing}
              refresh-error=${refreshError ?? nothing}
              ?busy=${this.isBusy(c.connector)}
              @toggle=${(e: Event) =>
                void this.onToggle(c, (e as CustomEvent<{ enabled: boolean }>).detail.enabled)}
              @refresh=${() => void this.onRefresh(c.connector)}
              @save-credential=${(e: Event) =>
                void this.onSaveCredential(
                  e.target as FbConnectorCard,
                  c.connector,
                  (e as CustomEvent<{ value: string }>).detail.value,
                )}
              @edit-endpoint=${(e: Event) =>
                this.onEditEndpoint(
                  c.connector,
                  c.enabled,
                  (e as CustomEvent<{ endpoint: string }>).detail.endpoint,
                )}
              @delete=${() => this.onRequestDelete(c.connector)}
            ></fb-connector-card>`;
          })}
        </div>
        <div class="mcp-empty">
          ${items.length
            ? nothing
            : html`<fb-state
                mode="empty"
                icon="command"
                heading="No external tool servers yet"
                message="Connect one to let BuildOS’s AI use its tools."
              ></fb-state>`}
        </div>
      </section>
    `;
  }

  private renderModal(): TemplateResult {
    const editing = this.editName !== null;
    // The submit button lives INSIDE fb-form (not the modal footer slot): fb-form
    // detects a `type=submit` click via its composed path, which only works for
    // buttons in its own subtree (a footer-slotted button is outside it).
    return html`
      <fb-modal
        ?open=${this.modalOpen}
        heading=${editing ? `Edit ${this.editName}` : 'Connect an external tool server'}
        @close=${this.closeModal}
      >
        <fb-form class="modal-form" @submit=${(e: Event) => void this.onModalSubmit(e)}>
          ${this.modalError
            ? html`<p class="modal-error" role="alert">
                <fb-icon name="alert-circle" size="16"></fb-icon>${this.modalError}
              </p>`
            : nothing}
          ${editing
            ? nothing
            : html`<fb-field
                label=${NAME_LABEL}
                hint="A short name you’ll recognize, e.g. my-estimator."
                required
              >
                <fb-input
                  name="name"
                  placeholder="my-estimator"
                  autocomplete="off"
                  maxlength="41"
                  .value=${this.addName}
                  @input=${(e: Event) =>
                    (this.addName = (e as CustomEvent<{ value: string }>).detail.value)}
                ></fb-input>
              </fb-field>`}
          <fb-field
            label=${ENDPOINT_LABEL}
            hint="Must start with https://; private/local addresses are rejected."
            required
          >
            <fb-input
              type="url"
              name="endpoint"
              placeholder="https://tools.example.com"
              .value=${this.editEndpoint}
              autocomplete="off"
            ></fb-input>
          </fb-field>
          <div class="modal-actions">
            <fb-button
              variant="secondary"
              type="button"
              ?disabled=${this.modalBusy}
              @click=${this.closeModal}
              >Cancel</fb-button
            >
            <fb-button variant="primary" type="submit" ?loading=${this.modalBusy}
              >${editing ? 'Save address' : 'Connect'}</fb-button
            >
          </div>
        </fb-form>
      </fb-modal>
    `;
  }

  private renderConfirm(): TemplateResult {
    const name = this.confirmName;
    const message = name
      ? this.confirmHasCred
        ? `Remove the connector “${name}”? This also removes its saved access token.`
        : `Remove the connector “${name}”? This won’t remove any saved access token.`
      : '';
    return html`
      <fb-confirm
        ?open=${!!name}
        heading="Remove connector?"
        message=${message}
        confirm-label="Remove"
        cancel-label="Keep"
        destructive
        @confirm=${() => void this.onConfirmDelete()}
        @cancel=${this.onCancelDelete}
      ></fb-confirm>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-connectors-page': FbConnectorsPage;
  }
}
