/**
 * Projects endpoints — /api/v1/projects/* (internal/api/projects.go +
 * financials.go ListBudgets). List/Get are any-authenticated; Create/Update are
 * owner/admin (router-gated). Budgets are owner/admin-only. The detail view
 * pulls tasks + budgets per tab. Cents arrive as JSON numbers and are coerced to
 * strings by `normalizeCents` so the money helpers stay BigInt-safe.
 */
import { api } from '../client.js';
import { normalizeCents } from '../wire.js';
import type { Project, ProjectTask, ProjectBudget } from '../../types/models.js';

export interface ListProjectsParams {
  status?: string;
  page?: number;
  per_page?: number;
}

function query(params: ListProjectsParams): string {
  const q = new URLSearchParams();
  if (params.status) q.set('status', params.status);
  if (params.page) q.set('page', String(params.page));
  if (params.per_page) q.set('per_page', String(params.per_page));
  const s = q.toString();
  return s ? `?${s}` : '';
}

export function listProjects(params: ListProjectsParams = {}): Promise<Project[]> {
  return api
    .get<{ projects: Project[] }>(`/api/v1/projects${query(params)}`)
    .then((r) => r.projects);
}

export function getProject(projectId: string): Promise<Project> {
  return api
    .get<{ project: Project }>(`/api/v1/projects/${encodeURIComponent(projectId)}`)
    .then((r) => r.project);
}

export function listProjectTasks(projectId: string): Promise<ProjectTask[]> {
  return api
    .get<{ tasks: ProjectTask[] }>(`/api/v1/projects/${encodeURIComponent(projectId)}/tasks`)
    .then((r) => r.tasks ?? []);
}

export function listProjectBudgets(projectId: string): Promise<ProjectBudget[]> {
  return api
    .get<{ budgets: ProjectBudget[] }>(`/api/v1/projects/${encodeURIComponent(projectId)}/budgets`)
    .then((r) => normalizeCents(r.budgets ?? []));
}

export interface CreateProjectInput {
  name: string;
  address?: string;
  permit_issued_date?: string;
  project_start_date?: string;
  gsf?: number;
}
export function createProject(input: CreateProjectInput): Promise<Project> {
  return api.post<{ project: Project }>('/api/v1/projects', input).then((r) => r.project);
}
