import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-chip.js';
import '../atoms/fb-icon.js';
import '../molecules/fb-feed-list.js';
import '../organisms/fb-state.js';
import { getDailyBriefing } from '../../api/endpoints/briefing.js';
import type { DailyBriefing } from '../../types/models.js';
import { ApiError, ErrorCode } from '../../api/errors.js';
import { aiConfigured, markAiUnconfigured } from '../../state/capabilityStore.js';
import { hasRole } from '../../state/authStore.js';
import { navigate } from '../../router.js';

/** AI state for the hero, derived from the capability signal + call outcome. */
type AiState = 'ok' | 'gated' | 'transient';

/**
 * `fb-briefing-page` — the Daily Briefing (UX_CORE_SCREENS §10). The hero renders
 * the native-AI morning summary (markdown), with `task_count`/`alert_count`
 * context chips and the critical+urgent feed below. AI is key-gated (§9): a 503
 * (or assume-off capability) replaces the hero with the `gated` panel while the
 * feed — which needs no AI — always renders. The briefing never hard-fails the
 * Command Center: any AI error degrades to a panel, not a blank screen.
 */
@customElement('fb-briefing-page')
export class FbBriefingPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .hero {
        padding: var(--fb-spacing-lg);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
        margin-bottom: var(--fb-spacing-lg);
      }
      .chips {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        margin-bottom: var(--fb-spacing-md);
      }
      .reply p {
        margin: 0 0 var(--fb-spacing-sm);
        color: var(--fb-text-primary);
        line-height: 1.6;
        white-space: pre-wrap;
      }
      .reply p:last-child {
        margin-bottom: 0;
      }
      .section-label {
        font-size: var(--fb-text-title-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin: 0 0 var(--fb-spacing-sm);
      }
    `,
  ];

  @state() private briefing: DailyBriefing | null = null;
  @state() private aiState: AiState = 'ok';
  @state() private loading = true;
  @state() private errorRequestId: string | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    // Proactive gate: if we positively know AI is off, skip the call entirely.
    if (!aiConfigured.get()) {
      this.aiState = 'gated';
      this.loading = false;
      return;
    }
    this.loading = true;
    this.aiState = 'ok';
    this.errorRequestId = null;
    try {
      this.briefing = await getDailyBriefing();
    } catch (err) {
      if (err instanceof ApiError) {
        this.errorRequestId = err.requestId ?? null;
        if (err.code === ErrorCode.SERVICE_UNAVAILABLE || err.isAiUnconfigured) {
          markAiUnconfigured(); // reactive soft-fail (§9)
          this.aiState = 'gated';
        } else {
          this.aiState = 'transient';
        }
      } else {
        this.aiState = 'transient';
      }
    } finally {
      this.loading = false;
    }
  }

  /** Minimal markdown: split blank-line-separated paragraphs (no new deps). */
  private renderReply(reply: string): TemplateResult {
    const paragraphs = reply
      .split(/\n{2,}/)
      .map((p) => p.trim())
      .filter(Boolean);
    return html`<div class="reply">${paragraphs.map((p) => html`<p>${p}</p>`)}</div>`;
  }

  private renderHero(): TemplateResult {
    if (this.loading)
      return html`<div class="hero">
        <fb-state mode="loading" skeleton="text" rows="4"></fb-state>
        <p class="page-sub">Generating your briefing…</p>
      </div>`;

    if (this.aiState === 'gated')
      return html`<fb-state
        mode="gated"
        heading="Daily briefing is off"
        message="Add an Anthropic API key to enable the AI morning briefing."
        ?can-configure=${hasRole('owner')}
        @configure=${() => navigate('/settings/integrations')}
      ></fb-state>`;

    if (this.aiState === 'transient')
      return html`<fb-state
        mode="error"
        error-code=${ErrorCode.UPSTREAM_ERROR}
        request-id=${this.errorRequestId ?? nothing}
        retryable
        @retry=${() => void this.load()}
      ></fb-state>`;

    const b = this.briefing!;
    const calm = b.task_count === 0 && b.alert_count === 0;
    return html`<div class="hero">
      <div class="chips">
        <fb-chip>${b.task_count} ${b.task_count === 1 ? 'task' : 'tasks'} assigned</fb-chip>
        <fb-chip>${b.alert_count} ${b.alert_count === 1 ? 'alert' : 'alerts'}</fb-chip>
      </div>
      ${calm ? html`<p class="section-label">Nothing urgent this morning</p>` : nothing}
      ${this.renderReply(b.reply)}
    </div>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Daily Briefing</h1>
            <p class="page-sub">Your morning summary and the alerts behind it.</p>
          </div>
        </div>
        ${this.renderHero()}
        <p class="section-label">Priority alerts</p>
        <fb-feed-list .priorities=${['critical', 'urgent']}></fb-feed-list>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-briefing-page': FbBriefingPage;
  }
}
