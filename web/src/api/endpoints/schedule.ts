/**
 * Schedule / CPM endpoints — /api/v1/projects/{projectID}/schedule/* + /tasks
 * (internal/api/schedule.go + agents.go). Read (gantt, tasks) is any-authenticated;
 * recalc, task edits, and AI adjust are min-superintendent (router-gated). The
 * physics engine is deterministic: durations/floats are integer days, times are
 * RFC3339. `recommendAdjustments` mounts only when AgentsService is wired and the
 * native AI key is configured — a missing key surfaces as 503 (§9 AI gating).
 */
import { api } from '../client.js';
import { normalizeCents } from '../wire.js';
import type {
  GanttView,
  ProjectTask,
  RecalcResult,
  ScheduleAdjustmentSet,
} from '../../types/models.js';

export function getGantt(projectId: string): Promise<GanttView> {
  return api.get<GanttView>(`/api/v1/projects/${encodeURIComponent(projectId)}/schedule/gantt`);
}

export function recalculateSchedule(projectId: string): Promise<RecalcResult> {
  return api.post<RecalcResult>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/schedule/recalculate`,
  );
}

export interface ListTasksParams {
  status?: string;
  is_critical?: boolean;
}
export function listTasks(projectId: string, params: ListTasksParams = {}): Promise<ProjectTask[]> {
  const q = new URLSearchParams();
  if (params.status) q.set('status', params.status);
  if (params.is_critical !== undefined) q.set('is_critical', String(params.is_critical));
  const s = q.toString();
  return api
    .get<{
      tasks: ProjectTask[];
    }>(`/api/v1/projects/${encodeURIComponent(projectId)}/tasks${s ? `?${s}` : ''}`)
    .then((r) => r.tasks ?? []);
}

export interface UpdateTaskInput {
  duration_days?: number;
  status?: string;
  percent_complete?: number;
  assigned_crew?: string[];
}
export function updateTask(
  projectId: string,
  taskId: string,
  input: UpdateTaskInput,
): Promise<ProjectTask> {
  return api
    .put<{
      task: ProjectTask;
    }>(
      `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}`,
      input,
    )
    .then((r) => r.task);
}

/** AI duration nudges — applied server-side, returned for post-hoc transparency (§3). */
export function recommendAdjustments(projectId: string): Promise<ScheduleAdjustmentSet> {
  return api
    .post<ScheduleAdjustmentSet>(
      `/api/v1/projects/${encodeURIComponent(projectId)}/schedule/recommend-adjustments`,
    )
    .then((r) => normalizeCents(r));
}
