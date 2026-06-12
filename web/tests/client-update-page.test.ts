import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Endpoint modules are mocked so the page never hits the network.
vi.mock('../src/api/endpoints/projects.js', () => ({
  listProjects: vi.fn(),
}));
vi.mock('../src/api/endpoints/reports.js', () => ({
  listDailyReports: vi.fn(),
  getDailyReport: vi.fn(),
}));
vi.mock('../src/api/endpoints/client-updates.js', () => ({
  createClientUpdate: vi.fn(),
  listClientUpdates: vi.fn(),
  updateClientUpdate: vi.fn(),
  sendClientUpdate: vi.fn(),
}));
vi.mock('../src/api/endpoints/share-links.js', () => ({
  createShareLink: vi.fn(),
  listShareLinks: vi.fn(),
  revokeShareLink: vi.fn(),
}));

// authStore is mocked so role-gated actions render deterministically.
vi.mock('../src/state/authStore.js', () => ({
  hasRole: vi.fn(() => true),
}));

import '../src/components/pages/fb-client-update-page.js';

import * as projectsApi from '../src/api/endpoints/projects.js';
import * as reportsApi from '../src/api/endpoints/reports.js';
import * as cuApi from '../src/api/endpoints/client-updates.js';
import * as shareApi from '../src/api/endpoints/share-links.js';
import * as authStore from '../src/state/authStore.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import { clearCapabilities } from '../src/state/capabilityStore.js';
import type {
  Project,
  DailyReport,
  DailyReportSummary,
  ClientUpdate,
} from '../src/types/models.js';

