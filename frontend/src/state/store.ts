/**
 * Simple signal-based reactive store for FutureBuild OS.
 * No external dependencies — just vanilla reactive primitives.
 */

type Listener = () => void;

export interface Signal<T> {
  get: () => T;
  set: (value: T) => void;
  subscribe: (listener: Listener) => () => void;
}

/**
 * Create a reactive signal with get/set/subscribe.
 * Components can subscribe to be notified when the value changes.
 */
export function createSignal<T>(initial: T): Signal<T> {
  let value = initial;
  const listeners = new Set<Listener>();

  return {
    get: () => value,
    set: (v: T) => {
      if (v !== value) {
        value = v;
        listeners.forEach((l) => l());
      }
    },
    subscribe: (l: Listener) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
  };
}

// ─── App-Level Signals ─────────────────────────────────────────────────────

/** The current organization ID (from JWT claims). */
export const currentOrg = createSignal<string>('');

/** The currently selected project ID, or null if none selected. */
export const currentProject = createSignal<string | null>(null);

/** Currency filter for financial views. */
export const currentCurrency = createSignal<'USD' | 'CAD' | 'ALL'>('ALL');

/** Global loading indicator. */
export const isLoading = createSignal<boolean>(false);
