/**
 * Typed API client for FutureBuild OS backend.
 * All endpoints follow the Composite Currency Pattern.
 * Base URL: /api/v1
 */

const BASE = '/api/v1';

// ─── Error Class ───────────────────────────────────────────────────────────

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: unknown,
  ) {
    super(`API ${status}`);
    this.name = 'ApiError';
  }
}

// ─── Core Fetch ────────────────────────────────────────────────────────────

interface ApiEnvelope<T> {
  data?: T;
  error?: { code: string; message: string; details?: Array<{ field: string; reason: string }> };
  meta: { request_id: string; timestamp: string; page?: number; per_page?: number; total?: number; total_pages?: number };
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });
  const body: ApiEnvelope<T> = await res.json();
  if (!res.ok || body.error) {
    throw new ApiError(res.status, body);
  }
  return body.data as T;
}

function qs(params: Record<string, string | number | boolean | undefined>): string {
  const entries = Object.entries(params).filter(
    (entry): entry is [string, string | number | boolean] => entry[1] !== undefined,
  );
  if (entries.length === 0) return '';
  return '?' + entries.map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`).join('&');
}

// ─── Type Definitions ──────────────────────────────────────────────────────

// Projects
export interface Project {
  id: string;
  org_id: string;
  name: string;
  address: string;
  permit_issued_date: string | null;
  project_start_date: string | null;
  status: 'active' | 'completed' | 'archived';
  gsf: number | null;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectRequest {
  name: string;
  address?: string;
  permit_issued_date?: string;
  gsf?: number;
  project_start_date?: string;
}

export interface UpdateProjectRequest {
  name?: string;
  address?: string;
  status?: 'active' | 'completed' | 'archived';
  gsf?: number;
}

// Schedule
export interface TaskSchedule {
  id: string;
  wbs_code: string;
  name: string;
  duration_days: number;
  early_start: string;
  early_finish: string;
  late_start: string;
  late_finish: string;
  total_float: number;
  is_critical: boolean;
  status: 'pending' | 'in_progress' | 'completed';
  percent_complete: number;
  assigned_crew: string[];
}

export interface CPMResult {
  cpm_result: unknown;
  recalculation_ms: number;
}

export interface GanttData {
  tasks: TaskSchedule[];
  critical_path: string[];
  project_end: string;
}

export interface UpdateTaskRequest {
  percent_complete?: number;
  assigned_crew?: string[];
  status?: 'pending' | 'in_progress' | 'completed';
}

// Financials
export interface CorporateBudget {
  id: string;
  org_id: string;
  fiscal_year: number;
  quarter: number;
  currency_code: string;
  total_estimated_cents: number;
  total_committed_cents: number;
  total_actual_cents: number;
  project_count: number;
}

export interface ARAgingSnapshot {
  id: string;
  currency_code: string;
  current_cents: number;
  days_30_cents: number;
  days_60_cents: number;
  days_90_cents: number;
  over_90_cents: number;
  total_cents: number;
  snapshot_date: string;
}

export interface FinancialSummary {
  corporate_budgets: CorporateBudget[];
  ar_aging: ARAgingSnapshot[];
}

export interface ProjectFinancial {
  project_id: string;
  project_name: string;
  currency_code: string;
  estimated_cost_cents: number;
  committed_cost_cents: number;
  actual_cost_cents: number;
  variance_cents: number;
}

export interface ProjectBudget {
  id: string;
  project_id: string;
  wbs_code: string;
  phase_name: string;
  estimated_cost_cents: number;
  estimated_cost_currency_code: string;
  committed_cost_cents: number;
  committed_cost_currency_code: string;
  actual_cost_cents: number;
  actual_cost_currency_code: string;
}

export interface CreateInvoiceRequest {
  vendor_name: string;
  amount_cents: number;
  currency_code: string;
  wbs_code?: string;
  invoice_number?: string;
  due_date?: string;
}

export interface UpdateInvoiceRequest {
  status?: string;
  paid_date?: string;
}

export interface Invoice {
  id: string;
  project_id: string;
  vendor_name: string;
  amount_cents: number;
  currency_code: string;
  wbs_code: string | null;
  invoice_number: string | null;
  due_date: string | null;
  status: string;
  paid_date: string | null;
  created_at: string;
}

// Pipeline
export type PipelineStage = 'LEAD' | 'QUALIFIED' | 'ESTIMATE_SENT' | 'VERBAL_COMMITMENT' | 'PERMIT_APPLIED' | 'PERMIT_ISSUED' | 'LOST';

export interface Prospect {
  id: string;
  org_id: string;
  name: string;
  client_name: string;
  client_email: string | null;
  client_phone: string | null;
  address: string | null;
  gsf: number | null;
  source: string | null;
  stage: PipelineStage;
  notes: string | null;
  project_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateProspectRequest {
  name: string;
  client_name: string;
  client_email?: string;
  client_phone?: string;
  address?: string;
  gsf?: number;
  source?: string;
}

export interface UpdateProspectRequest {
  name?: string;
  client_name?: string;
  client_email?: string;
  client_phone?: string;
  address?: string;
  gsf?: number;
  notes?: string;
}

export interface AdvanceProspectRequest {
  target_stage: PipelineStage;
  permit_issued_date?: string;
}

export interface LoseProspectRequest {
  reason: string;
}

export interface EstimateLineItem {
  wbs_code: string;
  description: string;
  estimated_cents: number;
  unit: string;
  quantity: number;
}

export interface Estimate {
  id: string;
  prospect_id: string;
  version: number;
  total_estimated_cents: number;
  currency_code: string;
  line_items: EstimateLineItem[];
  margin_pct: number;
  status: 'draft' | 'sent' | 'revised' | 'accepted';
  sent_at: string | null;
}

export interface CreateEstimateRequest {
  line_items: EstimateLineItem[];
  margin_pct: number;
  currency_code: string;
}

export interface UpdateEstimateRequest {
  line_items?: EstimateLineItem[];
  margin_pct?: number;
  status?: 'draft' | 'sent' | 'revised' | 'accepted';
}

export interface Permit {
  id: string;
  prospect_id: string;
  permit_type: 'building' | 'electrical' | 'plumbing' | 'mechanical';
  jurisdiction: string;
  application_number: string | null;
  submitted_date: string | null;
  expected_issue_date: string | null;
  actual_issue_date: string | null;
  fee_cents: number | null;
  fee_currency_code: string | null;
  status: 'not_submitted' | 'submitted' | 'under_review' | 'revisions_requested' | 'approved' | 'denied';
}

export interface CreatePermitRequest {
  permit_type: string;
  jurisdiction: string;
  application_number?: string;
  submitted_date?: string;
  fee_cents?: number;
  fee_currency_code?: string;
}

export interface UpdatePermitRequest {
  status?: string;
  actual_issue_date?: string;
  expected_issue_date?: string;
  notes?: string;
}

export interface PipelineStageAnalytics {
  stage: PipelineStage;
  count: number;
  weighted_revenue_cents: number;
}

export interface PipelineCurrencyAnalytics {
  currency_code: string;
  total_weighted_revenue_cents: number;
  stages: PipelineStageAnalytics[];
}

export interface PipelineAnalytics {
  by_currency: PipelineCurrencyAnalytics[];
}

// Procurement
export type ProcurementStatus = 'OK' | 'WARNING' | 'CRITICAL' | 'ORDERED';

export interface ProcurementItem {
  id: string;
  project_id: string;
  name: string;
  wbs_code: string;
  estimated_cost_cents: number;
  estimated_cost_currency_code: string;
  lead_time_days: number;
  weather_buffer_days: number;
  need_by_date: string;
  must_order_date: string;
  status: ProcurementStatus;
  po_number: string | null;
}

export interface CreateProcurementRequest {
  name: string;
  wbs_code: string;
  estimated_cost_cents: number;
  estimated_cost_currency_code: string;
  lead_time_days: number;
  need_by_date: string;
}

export interface UpdateProcurementRequest {
  status?: ProcurementStatus;
  po_number?: string;
  ordered_at?: string;
}

// Feed
export type FeedPriority = 'critical' | 'urgent' | 'normal' | 'low';

export interface FeedAction {
  label: string;
  action_type: string;
  payload: Record<string, unknown>;
}

export interface FeedCard {
  id: string;
  org_id: string;
  project_id: string;
  card_type: 'weather_alert' | 'procurement' | 'sub_confirmation' | 'progress' | 'delay' | 'permit_update';
  title: string;
  body: string;
  priority: FeedPriority;
  actions: FeedAction[];
  status: 'active' | 'dismissed' | 'actioned' | 'expired';
  created_at: string;
}

export interface ActionFeedRequest {
  action_type: string;
  payload: Record<string, unknown>;
}

// Fleet
export interface FleetAsset {
  id: string;
  org_id: string;
  name: string;
  asset_type: string;
  serial_number: string | null;
  status: string;
  current_project_id: string | null;
  created_at: string;
}

export interface CreateAssetRequest {
  name: string;
  asset_type: string;
  serial_number?: string;
}

export interface AllocateAssetRequest {
  project_id: string;
  start_date: string;
  end_date: string;
}

export interface EquipmentAllocation {
  id: string;
  asset_id: string;
  project_id: string;
  start_date: string;
  end_date: string;
}

// HR
export interface Employee {
  id: string;
  org_id: string;
  name: string;
  role: string;
  email: string | null;
  phone: string | null;
  status: string;
  hire_date: string;
}

export interface Certification {
  id: string;
  employee_id: string;
  name: string;
  issuer: string;
  issue_date: string;
  expiry_date: string | null;
  status: string;
}

// Field
export interface FieldSyncData {
  notifications: Array<{ type: string; payload: Record<string, unknown> }>;
  tasks: Array<{ id: string; wbs_code: string; percent_complete: number }>;
  server_time: string;
}

export interface ReportProgressRequest {
  task_id: string;
  percent_complete: number;
  photo_asset_id?: string;
  gps_lat?: number;
  gps_lng?: number;
  idempotency_key: string;
}

export interface CheckinRequest {
  project_id: string;
  crew_members: Array<{ worker_id: string; gps_lat: number; gps_lng: number }>;
  idempotency_key: string;
}

export interface DailyLogRequest {
  project_id: string;
  weather_conditions: string;
  work_summary: string;
  safety_incidents?: string;
  photos?: string[];
  idempotency_key: string;
}

// Prospect detail response
export interface ProspectDetail {
  prospect: Prospect;
  estimates: Estimate[];
  permits: Permit[];
}

// ─── API Methods ───────────────────────────────────────────────────────────

// Projects
export async function listProjects(params?: { status?: string; page?: number; per_page?: number }): Promise<{ projects: Project[] }> {
  return fetchJSON(`${BASE}/projects${qs(params ?? {})}`);
}

export async function getProject(projectID: string): Promise<{ project: Project }> {
  return fetchJSON(`${BASE}/projects/${projectID}`);
}

export async function createProject(body: CreateProjectRequest): Promise<{ project: Project }> {
  return fetchJSON(`${BASE}/projects`, { method: 'POST', body: JSON.stringify(body) });
}

export async function updateProject(projectID: string, body: UpdateProjectRequest): Promise<{ project: Project }> {
  return fetchJSON(`${BASE}/projects/${projectID}`, { method: 'PUT', body: JSON.stringify(body) });
}

// Schedule
export async function recalculateSchedule(projectID: string): Promise<CPMResult> {
  return fetchJSON(`${BASE}/projects/${projectID}/schedule/recalculate`, { method: 'POST', body: '{}' });
}

export async function getGantt(projectID: string): Promise<GanttData> {
  return fetchJSON(`${BASE}/projects/${projectID}/schedule/gantt`);
}

export async function listTasks(projectID: string, params?: { status?: string; is_critical?: boolean }): Promise<{ tasks: TaskSchedule[] }> {
  return fetchJSON(`${BASE}/projects/${projectID}/tasks${qs(params ?? {})}`);
}

export async function updateTask(projectID: string, taskID: string, body: UpdateTaskRequest): Promise<{ task: TaskSchedule }> {
  return fetchJSON(`${BASE}/projects/${projectID}/tasks/${taskID}`, { method: 'PUT', body: JSON.stringify(body) });
}

// Financials
export async function getFinancialSummary(orgID: string, currency?: string): Promise<FinancialSummary> {
  return fetchJSON(`${BASE}/org/${orgID}/financials/summary${qs({ currency })}`);
}

export async function getARAging(orgID: string, currency?: string): Promise<{ snapshots: ARAgingSnapshot[] }> {
  return fetchJSON(`${BASE}/org/${orgID}/financials/ar-aging${qs({ currency })}`);
}

export async function getProjectFinancials(orgID: string, currency?: string): Promise<{ projects: ProjectFinancial[] }> {
  return fetchJSON(`${BASE}/org/${orgID}/financials/projects${qs({ currency })}`);
}

export async function listBudgets(projectID: string): Promise<{ budgets: ProjectBudget[] }> {
  return fetchJSON(`${BASE}/projects/${projectID}/budgets`);
}

export async function createInvoice(projectID: string, body: CreateInvoiceRequest): Promise<{ invoice: Invoice }> {
  return fetchJSON(`${BASE}/projects/${projectID}/invoices`, { method: 'POST', body: JSON.stringify(body) });
}

export async function updateInvoice(projectID: string, invoiceID: string, body: UpdateInvoiceRequest): Promise<{ invoice: Invoice }> {
  return fetchJSON(`${BASE}/projects/${projectID}/invoices/${invoiceID}`, { method: 'PUT', body: JSON.stringify(body) });
}

// Pipeline
export async function listProspects(orgID: string, stage?: string): Promise<{ prospects: Prospect[] }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects${qs({ stage })}`);
}

