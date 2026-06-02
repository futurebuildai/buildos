import { css } from 'lit';

/**
 * Shared layout for the unauthenticated auth/onboarding screens (L1, B1, P1, P2,
 * use-the-app). Each page renders a single centered glass card; `fb-app`'s auth
 * shell supplies the centering canvas (max-width 420px). Keeping the card chrome
 * here avoids re-deriving the same heading/field/error rhythm per page.
 */
export const authCardStyles = css`
  :host {
    display: block;
  }
  .auth-card {
    width: 100%;
    padding: var(--fb-spacing-xl);
  }
  .auth-head {
    text-align: center;
    margin-bottom: var(--fb-spacing-lg);
  }
  .auth-head fb-icon {
    color: var(--fb-gable-green);
  }
  .auth-title {
    margin: var(--fb-spacing-sm) 0 var(--fb-spacing-xs);
    font-size: var(--fb-text-headline-sm);
    font-weight: 600;
    color: var(--fb-text-primary);
  }
  .auth-sub {
    margin: 0;
    font-size: var(--fb-text-body-sm);
    color: var(--fb-text-secondary);
  }
  .auth-fields {
    display: flex;
    flex-direction: column;
    gap: var(--fb-spacing-md);
    margin-bottom: var(--fb-spacing-lg);
  }
  .auth-error {
    display: flex;
    align-items: center;
    gap: var(--fb-spacing-xs);
    margin: 0 0 var(--fb-spacing-md);
    padding: var(--fb-spacing-sm) var(--fb-spacing-md);
    font-size: var(--fb-text-body-sm);
    color: var(--fb-safety-red);
    background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
    border: 1px solid var(--fb-safety-red);
    border-radius: var(--fb-radius-sm);
  }
  .auth-notice {
    display: flex;
    align-items: center;
    gap: var(--fb-spacing-xs);
    margin: 0 0 var(--fb-spacing-md);
    padding: var(--fb-spacing-sm) var(--fb-spacing-md);
    font-size: var(--fb-text-body-sm);
    color: var(--fb-text-secondary);
    background: var(--fb-surface-2);
    border: 1px solid var(--fb-glass-border);
    border-radius: var(--fb-radius-sm);
  }
  .auth-links {
    display: flex;
    justify-content: center;
    gap: var(--fb-spacing-md);
    margin-top: var(--fb-spacing-lg);
    font-size: var(--fb-text-body-sm);
  }
  .auth-links a {
    color: var(--fb-gable-green);
    text-decoration: none;
  }
  .auth-links a:hover {
    text-decoration: underline;
  }
`;
