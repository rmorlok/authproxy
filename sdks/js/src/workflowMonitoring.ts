import { AxiosRequestConfig } from 'axios';
import { client } from './client';
import {
  ActionResponse,
  ObjectMetadata,
  ObjectReference,
  ResourceList,
  TypeMeta,
} from './common';
import { OperationActionStatus } from './taskMonitoring';

export const WORKFLOW_INSTANCE_KIND = 'WorkflowInstance' as const;
export const WORKFLOW_HISTORY_EVENT_KIND = 'WorkflowHistoryEvent' as const;
export const WORKFLOW_INSTANCE_CANCEL_KIND = 'WorkflowInstanceCancel' as const;
export const WORKFLOW_INSTANCE_DELETE_KIND = 'WorkflowInstanceDelete' as const;

export interface WorkflowInstanceReference {
  target: ObjectReference<typeof WORKFLOW_INSTANCE_KIND>;
  instanceId: string;
}

export type WorkflowInstanceState = 'active' | 'continued_as_new' | 'finished' | string;

export interface WorkflowInstance extends TypeMeta<typeof WORKFLOW_INSTANCE_KIND> {
  /** metadata.id is the unique execution ID. */
  metadata: ObjectMetadata & { id: string };
  spec: {
    /** Logical workflow instance ID, required together with metadata.id for addressing. */
    instanceId: string;
    queue: string;
    parentRef?: WorkflowInstanceReference;
    workflowName?: string;
  };
  status: {
    state: WorkflowInstanceState;
    completedAt?: string;
    error?: boolean;
    history?: WorkflowHistoryEvent[];
    children?: WorkflowInstance[];
  };
}

export interface WorkflowHistoryEvent extends TypeMeta<typeof WORKFLOW_HISTORY_EVENT_KIND> {
  metadata: ObjectMetadata;
  spec: {
    sequenceId?: number;
    type?: string;
    scheduleEventId?: number;
    attributes?: unknown;
    visibleAt?: string;
  };
}

export type WorkflowInstanceList = ResourceList<WorkflowInstance>;
export type WorkflowHistoryEventList = ResourceList<WorkflowHistoryEvent>;

export type WorkflowInstanceActionKind =
  | typeof WORKFLOW_INSTANCE_CANCEL_KIND
  | typeof WORKFLOW_INSTANCE_DELETE_KIND;

export type WorkflowInstanceAction<K extends WorkflowInstanceActionKind> = ActionResponse<
  K,
  typeof WORKFLOW_INSTANCE_KIND,
  { instanceId: string },
  OperationActionStatus
>;

export interface ListWorkflowInstancesParams {
  cursor?: string;
  limit?: number;
}

const instancePath = (instanceId: string, executionId: string) =>
  `/api/v1/workflow-monitoring/instances/${encodeURIComponent(instanceId)}/${encodeURIComponent(executionId)}`;

export const listWorkflowInstances = (
  params?: ListWorkflowInstancesParams,
  config?: AxiosRequestConfig,
) =>
  client.get<WorkflowInstanceList>('/api/v1/workflow-monitoring/instances', {
    ...config,
    params,
  });

export const getWorkflowInstance = (instanceId: string, executionId: string) =>
  client.get<WorkflowInstance>(instancePath(instanceId, executionId));

export const listWorkflowHistory = (instanceId: string, executionId: string) =>
  client.get<WorkflowHistoryEventList>(`${instancePath(instanceId, executionId)}/history`);

export const getWorkflowTree = (instanceId: string, executionId: string) =>
  client.get<WorkflowInstance>(`${instancePath(instanceId, executionId)}/tree`);

export const cancelWorkflowInstance = (instanceId: string, executionId: string) =>
  client.post<WorkflowInstanceAction<typeof WORKFLOW_INSTANCE_CANCEL_KIND>>(
    `${instancePath(instanceId, executionId)}/_cancel`,
  );

export const removeWorkflowInstance = (instanceId: string, executionId: string) =>
  client.delete<WorkflowInstanceAction<typeof WORKFLOW_INSTANCE_DELETE_KIND>>(
    instancePath(instanceId, executionId),
  );

export const workflowMonitoring = {
  listWorkflowInstances,
  getWorkflowInstance,
  listWorkflowHistory,
  getWorkflowTree,
  cancelWorkflowInstance,
  removeWorkflowInstance,
};
