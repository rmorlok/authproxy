import { client } from './client';
import { ListResponse } from './common';

// Queue info
export interface QueueInfo {
  queue: string;
  memory_usage: number;
  latency_seconds: number;
  size: number;
  groups: number;
  pending: number;
  active: number;
  scheduled: number;
  retry: number;
  archived: number;
  completed: number;
  aggregating: number;
  processed: number;
  failed: number;
  processed_total: number;
  failed_total: number;
  paused: boolean;
  timestamp: string;
}

// Task info
export interface MonitoringTaskInfo {
  id: string;
  queue: string;
  type: string;
  payload: string;
  state: string;
  max_retry: number;
  retried: number;
  last_err?: string;
  last_failed_at?: string;
  next_process_at?: string;
  completed_at?: string;
  is_orphaned?: boolean;
  group?: string;
}

// Daily stats
export interface DailyStats {
  queue: string;
  processed: number;
  failed: number;
  date: string;
}

// Worker info
export interface WorkerInfo {
  task_id: string;
  task_type: string;
  queue: string;
  started: string;
  deadline: string;
}

// Server info
export interface ServerInfo {
  id: string;
  host: string;
  pid: number;
  concurrency: number;
  queues: Record<string, number>;
  strict_priority: boolean;
  started: string;
  status: string;
  active_workers: WorkerInfo[];
}

// Scheduler entry
export interface SchedulerEntry {
  id: string;
  spec: string;
  task_type: string;
  next: string;
  prev?: string;
}

// Bulk action response
export interface BulkActionResponse {
  affected_count: number;
}

// Query params
export interface ListTasksParams {
  cursor?: string;
  limit?: number;
}

export interface HistoryParams {
  days?: number;
}

export type MonitoringTaskState = 'pending' | 'active' | 'scheduled' | 'retry' | 'archived' | 'completed';

/**
 * List all queues with their info
 */
export const listQueues = () => {
  return client.get<ListResponse<QueueInfo>>('/api/v1/task-monitoring/queues');
};

/**
 * Get info for a specific queue
 */
export const getQueueInfo = (queue: string) => {
  return client.get<QueueInfo>(`/api/v1/task-monitoring/queues/${queue}`);
};

/**
 * Get daily stats history for a queue
 */
export const getQueueHistory = (queue: string, params?: HistoryParams) => {
  return client.get<DailyStats[]>(
    `/api/v1/task-monitoring/queues/${queue}/history`,
    { params }
  );
};

/**
 * List tasks by state in a queue
 */
export const listTasksByState = (
  queue: string,
  state: MonitoringTaskState,
  params?: ListTasksParams
) => {
  return client.get<ListResponse<MonitoringTaskInfo>>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${state}`,
    { params }
  );
};

/**
 * Get a specific task
 */
export const getTaskInfo = (queue: string, state: MonitoringTaskState, taskId: string) => {
  return client.get<MonitoringTaskInfo>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${state}/${taskId}`
  );
};

/**
 * List connected servers
 */
export const listServers = () => {
  return client.get<ListResponse<ServerInfo>>('/api/v1/task-monitoring/servers');
};

/**
 * List scheduler entries
 */
export const listSchedulerEntries = () => {
  return client.get<ListResponse<SchedulerEntry>>('/api/v1/task-monitoring/scheduler-entries');
};

/**
 * Run a task (move from scheduled/retry/archived to pending)
 */
export const runTask = (queue: string, taskId: string) => {
  return client.post<{ ok: boolean }>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}/_run`
  );
};

/**
 * Archive a task
 */
export const archiveTask = (queue: string, taskId: string) => {
  return client.post<{ ok: boolean }>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}/_archive`
  );
};

/**
 * Cancel a task that is currently being processed
 */
export const cancelTask = (queue: string, taskId: string) => {
  return client.post<{ ok: boolean }>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}/_cancel`
  );
};

/**
 * Delete a task
 */
export const deleteTask = (queue: string, taskId: string) => {
  return client.delete<{ ok: boolean }>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}`
  );
};

/**
 * Pause a queue
 */
export const pauseQueue = (queue: string) => {
  return client.post<{ ok: boolean }>(
    `/api/v1/task-monitoring/queues/${queue}/_pause`
  );
};

/**
 * Unpause a queue
 */
export const unpauseQueue = (queue: string) => {
  return client.post<{ ok: boolean }>(
    `/api/v1/task-monitoring/queues/${queue}/_unpause`
  );
};

/**
 * Run all archived tasks in a queue
 */
export const runAllArchivedTasks = (queue: string) => {
  return client.post<BulkActionResponse>(
    `/api/v1/task-monitoring/queues/${queue}/archived/_run-all`
  );
};

/**
 * Run all retry tasks in a queue
 */
export const runAllRetryTasks = (queue: string) => {
  return client.post<BulkActionResponse>(
    `/api/v1/task-monitoring/queues/${queue}/retry/_run-all`
  );
};

/**
 * Delete all archived tasks in a queue
 */
export const deleteAllArchivedTasks = (queue: string) => {
  return client.delete<BulkActionResponse>(
    `/api/v1/task-monitoring/queues/${queue}/archived`
  );
};

/**
 * Delete all completed tasks in a queue
 */
export const deleteAllCompletedTasks = (queue: string) => {
  return client.delete<BulkActionResponse>(
    `/api/v1/task-monitoring/queues/${queue}/completed`
  );
};

export const taskMonitoring = {
  listQueues,
  getQueueInfo,
  getQueueHistory,
  listTasksByState,
  getTaskInfo,
  listServers,
  listSchedulerEntries,
  runTask,
  archiveTask,
  cancelTask,
  deleteTask,
  pauseQueue,
  unpauseQueue,
  runAllArchivedTasks,
  runAllRetryTasks,
  deleteAllArchivedTasks,
  deleteAllCompletedTasks,
};
