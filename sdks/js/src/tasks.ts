import { BackoffConfig } from './backoff';
import { client } from './client';
import { ObjectMetadata, TypeMeta } from './common';

export const TASK_KIND = 'Task' as const;

export enum TaskState {
  UNKNOWN = 'unknown',
  ACTIVE = 'active',
  PENDING = 'pending',
  SCHEDULED = 'scheduled',
  RETRY = 'retry',
  FAILED = 'failed',
  COMPLETED = 'completed',
}

export interface Task extends TypeMeta<typeof TASK_KIND> {
  metadata: ObjectMetadata & { id: string };
  spec: {
    type: string;
  };
  status: {
    state: TaskState;
  };
}

export const getTask = (id: string) => client.get<Task>(`/api/v1/tasks/${id}`);

const defaultBackoffConfig: BackoffConfig = {
  initialDelay: 1_000,
  maxDelay: 120_000,
  maxAttempts: 10,
  backoffFactor: 2,
};

export enum PollForTaskResult {
  FINALIZED = 'finalized',
  RETRIES_EXHAUSTED = 'retries_exhausted',
  ERROR = 'error',
}

export interface PollForTaskFinalizedResult {
  result: PollForTaskResult;
  task?: Task;
}

/** Polls until the task is completed/failed or the configured backoff is exhausted. */
export const pollForTaskFinalized = async (
  taskId: string,
  config: BackoffConfig = defaultBackoffConfig,
): Promise<PollForTaskFinalizedResult> => {
  const fullConfig = { ...defaultBackoffConfig, ...config };
  const { initialDelay, maxDelay, maxAttempts, backoffFactor } = fullConfig;

  let attempts = 0;
  let delay = initialDelay;

  const poll = async (): Promise<PollForTaskFinalizedResult> => {
    attempts += 1;

    try {
      const response = await getTask(taskId);
      if (response.status !== 200) {
        return { result: PollForTaskResult.ERROR };
      }

      const task = response.data;
      if (task.status.state === TaskState.COMPLETED || task.status.state === TaskState.FAILED) {
        return { result: PollForTaskResult.FINALIZED, task };
      }

      if (attempts >= maxAttempts) {
        return { result: PollForTaskResult.RETRIES_EXHAUSTED };
      }

      await new Promise((resolve) => setTimeout(resolve, delay));
      delay = Math.min(delay * backoffFactor, maxDelay);
      return poll();
    } catch (_error) {
      if (attempts >= maxAttempts) {
        return { result: PollForTaskResult.ERROR };
      }
      await new Promise((resolve) => setTimeout(resolve, delay));
      delay = Math.min(delay * backoffFactor, maxDelay);
      return poll();
    }
  };

  return poll();
};

export const tasks = {
  getTask,
  pollForTaskFinalized,
};
