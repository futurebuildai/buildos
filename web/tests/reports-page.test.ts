import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Endpoint modules are mocked so the page never hits the network.
vi.mock('../src/api/endpoints/projects.js', () => ({
  listProjects: vi.fn(),
}));
vi.mock('../src/api/endpoints/reports.js', () => ({
  listDailyReports: vi.fn(),
  getDailyReport: vi.fn(),
  generateDigest: vi.fn(),
  draftClientUpdate: vi.fn(),
}));

// The asset (photo-upload) endpoint module is mocked so the "Add photos" flow
// never hits the network or the R2 PUT.
vi.mock('../src/api/endpoints/assets.js', () => ({
  uploadProjectPhoto: vi.fn(),
  linkPhotosToDailyLog: vi.fn(),
}));

// The capabilities endpoint is mocked so the storage gate is controllable via
// refreshCapabilities (the real capabilityStore is otherwise used unmocked).
vi.mock('../src/api/endpoints/capabilities.js', () => ({
  getCapabilities: vi.fn(),
}));

// authStore is mocked so role-gated actions render deterministically.
vi.mock('../src/state/authStore.js', () => ({
  hasRole: vi.fn(() => true),
  hasMinRole: vi.fn(() => true),
}));

import '../src/components/pages/fb-reports-page.js';

import * as projectsApi from '../src/api/endpoints/projects.js';
import * as reportsApi from '../src/api/endpoints/reports.js';
import * as assetsApi from '../src/api/endpoints/assets.js';
import * as capabilitiesApi from '../src/api/endpoints/capabilities.js';
import * as authStore from '../src/state/authStore.js';
import { ApiError, ErrorCode } from '../src/api/errors.js';
import { clearCapabilities, refreshCapabilities } from '../src/state/capabilityStore.js';
import type { Capabilities } from '../src/types/models.js';

/** Set the capability store via the mocked endpoint. */
async function setStorageConfigured(on: boolean): Promise<void> {
  vi.mocked(capabilitiesApi.getCapabilities).mockResolvedValueOnce({
    ai_configured: true,
    email_configured: true,
    storage_configured: on,
    providers: [],
  } satisfies Capabilities);
  await refreshCapabilities();
}
import type {
  Project,
  DailyReport,
  DailyReportSummary,
  ClientUpdateDraft,
} from '../src/types/models.js';

async function mount<T extends HTMLElement>(tag: string): Promise<T> {
  const el = document.createElement(tag) as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

async function flush(page: HTMLElement): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  await new Promise((r) => setTimeout(r, 0));
  await (page as unknown as { updateComplete: Promise<unknown> }).updateComplete;
}

function apiError(code: string, status = 400): ApiError {
  return new ApiError({ code, message: code, status });
}

const PROJECT: Project = {
  id: 'p-1',
  org_id: 'org-1',
  name: 'Maple Street Duplex',
  status: 'active',
  created_at: '2026-01-01',
  updated_at: '2026-01-01',
};

const SUMMARY: DailyReportSummary = {
  project_id: 'p-1',
  log_date: '2026-06-09',
  weather_conditions: 'Sunny',
  work_summary: 'Framed the second floor.',
  has_safety_incident: true,
  photo_count: 2,
  crew_count: 3,
  task_progress_count: 1,
  reported_at: '2026-06-09T18:00:00Z',
};

const REPORT: DailyReport = {
  project_id: 'p-1',
  project_name: 'Maple Street Duplex',
  log_date: '2026-06-09',
  weather_conditions: 'Sunny',
  work_summary: 'Framed the second floor.',
  safety_incidents: 'Scaffold issue near grid C.',
  photos: [{ asset_id: 'a-1', thumb_url: 'https://example.test/thumb/a-1' }],
  photo_count: 1,
  reported_by: 'u-1',
  crew_count: 3,
  task_progress: [
    {
      task_id: 't-1',
      wbs_code: '2.0',
      name: 'Framing',
      percent_complete: 60,
      reported_at: '2026-06-09T17:00:00Z',
    },
  ],
  reported_at: '2026-06-09T18:00:00Z',
  has_log: true,
};

beforeEach(() => {
  vi.clearAllMocks();
  clearCapabilities(); // AI assume-on
  vi.mocked(authStore.hasRole).mockReturnValue(true);
  vi.mocked(authStore.hasMinRole).mockReturnValue(true);
  vi.mocked(projectsApi.listProjects).mockResolvedValue([PROJECT]);
  vi.mocked(reportsApi.listDailyReports).mockResolvedValue([SUMMARY]);
  vi.mocked(reportsApi.getDailyReport).mockResolvedValue(REPORT);
  vi.mocked(reportsApi.generateDigest).mockResolvedValue('Office digest: framing progressed.');
  vi.mocked(reportsApi.draftClientUpdate).mockResolvedValue({
    subject: 'Your home is coming along!',
    body: 'This week the framing went up.',
    period_start: '2026-06-09',
    period_end: '2026-06-09',
    photo_count: 1,
  } satisfies ClientUpdateDraft);
});

