import { client } from './client';
import { ActionResponse, ObjectMetadata, ResourceList, TypeMeta } from './common';

export const TASK_QUEUE_KIND = 'TaskQueue' as const;
export const TASK_QUEUE_HISTORY_KIND = 'TaskQueueHistory' as const;
export const TASK_EXECUTION_KIND = 'TaskExecution' as const;
export const TASK_SERVER_KIND = 'TaskServer' as const;
export const TASK_SCHEDULE_KIND = 'TaskSchedule' as const;

export const TASK_EXECUTION_RUN_KIND = 'TaskExecutionRun' as const;
export const TASK_EXECUTION_ARCHIVE_KIND = 'TaskExecutionArchive' as const;
export const TASK_EXECUTION_CANCEL_KIND = 'TaskExecutionCancel' as const;
export const TASK_EXECUTION_DELETE_KIND = 'TaskExecutionDelete' as const;
export const TASK_QUEUE_PAUSE_KIND = 'TaskQueuePause' as const;
export const TASK_QUEUE_UNPAUSE_KIND = 'TaskQueueUnpause' as const;
export const TASK_QUEUE_RUN_ALL_KIND = 'TaskQueueRunAll' as const;
export const TASK_QUEUE_DELETE_ALL_KIND = 'TaskQueueDeleteAll' as const;

export interface TaskQueue extends TypeMeta<typeof TASK_QUEUE_KIND> {
  metadata: ObjectMetadata & { id: string };
  spec: Record<string, never>;
  status: {
    memoryUsage: number;
    latencySeconds: number;
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
    processedTotal: number;
    failedTotal: number;
    paused: boolean;
  };
}

export type TaskQueueList = ResourceList<TaskQueue>;

export interface TaskExecution extends TypeMeta<typeof TASK_EXECUTION_KIND> {
  metadata: ObjectMetadata & { id: string };
  spec: {
    queue: string;
    type: string;
    payload: string;
    maxRetry: number;
    group?: string;
  };
  status: {
    state: string;
    retried: number;
    lastError?: string;
    lastFailedAt?: string;
    nextProcessAt?: string;
    completedAt?: string;
    orphaned?: boolean;
  };
}

export type TaskExecutionList = ResourceList<TaskExecution>;

export interface TaskQueueDailyStats {
  date: string;
  processed: number;
  failed: number;
}

export interface TaskQueueHistory extends TypeMeta<typeof TASK_QUEUE_HISTORY_KIND> {
  metadata: ObjectMetadata & { id: string };
  spec: {
    days: number;
  };
  status: {
    items: TaskQueueDailyStats[];
  };
}

export interface TaskWorker {
  taskId: string;
  taskType: string;
  queue: string;
  startedAt: string;
  deadline: string;
}

export interface TaskServer extends TypeMeta<typeof TASK_SERVER_KIND> {
  metadata: ObjectMetadata & { id: string };
  spec: {
    host: string;
    pid: number;
    concurrency: number;
    queues: Record<string, number>;
    strictPriority: boolean;
  };
  status: {
    state: string;
    activeWorkers: TaskWorker[];
  };
}

export type TaskServerList = ResourceList<TaskServer>;

export interface TaskSchedule extends TypeMeta<typeof TASK_SCHEDULE_KIND> {
  metadata: ObjectMetadata & { id: string };
  spec: {
    schedule: string;
    taskType: string;
  };
  status: {
    nextRunAt: string;
    previousRunAt?: string;
  };
}

export type TaskScheduleList = ResourceList<TaskSchedule>;

export interface OperationActionStatus {
  succeeded: boolean;
  affectedCount: number;
}

export type TaskExecutionActionKind =
  | typeof TASK_EXECUTION_RUN_KIND
  | typeof TASK_EXECUTION_ARCHIVE_KIND
  | typeof TASK_EXECUTION_CANCEL_KIND
  | typeof TASK_EXECUTION_DELETE_KIND;

