import { describe, it, expect, afterEach } from 'vitest';
import '../src/components/atoms/index.js';
import '../src/components/molecules/index.js';
import '../src/components/shell/index.js';
import type { FbNavRail } from '../src/components/shell/fb-nav-rail.js';
import type { FbTopBar } from '../src/components/shell/fb-top-bar.js';
import type { FbContextPanel } from '../src/components/shell/fb-context-panel.js';
import type { FbWizardStepper, WizardStep } from '../src/components/shell/fb-wizard-stepper.js';
import type { FbSyncStatus } from '../src/components/shell/fb-sync-status.js';
import type { FbOrgShell } from '../src/components/shell/fb-org-shell.js';

async function mount<T extends HTMLElement>(html: string): Promise<T> {
  const host = document.createElement('div');
  host.innerHTML = html;
  const el = host.firstElementChild as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

function navLabels(el: FbNavRail): string[] {
  return Array.from(el.shadowRoot!.querySelectorAll('fb-nav-item')).map(
    (n) => n.getAttribute('label') ?? '',
  );
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('fb-nav-rail', () => {
  it('shows owner-only and admin sections for an owner', async () => {
    const el = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    el.userRole = 'owner';
    await el.updateComplete;
    const labels = navLabels(el);
    expect(labels).toContain('Financials');
    expect(labels).toContain('HR & Certs');
    expect(labels).toContain('Integrations'); // owner-only (holds secrets)
    expect(labels).toContain('Activity');
  });

  it('hides owner/admin-only sections from a superintendent', async () => {
    const el = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    el.userRole = 'superintendent';
    await el.updateComplete;
    const labels = navLabels(el);
    expect(labels).toContain('Financials'); // minRole super
    expect(labels).toContain('Schedule'); // ungated
    expect(labels).not.toContain('HR & Certs'); // owner/admin only
    expect(labels).not.toContain('Integrations'); // owner only
    expect(labels).not.toContain('Activity'); // owner/admin only
  });

  it('shows the AI Assistant for superintendent+ regardless of plan tier (ESC-002)', async () => {
    // ESC-002 dropped the pro gate; the AI Assistant is role-gated only, so a
    // real token (plan_tier="" / "free") still reaches it at superintendent+.
    const free = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    free.userRole = 'superintendent';
    free.plan = 'free';
    await free.updateComplete;
    expect(navLabels(free)).toContain('AI Assistant');

    // Still hidden below the role floor (the role gate stays).
    const field = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    field.userRole = 'field_worker';
    field.plan = 'free';
    await field.updateComplete;
    expect(navLabels(field)).not.toContain('AI Assistant');
  });

  it('marks the active route and bubbles navigate', async () => {
    const el = await mount<FbNavRail>('<fb-nav-rail current="/portfolio/projects"></fb-nav-rail>');
    el.userRole = 'owner';
    await el.updateComplete;
    const active = el.shadowRoot!.querySelector('fb-nav-item[active]')!;
    expect(active.getAttribute('href')).toBe('/portfolio/projects');

    let href: string | null = null;
    el.addEventListener('navigate', (e) => (href = (e as CustomEvent).detail.href));
    const projects = el.shadowRoot!.querySelector('fb-nav-item[href="/portfolio/projects"]')!;
    (projects.shadowRoot!.querySelector('a') as HTMLElement).click();
    expect(href).toBe('/portfolio/projects');
  });
});

describe('fb-top-bar', () => {
  it('hides the Command Center workspace from a field worker', async () => {
    const el = await mount<FbTopBar>('<fb-top-bar user-role="field_worker"></fb-top-bar>');
    expect(el.shadowRoot!.querySelector('.switcher')).toBeNull();
  });

  it('offers the workspace switcher to a superintendent and emits switch', async () => {
    const el = await mount<FbTopBar>('<fb-top-bar user-role="superintendent"></fb-top-bar>');
    const switcher = el.shadowRoot!.querySelector('.switcher')!;
    expect(switcher).toBeTruthy();
    let ws: string | null = null;
    el.addEventListener('workspace-change', (e) => (ws = (e as CustomEvent).detail.workspace));
    const buttons = switcher.querySelectorAll('button');
    (buttons[1] as HTMLElement).click(); // Command Center
    expect(ws).toBe('command');
  });

  it('toggles density and emits the next value', async () => {
    const el = await mount<FbTopBar>(
      '<fb-top-bar user-role="owner" density="comfortable"></fb-top-bar>',
    );
    let density: string | null = null;
    el.addEventListener('density-change', (e) => (density = (e as CustomEvent).detail.density));
    const toggle = el.shadowRoot!.querySelector('.iconbtn[aria-pressed]')! as HTMLElement;
    toggle.click();
    expect(density).toBe('compact');
  });

  it('shows an unread notification badge', async () => {
    const el = await mount<FbTopBar>(
      '<fb-top-bar user-role="owner" notifications="5"></fb-top-bar>',
    );
    expect(el.shadowRoot!.querySelector('.badge')!.textContent).toContain('5');
  });
});

describe('fb-context-panel', () => {
  it('renders nothing while closed', async () => {
    const el = await mount<FbContextPanel>(
      '<fb-context-panel heading="Detail"></fb-context-panel>',
    );
    expect(el.shadowRoot!.querySelector('aside')).toBeNull();
  });

  it('renders a labelled complementary region and emits close on Esc', async () => {
    const el = await mount<FbContextPanel>('<fb-context-panel heading="Audit"></fb-context-panel>');
    el.open = true;
    await el.updateComplete;
    const aside = el.shadowRoot!.querySelector('[role="complementary"]')!;
    const labelId = aside.getAttribute('aria-labelledby')!;
    expect(el.shadowRoot!.getElementById(labelId)!.textContent).toContain('Audit');

    let closed = false;
    el.addEventListener('close', () => (closed = true));
    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(closed).toBe(true);
  });
});

describe('fb-wizard-stepper', () => {
  const steps: WizardStep[] = [
    { id: 'company', label: 'Company info', state: 'done' },
    { id: 'trades', label: 'Trades', state: 'current' },
    { id: 'codes', label: 'Cost codes', state: 'upcoming' },
    { id: 'finish', label: 'Finish', state: 'blocked' },
  ];

  it('reflects step states with the current step as the tab stop', async () => {
    const el = await mount<FbWizardStepper>('<fb-wizard-stepper></fb-wizard-stepper>');
    el.steps = steps;
    await el.updateComplete;
    const current = el.shadowRoot!.querySelector('[aria-current="step"]')!;
    expect(current.textContent).toContain('Trades');
    expect(current.getAttribute('tabindex')).toBe('0');
    const blocked = el.shadowRoot!.querySelectorAll('button.step')[3]! as HTMLButtonElement;
    expect(blocked.disabled).toBe(true);
  });

  it('emits step on revisiting a completed step but not a blocked one', async () => {
    const el = await mount<FbWizardStepper>('<fb-wizard-stepper></fb-wizard-stepper>');
    el.steps = steps;
    await el.updateComplete;
    let picked: string | null = null;
    el.addEventListener('step', (e) => (picked = (e as CustomEvent).detail.id));
    const buttons = el.shadowRoot!.querySelectorAll('button.step');
    (buttons[0] as HTMLElement).click(); // done → revisit
    expect(picked).toBe('company');
    picked = null;
    (buttons[3] as HTMLElement).click(); // blocked → no-op (disabled)
    expect(picked).toBeNull();
  });
});

describe('fb-sync-status', () => {
  it('renders nothing while idle (passive web variant)', async () => {
    const el = await mount<FbSyncStatus>('<fb-sync-status></fb-sync-status>');
    expect(el.shadowRoot!.querySelector('.chip')).toBeNull();
  });

  it('surfaces a retry affordance on a failed write', async () => {
    const el = await mount<FbSyncStatus>('<fb-sync-status state="error"></fb-sync-status>');
    const chip = el.shadowRoot!.querySelector('[role="status"]')!;
    expect(chip.textContent).toContain("Couldn't save changes");
    let retried = false;
    el.addEventListener('retry', () => (retried = true));
    (el.shadowRoot!.querySelector('.retry') as HTMLElement).click();
    expect(retried).toBe(true);
  });

  it('shows the queued count while offline', async () => {
    const el = await mount<FbSyncStatus>(
      '<fb-sync-status state="offline" queued="3"></fb-sync-status>',
    );
    expect(el.shadowRoot!.querySelector('.chip')!.textContent).toContain('3 queued');
  });
});

describe('fb-org-shell', () => {
  it('exposes the five landmarks and gates the context panel', async () => {
    const el = await mount<FbOrgShell>(
      '<fb-org-shell user-role="owner" current="/portfolio/projects"></fb-org-shell>',
    );
    // main + contentinfo are in the shell's own shadow root.
    expect(el.shadowRoot!.querySelector('[role="main"]')).toBeTruthy();
    expect(el.shadowRoot!.querySelector('[role="contentinfo"]')).toBeTruthy();
    // banner (top bar) and navigation (rail) render in the children's shadow roots.
    const top = el.shadowRoot!.querySelector('fb-top-bar')!;
    await (top as unknown as { updateComplete: Promise<unknown> }).updateComplete;
    expect(top.shadowRoot!.querySelector('[role="banner"]')).toBeTruthy();
    const rail = el.shadowRoot!.querySelector('fb-nav-rail')!;
    await (rail as unknown as { updateComplete: Promise<unknown> }).updateComplete;
    expect(rail.shadowRoot!.querySelector('nav[aria-label="Primary"]')).toBeTruthy();
    // Context panel is on-demand: absent until context-open.
    expect(el.shadowRoot!.querySelector('fb-context-panel')).toBeNull();
  });

  it('mounts the context panel when context-open is set', async () => {
    const el = await mount<FbOrgShell>(
      '<fb-org-shell user-role="owner" context-open context-heading="Detail"></fb-org-shell>',
    );
    expect(el.shadowRoot!.querySelector('fb-context-panel')).toBeTruthy();
  });
});