export async function getProspect(orgID: string, prospectID: string): Promise<ProspectDetail> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects/${prospectID}`);
}

export async function createProspect(orgID: string, body: CreateProspectRequest): Promise<{ prospect: Prospect }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects`, { method: 'POST', body: JSON.stringify(body) });
}

export async function updateProspect(orgID: string, prospectID: string, body: UpdateProspectRequest): Promise<{ prospect: Prospect }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects/${prospectID}`, { method: 'PUT', body: JSON.stringify(body) });
}

export async function advanceProspect(orgID: string, prospectID: string, body: AdvanceProspectRequest): Promise<{ prospect: Prospect; project_id?: string }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects/${prospectID}/advance`, { method: 'POST', body: JSON.stringify(body) });
}

export async function loseProspect(orgID: string, prospectID: string, body: LoseProspectRequest): Promise<{ prospect: Prospect }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects/${prospectID}/lose`, { method: 'POST', body: JSON.stringify(body) });
}

export async function createEstimate(orgID: string, prospectID: string, body: CreateEstimateRequest): Promise<{ estimate: Estimate }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects/${prospectID}/estimates`, { method: 'POST', body: JSON.stringify(body) });
}

export async function updateEstimate(orgID: string, estimateID: string, body: UpdateEstimateRequest): Promise<{ estimate: Estimate }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/estimates/${estimateID}`, { method: 'PUT', body: JSON.stringify(body) });
}

export async function createPermit(orgID: string, prospectID: string, body: CreatePermitRequest): Promise<{ permit: Permit }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/prospects/${prospectID}/permits`, { method: 'POST', body: JSON.stringify(body) });
}

