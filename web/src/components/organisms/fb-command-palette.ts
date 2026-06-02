import { html, css, nothing, type TemplateResult, type PropertyValues } from 'lit';
import { customElement, property, state, query } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import type { IconName } from '../atoms/icons.js';

export interface Command {
  id: string;
  label: string;
  /** Secondary text (e.g. route, shortcut, section). */
  hint?: string;
  icon?: IconName;
}

/**
 * `fb-command-palette` — ⌘K power-navigation overlay (DSC §9 keyboard).
 *
 * A combobox over a filtered command list: type to filter (case-insensitive),
 * ArrowUp/Down move the active option, Enter selects it, Esc closes. Uses the
 * ARIA combobox/listbox pattern with `aria-activedescendant` so screen readers
 * track the highlighted command without moving DOM focus off the input. The
 * parent owns `open` and the `commands` source; the palette emits `select`
 * ({ id }) and `close`.
 */
@customElement('fb-command-palette')
export class FbCommandPalette extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: contents;
      }
      .backdrop {
        position: fixed;
        inset: 0;
        z-index: var(--fb-z-command-palette);
        background: rgba(0, 0, 0, 0.5);
        display: flex;
        align-items: flex-start;
        justify-content: center;
        padding-top: 12vh;
      }
      .panel {
        width: 100%;
        max-width: 560px;
        max-height: 60vh;
        display: flex;
        flex-direction: column;
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
        box-shadow: var(--md-sys-elevation-3);
        overflow: hidden;
      }
      .search {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        border-bottom: 1px solid var(--fb-border);
        color: var(--fb-text-secondary);
      }
      input {
        flex: 1;
        font: inherit;
        font-size: var(--fb-text-body-lg);
        color: var(--fb-text-primary);
        background: transparent;
        border: none;
        outline: none;
      }
      ul {
        list-style: none;
        margin: 0;
        padding: var(--fb-spacing-xs);
        overflow: auto;
      }
      li {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        border-radius: var(--fb-radius-sm);
        color: var(--fb-text-primary);
        cursor: pointer;
      }
      li[aria-selected='true'] {
        background: color-mix(in srgb, var(--fb-gable-green) 12%, transparent);
      }
      .hint {
        margin-left: auto;
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
      }
      .none {
        padding: var(--fb-spacing-lg);
        text-align: center;
        color: var(--fb-text-secondary);
      }
    `,
  ];

  @property({ type: Boolean, reflect: true }) open = false;
  @property({ type: Array }) commands: Command[] = [];
  @property({ type: String }) placeholder = 'Type a command or search…';

  @state() private query = '';
  @state() private active = 0;

  @query('input') private input?: HTMLInputElement;

  private readonly listId = `fb-cmd-list-${Math.random().toString(36).slice(2, 9)}`;

  private get filtered(): Command[] {
    const q = this.query.trim().toLowerCase();
    if (!q) return this.commands;
    return this.commands.filter(
      (c) => c.label.toLowerCase().includes(q) || c.hint?.toLowerCase().includes(q),
    );
  }

  override updated(changed: PropertyValues<this>): void {
    if (changed.has('open') && this.open) {
      this.query = '';
      this.active = 0;
      void this.updateComplete.then(() => this.input?.focus());
    }
  }

  private onInput(e: Event): void {
    this.query = (e.target as HTMLInputElement).value;
    this.active = 0;
  }

  private onKeydown(e: KeyboardEvent): void {
    const list = this.filtered;
    if (e.key === 'Escape') {
      e.preventDefault();
      this.emit('close');
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.active = list.length === 0 ? 0 : (this.active + 1) % list.length;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this.active = list.length === 0 ? 0 : (this.active - 1 + list.length) % list.length;
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const cmd = list[this.active];
      if (cmd) this.select(cmd);
    }
  }

  private select(cmd: Command): void {
    this.emit('select', { id: cmd.id });
  }

  private onBackdrop(e: MouseEvent): void {
    if (e.target === e.currentTarget) this.emit('close');
  }

  private optionId(i: number): string {
    return `${this.listId}-opt-${i}`;
  }

  override render(): TemplateResult {
    if (!this.open) return html`${nothing}`;
    const list = this.filtered;
    return html`
      <div class="backdrop" @click=${this.onBackdrop}>
        <div class="panel" role="dialog" aria-modal="true" aria-label="Command palette">
          <div class="search">
            <fb-icon name="search" size="18"></fb-icon>
            <input
              type="text"
              role="combobox"
              aria-expanded="true"
              aria-controls=${this.listId}
              aria-activedescendant=${list.length ? this.optionId(this.active) : nothing}
              placeholder=${this.placeholder}
              .value=${this.query}
              @input=${this.onInput}
              @keydown=${this.onKeydown}
            />
          </div>
          ${list.length === 0
            ? html`<p class="none">No commands match “${this.query}”.</p>`
            : html`<ul id=${this.listId} role="listbox" aria-label="Commands">
                ${list.map(
                  (cmd, i) =>
                    html`<li
                      id=${this.optionId(i)}
                      role="option"
                      aria-selected=${i === this.active ? 'true' : 'false'}
                      @click=${() => this.select(cmd)}
                      @mousemove=${() => (this.active = i)}
                    >
                      ${cmd.icon ? html`<fb-icon name=${cmd.icon} size="16"></fb-icon>` : nothing}
                      <span>${cmd.label}</span>
                      ${cmd.hint ? html`<span class="hint">${cmd.hint}</span>` : nothing}
                    </li>`,
                )}
              </ul>`}
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-command-palette': FbCommandPalette;
  }
}
