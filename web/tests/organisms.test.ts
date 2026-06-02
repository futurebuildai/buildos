import { describe, it, expect, afterEach } from 'vitest';
import '../src/components/atoms/index.js';
import '../src/components/organisms/index.js';
import { humanizeAction } from '../src/components/organisms/fb-audit-trail.js';
import type { FbState } from '../src/components/organisms/fb-state.js';
import type { FbModal } from '../src/components/organisms/fb-modal.js';
import type { FbConfirm } from '../src/components/organisms/fb-confirm.js';
import type { FbDataTable, Column, Row } from '../src/components/organisms/fb-data-table.js';
import type { FbCommandPalette, Command } from '../src/components/organisms/fb-command-palette.js';
import type { FbAuditTrail } from '../src/components/organisms/fb-audit-trail.js';
import type { AuditEntry } from '../src/types/models.js';

async function mount<T extends HTMLElement>(html: string): Promise<T> {
  const host = document.createElement('div');
  host.innerHTML = html;
  const el = host.firstElementChild as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('fb-state', () => {
  it('announces a busy live region while loading', async () => {
    const el = await mount<FbState>('<fb-state mode="loading" skeleton="table"></fb-state>');
    const region = el.shadowRoot!.querySelector('[role="status"]')!;
    expect(region.getAttribute('aria-busy')).toBe('true');
    expect(el.shadowRoot!.querySelectorAll('.skeleton').length).toBeGreaterThan(0);
  });

  it('maps an error code to friendly copy and offers retry', async () => {
    const el = await mount<FbState>(
      '<fb-state mode="error" error-code="SERVICE_UNAVAILABLE" request-id="req_123" retryable></fb-state>',
    );
    const alert = el.shadowRoot!.querySelector('[role="alert"]')!;
    expect(alert.textContent).toContain('Something went wrong');
    expect(alert.textContent).toContain('req_123');
    expect(el.shadowRoot!.querySelector('fb-button')).toBeTruthy();
  });

  it('gated mode shows the configure link only to owners', async () => {
    const owner = await mount<FbState>('<fb-state mode="gated" can-configure></fb-state>');
    expect(owner.shadowRoot!.querySelector('fb-button')).toBeTruthy();
    const member = await mount<FbState>('<fb-state mode="gated"></fb-state>');
    expect(member.shadowRoot!.querySelector('fb-button')).toBeNull();
    expect(member.shadowRoot!.textContent).toContain('Ask your account owner');
  });
});

describe('fb-modal', () => {
  it('renders a labelled dialog only when open and emits close on Esc', async () => {
    const el = await mount<FbModal>('<fb-modal heading="Edit task">body</fb-modal>');
    expect(el.shadowRoot!.querySelector('[role="dialog"]')).toBeNull();
    el.open = true;
    await el.updateComplete;
    const dialog = el.shadowRoot!.querySelector('[role="dialog"]')!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    const labelId = dialog.getAttribute('aria-labelledby')!;
    expect(el.shadowRoot!.getElementById(labelId)!.textContent).toContain('Edit task');

    let closed = false;
    el.addEventListener('close', () => (closed = true));
    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(closed).toBe(true);
  });
});

describe('fb-confirm', () => {
  it('emits confirm/cancel and uses destructive styling when flagged', async () => {
    const el = await mount<FbConfirm>(
      '<fb-confirm heading="Delete?" confirm-label="Delete" destructive></fb-confirm>',
    );
    el.open = true;
    await el.updateComplete;
    const modal = el.shadowRoot!.querySelector('fb-modal')!;
    await (modal as unknown as { updateComplete: Promise<unknown> }).updateComplete;
    const buttons = Array.from(el.shadowRoot!.querySelectorAll('fb-button'));
    const destructive = buttons.find((b) => b.getAttribute('variant') === 'destructive')!;
    expect(destructive.textContent).toContain('Delete');

    let confirmed = false;
    el.addEventListener('confirm', () => (confirmed = true));
    (destructive as HTMLElement).click();
    expect(confirmed).toBe(true);
  });
});

describe('fb-data-table', () => {
  const columns: Column[] = [
    { key: 'name', label: 'Name', sortable: true },
    { key: 'total_cents', label: 'Total', type: 'money', currencyKey: 'currency_code' },
    { key: 'status', label: 'Status', type: 'status' },
  ];
  const rows: Row[] = [
    { id: 'p1', name: 'Maple St', total_cents: '1234500', currency_code: 'USD', status: 'active' },
    { id: 'p2', name: 'Oak Ave', total_cents: '50000', currency_code: 'CAD', status: 'warning' },
  ];

  it('renders accessible headers with aria-sort and emits sort on header click', async () => {
    const el = await mount<FbDataTable>('<fb-data-table></fb-data-table>');
    el.columns = columns;
    el.rows = rows;
    await el.updateComplete;
    const headers = el.shadowRoot!.querySelectorAll('th[scope="col"]');
    expect(headers.length).toBe(3);
    const nameHeader = headers[0]!;
    expect(nameHeader.getAttribute('aria-sort')).toBe('none');

    let sort: { key: string; dir: string } | null = null;
    el.addEventListener('sort', (e) => (sort = (e as CustomEvent).detail));
    (nameHeader.querySelector('button') as HTMLElement).click();
    expect(sort).toEqual({ key: 'name', dir: 'asc' });
    // aria-rowcount reflects the true total.
    expect(el.shadowRoot!.querySelector('table')!.getAttribute('aria-rowcount')).toBe('2');
    // Money cells delegate to fb-money.
    expect(el.shadowRoot!.querySelectorAll('fb-money').length).toBe(2);
  });

  it('selection emits the selected row keys', async () => {
    const el = await mount<FbDataTable>('<fb-data-table selectable></fb-data-table>');
    el.columns = columns;
    el.rows = rows;
    await el.updateComplete;
    let keys: string[] = [];
    el.addEventListener('select', (e) => (keys = (e as CustomEvent).detail.keys));
    const rowChecks = el.shadowRoot!.querySelectorAll('tbody input[type="checkbox"]');
    (rowChecks[0] as HTMLInputElement).click();
    expect(keys).toEqual(['p1']);
  });
});

describe('fb-command-palette', () => {
  const commands: Command[] = [
    { id: 'projects', label: 'Go to Projects', hint: '/portfolio/projects' },
    { id: 'feed', label: 'Open Feed' },
    { id: 'fin', label: 'Financials' },
  ];

  it('filters by query and selects the active option with Enter', async () => {
    const el = await mount<FbCommandPalette>('<fb-command-palette></fb-command-palette>');
    el.commands = commands;
    el.open = true;
    await el.updateComplete;
    const input = el.shadowRoot!.querySelector('input')!;
    input.value = 'feed';
    input.dispatchEvent(new Event('input'));
    await el.updateComplete;
    const options = el.shadowRoot!.querySelectorAll('[role="option"]');
    expect(options.length).toBe(1);

    let picked: string | null = null;
    el.addEventListener('select', (e) => (picked = (e as CustomEvent).detail.id));
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    expect(picked).toBe('feed');
  });
});

describe('fb-audit-trail', () => {
  it('humanizes dotted action keys', () => {
    expect(humanizeAction('setup.trade.created')).toBe('Created trade');
    expect(humanizeAction('setup.cost_code.updated')).toBe('Updated cost code');
  });

  it('groups entries by day and renders a before/after diff', async () => {
    const entries: AuditEntry[] = [
      {
        id: 'a1',
        org_id: 'o1',
        actor_name: 'Dana',
        actor_role: 'owner',
        action: 'setup.trade.created',
        resource_type: 'trade',
        resource_id: 't1',
        before: {},
        after: { name: 'Framing' },
        created_at: new Date().toISOString(),
      },
    ];
    const el = await mount<FbAuditTrail>('<fb-audit-trail></fb-audit-trail>');
    el.entries = entries;
    await el.updateComplete;
    expect(el.shadowRoot!.querySelector('.day')!.textContent).toContain('Today');
    expect(el.shadowRoot!.querySelector('.action')!.textContent).toContain('Created trade');
    expect(el.shadowRoot!.querySelector('details')).toBeTruthy();
  });
});
