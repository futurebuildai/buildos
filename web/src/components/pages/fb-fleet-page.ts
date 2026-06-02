import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-input.js';
import '../atoms/fb-select.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-state.js';
import { listFleetAssets, allocateAsset } from '../../api/endpoints/fleet.js';
import { listProjects } from '../../api/endpoints/projects.js';
import type { FleetAsset, FleetAssetStatus, Project } from '../../types/models.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { authClaims } from '../../state/authStore.js';
import type { BadgeStatus } from '../atoms/fb-badge.js';
import type { SelectOption } from '../atoms/fb-select.js';

function assetBadge(status: FleetAssetStatus): { status: BadgeStatus; label: string } {
  switch (status) {
    case 'available':
      return { status: 'active', label: 'Available' };
    case 'maintenance':
      return { status: 'warning', label: 'Maintenance' };
    case 'unavailable':
    default:
      return { status: 'offline', label: 'Unavailable' };
  }
}

/**
 * `fb-fleet-page` — equipment fleet (UX_CORE_SCREENS §7). Superintendent+ may
 * list and allocate; create is owner/admin (not surfaced here). Allocation is
 * enforced server-side by a GiST no-overlap constraint: an overlapping booking
 * returns 409 CONFLICT, which this page surfaces inline in the allocate dialog
 * rather than as a destructive toast.
 */
@customElement('fb-fleet-page')
export class FbFleetPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .asset-card {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
      .ac-top {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
      }
      .ac-name {
        font-size: var(--fb-text-title-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .ac-meta {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .ac-actions {
        margin-top: var(--fb-spacing-xs);
      }
      .field {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        margin-bottom: var(--fb-spacing-md);
      }
      .field label {
        font-size: var(--fb-text-label-lg);
        color: var(--fb-text-secondary);
      }
      .dialog-error {
        margin-bottom: var(--fb-spacing-md);
        color: var(--fb-safety-red);
        font-size: var(--fb-text-body-sm);
      }
    `,
  ];

  @state() private assets: FleetAsset[] = [];
  @state() private projects: Project[] = [];
  @state() private loading = true;
  @state() private errorCode: string | null = null;
  @state() private notice: { kind: 'ok' | 'err'; text: string } | null = null;

  // Allocate-dialog state.
  @state() private allocating: FleetAsset | null = null;
  @state() private allocProjectId = '';
  @state() private allocStart = '';
  @state() private allocEnd = '';
  @state() private allocError: string | null = null;
  @state() private allocBusy = false;

  private get orgId(): string {
    return authClaims.get()?.orgId ?? '';
  }

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      const [assets, projects] = await Promise.all([listFleetAssets(this.orgId), listProjects()]);
      this.assets = assets;
      this.projects = projects;
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private openAllocate(asset: FleetAsset): void {
    this.allocating = asset;
    this.allocProjectId = '';
    this.allocStart = '';
    this.allocEnd = '';
    this.allocError = null;
  }

  private closeAllocate(): void {
    this.allocating = null;
  }

  private async submitAllocate(): Promise<void> {
    if (!this.allocating) return;
    if (!this.allocProjectId || !this.allocStart || !this.allocEnd) {
      this.allocError = 'Pick a project and a start and end date.';
      return;
    }
    this.allocBusy = true;
    this.allocError = null;
    try {
      await allocateAsset(this.orgId, this.allocating.id, {
        project_id: this.allocProjectId,
        start_date: this.allocStart,
        end_date: this.allocEnd,
      });
      this.notice = { kind: 'ok', text: `${this.allocating.name} allocated.` };
      this.closeAllocate();
      await this.load();
    } catch (err) {
      if (err instanceof ApiError && err.code === ErrorCode.CONFLICT) {
        this.allocError = 'That asset is already booked for an overlapping date range.';
      } else {
        this.allocError =
          err instanceof ApiError ? userMessageForCode(err.code) : 'Could not allocate the asset.';
      }
    } finally {
      this.allocBusy = false;
    }
  }

  private projectOptions(): SelectOption[] {
    return this.projects.map((p) => ({ value: p.id, label: p.name }));
  }

  private renderCard(a: FleetAsset): TemplateResult {
    const badge = assetBadge(a.status);
    return html`<div class="asset-card">
      <div class="ac-top">
        <span class="ac-name">${a.name}</span>
        <fb-badge size="sm" status=${badge.status}>${badge.label}</fb-badge>
      </div>
      <span class="ac-meta">${a.asset_type}${a.serial_number ? ` · ${a.serial_number}` : ''}</span>
      <div class="ac-actions">
        <fb-button
          size="sm"
          variant="secondary"
          icon="truck"
          ?disabled=${a.status !== 'available'}
          @click=${() => this.openAllocate(a)}
          >Allocate</fb-button
        >
      </div>
    </div>`;
  }

  private renderDialog(): TemplateResult {
    const a = this.allocating;
    if (!a) return html`${nothing}`;
    return html`<fb-modal open heading="Allocate ${a.name}" @close=${this.closeAllocate}>
      ${this.allocError
        ? html`<p class="dialog-error" role="alert">${this.allocError}</p>`
        : nothing}
      <div class="field">
        <label for="alloc-project">Project</label>
        <fb-select
          id="alloc-project"
          placeholder="Select a project"
          .options=${this.projectOptions()}
          value=${this.allocProjectId}
          @change=${(e: Event) =>
            (this.allocProjectId = (e as CustomEvent<{ value: string }>).detail.value)}
        ></fb-select>
      </div>
      <div class="field">
        <label for="alloc-start">Start date</label>
        <fb-input
          id="alloc-start"
          type="date"
          value=${this.allocStart}
          @change=${(e: Event) =>
            (this.allocStart = (e as CustomEvent<{ value: string }>).detail.value)}
        ></fb-input>
      </div>
      <div class="field">
        <label for="alloc-end">End date</label>
        <fb-input
          id="alloc-end"
          type="date"
          value=${this.allocEnd}
          @change=${(e: Event) =>
            (this.allocEnd = (e as CustomEvent<{ value: string }>).detail.value)}
        ></fb-input>
      </div>
      <fb-button slot="footer" variant="ghost" @click=${this.closeAllocate}>Cancel</fb-button>
      <fb-button
        slot="footer"
        variant="primary"
        ?loading=${this.allocBusy}
        @click=${() => void this.submitAllocate()}
        >Allocate</fb-button
      >
    </fb-modal>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Fleet</h1>
            <p class="page-sub">Equipment and vehicles available to allocate to projects.</p>
          </div>
        </div>

        ${this.notice
          ? html`<p class="toast ${this.notice.kind}" role="status">
              <fb-icon
                name=${this.notice.kind === 'ok' ? 'check-circle' : 'alert-circle'}
                size="16"
              ></fb-icon>
              ${this.notice.text}
            </p>`
          : nothing}
        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="6"></fb-state>`
          : this.errorCode
            ? html`<fb-state
                mode="error"
                error-code=${this.errorCode}
                retryable
                @retry=${() => void this.load()}
              ></fb-state>`
            : this.assets.length === 0
              ? html`<fb-state
                  mode="empty"
                  icon="truck"
                  heading="No fleet assets"
                  message="Equipment will appear here once it’s added by an owner or admin."
                ></fb-state>`
              : html`<div class="card-grid">${this.assets.map((a) => this.renderCard(a))}</div>`}
        ${this.renderDialog()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-fleet-page': FbFleetPage;
  }
}
