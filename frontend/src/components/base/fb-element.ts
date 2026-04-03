import { LitElement } from 'lit';
import { tokens, glassCard, dataMono } from '../../styles/tokens.js';

/**
 * FBBaseElement — base class for all FutureBuild OS components.
 * Provides design tokens, glassmorphism utilities, and shared patterns.
 * All components in the OS frontend extend this class.
 */
export class FBBaseElement extends LitElement {
  static styles = [tokens, glassCard, dataMono];

  /** Emit a custom event that bubbles through the DOM. */
  protected emitEvent(name: string, detail?: unknown): void {
    this.dispatchEvent(
      new CustomEvent(name, {
        detail,
        bubbles: true,
        composed: true,
      }),
    );
  }

  /** Show a toast notification. */
  protected showToast(message: string, variant: 'success' | 'error' | 'warning' | 'info' = 'info'): void {
    this.dispatchEvent(
      new CustomEvent('fb-toast', {
        detail: { message, variant },
        bubbles: true,
        composed: true,
      }),
    );
  }
}
