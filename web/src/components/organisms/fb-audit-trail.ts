import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import './../atoms/fb-role-badge.js';
import type { AuditEntry } from '../../types/models.js';

const VERB_LABEL: Record<string, string> = {
  created: 'Created',
  added: 'Added',
  updated: 'Updated',
  changed: 'Changed',
  deleted: 'Deleted',
  removed: 'Removed',
  completed: 'Completed',
  redeemed: 'Redeemed',
  failed: 'Failed',
};

/**
 * Humanizes a dotted action key for display (DSC §7.16): the trailing segment is
 * the verb, the preceding segments are the resource. `setup.trade.created` →
 * "Created trade". Unknown verbs fall back to the de-dotted phrase.
 */
export function humanizeAction(action: string): string {
  const parts = action.split('.').filter(Boolean);
  if (parts.length === 0) return action;
  const verb = parts[parts.length - 1]!;
  // The resource is the segment immediately preceding the verb (e.g.
  // `setup.trade.created` → "trade"), not the whole namespace path.
  const resource = (parts[parts.length - 2] ?? '').replace(/_/g, ' ');
  const label = VERB_LABEL[verb];
  if (label && resource) return `${label} ${resource}`;
  return parts.join(' ').replace(/_/g, ' ');
}

/** Groups entries by calendar day (local), preserving reverse-chron order within each day. */
function groupByDay(
  entries: AuditEntry[],
): Array<{ day: string; label: string; items: AuditEntry[] }> {
  const order: string[] = [];
  const map = new Map<string, AuditEntry[]>();
  for (const e of entries) {
    const d = new Date(e.created_at);
    const key = Number.isNaN(d.getTime()) ? e.created_at : d.toISOString().slice(0, 10);
    if (!map.has(key)) {
      map.set(key, []);
      order.push(key);
    }
    map.get(key)!.push(e);
  }
  const today = new Date().toISOString().slice(0, 10);
  const yest = new Date(Date.now() - 86_400_000).toISOString().slice(0, 10);
  return order.map((key) => ({
    day: key,
    label: key === today ? 'Today' : key === yest ? 'Yesterday' : key,
    items: map.get(key)!,
  }));
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

/**
 * `fb-audit-trail` — reverse-chronological audit-log viewer (DSC §7.16). Reads
 * the audit log (`migration 008_audit_log`, `store/audit.go`). Entries are grouped
 * by day; each row shows a mono timestamp, the actor (resolved display name, else
 * "system") with their role badge, the humanized action, the resource, and an
 * expandable before/after diff. The backend already scrubs Restricted-class fields
 * from the JSONB payloads (`scrubAuditPayloads`), so this viewer renders what it's
 * given and never attempts to unmask. Owner/admin only (gated by the route).
 *
 * Filters (`action_prefix`, date range) emit `filter` ({ actionPrefix, from, to });
 * fetching the filtered slice is the parent's job.
 */
@customElement('fb-audit-trail')
export class FbAuditTrail extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .filters {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        align-items: flex-end;
        margin-bottom: var(--fb-spacing-md);
      }
      .filters label {
        display: flex;
        flex-direction: column;
        gap: 2px;
        font-size: var(--fb-text-label-sm);
        color: var(--fb-text-secondary);
      }
      .filters input {
        font: inherit;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
        padding: 6px var(--fb-spacing-sm);
      }
      .day {
        margin-top: var(--fb-spacing-md);
        font-size: var(--fb-text-label-lg);
        font-weight: 600;
        color: var(--fb-text-secondary);
        padding-bottom: var(--fb-spacing-xs);
        border-bottom: 1px solid var(--fb-border);
      }
      ol {
        list-style: none;
        margin: 0;
        padding: 0;
      }
      .entry {
        display: flex;
        gap: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) 0;
        border-bottom: 1px solid var(--fb-border);
      }
      .time {
        flex: none;
        width: 88px;
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
      }
      .main {
        flex: 1;
        min-width: 0;
      }
      .line {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: var(--fb-spacing-sm);
      }
      .action {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .actor {
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      .resource {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
      }
      details {
        margin-top: var(--fb-spacing-xs);
      }
      summary {
        cursor: pointer;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-blueprint-blue);
      }
      .diff {
        margin: var(--fb-spacing-xs) 0 0;
        display: grid;
        grid-template-columns: auto 1fr;
        gap: 2px var(--fb-spacing-sm);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
      }
      .diff dt {
        color: var(--fb-text-secondary);
      }
      .diff dd {
        margin: 0;
        color: var(--fb-text-primary);
        word-break: break-word;
      }
      .empty {
        padding: var(--fb-spacing-lg);
        text-align: center;
        color: var(--fb-text-secondary);
      }
    `,
  ];

  @property({ type: Array }) entries: AuditEntry[] = [];
  @property({ type: String, attribute: 'action-prefix' }) actionPrefix = '';
  @property({ type: String }) from = '';
  @property({ type: String }) to = '';

  private emitFilter(patch: Partial<{ actionPrefix: string; from: string; to: string }>): void {
    this.emit('filter', {
      actionPrefix: patch.actionPrefix ?? this.actionPrefix,
      from: patch.from ?? this.from,
      to: patch.to ?? this.to,
    });
  }

  private renderDiff(entry: AuditEntry): TemplateResult | typeof nothing {
    const before = entry.before ?? {};
    const after = entry.after ?? {};
    const keys = [...new Set([...Object.keys(before), ...Object.keys(after)])];
    if (keys.length === 0) return nothing;
    return html`<details>
      <summary>Details</summary>
      <dl class="diff">
        ${keys.map(
          (k) =>
            html`<dt>${k}</dt>
              <dd>${fmt(before[k])} → ${fmt(after[k])}</dd>`,
        )}
      </dl>
    </details>`;
  }

  override render(): TemplateResult {
    const groups = groupByDay(this.entries);
    // Single root wrapper: a static filters block followed by a trailing dynamic
    // sibling at the template root is dropped by happy-dom on commit (same quirk
    // as fb-field) — wrapping keeps the grouped list in the rendered tree.
    return html`<div class="root">
      <div class="filters">
        <label>
          Action prefix
          <input
            type="text"
            placeholder="e.g. setup."
            .value=${this.actionPrefix}
            @change=${(e: Event) =>
              this.emitFilter({ actionPrefix: (e.target as HTMLInputElement).value })}
          />
        </label>
        <label>
          From
          <input
            type="date"
            .value=${this.from}
            @change=${(e: Event) => this.emitFilter({ from: (e.target as HTMLInputElement).value })}
          />
        </label>
        <label>
          To
          <input
            type="date"
            .value=${this.to}
            @change=${(e: Event) => this.emitFilter({ to: (e.target as HTMLInputElement).value })}
          />
        </label>
      </div>

      ${groups.length === 0
        ? html`<p class="empty">No activity in this range.</p>`
        : groups.map(
            (g) => html`
              <h3 class="day">${g.label}</h3>
              <ol>
                ${g.items.map(
                  (entry) =>
                    html`<li class="entry">
                      <span class="time">${formatTime(entry.created_at)}</span>
                      <div class="main">
                        <div class="line">
                          <span class="action">${humanizeAction(entry.action)}</span>
                          ${entry.actor_role
                            ? html`<fb-role-badge .role=${entry.actor_role}></fb-role-badge>`
                            : nothing}
                          <span class="actor">${entry.actor_name || 'system'}</span>
                          ${entry.resource_type
                            ? html`<span class="resource"
                                >${entry.resource_type}${entry.resource_id
                                  ? html` ${entry.resource_id}`
                                  : nothing}</span
                              >`
                            : nothing}
                        </div>
                        ${this.renderDiff(entry)}
                      </div>
                    </li>`,
                )}
              </ol>
            `,
          )}
    </div>`;
  }
}

/** Renders an arbitrary JSON value compactly for the diff grid. */
function fmt(v: unknown): string {
  if (v === undefined) return '∅';
  if (v === null) return 'null';
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-audit-trail': FbAuditTrail;
  }
}
