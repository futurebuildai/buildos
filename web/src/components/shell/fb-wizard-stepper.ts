import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, query } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';

export type StepState = 'done' | 'current' | 'upcoming' | 'blocked';

export interface WizardStep {
  id: string;
  label: string;
  state: StepState;
}

/**
 * `fb-wizard-stepper` — setup-wizard progress (DSC §4.4, §1.4). Steps reflect the
 * `setup` service step order; each carries a `state` (done | current | upcoming |
 * blocked). `blocked` = an unmet prereq and is not focusable/activatable.
 *
 * Keyboard (roving tabindex over an ordered list): ArrowLeft/Right (and Up/Down)
 * move focus between non-blocked steps; Home/End jump to the ends; Enter/Space on
 * a `done` step emits `step` ({ id }) so the parent can navigate back to revisit
 * a completed step. The current step is always the initial tab stop.
 */
@customElement('fb-wizard-stepper')
export class FbWizardStepper extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      ol {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        gap: var(--fb-spacing-xs);
      }
      li {
        flex: 1;
        min-width: 0;
      }
      .step {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        width: 100%;
        font: inherit;
        text-align: left;
        padding: var(--fb-spacing-sm);
        background: none;
        border: none;
        border-top: 2px solid var(--fb-border);
        color: var(--fb-text-muted);
        cursor: default;
      }
      .step.done {
        color: var(--fb-text-secondary);
        border-top-color: var(--fb-gable-green);
      }
      .step.done {
        cursor: pointer;
      }
      .step.current {
        color: var(--fb-text-primary);
        border-top-color: var(--fb-gable-green);
      }
      .step.blocked {
        color: var(--fb-text-muted);
      }
      .marker {
        flex: none;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 24px;
        height: 24px;
        border-radius: var(--fb-radius-full);
        font-size: var(--fb-text-label-sm);
        font-weight: 700;
        border: 1px solid currentColor;
      }
      .step.done .marker {
        color: var(--fb-deep-space);
        background: var(--fb-gable-green);
        border-color: var(--fb-gable-green);
      }
      .step.current .marker {
        color: var(--fb-deep-space);
        background: var(--fb-gable-green);
        border-color: var(--fb-gable-green);
      }
      .meta {
        display: flex;
        flex-direction: column;
        min-width: 0;
      }
      .num {
        font-size: var(--fb-text-label-sm);
        letter-spacing: 0.04em;
        text-transform: uppercase;
      }
      .label {
        font-size: var(--fb-text-body-md);
        font-weight: 600;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }
      /* Vertical drawer at the Tablet breakpoint (DSC §1.4). */
      @media (max-width: 1024px) {
        ol {
          flex-direction: column;
        }
        .step {
          border-top: none;
          border-left: 2px solid var(--fb-border);
        }
        .step.done,
        .step.current {
          border-left-color: var(--fb-gable-green);
        }
      }
    `,
  ];

  @property({ type: Array }) steps: WizardStep[] = [];

  @query('ol') private list?: HTMLOListElement;

  private isInteractive(s: WizardStep): boolean {
    return s.state === 'done' || s.state === 'current';
  }

  private onActivate(s: WizardStep): void {
    if (s.state === 'done') this.emit('step', { id: s.id });
  }

  private focusableIndices(): number[] {
    return this.steps.map((s, i) => (s.state === 'blocked' ? -1 : i)).filter((i) => i >= 0);
  }

  private moveFocus(from: number, delta: number): void {
    const order = this.focusableIndices();
    if (order.length === 0) return;
    const pos = order.indexOf(from);
    const nextPos = Math.min(Math.max(pos + delta, 0), order.length - 1);
    this.focusStep(order[nextPos]!);
  }

  private focusStep(index: number): void {
    const buttons = this.list?.querySelectorAll<HTMLButtonElement>('button.step');
    buttons?.[index]?.focus();
  }

  private onKeydown(e: KeyboardEvent, index: number): void {
    const order = this.focusableIndices();
    if (order.length === 0) return;
    switch (e.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        this.moveFocus(index, 1);
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        this.moveFocus(index, -1);
        break;
      case 'Home':
        e.preventDefault();
        this.focusStep(order[0]!);
        break;
      case 'End':
        e.preventDefault();
        this.focusStep(order[order.length - 1]!);
        break;
    }
  }

  override render(): TemplateResult {
    const total = this.steps.length;
    return html`<ol aria-label="Setup progress">
      ${this.steps.map((s, i) => {
        const interactive = this.isInteractive(s);
        const tabindex = s.state === 'current' ? 0 : interactive ? -1 : undefined;
        return html`<li>
          <button
            class=${`step ${s.state}`}
            type="button"
            ?disabled=${!interactive}
            tabindex=${tabindex ?? nothing}
            aria-current=${s.state === 'current' ? 'step' : nothing}
            @click=${() => this.onActivate(s)}
            @keydown=${(e: KeyboardEvent) => this.onKeydown(e, i)}
          >
            <span class="marker" aria-hidden="true">
              ${s.state === 'done'
                ? html`<fb-icon name="check" size="14"></fb-icon>`
                : s.state === 'blocked'
                  ? html`<fb-icon name="lock" size="12"></fb-icon>`
                  : i + 1}
            </span>
            <span class="meta">
              <span class="num">Step ${i + 1} of ${total}</span>
              <span class="label">${s.label}</span>
            </span>
          </button>
        </li>`;
      })}
    </ol>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-wizard-stepper': FbWizardStepper;
  }
}
