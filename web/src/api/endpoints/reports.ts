/**
 * Daily Reports endpoints — /api/v1/projects/{projectID}/daily-reports/*
 * (internal/api/reports.go). A DERIVED read model (daily_logs + crew_checkins +
 * task_progress aggregated per project/date — no daily_reports table). Read +
 * digest are min-superintendent; the client-update DRAFT is owner/admin
 * (router-gated). The two AI compositions (office digest, client-safe homeowner
 * draft) mount only when the AI key is configured — a missing key surfaces as
 * 503 SERVICE_UNAVAILABLE (§9 AI gating). Photo thumbnails resolve to
 * short-lived signed GET URLs only when object storage is configured; text
 * reads always work with zero photos.
 */
import { api } from '../client.js';
import type { DailyReport, DailyReportSummary, ClientUpdateDraft } from '../../types/models.js';

export interface ListReportsParams {
  /** Inclusive calendar bounds (YYYY-MM-DD). Omit both for the last-14-days default. */
  since?: string;
  until?: string;
}

function query(params: ListReportsParams): string {
  const q = new URLSearchParams();
  if (params.since) q.set('since', params.since);
  if (params.until) q.set('until', params.until);
  const s = q.toString();
  return s ? `?${s}` : '';
}

/** GET the date-bucketed report summaries for a project (newest first). */
export function listDailyReports(
  projectId: string,
  params: ListReportsParams = {},
): Promise<DailyReportSummary[]> {
  return api
    .get<
      DailyReportSummary[]
    >(`/api/v1/projects/${encodeURIComponent(projectId)}/daily-reports${query(params)}`)
    .then((r) => r ?? []);
}

/** GET one day's full derived report (incl. photo thumbnails when storage is on). */
export function getDailyReport(projectId: string, date: string): Promise<DailyReport> {
  return api.get<DailyReport>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/daily-reports/${encodeURIComponent(date)}`,
  );
}

/** POST — generate the AI internal office digest for a day. 503 when AI is off. */
export function generateDigest(projectId: string, date: string): Promise<string> {
  return api
    .post<{
      digest: string;
    }>(
      `/api/v1/projects/${encodeURIComponent(projectId)}/daily-reports/${encodeURIComponent(date)}/digest`,
    )
    .then((r) => r.digest);
}

/**
 * POST — generate the client-SAFE homeowner progress DRAFT for a day (owner/admin).
 * Chunk C returns the draft for the operator to review; the editable composer +
 * send lands in Chunk D. 503 when AI is off.
 */
export function draftClientUpdate(projectId: string, date: string): Promise<ClientUpdateDraft> {
  return api.post<ClientUpdateDraft>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/daily-reports/${encodeURIComponent(date)}/client-update-draft`,
  );
}
