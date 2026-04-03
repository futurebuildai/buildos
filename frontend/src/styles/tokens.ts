import { css } from 'lit';

/**
 * GableLBM Industrial Dark design tokens.
 * All color, typography, spacing, and effect tokens for FutureBuild OS UI.
 */
export const tokens = css`
  :host {
    /* ── Colors ── */
    --fb-gable-green: #00FFA3;
    --fb-deep-space: #0A0B10;
    --fb-slate-steel: #161821;
    --fb-surface: #1E2030;
    --fb-surface-hover: #252738;
    --fb-border: rgba(255, 255, 255, 0.05);
    --fb-border-hover: rgba(255, 255, 255, 0.1);
    --fb-text-primary: #E2E8F0;
    --fb-text-secondary: #94A3B8;
    --fb-text-muted: #64748B;

    /* ── Accent Colors ── */
    --fb-blueprint-blue: #38BDF8;
    --fb-safety-red: #F43F5E;
    --fb-amber: #F59E0B;

    /* ── Typography ── */
    --fb-font-body: 'Outfit', sans-serif;
    --fb-font-mono: 'JetBrains Mono', monospace;
    --fb-text-xs: 0.75rem;
    --fb-text-sm: 0.875rem;
    --fb-text-base: 1rem;
    --fb-text-lg: 1.125rem;
    --fb-text-xl: 1.25rem;
    --fb-text-2xl: 1.5rem;
    --fb-text-3xl: 1.875rem;
    --fb-text-4xl: 2.25rem;

    /* ── Spacing ── */
    --fb-space-1: 0.25rem;
    --fb-space-2: 0.5rem;
    --fb-space-3: 0.75rem;
    --fb-space-4: 1rem;
    --fb-space-5: 1.25rem;
    --fb-space-6: 1.5rem;
    --fb-space-8: 2rem;
    --fb-space-10: 2.5rem;
    --fb-space-12: 3rem;

    /* ── Radii ── */
    --fb-radius-sm: 0.375rem;
    --fb-radius-md: 0.5rem;
    --fb-radius-lg: 0.75rem;
    --fb-radius-xl: 1rem;

    /* ── Shadows ── */
    --fb-shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.3);
    --fb-shadow-md: 0 4px 6px rgba(0, 0, 0, 0.4);
    --fb-shadow-lg: 0 10px 15px rgba(0, 0, 0, 0.5);
    --fb-shadow-glow: 0 0 20px rgba(0, 255, 163, 0.15);

    /* ── Glassmorphism ── */
    --fb-glass-bg: rgba(22, 24, 33, 0.6);
    --fb-glass-blur: blur(24px);
    --fb-glass-border: 1px solid rgba(255, 255, 255, 0.05);

    /* ── Transitions ── */
    --fb-transition-fast: 150ms ease;
    --fb-transition-normal: 250ms ease;
    --fb-transition-slow: 350ms ease;
  }
`;

/** Glassmorphism card mixin */
export const glassCard = css`
  .glass-card {
    background: var(--fb-glass-bg);
    backdrop-filter: var(--fb-glass-blur);
    -webkit-backdrop-filter: var(--fb-glass-blur);
    border: var(--fb-glass-border);
    border-radius: var(--fb-radius-lg);
    padding: var(--fb-space-6);
    transition: border-color var(--fb-transition-fast);
  }
  .glass-card:hover {
    border-color: var(--fb-border-hover);
  }
`;

/** JetBrains Mono for data fields */
export const dataMono = css`
  .data-mono, [data-mono] {
    font-family: var(--fb-font-mono);
    font-variant-numeric: tabular-nums;
  }
`;
