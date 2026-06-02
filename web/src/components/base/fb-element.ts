import { LitElement, css, type CSSResultGroup } from 'lit';

/**
 * FBElement — shared Lit base class for all `fb-*` components.
 *
 * Per DESIGN_SYSTEM.md §8: provides the GableLBM shared styles (glass, glow,
 * hover-lift, active indicator, button bases, skeleton shimmer, data typography)
 * and the `emit` helper for composed custom events. Tokens are consumed from the
 * document-level `:root` in styles/variables.css (custom properties pierce the
 * shadow boundary), so components reference `var(--fb-*)` directly.
 *
 * Shared cross-cutting helpers added here (not in the spec snippet but required
 * by DSC §9 a11y): reduced-motion + visually-hidden utilities scoped to shadow DOM.
 */
export abstract class FBElement extends LitElement {
  static override styles: CSSResultGroup = css`
    :host {
      box-sizing: border-box;
    }
    :host *,
    :host *::before,
    :host *::after {
      box-sizing: inherit;
    }

    /* === Glassmorphism (§7) === */
    .glass-card {
      background: var(--fb-glass-bg);
      backdrop-filter: blur(24px);
      -webkit-backdrop-filter: blur(24px);
      border: 1px solid var(--fb-glass-border);
      border-radius: var(--fb-radius-lg);
      box-shadow: var(--md-sys-elevation-1);
    }
    .glass-panel {
      background: var(--fb-glass-panel);
      backdrop-filter: blur(48px);
      -webkit-backdrop-filter: blur(48px);
      border: 1px solid var(--fb-glass-border);
    }

    /* === Interaction === */
    .hover-lift {
      transition:
        transform var(--fb-motion-emphasized) var(--fb-ease-out),
        box-shadow var(--fb-motion-emphasized) var(--fb-ease-out);
    }
    .hover-lift:hover {
      transform: translateY(-4px);
      box-shadow: var(--md-sys-elevation-2);
    }

    /* === Glow === */
    .glow-accent {
      box-shadow: var(--fb-glow);
    }
    .glow-accent-strong {
      box-shadow: var(--fb-glow-strong);
    }

    /* === Active indicator (§8.1) === */
    .active-indicator {
      position: relative;
    }
    .active-indicator::before {
      content: '';
      position: absolute;
      left: 0;
      top: 50%;
      transform: translateY(-50%);
      width: 3px;
      height: 60%;
      background: var(--fb-gable-green);
      border-radius: 0 3px 3px 0;
    }

    /* === Buttons === */
    .btn-primary {
      transition:
        transform var(--fb-motion-fast) var(--fb-ease-out),
        box-shadow var(--fb-motion-fast) var(--fb-ease-out);
    }
    .btn-primary:hover {
      box-shadow: var(--fb-glow);
      transform: translateY(-1px);
    }
    .btn-primary:active {
      transform: scale(0.95);
      box-shadow: none;
    }
    .btn-destructive {
      border: 1px solid rgba(244, 63, 94, 0.3);
      transition: box-shadow var(--fb-motion-fast) var(--fb-ease-out);
    }
    .btn-destructive:hover {
      box-shadow: 0 0 20px rgba(244, 63, 94, 0.15);
    }

    /* === Skeleton loading (§8.1) === */
    .skeleton {
      background: linear-gradient(
        90deg,
        var(--md-sys-color-surface-container-high) 25%,
        var(--md-sys-color-surface-container) 50%,
        var(--md-sys-color-surface-container-high) 75%
      );
      background-size: 200% 100%;
      animation: shimmer 2s linear infinite;
      border-radius: 4px;
    }
    .skeleton-text {
      height: 1em;
      width: 100%;
      margin-bottom: 0.5em;
      display: block;
    }
    .skeleton-box {
      height: 100px;
      width: 100%;
      display: block;
    }
    .skeleton-bar {
      height: 8px;
      width: 100%;
      display: block;
    }
    @keyframes shimmer {
      0% {
        background-position: 200% 0;
      }
      100% {
        background-position: -200% 0;
      }
    }

    /* === Data typography (§8.1) === */
    .data-mono {
      font-family: var(--fb-font-mono);
      font-variant-numeric: tabular-nums;
    }
    .data-currency {
      font-family: var(--fb-font-mono);
      font-variant-numeric: tabular-nums;
    }
    .data-positive {
      color: var(--fb-variance-positive);
    }
    .data-negative {
      color: var(--fb-variance-negative);
    }

    /* === Focus ring (DSC §9) === */
    :host(:focus-visible),
    *:focus-visible {
      outline: 2px solid var(--fb-gable-green);
      outline-offset: 2px;
    }

    /* === SR-only === */
    .visually-hidden {
      position: absolute;
      width: 1px;
      height: 1px;
      padding: 0;
      margin: -1px;
      overflow: hidden;
      clip: rect(0, 0, 0, 0);
      white-space: nowrap;
      border: 0;
    }

    /* === Reduced motion (DSC §9 / DESIGN_SYSTEM §10.2) === */
    @media (prefers-reduced-motion: reduce) {
      .hover-lift,
      .btn-primary,
      .btn-destructive {
        transition: none;
      }
      .hover-lift:hover {
        transform: none;
      }
      .skeleton {
        animation: none;
      }
    }
  `;

  /**
   * Dispatches a composed, bubbling CustomEvent so consumers outside the shadow
   * boundary can listen. Returns the event for `defaultPrevented` inspection.
   */
  protected emit<T = unknown>(name: string, detail?: T): CustomEvent<T> {
    const event = new CustomEvent<T>(name, {
      bubbles: true,
      composed: true,
      detail: detail as T,
    });
    this.dispatchEvent(event);
    return event;
  }
}