export type TaskExecutionAction<K extends TaskExecutionActionKind> = ActionResponse<
  K,
  typeof TASK_EXECUTION_KIND,
  { queue: string },
  OperationActionStatus
>;

export type TaskQueueActionKind =
  | typeof TASK_QUEUE_PAUSE_KIND
  | typeof TASK_QUEUE_UNPAUSE_KIND
  | typeof TASK_QUEUE_RUN_ALL_KIND
  | typeof TASK_QUEUE_DELETE_ALL_KIND;

export type TaskQueueAction<K extends TaskQueueActionKind> = ActionResponse<
  K,
  typeof TASK_QUEUE_KIND,
  { state?: string },
  OperationActionStatus
>;

export interface ListTasksParams {
  cursor?: string;
  limit?: number;
}

export interface HistoryParams {
  days?: number;
}

export type MonitoringTaskState =
  | 'pending'
  | 'active'
  | 'scheduled'
  | 'retry'
  | 'archived'
  | 'completed';

export const listQueues = () =>
  client.get<TaskQueueList>('/api/v1/task-monitoring/queues');

export const getQueueInfo = (queue: string) =>
  client.get<TaskQueue>(`/api/v1/task-monitoring/queues/${queue}`);

export const getQueueHistory = (queue: string, params?: HistoryParams) =>
  client.get<TaskQueueHistory>(`/api/v1/task-monitoring/queues/${queue}/history`, { params });

export const listTasksByState = (
  queue: string,
  state: MonitoringTaskState,
  params?: ListTasksParams,
) =>
  client.get<TaskExecutionList>(`/api/v1/task-monitoring/queues/${queue}/tasks/${state}`, {
    params,
  });

export const getTaskInfo = (queue: string, state: MonitoringTaskState, taskId: string) =>
  client.get<TaskExecution>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${state}/${taskId}`,
  );

export const listServers = () =>
  client.get<TaskServerList>('/api/v1/task-monitoring/servers');

export const listSchedulerEntries = () =>
  client.get<TaskScheduleList>('/api/v1/task-monitoring/scheduler-entries');

export const runTask = (queue: string, taskId: string) =>
  client.post<TaskExecutionAction<typeof TASK_EXECUTION_RUN_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}/_run`,
  );

export const archiveTask = (queue: string, taskId: string) =>
  client.post<TaskExecutionAction<typeof TASK_EXECUTION_ARCHIVE_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}/_archive`,
  );

export const cancelTask = (queue: string, taskId: string) =>
  client.post<TaskExecutionAction<typeof TASK_EXECUTION_CANCEL_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}/_cancel`,
  );

export const deleteTask = (queue: string, taskId: string) =>
  client.delete<TaskExecutionAction<typeof TASK_EXECUTION_DELETE_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/tasks/${taskId}`,
  );

export const pauseQueue = (queue: string) =>
  client.post<TaskQueueAction<typeof TASK_QUEUE_PAUSE_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/_pause`,
  );

export const unpauseQueue = (queue: string) =>
  client.post<TaskQueueAction<typeof TASK_QUEUE_UNPAUSE_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/_unpause`,
  );

export const runAllArchivedTasks = (queue: string) =>
  client.post<TaskQueueAction<typeof TASK_QUEUE_RUN_ALL_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/archived/_runAll`,
  );

export const runAllRetryTasks = (queue: string) =>
  client.post<TaskQueueAction<typeof TASK_QUEUE_RUN_ALL_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/retry/_runAll`,
  );

export const deleteAllArchivedTasks = (queue: string) =>
  client.delete<TaskQueueAction<typeof TASK_QUEUE_DELETE_ALL_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/archived`,
  );

export const deleteAllCompletedTasks = (queue: string) =>
  client.delete<TaskQueueAction<typeof TASK_QUEUE_DELETE_ALL_KIND>>(
    `/api/v1/task-monitoring/queues/${queue}/completed`,
  );

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
