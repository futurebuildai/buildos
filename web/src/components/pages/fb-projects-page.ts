import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-icon.js';
import '../organisms/fb-state.js';
import { listProjects } from '../../api/endpoints/projects.js';
import type { Project } from '../../types/models.js';
import { ApiError } from '../../api/errors.js';
import type { BadgeStatus } from '../atoms/fb-badge.js';

/** Backend project.status → badge vocabulary (color+icon+text, never color-only). */
function projectBadge(status: string): { status: BadgeStatus; label: string } {
  switch (status) {
    case 'active':
    case 'in_progress':
      return { status: 'active', label: 'Active' };
    case 'complete':
    case 'completed':
      return { status: 'complete', label: 'Complete' };
    case 'on_hold':
    case 'paused':
      return { status: 'pending', label: 'On hold' };
    case 'cancelled':
    case 'canceled':
      return { status: 'critical', label: 'Cancelled' };
    default:
      return { status: 'neutral', label: status || 'Unknown' };
  }
}

/**
 * `fb-projects-page` — the portfolio Projects grid (UX_CORE_SCREENS §1). Any
 * authenticated user may list; create/edit is owner/admin (router-gated, not
 * shown here). Each card deep-links to the project detail tabs. There is no
 * server-side completion rollup yet (OQ-4), so cards show status + key dates
 * rather than a progress bar — the bar lands when the summary endpoint does.
 */
@customElement('fb-projects-page')
export class FbProjectsPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .project-card {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
        color: inherit;
        text-decoration: none;
      }
      .project-card:hover {
        border-color: var(--fb-gable-green);
      }
      .project-card:focus-visible {
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 2px;
      }
      .pc-top {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
      }
      .pc-name {
        font-size: var(--fb-text-title-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .pc-meta {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
    `,
  ];

  @state() private projects: Project[] = [];
  @state() private loading = true;
  @state() private errorCode: string | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.projects = await listProjects();
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private renderCard(p: Project): TemplateResult {
    const badge = projectBadge(p.status);
    return html`<a class="project-card" href="/portfolio/projects/${p.id}">
      <div class="pc-top">
        <span class="pc-name">${p.name}</span>
        <fb-badge size="sm" status=${badge.status}>${badge.label}</fb-badge>
      </div>
      ${p.address
        ? html`<span class="pc-meta"
            ><fb-icon name="building" size="14"></fb-icon>${p.address}</span
          >`
        : nothing}
      ${p.project_start_date
        ? html`<span class="pc-meta"
            ><fb-icon name="calendar" size="14"></fb-icon>Starts ${p.project_start_date}</span
          >`
        : nothing}
    </a>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Projects</h1>
            <p class="page-sub">Every active and planned build in your portfolio.</p>
          </div>
        </div>

        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="6"></fb-state>`
          : this.errorCode
            ? html`<fb-state
                mode="error"
                error-code=${this.errorCode}
                retryable
                @retry=${() => void this.load()}
              ></fb-state>`
            : this.projects.length === 0
              ? html`<fb-state
                  mode="empty"
                  icon="folder"
                  heading="No projects yet"
                  message="Projects will appear here once they’re created."
                ></fb-state>`
              : html`<div class="card-grid">${this.projects.map((p) => this.renderCard(p))}</div>`}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-projects-page': FbProjectsPage;
  }
}
