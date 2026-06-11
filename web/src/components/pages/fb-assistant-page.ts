import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state, query } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-chip.js';
import '../atoms/fb-button.js';
import '../atoms/fb-markdown.js';
import '../organisms/fb-state.js';
import { sendChat } from '../../api/endpoints/assistant.js';
import type { ChatTurn, ToolTrace } from '../../types/models.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { aiConfigured, markAiUnconfigured } from '../../state/capabilityStore.js';
import { hasRole, hasMinRole } from '../../state/authStore.js';
import { navigate } from '../../router.js';
import type { IconName } from '../atoms/icons.js';

/**
 * AI availability / outcome state for the page:
 *  - `ok`       — chat is live.
 *  - `gated`    — 503 SERVICE_UNAVAILABLE: no Anthropic key. Owner can deep-link
 *                 to Integrations to add one.
 *  - `disabled` — 403 CAPABILITY_DISABLED: an admin turned the assistant off. A
 *                 config decision, NOT a missing key → no owner key-link.
 *  - `transient`— a per-turn outage (rendered inline on the turn, never replaces
 *                 the whole thread).
 */
type AiState = 'ok' | 'gated' | 'disabled' | 'transient';

/** A rendered turn in the thread. Assistant turns carry grounding + truncation metadata. */
interface ThreadMessage {
  role: 'user' | 'assistant';
  text: string;
  tools?: ToolTrace[];
  truncated?: boolean;
  /** Inline transient-error copy attached to a failed send (the user's typed turn is kept). */
  error?: string;
}

/** Friendly grounding-chip metadata for a backend tool name (assistant.go transparency surface). */
interface ToolMeta {
  label: string;
  icon: IconName;
}

// Client-supplied history caps — mirror internal/api/assistant.go:59-63 so the
// server's hard 400s never trip in normal use. We trim BEFORE every send.
// The server counts UTF-8 BYTES (Go len(string)); JS .length is UTF-16 code
// units and under-counts multibyte (CJK/emoji) text, so we count bytes too.
const chatHistoryMaxTurns = 10;
const chatHistoryMaxTotalChars = 24000;
const chatMessageMaxChars = 8000;

/** UTF-8 byte length — matches the server's Go len(string) budget unit. */
function byteLen(s: string): number {
  return new TextEncoder().encode(s).length;
}

/** Maps a backend tool name → friendly grounding label + icon (AGENTIC_UX §Chunk 1). */
function toolMeta(name: string): ToolMeta {
  if (name.startsWith('conn__')) return { label: 'Connector', icon: 'command' };
  switch (name) {
    case 'list_projects':
    case 'get_project':
      return { label: 'Projects', icon: 'folder' };
    case 'get_schedule_gantt':
    case 'list_project_tasks':
      return { label: 'Schedule', icon: 'calendar' };
    case 'list_procurement':
      return { label: 'Procurement', icon: 'package' };
    case 'list_feed_cards':
      return { label: 'Feed', icon: 'inbox' };
    case 'get_project_budgets':
    case 'get_org_financials':
      return { label: 'Financials', icon: 'dollar' };
    default:
      // Unknown future tool: show its raw name rather than hide the grounding.
      return { label: name, icon: 'sparkles' };
  }
}

/** Starter prompts shown on an empty thread. Some are role-gated. */
interface Starter {
  text: string;
  minRole?: 'admin';
}
const STARTERS: Starter[] = [
  { text: 'What needs my attention today?' },
  { text: 'Which projects have critical-path risk?' },
  { text: 'Show procurement items at risk' },
  { text: 'Summarize budget variance by project', minRole: 'admin' },
];

/**
 * `fb-assistant-page` — the conversational ERP assistant (VISION: "the harness IS
 * the product"). A real multi-turn chat over `POST /api/v1/agents/chat`. The
 * endpoint is STATELESS server-side: this page owns the running thread and resends
 * it (capped: ≤10 turns / ≤24k total chars / ≤8k per message) on every send.
 *
 * Replies render as model markdown (`fb-markdown`); each reply surfaces the ERP
 * tools the model consulted as grounding chips (`tools_used` — name + error flag
 * only). AI is key-gated (§9): a 503 swaps in the gated panel with an owner-only
 * Integrations link; a 403 CAPABILITY_DISABLED shows the admin-off copy with no
 * key-link. The Daily-Briefing / Schedule quick-links stay accessible but secondary.
 */