afterEach(() => {
  document.body.innerHTML = '';
  clearCapabilities();
});

describe('fb-reports-page', () => {
  it('shows the no-projects empty state', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue([]);
    const el = await mount('fb-reports-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('fb-state')?.getAttribute('mode')).toBe('empty');
  });

  it('renders the derived report for the first project + date', async () => {
    const el = await mount('fb-reports-page');
    await flush(el);
    expect(reportsApi.listDailyReports).toHaveBeenCalledWith('p-1');
    expect(reportsApi.getDailyReport).toHaveBeenCalledWith('p-1', '2026-06-09');
    // Field notes render through fb-markdown.
    expect(el.shadowRoot!.querySelector('fb-markdown')).not.toBeNull();
    // Task-progress table present.
    expect(el.shadowRoot!.querySelector('fb-data-table')).not.toBeNull();
    // Safety incident surfaces a critical badge (operator surface).
    const badges = [...el.shadowRoot!.querySelectorAll('fb-badge')];
    expect(badges.some((b) => b.getAttribute('status') === 'critical')).toBe(true);
  });

  it('renders the photo thumbnail strip when assets are present', async () => {
    const el = await mount('fb-reports-page');
    await flush(el);
    const imgs = [...el.shadowRoot!.querySelectorAll('.photo-strip img')];
    expect(imgs.length).toBe(1);
    expect(imgs[0]?.getAttribute('src')).toBe('https://example.test/thumb/a-1');
  });

  it('works text-only with zero photos (photos are additive)', async () => {
    vi.mocked(reportsApi.getDailyReport).mockResolvedValue({
      ...REPORT,
      photos: [],
      photo_count: 0,
    });
    const el = await mount('fb-reports-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('.photo-strip')).toBeNull();
    // The report body still renders.
    expect(el.shadowRoot!.querySelector('fb-markdown')).not.toBeNull();
  });

  it('generates the office digest via the endpoint and renders it', async () => {
    const el = await mount('fb-reports-page');
    await flush(el);
    await (el as unknown as { onGenerateDigest(): Promise<void> }).onGenerateDigest();
    await flush(el);
    expect(reportsApi.generateDigest).toHaveBeenCalledWith('p-1', '2026-06-09');
    expect(el.shadowRoot!.textContent).toContain('Office digest');
  });

  it('drafts a client update and shows a read-only preview', async () => {
    const el = await mount('fb-reports-page');
    await flush(el);
    await (el as unknown as { onDraftClientUpdate(): Promise<void> }).onDraftClientUpdate();
    await flush(el);
    expect(reportsApi.draftClientUpdate).toHaveBeenCalledWith('p-1', '2026-06-09');
    const subject = el.shadowRoot!.querySelector('[data-test="draft-subject"]');
    expect(subject?.textContent).toContain('Your home is coming along!');
    // Read-only note present (no editable composer here — that's Chunk D).
    expect(el.shadowRoot!.querySelector('.draft-note')).not.toBeNull();
    // Chunk D: a deep-link into the editable composer is offered.
    expect(el.shadowRoot!.querySelector('[data-test="open-composer"]')).not.toBeNull();
  });

  it('degrades the digest action to a gated panel on a 503 soft-fail', async () => {
    vi.mocked(reportsApi.generateDigest).mockRejectedValueOnce(
      apiError(ErrorCode.SERVICE_UNAVAILABLE, 503),
    );
    const el = await mount('fb-reports-page');
    await flush(el);
    await (el as unknown as { onGenerateDigest(): Promise<void> }).onGenerateDigest();
    await flush(el);
    const states = [...el.shadowRoot!.querySelectorAll('fb-state')];
    expect(states.some((s) => s.getAttribute('mode') === 'gated')).toBe(true);
  });

  it('shows a retryable error when the report fails to load', async () => {
    vi.mocked(reportsApi.getDailyReport).mockRejectedValueOnce(
      apiError(ErrorCode.UPSTREAM_ERROR, 502),
    );
    const el = await mount('fb-reports-page');
    await flush(el);
    const states = [...el.shadowRoot!.querySelectorAll('fb-state')];
    expect(states.some((s) => s.getAttribute('mode') === 'error')).toBe(true);
  });

  it('hides the office digest action from a field_worker (min-superintendent gate)', async () => {
    vi.mocked(authStore.hasMinRole).mockReturnValue(false);
    vi.mocked(authStore.hasRole).mockReturnValue(false);
    const el = await mount('fb-reports-page');
    await flush(el);
    expect(el.shadowRoot!.textContent).not.toContain('Generate office digest');
    expect(el.shadowRoot!.textContent).not.toContain('Draft client update');
  });

  it('does not render the client-draft action for a superintendent (owner/admin only)', async () => {
    // superintendent: hasMinRole(superintendent)=true but hasRole(owner,admin)=false.
    vi.mocked(authStore.hasMinRole).mockReturnValue(true);
    vi.mocked(authStore.hasRole).mockReturnValue(false);
    const el = await mount('fb-reports-page');
    await flush(el);
    expect(el.shadowRoot!.textContent).toContain('Generate office digest');
    expect(el.shadowRoot!.textContent).not.toContain('Draft client update');
  });

  // --- Chunk B: the "Add photos" affordance ---

  it('enables the Add-photos button when storage is configured', async () => {
    await setStorageConfigured(true);
    const el = await mount('fb-reports-page');
    await flush(el);
    const btn = el.shadowRoot!.querySelector('[data-test="add-photos"]');
    expect(btn).not.toBeNull();
    expect(btn?.hasAttribute('disabled')).toBe(false);
    // No "object storage not configured" hint when on.
    expect(el.shadowRoot!.querySelector('[data-test="storage-off"]')).toBeNull();
  });

  it('disables the Add-photos affordance and explains when storage is OFF', async () => {
    await setStorageConfigured(false);
    const el = await mount('fb-reports-page');
    await flush(el);
    const btn = el.shadowRoot!.querySelector('[data-test="add-photos"]');
    expect(btn?.hasAttribute('disabled')).toBe(true);
    expect(el.shadowRoot!.querySelector('[data-test="storage-off"]')?.textContent).toContain(
      'Object storage not configured',
    );
  });

  it('uploads (presign+PUT+confirm), links the asset, and refreshes the strip', async () => {
    await setStorageConfigured(true);
    vi.mocked(assetsApi.uploadProjectPhoto).mockResolvedValue('a-new');
    vi.mocked(assetsApi.linkPhotosToDailyLog).mockResolvedValue({
      id: 'log-1',
      org_id: 'org-1',
      project_id: 'p-1',
      reported_by: 'u-1',
      log_date: '2026-06-09',
      work_summary: 'Framed.',
      photo_asset_ids: ['a-1', 'a-new'],
      idempotency_key: 'k',
      reported_at: '2026-06-09T18:00:00Z',
    });
    // After the link, the refreshed report carries the new photo.
    vi.mocked(reportsApi.getDailyReport)
      .mockResolvedValueOnce(REPORT)
      .mockResolvedValueOnce({
        ...REPORT,
        photos: [
          { asset_id: 'a-1', thumb_url: 'https://example.test/thumb/a-1' },
          { asset_id: 'a-new', thumb_url: 'https://example.test/thumb/a-new' },
        ],
        photo_count: 2,
      });

    const el = await mount('fb-reports-page');
    await flush(el);

    const file = new File([new Uint8Array([1, 2, 3])], 'site.jpg', { type: 'image/jpeg' });
    const input = el.shadowRoot!.querySelector<HTMLInputElement>('#photo-input')!;
    Object.defineProperty(input, 'files', { value: [file], configurable: true });
    input.dispatchEvent(new Event('change'));
    await flush(el);
    await flush(el);

    expect(assetsApi.uploadProjectPhoto).toHaveBeenCalledWith('p-1', file);
    expect(assetsApi.linkPhotosToDailyLog).toHaveBeenCalledWith('p-1', '2026-06-09', ['a-new']);
    // Strip refreshed to two photos.
    const imgs = [...el.shadowRoot!.querySelectorAll('.photo-strip img')];
    expect(imgs.length).toBe(2);
  });

  it('flips to the disabled state on a 503 during upload', async () => {
    await setStorageConfigured(true);
    vi.mocked(assetsApi.uploadProjectPhoto).mockRejectedValue(
      new ApiError({ code: ErrorCode.SERVICE_UNAVAILABLE, message: '503', status: 503 }),
    );

    const el = await mount('fb-reports-page');
    await flush(el);
    const file = new File([new Uint8Array([1])], 'x.jpg', { type: 'image/jpeg' });
    const input = el.shadowRoot!.querySelector<HTMLInputElement>('#photo-input')!;
    Object.defineProperty(input, 'files', { value: [file], configurable: true });
    input.dispatchEvent(new Event('change'));
    await flush(el);

    expect(el.shadowRoot!.querySelector('[data-test="storage-off"]')).not.toBeNull();
    expect(
      el
        .shadowRoot!.querySelector<HTMLElement>('[data-test="add-photos"]')
        ?.hasAttribute('disabled'),
    ).toBe(true);
  });

  it('hides the Add-photos affordance from a field_worker', async () => {
    await setStorageConfigured(true);
    vi.mocked(authStore.hasMinRole).mockReturnValue(false);
    const el = await mount('fb-reports-page');
    await flush(el);
    expect(el.shadowRoot!.querySelector('[data-test="add-photos"]')).toBeNull();
  });
});