async function mount<T extends HTMLElement>(tag: string): Promise<T> {
  const el = document.createElement(tag) as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

async function flush(page: HTMLElement): Promise<void> {
  for (let i = 0; i < 3; i++) {
    await new Promise((r) => setTimeout(r, 0));
    await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  }
}

function apiError(code: string, status = 400): ApiError {
  return new ApiError({ code, message: code, status });
}

const PROJECT: Project = {
  id: 'p-1',
  org_id: 'org-1',
  name: 'Maple Street Duplex',
  status: 'active',
  // Homeowner contact is no longer serialized on Project (server json:"-",
  // review finding M2); the composer never receives it over the wire.
  created_at: '2026-01-01',
  updated_at: '2026-01-01',
};

const SUMMARY: DailyReportSummary = {
  project_id: 'p-1',
  log_date: '2026-06-09',
  work_summary: 'Framed the second floor.',
  has_safety_incident: false,
  photo_count: 1,
  crew_count: 3,
  task_progress_count: 1,
  reported_at: '2026-06-09T18:00:00Z',
};

const REPORT: DailyReport = {
  project_id: 'p-1',
  project_name: 'Maple Street Duplex',
  log_date: '2026-06-09',
  work_summary: 'Framed the second floor.',
  photos: [
    { asset_id: 'a-1', thumb_url: 'https://example.test/thumb/a-1' },
    { asset_id: 'a-2', thumb_url: 'https://example.test/thumb/a-2' },
  ],
  photo_count: 2,
  reported_by: 'u-1',
  crew_count: 3,
  reported_at: '2026-06-09T18:00:00Z',
  has_log: true,
};

const DRAFT: ClientUpdate = {
  id: 'cu-1',
  org_id: 'org-1',
  project_id: 'p-1',
  period_start: '2026-06-09',
  period_end: '2026-06-09',
  status: 'draft',
  ai_draft: 'AI body',
  edited_body: 'AI body',
  subject: 'AI subject',
  photo_asset_ids: [],
  created_by: 'u-1',
  created_at: '2026-06-09T19:00:00Z',
  updated_at: '2026-06-09T19:00:00Z',
};

const SENT_HISTORY: ClientUpdate = {
  ...DRAFT,
  id: 'cu-prev',
  status: 'sent',
  subject: 'Last week',
  sent_at: '2026-06-02T12:00:00Z',
};

beforeEach(() => {
  vi.clearAllMocks();
  clearCapabilities(); // AI assume-on
  vi.mocked(authStore.hasRole).mockReturnValue(true);
  vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
  vi.mocked(reportsApi.listDailyReports).mockResolvedValue([SUMMARY]);
  vi.mocked(reportsApi.getDailyReport).mockResolvedValue(REPORT);
  vi.mocked(cuApi.createClientUpdate).mockResolvedValue(DRAFT);
  vi.mocked(cuApi.listClientUpdates).mockResolvedValue([SENT_HISTORY]);
  vi.mocked(cuApi.updateClientUpdate).mockImplementation(async (id, body) => ({
    ...DRAFT,
    id,
    subject: body.subject,
    edited_body: body.edited_body,
    photo_asset_ids: body.photo_asset_ids ?? [],
  }));
  vi.mocked(cuApi.sendClientUpdate).mockResolvedValue({
    ...DRAFT,
    status: 'sent',
    sent_at: '2026-06-09T20:00:00Z',
  });
  vi.mocked(shareApi.listShareLinks).mockResolvedValue([]);
  vi.mocked(shareApi.createShareLink).mockResolvedValue({
    url: 'https://acme.example/p/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
    link: {
      id: 'sl-1',
      client_update_id: 'cu-prev',
      status: 'active',
      expires_at: '2026-07-02T12:00:00Z',
      view_count: 0,
      created_at: '2026-06-02T12:00:00Z',
    },
  });
  vi.mocked(shareApi.revokeShareLink).mockResolvedValue(undefined);
  // jsdom location is /; ensure no deep-link query bleeds across tests.
  window.history.replaceState({}, '', '/portfolio/client-updates');
});

afterEach(() => {
  document.body.innerHTML = '';
  clearCapabilities();
});

describe('fb-client-update-page', () => {
  it('shows the no-projects empty state', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([]);
    const el = await mount('fb-client-update-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-state')?.getAttribute('mode')).toBe('empty');
  });

  it('loads the project + dates + sent history on mount', async () => {
    const el = await mount('fb-client-update-page');
    await flush(el);
    expect(reportsApi.listDailyReports).toHaveBeenCalledWith('p-1');
    expect(cuApi.listClientUpdates).toHaveBeenCalledWith('p-1');
    // History list shows the prior sent update with a "Sent" badge.
    const history = el.shadowRoot!.querySelector('[data-test="history"]');
    expect(history?.textContent).toContain('Last week');
    const badges = [...el.shadowRoot!.querySelectorAll('fb-badge')];
    expect(badges.some((b) => b.getAttribute('status') === 'active')).toBe(true);
  });

  it('generates a draft and opens the editor seeded with the AI text', async () => {
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { onGenerateDraft(): Promise<void> }).onGenerateDraft();
    await flush(el);
    expect(cuApi.createClientUpdate).toHaveBeenCalledWith('p-1', '2026-06-09');
    const subject = el.shadowRoot!.querySelector<HTMLElement>('[data-test="cu-subject"]');
    expect(subject).not.toBeNull();
    const body = el.shadowRoot!.querySelector<HTMLTextAreaElement>('[data-test="cu-body"]');
    expect(body?.value).toBe('AI body');
    // The day's photos are offered for curation.
    expect(el.shadowRoot!.querySelectorAll('[data-test="photo-pick"]').length).toBe(2);
  });

  it('edits subject/body + curates a photo, then previews (PATCH called)', async () => {
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { onGenerateDraft(): Promise<void> }).onGenerateDraft();
    await flush(el);

    // Operator edits.
    (el as unknown as { subject: string }).subject = 'My edited subject';
    (el as unknown as { body: string }).body = 'My edited body';
    (el as unknown as { togglePhoto(id: string): void }).togglePhoto('a-1');

    await (el as unknown as { onSaveAndPreview(): Promise<void> }).onSaveAndPreview();
    await flush(el);

    expect(cuApi.updateClientUpdate).toHaveBeenCalledWith('cu-1', {
      subject: 'My edited subject',
      edited_body: 'My edited body',
      photo_asset_ids: ['a-1'],
    });
    // Preview shows the edited subject + body via markdown.
    expect(el.shadowRoot!.querySelector('[data-test="preview-subject"]')?.textContent).toContain(
      'My edited subject',
    );
    expect(el.shadowRoot!.querySelector('fb-markdown')).not.toBeNull();
    // Only the curated photo (a-1) is in the preview strip.
    const imgs = [...el.shadowRoot!.querySelectorAll('.photo-strip img')];
    expect(imgs.length).toBe(1);
    expect(imgs[0]?.getAttribute('src')).toContain('a-1');
  });

  it('sends after the confirm step (sendClientUpdate called) and shows the sent state', async () => {
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { onGenerateDraft(): Promise<void> }).onGenerateDraft();
    await flush(el);
    await (el as unknown as { onSaveAndPreview(): Promise<void> }).onSaveAndPreview();
    await flush(el);
    // The Send button is only reachable from the preview/confirm step.
    expect(el.shadowRoot!.querySelector('[data-test="confirm-send"]')).not.toBeNull();
    await (el as unknown as { onConfirmSend(): Promise<void> }).onConfirmSend();
    await flush(el);
    expect(cuApi.sendClientUpdate).toHaveBeenCalledWith('cu-1');
    expect(el.shadowRoot!.querySelector('[data-test="compose-another"]')).not.toBeNull();
  });

  it('surfaces MAILER_UNCONFIGURED loudly (the update was NOT sent)', async () => {
    vi.mocked(cuApi.sendClientUpdate).mockRejectedValueOnce(apiError('MAILER_UNCONFIGURED', 422));
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { onGenerateDraft(): Promise<void> }).onGenerateDraft();
    await flush(el);
    await (el as unknown as { onSaveAndPreview(): Promise<void> }).onSaveAndPreview();
    await flush(el);
    await (el as unknown as { onConfirmSend(): Promise<void> }).onConfirmSend();
    await flush(el);
    const err = el.shadowRoot!.querySelector('[data-test="send-error"]');
    expect(err).not.toBeNull();
    expect(err?.textContent).toContain('Not sent');
    expect(err?.textContent).toContain('Resend');
  });

  it('surfaces NO_CLIENT_CONTACT loudly', async () => {
    vi.mocked(cuApi.sendClientUpdate).mockRejectedValueOnce(apiError('NO_CLIENT_CONTACT', 422));
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { onGenerateDraft(): Promise<void> }).onGenerateDraft();
    await flush(el);
    await (el as unknown as { onSaveAndPreview(): Promise<void> }).onSaveAndPreview();
    await flush(el);
    await (el as unknown as { onConfirmSend(): Promise<void> }).onConfirmSend();
    await flush(el);
    const err = el.shadowRoot!.querySelector('[data-test="send-error"]');
    expect(err?.textContent).toContain('homeowner email');
  });

  it('degrades the draft action to a gated panel on a 503 soft-fail', async () => {
    vi.mocked(cuApi.createClientUpdate).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { onGenerateDraft(): Promise<void> }).onGenerateDraft();
    await flush(el);
    const states = [...el.shadowRoot!.querySelectorAll('fb-state')];
    expect(states.some((s) => s.getAttribute('mode') === 'gated')).toBe(true);
  });

  it('shows the no-history message when a project has no client updates', async () => {
    vi.mocked(cuApi.listClientUpdates).mockResolvedValue([]);
    const el = await mount('fb-client-update-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('[data-test="no-history"]')).not.toBeNull();
  });

  // ---- Public share links (Chunk E) -------------------------------------

  it('offers a Share link action ONLY on sent history rows', async () => {
    // History has one sent + one draft row.
    vi.mocked(cuApi.listClientUpdates).mockResolvedValue([SENT_HISTORY, DRAFT]);
    const el = await mount('fb-client-update-page');
    await flush(el);
    const toggles = el.shadowRoot!.querySelectorAll('[data-test="share-toggle"]');
    // Exactly one (the sent row); the draft row offers no share action.
    expect(toggles.length).toBe(1);
  });

  it('expands, creates a public link, and shows the one-time URL once', async () => {
    const el = await mount('fb-client-update-page');
    await flush(el);

    await (el as unknown as { toggleShare(cu: ClientUpdate): Promise<void> }).toggleShare(
      SENT_HISTORY,
    );
    await flush(el);
    expect(shareApi.listShareLinks).toHaveBeenCalledWith('cu-prev');
    expect(el.shadowRoot!.querySelector('[data-test="share-panel"]')).not.toBeNull();

    // After listing returns the created link, the panel reflects it.
    vi.mocked(shareApi.listShareLinks).mockResolvedValue([
      {
        id: 'sl-1',
        client_update_id: 'cu-prev',
        status: 'active',
        expires_at: '2026-07-02T12:00:00Z',
        view_count: 0,
        created_at: '2026-06-02T12:00:00Z',
      },
    ]);
    await (
      el as unknown as { onCreateShareLink(cu: ClientUpdate): Promise<void> }
    ).onCreateShareLink(SENT_HISTORY);
    await flush(el);

    expect(shareApi.createShareLink).toHaveBeenCalledWith('cu-prev');
    const url = el.shadowRoot!.querySelector<HTMLInputElement>('[data-test="share-url"]');
    expect(url).not.toBeNull();
    expect(url!.value).toContain('/p/');
    // The active link is listed with a revoke control.
    expect(el.shadowRoot!.querySelector('[data-test="revoke-share-link"]')).not.toBeNull();
  });

  it('surfaces UPDATE_NOT_SENT when the server rejects a link on an unsent update', async () => {
    vi.mocked(shareApi.createShareLink).mockRejectedValue(apiError('UPDATE_NOT_SENT', 422));
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { toggleShare(cu: ClientUpdate): Promise<void> }).toggleShare(
      SENT_HISTORY,
    );
    await (
      el as unknown as { onCreateShareLink(cu: ClientUpdate): Promise<void> }
    ).onCreateShareLink(SENT_HISTORY);
    await flush(el);
    const err = el.shadowRoot!.querySelector('.share-error');
    expect(err?.textContent).toContain('after the update is sent');
  });

  it('revokes a link and re-loads the list', async () => {
    const el = await mount('fb-client-update-page');
    await flush(el);
    await (el as unknown as { toggleShare(cu: ClientUpdate): Promise<void> }).toggleShare(
      SENT_HISTORY,
    );
    await flush(el);
    await (
      el as unknown as { onRevokeShareLink(cu: ClientUpdate, linkId: string): Promise<void> }
    ).onRevokeShareLink(SENT_HISTORY, 'sl-1');
    await flush(el);
    expect(shareApi.revokeShareLink).toHaveBeenCalledWith('sl-1');
  });
});
