import { client } from './client';
import {BackoffConfig} from "./backoff";

// Task models
export enum TaskState {
    UNKNOWN = 'unknown',
    ACTIVE = 'active',
    PENDING = 'pending',
    SCHEDULED = 'scheduled',
    RETRY = 'retry',
    FAILED = 'failed',
    COMPLETED = 'completed',
}

export interface TaskInfoJson {
    id: string;
    type: string;
    state: TaskState;
    updated_at?: string;
}

/**
 * Get task information
 * @param id The ID of the task to get
 * @returns Promise with the task information
 */
export const getTask = (id: string) => {
    return client.get<TaskInfoJson>(`/api/v1/tasks/${id}`);
};

// Default configuration
const defaultBackoffConfig: BackoffConfig = {
    initialDelay: 1000,    // Start with 1 second
    maxDelay: 120_000,     // Max delay of 20 minutes
    maxAttempts: 10,       // Try up to 10 times
    backoffFactor: 2       // Double the delay each time
};

/**
 * The final state of a call to poll for a task to be finalized.
 */
export enum PollForTaskResult {
    FINALIZED = 'finalized',
    RETRIES_EXHAUSTED = 'retries_exhausted',
    ERROR = 'error',
}

/**
 * Get task information
 * @param taskId The ID of the task to get
 * @param config The backoff configuration for the polling
 * @returns Promise with the task information
 */
export const pollForTaskFinalized =  async (
        taskId :string,
        config = defaultBackoffConfig,
): Promise<{
    result: PollForTaskResult
    taskInfo?: TaskInfoJson
}> => {
    // Merge provided config with defaults
    const fullConfig = { ...defaultBackoffConfig, ...config };
    let { initialDelay, maxDelay, maxAttempts, backoffFactor } = fullConfig;

    let attempts = 0;
    let delay = initialDelay;

    const poll = async (): Promise<{
        result: PollForTaskResult
        taskInfo?: TaskInfoJson
    }> => {
        attempts++;

        try {
            const response = await getTask(taskId);
            if (response.status !== 200) {
                return { result: PollForTaskResult.ERROR };
            }
            
            const taskInfo = response.data;

            // If task is completed, refresh connections and return result
            if (taskInfo.state === TaskState.COMPLETED || taskInfo.state === TaskState.FAILED) {
                return { result: PollForTaskResult.FINALIZED, taskInfo };
            }

            // If we've reached max attempts, return with an error
            if (attempts >= maxAttempts) {
                return { result: PollForTaskResult.RETRIES_EXHAUSTED };
            }

            // Otherwise wait and try again with exponential backoff
            await new Promise(resolve => setTimeout(resolve, delay));

            // Increase delay for next attempt (with a maximum cap)
            delay = Math.min(delay * backoffFactor, maxDelay);

            // Recursive call to continue polling
            return poll();
        } catch (error) {
            // Handle API errors
            if (attempts >= maxAttempts) {
                // We failed to get a success and ran out of attempts
                throw { result: PollForTaskResult.ERROR };
            }

            // Wait and try again
            await new Promise(resolve => setTimeout(resolve, delay));
            delay = Math.min(delay * backoffFactor, maxDelay);
            return poll();
        }
    };

    return poll();
}

export const tasks = {
    getTask,
    pollForTaskFinalized,
};