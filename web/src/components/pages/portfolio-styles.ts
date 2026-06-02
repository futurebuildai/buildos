import { css } from 'lit';

/**
 * Shared chrome for the Phase D portfolio pages (DSC §1.4 content area). A
 * page-header (title + optional subtitle + actions slot), a responsive card
 * grid, and a per-currency section divider. Pages compose these on top of
 * `FBElement.styles`; page-specific rules stay in the page file.
 */
export const portfolioStyles = css`
  :host {
    display: block;
  }
  .page {
    display: flex;
    flex-direction: column;
    gap: var(--fb-spacing-lg);
  }
  .page-head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--fb-spacing-md);
    flex-wrap: wrap;
  }
  .page-title {
    margin: 0 0 var(--fb-spacing-xs);
    font-size: var(--fb-text-headline-sm);
    font-weight: 600;
    color: var(--fb-text-primary);
  }
  .page-sub {
    margin: 0;
    color: var(--fb-text-secondary);
    font-size: var(--fb-text-body-sm);
  }
  .card-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: var(--fb-spacing-md);
  }
  .stat-row {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: var(--fb-spacing-md);
  }
  .currency-group {
    display: flex;
    flex-direction: column;
    gap: var(--fb-spacing-sm);
  }
  .currency-label {
    display: inline-flex;
    align-items: center;
    gap: var(--fb-spacing-xs);
    font-size: var(--fb-text-label-lg);
    font-weight: 600;
    color: var(--fb-text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .banner {
    display: flex;
    align-items: center;
    gap: var(--fb-spacing-sm);
    padding: var(--fb-spacing-sm) var(--fb-spacing-md);
    font-size: var(--fb-text-body-sm);
    border-radius: var(--fb-radius-sm);
    border: 1px solid var(--fb-glass-border);
    background: var(--fb-surface-2);
    color: var(--fb-text-secondary);
  }
  .banner.warn {
    border-color: var(--fb-amber, #f59e0b);
    color: var(--fb-text-primary);
  }
  .banner.warn fb-icon {
    color: var(--fb-amber, #f59e0b);
  }
  .toast {
    display: flex;
    align-items: center;
    gap: var(--fb-spacing-xs);
    padding: var(--fb-spacing-sm) var(--fb-spacing-md);
    font-size: var(--fb-text-body-sm);
    border-radius: var(--fb-radius-sm);
  }
  .toast.ok {
    color: var(--fb-gable-green);
    background: color-mix(in srgb, var(--fb-gable-green) 12%, transparent);
    border: 1px solid var(--fb-gable-green);
  }
  .toast.err {
    color: var(--fb-safety-red);
    background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
    border: 1px solid var(--fb-safety-red);
  }
`;
