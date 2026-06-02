import { html, css, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-icon.js';
import '../organisms/fb-state.js';
import { aiConfigured } from '../../state/capabilityStore.js';
import { hasRole } from '../../state/authStore.js';
import { navigate } from '../../router.js';

interface Capability {
  icon: string;
  title: string;
  blurb: string;
  href: string;
  cta: string;
}

/** The AI surfaces that actually have a backend today (native-AI agents, §3/§10). */
const CAPABILITIES: Capability[] = [
  {
    icon: 'sun',
    title: 'Daily Briefing',
    blurb: 'A morning summary of what needs attention, generated from your live project state.',
    href: '/command/briefing',
    cta: 'Open briefing',
  },
  {
    icon: 'sliders',
    title: 'Schedule adjustments',
    blurb:
      'Ask the model to review a schedule and propose duration changes, applied transparently.',
    href: '/command/schedule',
    cta: 'Open schedule',
  },
];

/**
 * `fb-assistant-page` — the AI Assistant surface (UX_CORE_SCREENS §9). Pro tier,
 * superintendent+ (router-gated). There is no conversational endpoint today; the
 * native-AI agents are task-scoped (daily briefing, schedule adjustments), so this
 * page is an honest hub into those live surfaces rather than a chat box. It gates
 * on AI availability (§9): with no key configured it renders the `gated` panel and
 * an owner-only deep link to Integrations.
 */
@customElement('fb-assistant-page')
export class FbAssistantPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
        gap: var(--fb-spacing-md);
      }
      .cap {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-lg);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
      }
      .cap fb-icon {
        color: var(--fb-gable-green);
      }
      .cap h2 {
        margin: 0;
        font-size: var(--fb-text-title-sm);
        color: var(--fb-text-primary);
      }
      .cap p {
        margin: 0;
        flex: 1;
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
        line-height: 1.5;
      }
      .cap a {
        align-self: flex-start;
        color: var(--fb-blueprint-blue);
        font-weight: 600;
        font-size: var(--fb-text-body-sm);
        text-decoration: none;
      }
      .cap a:hover {
        text-decoration: underline;
      }
    `,
  ];

  /** Snapshot at connect — FBElement is not signal-reactive, so read imperatively. */
  @state() private aiOn = true;

  override connectedCallback(): void {
    super.connectedCallback();
    this.aiOn = aiConfigured.get();
  }

  private renderBody(): TemplateResult {
    if (!this.aiOn)
      return html`<fb-state
        mode="gated"
        heading="AI features are off"
        message="Add an Anthropic API key to turn on the AI assistant and its agents."
        ?can-configure=${hasRole('owner')}
        @configure=${() => navigate('/settings/integrations')}
      ></fb-state>`;

    return html`<div class="grid">
      ${CAPABILITIES.map(
        (c) =>
          html`<div class="cap">
            <fb-icon name=${c.icon} size="24"></fb-icon>
            <h2>${c.title}</h2>
            <p>${c.blurb}</p>
            <a href=${c.href}>${c.cta} →</a>
          </div>`,
      )}
    </div>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">AI Assistant</h1>
            <p class="page-sub">Native-AI agents that work from your live project data.</p>
          </div>
        </div>
        ${this.renderBody()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-assistant-page': FbAssistantPage;
  }
}
