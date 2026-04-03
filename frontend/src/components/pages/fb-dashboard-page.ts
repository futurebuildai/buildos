import { html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { listProjects, listFeed, listProcurement, listTasks, type Project, type FeedCard, type TaskSchedule, ApiError } from '../../state/api.js';
import { currentProject } from '../../state/store.js';
import { navigateTo } from '../../router.js';

@customElement('fb-dashboard-page')
export class FBDashboardPage extends FBBaseElement {
  @state() private _loading = true;
  @state() private _projects: Project[] = [];
  @state() private _feedCards: FeedCard[] = [];
  @state() private _procurementAlerts = 0;
  @state() private _upcomingTasks: TaskSchedule[] = [];
  @state() private _error = '';

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host {
        display: block;
        padding: var(--fb-space-6);
      }

      .page-header {
        margin-bottom: var(--fb-space-6);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .page-header p {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-top: var(--fb-space-1);
      }

      .stats-grid {
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        gap: var(--fb-space-4);
        margin-bottom: var(--fb-space-6);
      }

      @media (max-width: 1024px) {
        .stats-grid { grid-template-columns: repeat(2, 1fr); }
      }

      @media (max-width: 640px) {
        .stats-grid { grid-template-columns: 1fr; }
      }

      .stat-card {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        transition: border-color var(--fb-transition-fast), box-shadow var(--fb-transition-fast);
        cursor: pointer;
      }

      .stat-card:hover {
        border-color: var(--fb-border-hover);
        box-shadow: var(--fb-shadow-glow);
      }

      .stat-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-3xl);
        font-weight: 700;
        color: var(--fb-gable-green);
        font-variant-numeric: tabular-nums;
      }

      .stat-label {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-top: var(--fb-space-1);
      }

      .stat-card.alert .stat-value {
        color: var(--fb-safety-red);
      }

      .stat-card.warning .stat-value {
        color: var(--fb-amber);
      }

      .content-grid {
        display: grid;
        grid-template-columns: 2fr 1fr;
        gap: var(--fb-space-6);
      }

      @media (max-width: 1024px) {
        .content-grid { grid-template-columns: 1fr; }
      }

      .section-title {
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
      }

      .feed-list {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-3);
      }

      .feed-item {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-4);
        transition: border-color var(--fb-transition-fast);
      }

      .feed-item:hover {
        border-color: var(--fb-border-hover);
      }

      .feed-item-header {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
        margin-bottom: var(--fb-space-2);
      }

      .feed-priority {
        display: inline-block;
        width: 8px;
        height: 8px;
        border-radius: 50%;
        flex-shrink: 0;
      }

      .feed-priority.critical { background: var(--fb-safety-red); }
      .feed-priority.urgent { background: var(--fb-amber); }
      .feed-priority.normal { background: var(--fb-blueprint-blue); }
      .feed-priority.low { background: var(--fb-text-muted); }

      .feed-title {
        font-size: var(--fb-text-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }

      .feed-body {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
        line-height: 1.4;
      }

      .feed-time {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        font-family: var(--fb-font-mono);
        margin-top: var(--fb-space-2);
      }

      .quick-links {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
      }

      .quick-link {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
        padding: var(--fb-space-3) var(--fb-space-4);
        background: var(--fb-glass-bg);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-md);
        color: var(--fb-text-primary);
        cursor: pointer;
        transition: border-color var(--fb-transition-fast), background var(--fb-transition-fast);
        font-size: var(--fb-text-sm);
      }

      .quick-link:hover {
        border-color: var(--fb-gable-green);
        background: rgba(0, 255, 163, 0.05);
      }

      .quick-link-icon {
        font-size: var(--fb-text-lg);
        width: 24px;
        text-align: center;
      }

      .error-banner {
        color: var(--fb-safety-red);
        padding: var(--fb-space-4);
        background: rgba(244, 63, 94, 0.1);
        border-radius: var(--fb-radius-md);
        border: 1px solid rgba(244, 63, 94, 0.2);
        margin-bottom: var(--fb-space-4);
        font-size: var(--fb-text-sm);
      }

      .loading-container {
        display: flex;
        align-items: center;
        justify-content: center;
        min-height: 300px;
      }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._loadData();
  }

  render() {
    if (this._loading) {
      return html`
        <div class="loading-container">
          <fb-spinner></fb-spinner>
        </div>
      `;
    }

    return html`
      <div class="page-header">
        <h1>Dashboard</h1>
        <p>Welcome to FutureBuild OS — your construction command center.</p>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : ''}

      <div class="stats-grid">
        <div class="stat-card" @click=${() => navigateTo('/schedule')}>
          <div class="stat-value">${this._projects.filter((p) => p.status === 'active').length}</div>
          <div class="stat-label">Active Projects</div>
        </div>
        <div class="stat-card ${this._feedCards.length > 5 ? 'warning' : ''}" @click=${() => navigateTo('/briefing')}>
          <div class="stat-value">${this._feedCards.length}</div>
          <div class="stat-label">Pending Feed Cards</div>
        </div>
        <div class="stat-card ${this._procurementAlerts > 0 ? 'alert' : ''}" @click=${() => navigateTo('/procurement')}>
          <div class="stat-value">${this._procurementAlerts}</div>
          <div class="stat-label">Procurement Alerts</div>
        </div>
        <div class="stat-card" @click=${() => navigateTo('/schedule')}>
          <div class="stat-value">${this._upcomingTasks.length}</div>
          <div class="stat-label">Upcoming Tasks</div>
        </div>
      </div>

      <div class="content-grid">
        <div>
          <h2 class="section-title">Recent Feed</h2>
          <div class="feed-list">
            ${this._feedCards.length === 0
              ? html`<div class="feed-item"><div class="feed-body">No active feed cards.</div></div>`
              : this._feedCards.slice(0, 5).map(
                  (card) => html`
                    <div class="feed-item">
                      <div class="feed-item-header">
                        <span class="feed-priority ${card.priority}"></span>
                        <span class="feed-title">${card.title}</span>
                      </div>
                      <div class="feed-body">${card.body}</div>
                      <div class="feed-time">${this._formatTime(card.created_at)}</div>
                    </div>
                  `,
                )}
          </div>
        </div>
        <div>
          <h2 class="section-title">Quick Links</h2>
          <div class="quick-links">
            <div class="quick-link" @click=${() => navigateTo('/financials')}>
              <span class="quick-link-icon">$</span>
              <span>Financials</span>
            </div>
            <div class="quick-link" @click=${() => navigateTo('/schedule')}>
              <span class="quick-link-icon">&#128197;</span>
              <span>Schedule & Gantt</span>
            </div>
            <div class="quick-link" @click=${() => navigateTo('/pipeline')}>
              <span class="quick-link-icon">&#128200;</span>
              <span>Pipeline</span>
            </div>
            <div class="quick-link" @click=${() => navigateTo('/procurement')}>
              <span class="quick-link-icon">&#128230;</span>
              <span>Procurement</span>
            </div>
            <div class="quick-link" @click=${() => navigateTo('/fleet')}>
              <span class="quick-link-icon">&#128666;</span>
              <span>Fleet Management</span>
            </div>
            <div class="quick-link" @click=${() => navigateTo('/briefing')}>
              <span class="quick-link-icon">&#128276;</span>
              <span>Daily Briefing</span>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  private _formatTime(isoString: string): string {
    try {
      const date = new Date(isoString);
      const now = new Date();
      const diffMs = now.getTime() - date.getTime();
      const diffMins = Math.floor(diffMs / 60000);
      if (diffMins < 60) return `${diffMins}m ago`;
      const diffHours = Math.floor(diffMins / 60);
      if (diffHours < 24) return `${diffHours}h ago`;
      const diffDays = Math.floor(diffHours / 24);
      return `${diffDays}d ago`;
    } catch {
      return isoString;
    }
  }

  private async _loadData() {
    this._loading = true;
    this._error = '';

    try {
      const [projectsRes, feedRes] = await Promise.allSettled([
        listProjects({ status: 'active' }),
        listFeed({ status: 'active' }),
      ]);

      if (projectsRes.status === 'fulfilled') {
        this._projects = projectsRes.value.projects;
      }

      if (feedRes.status === 'fulfilled') {
        this._feedCards = feedRes.value.cards;
      }

      // Load procurement alerts for first active project
      const activeProject = this._projects[0];
      if (activeProject) {
        currentProject.set(activeProject.id);
        try {
          const procRes = await listProcurement(activeProject.id, 'WARNING,CRITICAL');
          this._procurementAlerts = procRes.items.length;
        } catch {
          // Non-critical, ignore
        }

        try {
          const taskRes = await listTasks(activeProject.id, { status: 'pending' });
          this._upcomingTasks = taskRes.tasks.slice(0, 10);
        } catch {
          // Non-critical, ignore
        }
      }
    } catch (err) {
      if (err instanceof ApiError) {
        this._error = `Failed to load dashboard data (${err.status})`;
      } else {
        this._error = 'Failed to load dashboard data';
      }
    } finally {
      this._loading = false;
    }
  }
}