export async function updatePermit(orgID: string, permitID: string, body: UpdatePermitRequest): Promise<{ permit: Permit }> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/permits/${permitID}`, { method: 'PUT', body: JSON.stringify(body) });
}

export async function getPipelineAnalytics(orgID: string): Promise<PipelineAnalytics> {
  return fetchJSON(`${BASE}/org/${orgID}/pipeline/analytics`);
}

// Procurement
export async function listProcurement(projectID: string, status?: string): Promise<{ items: ProcurementItem[] }> {
  return fetchJSON(`${BASE}/projects/${projectID}/procurement${qs({ status })}`);
}

export async function createProcurement(projectID: string, body: CreateProcurementRequest): Promise<{ item: ProcurementItem }> {
  return fetchJSON(`${BASE}/projects/${projectID}/procurement`, { method: 'POST', body: JSON.stringify(body) });
}

export async function updateProcurement(projectID: string, itemID: string, body: UpdateProcurementRequest): Promise<{ item: ProcurementItem }> {
  return fetchJSON(`${BASE}/projects/${projectID}/procurement/${itemID}`, { method: 'PUT', body: JSON.stringify(body) });
}

// Feed
export async function listFeed(params?: { status?: string; priority?: string; page?: number; per_page?: number }): Promise<{ cards: FeedCard[] }> {
  return fetchJSON(`${BASE}/feed${qs(params ?? {})}`);
}

export async function actionFeed(cardID: string, body: ActionFeedRequest): Promise<{ card: FeedCard; result: unknown }> {
  return fetchJSON(`${BASE}/feed/${cardID}/action`, { method: 'POST', body: JSON.stringify(body) });
}

export async function dismissFeed(cardID: string): Promise<{ card: FeedCard }> {
  return fetchJSON(`${BASE}/feed/${cardID}/dismiss`, { method: 'POST' });
}

// Fleet
export async function listAssets(orgID: string): Promise<{ assets: FleetAsset[] }> {
  return fetchJSON(`${BASE}/org/${orgID}/fleet`);
}

export async function createAsset(orgID: string, body: CreateAssetRequest): Promise<{ asset: FleetAsset }> {
  return fetchJSON(`${BASE}/org/${orgID}/fleet`, { method: 'POST', body: JSON.stringify(body) });
}

export async function allocateAsset(orgID: string, assetID: string, body: AllocateAssetRequest): Promise<{ allocation: EquipmentAllocation }> {
  return fetchJSON(`${BASE}/org/${orgID}/fleet/${assetID}/allocate`, { method: 'POST', body: JSON.stringify(body) });
}

// HR
export async function listEmployees(orgID: string): Promise<{ employees: Employee[] }> {
  return fetchJSON(`${BASE}/org/${orgID}/employees`);
}

export async function listCertifications(orgID: string, employeeID: string): Promise<{ certifications: Certification[] }> {
  return fetchJSON(`${BASE}/org/${orgID}/employees/${employeeID}/certifications`);
}

// Field
export async function syncField(since?: string): Promise<FieldSyncData> {
  return fetchJSON(`${BASE}/field/sync${qs({ since })}`);
}

export async function reportProgress(body: ReportProgressRequest): Promise<void> {
  await fetchJSON(`${BASE}/field/progress`, { method: 'POST', body: JSON.stringify(body) });
}

export async function checkin(body: CheckinRequest): Promise<void> {
  await fetchJSON(`${BASE}/field/checkin`, { method: 'POST', body: JSON.stringify(body) });
}

export async function submitDailyLog(body: DailyLogRequest): Promise<void> {
  await fetchJSON(`${BASE}/field/daily-log`, { method: 'POST', body: JSON.stringify(body) });
}