@customElement('fb-assistant-page')
export class FbAssistantPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .chat {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
      }
      .thread {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-md);
        min-height: 200px;
      }
      .turn {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        max-width: 80%;
      }
      .turn.user {
        align-self: flex-end;
        align-items: flex-end;
      }
      .turn.assistant {
        align-self: flex-start;
        align-items: flex-start;
      }
      .bubble {
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        border-radius: var(--fb-radius-lg);
        font-size: var(--fb-text-body-md);
      }
      .turn.user .bubble {
        background: var(--fb-surface-2);
        border: 1px solid var(--fb-border);
        color: var(--fb-text-primary);
        white-space: pre-wrap;
      }
      .turn.assistant .bubble {
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        color: var(--fb-text-primary);
        width: 100%;
      }
      .turn-error {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        color: var(--fb-safety-red-text);
        font-size: var(--fb-text-body-sm);
      }
      .sources {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: var(--fb-spacing-xs);
      }
      .sources-label {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
      }
      .source-failed {
        color: var(--fb-safety-red-text);
      }
      .truncated {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
        font-style: italic;
      }
      .thinking {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      .dots {
        display: inline-flex;
        gap: 3px;
      }
      .dots span {
        width: 5px;
        height: 5px;
        border-radius: var(--fb-radius-full);
        background: var(--fb-text-muted);
        animation: fb-typing 1.2s ease-in-out infinite;
      }
      .dots span:nth-child(2) {
        animation-delay: 0.18s;
      }
      .dots span:nth-child(3) {
        animation-delay: 0.36s;
      }
      @keyframes fb-typing {
        0%,
        60%,
        100% {
          opacity: 0.25;
        }
        30% {
          opacity: 1;
        }
      }
      @media (prefers-reduced-motion: reduce) {
        .dots span {
          animation: none;
        }
      }
      .starters {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-lg);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
      }
      .starters-label {
        margin: 0;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .starter-chips {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
      }
      .composer {
        display: flex;
        align-items: flex-end;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
      }
      textarea {
        flex: 1;
        resize: none;
        min-height: 24px;
        max-height: 160px;
        padding: var(--fb-spacing-xs) var(--fb-spacing-sm);
        background: transparent;
        border: 0;
        color: var(--fb-text-primary);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        line-height: 1.5;
      }
      textarea:focus-visible {
        outline: none;
      }
      .composer:focus-within {
        border-color: var(--fb-gable-green);
      }
      .composer-hint {
        margin: var(--fb-spacing-xs) 0 0;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-safety-red-text);
      }
      .secondary-links {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-md);
        margin-top: var(--fb-spacing-sm);
      }
      .secondary-links a {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        color: var(--fb-blueprint-blue);
        font-weight: 600;
        font-size: var(--fb-text-body-sm);
        text-decoration: none;
      }
      .secondary-links a:hover {
        text-decoration: underline;
      }
    `,
  ];

  @state() private messages: ThreadMessage[] = [];
  @state() private draft = '';
  @state() private sending = false;
  @state() private aiState: AiState = 'ok';

  @query('textarea') private textarea?: HTMLTextAreaElement;

  /** Snapshot at connect — FBElement is not signal-reactive, so read imperatively. */
  override connectedCallback(): void {
    super.connectedCallback();
    if (!aiConfigured.get()) this.aiState = 'gated';
  }

  /** True when the draft is over the per-message ceiling (block the send). */
  private get draftTooLong(): boolean {
    return byteLen(this.draft) > chatMessageMaxChars;
  }

  /** Whether send is currently allowed (non-empty, in-budget, not in flight, AI on). */
  private get canSend(): boolean {
    return (
      this.aiState === 'ok' && !this.sending && this.draft.trim().length > 0 && !this.draftTooLong
    );
  }

  private onInput(e: Event): void {
    this.draft = (e.target as HTMLTextAreaElement).value;
  }

  private onKeyDown(e: KeyboardEvent): void {
    // Enter sends; Shift+Enter inserts a newline (textarea default).
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void this.send();
    }
  }

  private onStarter(text: string): void {
    this.draft = text;
    void this.send();
  }

  /**
   * Builds the capped `history` from prior turns: strip metadata to bare
   * {role,text}, slice to the last 10 turns, then drop oldest turns until the
   * cumulative char budget (history + the new message) is under the ceiling.
   */
  private buildHistory(newMessage: string): ChatTurn[] {
    // Drop any turn that EXCEEDS the per-message byte cap (e.g. a long
    // assistant reply — the tool loop has no char cap on its output). Such a
    // turn stays visible in the thread but is never resent, so it can't trip
    // the server's per-turn 400 and permanently wedge the conversation.
    let history: ChatTurn[] = this.messages
      .filter((m) => byteLen(m.text) <= chatMessageMaxChars)
      .map((m) => ({ role: m.role, text: m.text }));
    if (history.length > chatHistoryMaxTurns) {
      history = history.slice(-chatHistoryMaxTurns);
    }
    // Byte-counted cumulative budget (history + the new message), oldest dropped first.
    let total = byteLen(newMessage) + history.reduce((sum, t) => sum + byteLen(t.text), 0);
    while (total > chatHistoryMaxTotalChars) {
      const oldest = history.shift();
      if (!oldest) break;
      total -= byteLen(oldest.text);
    }
    return history;
  }

  private async send(): Promise<void> {
    if (!this.canSend) return;
    const message = this.draft.trim();
    const history = this.buildHistory(message);

    // Optimistically show the user's turn and clear the composer.
    this.messages = [...this.messages, { role: 'user', text: message }];
    this.draft = '';
    this.sending = true;
    await this.updateComplete;
    this.scrollToEnd();

    try {
      const res = await sendChat(message, history);
      this.messages = [
        ...this.messages,
        {
          role: 'assistant',
          text: res.reply,
          tools: res.tools_used,
          truncated: res.truncated,
        },
      ];
    } catch (err) {
      this.handleSendError(err);
    } finally {
      this.sending = false;
      await this.updateComplete;
      this.scrollToEnd();
    }
  }

  /**
   * On a failed send, branch on the machine error code:
   *  - 503 SERVICE_UNAVAILABLE → AI off (reactive soft-fail + gated panel).
   *  - 403 CAPABILITY_DISABLED → admin kill-switch (config state, no key-link).
   *  - everything else → an inline transient error attached to the user turn so
   *    the typed message is preserved (the user can retype/resend).
   */
  private handleSendError(err: unknown): void {
    if (err instanceof ApiError) {
      if (err.code === ErrorCode.SERVICE_UNAVAILABLE || err.isAiUnconfigured) {
        markAiUnconfigured();
        this.aiState = 'gated';
        return;
      }
      if (err.code === ErrorCode.CAPABILITY_DISABLED) {
        this.aiState = 'disabled';
        return;
      }
      this.attachTurnError(userMessageForCode(err.code));
      return;
    }
    this.attachTurnError(userMessageForCode(ErrorCode.UNKNOWN));
  }

  /** Attaches an inline error to the most recent (user) turn without dropping it. */
  private attachTurnError(message: string): void {
    const last = this.messages[this.messages.length - 1];
    if (last?.role === 'user') {
      const updated = { ...last, error: message };
      this.messages = [...this.messages.slice(0, -1), updated];
    }
  }

  private scrollToEnd(): void {
    const thread = this.renderRoot.querySelector('.thread');
    thread?.scrollTo({ top: thread.scrollHeight });
    this.textarea?.focus();
  }

  // ----------------------------- render -----------------------------

  private renderGated(): TemplateResult {
    return html`<fb-state
      mode="gated"
      heading="AI assistant is off"
      message="Add an Anthropic API key to turn on the conversational assistant and its agents."
      ?can-configure=${hasRole('owner')}
      @configure=${() => navigate('/settings/integrations')}
    ></fb-state>`;
  }

  private renderDisabled(): TemplateResult {
    // CAPABILITY_DISABLED is a config decision, not a missing key — never offer a key-link.
    return html`<fb-state
      mode="gated"
      heading="AI assistant is turned off"
      message=${userMessageForCode(ErrorCode.CAPABILITY_DISABLED)}
    ></fb-state>`;
  }

  private renderStarters(): TemplateResult {
    const starters = STARTERS.filter((s) => !s.minRole || hasMinRole(s.minRole));
    return html`<div class="starters">
      <p class="starters-label">Try asking…</p>
      <div class="starter-chips" role="group" aria-label="Suggested prompts">
        ${starters.map(
          (s) =>
            html`<fb-chip
              selectable
              .value=${s.text}
              @click=${() => this.onStarter(s.text)}
              @keydown=${(e: KeyboardEvent) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  this.onStarter(s.text);
                }
              }}
              >${s.text}</fb-chip
            >`,
        )}
      </div>
    </div>`;
  }

  private renderSources(tools: ToolTrace[]): TemplateResult {
    if (tools.length === 0) return html``;
    // Dedupe by friendly label, but preserve an error flag if ANY call of that
    // tool group failed (so a failed lookup is never hidden behind a clean one).
    const byLabel = new Map<string, { meta: ToolMeta; isError: boolean }>();
    for (const t of tools) {
      const meta = toolMeta(t.name);
      const existing = byLabel.get(meta.label);
      if (existing) existing.isError = existing.isError || t.is_error;
      else byLabel.set(meta.label, { meta, isError: t.is_error });
    }
    return html`<div class="sources" role="group" aria-label="Sources used">
      <span class="sources-label">Sources:</span>
      ${[...byLabel.values()].map(
        ({ meta, isError }) =>
          html`<fb-chip class=${isError ? 'source-failed' : ''}>
            <fb-icon name=${meta.icon} size="12"></fb-icon>
            ${meta.label}${isError ? ' (failed)' : ''}
          </fb-chip>`,
      )}
    </div>`;
  }

  private renderTurn(m: ThreadMessage): TemplateResult {
    if (m.role === 'user') {
      return html`<div class="turn user">
        <div class="bubble">${m.text}</div>
        ${m.error
          ? html`<p class="turn-error">
              <fb-icon name="alert-circle" size="14"></fb-icon>${m.error}
            </p>`
          : nothing}
      </div>`;
    }
    return html`<div class="turn assistant">
      <div class="bubble"><fb-markdown .source=${m.text}></fb-markdown></div>
      ${m.tools && m.tools.length > 0 ? this.renderSources(m.tools) : nothing}
      ${m.truncated
        ? html`<p class="truncated">This answer may be incomplete (hit a reasoning limit).</p>`
        : nothing}
    </div>`;
  }

  private renderChat(): TemplateResult {
    return html`<div class="chat">
      <div class="thread" role="log" aria-live="polite" aria-label="Conversation">
        ${this.messages.map((m) => this.renderTurn(m))}
        ${this.sending
          ? html`<div class="turn assistant">
              <div class="thinking" role="status" aria-live="polite">
                <span class="dots" aria-hidden="true"><span></span><span></span><span></span></span>
                Assistant is thinking…
              </div>
            </div>`
          : nothing}
      </div>

      ${this.messages.length === 0 ? this.renderStarters() : nothing}

      <div>
        <div class="composer">
          <textarea
            aria-label="Ask the assistant"
            placeholder="Ask about your projects, schedule, procurement, or budget…"
            rows="1"
            .value=${this.draft}
            ?disabled=${this.sending}
            @input=${this.onInput}
            @keydown=${this.onKeyDown}
          ></textarea>
          <fb-button
            icon="arrow-up"
            label="Send"
            ?loading=${this.sending}
            ?disabled=${!this.canSend}
            @click=${() => void this.send()}
          ></fb-button>
        </div>
        ${this.draftTooLong
          ? html`<p class="composer-hint" role="alert">
              Message is too long (max ${chatMessageMaxChars.toLocaleString()} characters).
            </p>`
          : nothing}
      </div>
    </div>`;
  }

  private renderBody(): TemplateResult {
    if (this.aiState === 'gated') return this.renderGated();
    if (this.aiState === 'disabled') return this.renderDisabled();
    return this.renderChat();
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">AI Assistant</h1>
            <p class="page-sub">
              Ask anything about your live project data — schedule, procurement, budget, and feed.
            </p>
          </div>
        </div>
        ${this.renderBody()}
        <nav class="secondary-links" aria-label="Other AI surfaces">
          <a href="/command/briefing"> <fb-icon name="sun" size="14"></fb-icon>Daily briefing </a>
          <a href="/command/schedule">
            <fb-icon name="sliders" size="14"></fb-icon>Schedule adjustments
          </a>
        </nav>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-assistant-page': FbAssistantPage;
  }
}
