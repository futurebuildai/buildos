import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, query } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-badge.js';
import './../atoms/fb-button.js';
import './../atoms/fb-switch.js';
import './../atoms/fb-secret-input.js';
import './../atoms/fb-text.js';
import './../atoms/fb-icon.js';
import type { EffectiveConnector } from '../../types/models.js';
import type { FbSecretInput } from '../atoms/fb-secret-input.js';

/**
 * Credential presence as the page derived it from the active-only integrations
 * row. `unknown` is distinct from `none`: it means the secondary
 * `/api/v1/integrations` fetch failed (partial load), so we must NOT assert
 * "no token" as authoritative.
 */
export type CredState = 'set' | 'none' | 'unknown';

/**
 * `fb-connector-card` — one connector row (Phase 3c §5.2/§5.3). A **dumb**
 * molecule: it renders connector state and emits intent events; the PAGE owns
 * every async call + all state (mirrors how `fb-integrations-page` does the work
 * while `fb-integration-card` only emits save/toggle/test).
 *
 * Branches strictly on `connector.kind` (`'builtin' | 'mcp'`) — NEVER the name
 * string, since an MCP instance can be named anything. All MCP affordances
 * (endpoint, refresh, credential, edit, delete) render regardless of
 * `enabled`/`source`, so a disabled instance is never stranded.
 *
 * Emits (all composed/bubbling):
 *  - `toggle`          `{ enabled: boolean }` — page maps to enable-PUT or, for a
 *                      default-OFF built-in turned off, a DELETE.
 *  - `refresh`         (no detail) — re-run tools/list upstream.
 *  - `save-credential` `{ value: string }` — set the MCP bearer token in the vault.
 *  - `edit-endpoint`   `{ endpoint: string }` — re-PUT the endpoint (502 typo recovery).
 *  - `delete`          (no detail) — page opens its own confirm + deletes.
 *
 * Never renders secret material: the credential is write-only (`last4` only).
 */
