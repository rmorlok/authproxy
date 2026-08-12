import { AxiosRequestConfig } from 'axios';
import { client } from './client';

export interface WorkflowInstance {
  instanceId: string;
  executionId: string;
  parent?: WorkflowInstance;
}

export type WorkflowInstanceState = 'active' | 'continued_as_new' | 'finished' | string;

export interface WorkflowInstanceRef {
  instance?: WorkflowInstance;
  createdAt?: string;
  completedAt?: string;
  state: WorkflowInstanceState;
  queue: string;
}

export interface WorkflowHistoryEvent {
  id?: string;
  sequenceId?: number;
  type?: string;
  timestamp?: string;
  scheduleEventId?: number;
  attributes?: unknown;
  visibleAt?: string;
}

export interface WorkflowInstanceInfo extends WorkflowInstanceRef {
  history?: WorkflowHistoryEvent[];
}

export interface WorkflowInstanceTree extends WorkflowInstanceRef {
  workflowName?: string;
  error?: boolean;
  children?: WorkflowInstanceTree[];
}

export interface ListWorkflowInstancesResponse {
  items: WorkflowInstanceRef[];
  cursor?: string;
}

export interface ListWorkflowHistoryResponse {
  items: WorkflowHistoryEvent[];
}

export interface ListWorkflowInstancesParams {
  cursor?: string;
  limit?: number;
}

const instancePath = (instanceId: string, executionId: string) =>
  `/api/v1/workflow-monitoring/instances/${encodeURIComponent(instanceId)}/${encodeURIComponent(executionId)}`;

export const listWorkflowInstances = (
  params?: ListWorkflowInstancesParams,
  config?: AxiosRequestConfig
) => {
  return client.get<ListWorkflowInstancesResponse>('/api/v1/workflow-monitoring/instances', {
    ...config,
    params,
  });
};

export const getWorkflowInstance = (instanceId: string, executionId: string) => {
  return client.get<WorkflowInstanceInfo>(instancePath(instanceId, executionId));
};

export const listWorkflowHistory = (instanceId: string, executionId: string) => {
  return client.get<ListWorkflowHistoryResponse>(`${instancePath(instanceId, executionId)}/history`);
};

export const getWorkflowTree = (instanceId: string, executionId: string) => {
  return client.get<WorkflowInstanceTree>(`${instancePath(instanceId, executionId)}/tree`);
};

export const cancelWorkflowInstance = (instanceId: string, executionId: string) => {
  return client.post<{ ok: boolean }>(`${instancePath(instanceId, executionId)}/_cancel`);
};

export const removeWorkflowInstance = (instanceId: string, executionId: string) => {
  return client.delete<{ ok: boolean }>(instancePath(instanceId, executionId));
};

export const workflowMonitoring = {
  listWorkflowInstances,
  getWorkflowInstance,
  listWorkflowHistory,
  getWorkflowTree,
  cancelWorkflowInstance,
  removeWorkflowInstance,
};