@customElement('fb-connector-card')
export class FbConnectorCard extends FBElement {
  // Delegate focus so the page can restore keyboard focus into this card after a
  // refetch re-render (host.focus() → the card's first focusable control) without
  // reaching across the shadow boundary. See fb-connectors-page.updated().
  static override shadowRootOptions = { ...FBElement.shadowRootOptions, delegatesFocus: true };

  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .card {
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
      .head {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
      }
      .name {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        font-weight: 600;
        font-size: var(--fb-text-title-md);
        color: var(--fb-text-primary);
      }
      .desc {
        margin: var(--fb-spacing-xs) 0 0;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .endpoint {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-sm);
        color: var(--fb-text-muted);
      }
      .endpoint fb-text {
        word-break: break-all;
      }
      .status-row {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: var(--fb-spacing-sm);
        margin-top: var(--fb-spacing-sm);
      }
      .badge-src {
        margin-left: var(--fb-spacing-xs);
      }
      .card-error {
        display: flex;
        align-items: flex-start;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-safety-red-text);
        background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
        border: 1px solid var(--fb-safety-red);
        border-radius: var(--fb-radius-sm);
      }
      .card-error fb-icon {
        flex: none;
        margin-top: 2px;
      }
      .actions {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: var(--fb-spacing-sm);
        margin-top: var(--fb-spacing-md);
      }
      .cred {
        margin-top: var(--fb-spacing-md);
        padding-top: var(--fb-spacing-md);
        border-top: 1px solid var(--fb-border, var(--md-sys-color-outline));
      }
      .cred-label {
        display: block;
        margin-bottom: var(--fb-spacing-xs);
        font-size: var(--fb-text-label-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .cred-row {
        display: flex;
        align-items: stretch;
        gap: var(--fb-spacing-sm);
      }
      .cred-row fb-secret-input {
        flex: 1;
      }
      .cred-hint {
        margin: var(--fb-spacing-xs) 0 0;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
    `,
  ];

  /** The effective connector row from GET /admin/connectors. */
  @property({ attribute: false }) connector!: EffectiveConnector;
  /** Active-only credential presence the page derived from /api/v1/integrations. */
  @property({ type: String, attribute: 'cred-state' }) credState: CredState = 'none';
  /** Last 4 of the active bearer token (recognition only; never the key). */
  @property({ type: String, attribute: 'cred-last4' }) credLast4?: string;
  /** Page-owned in-flight gate: disables this card's controls + shows spinners. */
  @property({ type: Boolean }) busy = false;
  /**
   * Persistent card-level error copy (e.g. a 502 from refresh). Rendered as a
   * standing region, NOT a vanishing toast — the page sets/clears it.
   */
  @property({ type: String, attribute: 'refresh-error' }) refreshError?: string;

  @query('fb-secret-input') private secret?: FbSecretInput;

  private pendingCred = '';

  /** The display name (alias for the wire `connector`). */
  private get name(): string {
    return this.connector.connector;
  }

  private onToggle(e: Event): void {
    const enabled = (e as CustomEvent<{ checked: boolean }>).detail.checked;
    this.emit('toggle', { enabled });
  }

  private onRefresh(): void {
    this.emit('refresh');
  }

  private onEditEndpoint(): void {
    this.emit('edit-endpoint', { endpoint: this.connector.endpoint ?? '' });
  }

  private onDelete(): void {
    this.emit('delete');
  }

  private onCredInput(e: Event): void {
    this.pendingCred = (e as CustomEvent<{ value: string }>).detail.value;
  }

  private onSaveCredential(): void {
    if (!this.pendingCred) return;
    this.emit('save-credential', { value: this.pendingCred });
    this.pendingCred = '';
  }

  /**
   * Wipe the plaintext from the DOM. The page calls this in its `finally` (both
   * success and error) after `setCredential`, so the bearer never lingers.
   */
  clearCredential(): void {
    this.pendingCred = '';
    this.secret?.submit();
  }

  // -------------------------------- Builtin --------------------------------

  private renderSourceBadge(): TemplateResult {
    const override = this.connector.source === 'override';
    return html`<fb-badge class="badge-src" size="sm" status=${override ? 'active' : 'neutral'}
      >${override ? 'Your settings' : 'Standard settings'}</fb-badge
    >`;
  }

  private renderBuiltin(): TemplateResult {
    return html`
      <div class="card">
        <div class="head">
          <div>
            <span class="name"><fb-icon name="hexagon" size="18"></fb-icon>${this.name}</span>
            <p class="desc">${this.connector.description}</p>
          </div>
          <fb-switch
            ?checked=${this.connector.enabled}
            ?disabled=${this.busy}
            label="Enable reference connector"
            @change=${this.onToggle}
          ></fb-switch>
        </div>
        <div class="status-row">${this.renderSourceBadge()}</div>
      </div>
    `;
  }

  // ---------------------------------- MCP ----------------------------------

  /** Tools-status badge: refreshed (neutral) · never-refreshed-when-enabled (warning). */
  private renderToolsBadge(): TemplateResult {
    const c = this.connector;
    if (c.tools_fetched_at) {
      // Refreshed (incl. a genuine zero — a working empty server is NOT nagged).
      const noun = c.tools_count === 1 ? 'tool' : 'tools';
      return html`<fb-badge status="neutral"
        >${c.tools_count} ${noun} · refreshed ${c.tools_fetched_at}</fb-badge
      >`;
    }
    if (c.enabled) {
      // Enabled but never refreshed → a toolless no-op; nag for a refresh.
      return html`<fb-badge status="warning">No tools loaded</fb-badge>`;
    }
    return html`<fb-badge status="neutral">Not loaded yet</fb-badge>`;
  }

  private renderCredential(): TemplateResult {
    const hasValue = this.credState === 'set';
    return html`
      <div class="cred">
        <span class="cred-label" id="cred-${this.connector.connector}"
          >${this.name} access token</span
        >
        <div class="cred-row">
          <fb-secret-input
            label="${this.name} bearer token"
            describedby="cred-hint-${this.connector.connector}"
            ?has-value=${hasValue}
            last4=${this.credState === 'set' && this.credLast4 ? this.credLast4 : nothing}
            ?disabled=${this.busy}
            @input=${this.onCredInput}
          ></fb-secret-input>
          <fb-button
            variant="secondary"
            size="sm"
            ?disabled=${this.busy}
            @click=${this.onSaveCredential}
            >Save token</fb-button
          >
        </div>
        <p class="cred-hint" id="cred-hint-${this.connector.connector}">
          ${this.credState === 'unknown'
            ? "We couldn't check whether a token is saved. Saving one will still work."
            : 'Access token — only if the server requires sign-in. Leave blank if it’s open.'}
        </p>
      </div>
    `;
  }

  private renderMcp(): TemplateResult {
    const c = this.connector;
    return html`
      <div class="card">
        <div class="head">
          <div>
            <span class="name"><fb-icon name="command" size="18"></fb-icon>${this.name}</span>
            ${c.description ? html`<p class="desc">${c.description}</p>` : nothing}
          </div>
          <fb-switch
            ?checked=${c.enabled}
            ?disabled=${this.busy}
            label="Enable ${this.name}"
            @change=${this.onToggle}
          ></fb-switch>
        </div>

        ${c.endpoint
          ? html`<p class="endpoint">
              <fb-icon name="command" size="14"></fb-icon>
              <fb-text variant="body-sm" tone="muted" mono inline>${c.endpoint}</fb-text>
            </p>`
          : nothing}

        <div class="status-row" aria-live="polite">
          ${this.renderToolsBadge()}${this.renderSourceBadge()}
        </div>

        ${this.refreshError
          ? html`<p class="card-error" role="alert">
              <fb-icon name="alert-circle" size="16"></fb-icon>${this.refreshError}
            </p>`
          : nothing}

        <div class="actions">
          <fb-button
            variant="secondary"
            size="sm"
            icon="refresh"
            label="Refresh tools for ${this.name}"
            ?loading=${this.busy}
            ?disabled=${this.busy}
            @click=${this.onRefresh}
            >Refresh tools</fb-button
          >
          <fb-button
            variant="ghost"
            size="sm"
            icon="pencil"
            ?disabled=${this.busy}
            @click=${this.onEditEndpoint}
            >Edit address</fb-button
          >
          <fb-button
            variant="destructive"
            size="sm"
            icon="trash"
            label="Delete connector ${this.name}"
            ?disabled=${this.busy}
            @click=${this.onDelete}
            >Remove</fb-button
          >
        </div>

        ${this.renderCredential()}
      </div>
    `;
  }

  override render(): TemplateResult {
    if (!this.connector) return html`${nothing}`;
    // Branch on KIND, never the name. Built-in is enable-only; MCP renders all
    // affordances regardless of enabled/source.
    return this.connector.kind === 'builtin' ? this.renderBuiltin() : this.renderMcp();
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-connector-card': FbConnectorCard;
  }
}
