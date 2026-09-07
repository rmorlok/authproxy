import { describe, expect, expectTypeOf, it } from 'vitest';
import {
  Actor,
  ActorSpec,
  CreateActorRequest,
  actorResourceToClaim,
  createActorClaim,
} from './actors';
import { API_VERSION, ManagedResourceKind, objectReference } from './common';
import {
  Connection,
  ConnectionHealthState,
  ConnectionSetupResponse,
  ConnectionSetupResponseType,
  ConnectionState,
} from './connections';
import { Connector, ConnectorReleaseState } from './connectors';
import { Key, KeyMaterialType, KeyState, KeyUsage } from './keys';
import { Namespace, NamespaceState } from './namespaces';
import { Notification, NotificationLevel, NotificationState } from './notifications';
import { RateLimit, RateLimitMode } from './rateLimits';
import { RequestEvent, RequestType } from './requestEvents';
import { SearchResultList } from './search';
import {
  TaskExecution,
  TaskQueue,
  TaskQueueHistory,
  TaskSchedule,
  TaskServer,
} from './taskMonitoring';
import { Task, TaskState } from './tasks';
import { WorkflowHistoryEvent, WorkflowInstance } from './workflowMonitoring';

const connectorRef = objectReference('Connector', {
  id: 'cxr_test',
  namespace: 'root.acme',
  name: 'provider',
  generation: 2,
});

const actor = {
  apiVersion: API_VERSION,
  kind: 'Actor',
  metadata: {
    id: 'act_test',
    name: 'service',
    namespace: 'root.acme',
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:05:06Z',
  },
  spec: { externalId: 'subject-1', permissions: [] },
  status: { signingKeyConfigured: true },
} satisfies Actor;

const connector = {
  apiVersion: API_VERSION,
  kind: 'Connector',
  metadata: {
    id: 'cxr_test',
    name: 'provider',
    namespace: 'root.acme',
    generation: 2,
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:05:06Z',
  },
  spec: {
    release: { desiredState: ConnectorReleaseState.PRIMARY },
    definition: { displayName: 'Provider', auth: { type: 'no-auth' } },
  },
  status: { release: { state: ConnectorReleaseState.PRIMARY } },
} satisfies Connector;

const connection = {
  apiVersion: API_VERSION,
  kind: 'Connection',
  metadata: {
    id: 'cxn_test',
    name: 'production',
    namespace: 'root.acme',
    labels: { team: 'platform' },
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:05:06Z',
  },
  spec: {
    connectorRef,
    configuration: { tenant: 'acme' },
  },
  status: {
    lifecycle: { state: ConnectionState.CONFIGURED },
    health: { state: ConnectionHealthState.HEALTHY },
    configuration: {
      configured: true,
      schema: { type: 'object', properties: { tenant: { type: 'string' } } },
    },
  },
} satisfies Connection;

const key = {
  apiVersion: API_VERSION,
  kind: 'Key',
  metadata: {
    id: 'key_test',
    name: 'primary',
    namespace: 'root.acme',
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:05:06Z',
  },
  spec: {
    usage: KeyUsage.DATA_ENCRYPTION,
    materialType: KeyMaterialType.SYMMETRIC,
    desiredState: KeyState.ACTIVE,
    keyData: { value: '****************' },
  },
  status: { state: KeyState.ACTIVE, keyDataConfigured: true },
} satisfies Key;

const namespace = {
  apiVersion: API_VERSION,
  kind: 'Namespace',
  metadata: {
    id: 'root.acme',
    name: 'acme',
    namespace: 'root',
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:05:06Z',
  },
  spec: { encryptionKeyRef: objectReference('Key', { id: 'key_test' }) },
  status: { state: NamespaceState.ACTIVE },
} satisfies Namespace;

const rateLimit = {
  apiVersion: API_VERSION,
  kind: 'RateLimit',
  metadata: {
    id: 'rl_test',
    name: 'writes',
    namespace: 'root.acme',
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:05:06Z',
  },
  spec: {
    scope: { connectorRef: objectReference('Connector', { id: 'cxr_test' }) },
    mode: RateLimitMode.ENFORCE,
    selector: { methods: ['POST'] },
    bucket: { dimensions: ['actor'] },
    algorithm: { fixedWindow: { window: '1m', limit: 100 } },
  },
  status: { effectiveMode: RateLimitMode.ENFORCE },
} satisfies RateLimit;

describe('v1alpha1 managed resource contracts', () => {
  it('uses one discriminated envelope for every managed kind', () => {
    const resources: Array<Actor | Connector | Connection | Key | Namespace | RateLimit> = [
      actor,
      connector,
      connection,
      key,
      namespace,
      rateLimit,
    ];

    expect(resources.map(({ kind }) => kind)).toEqual([
      'Actor',
      'Connector',
      'Connection',
      'Key',
      'Namespace',
      'RateLimit',
    ] satisfies ManagedResourceKind[]);
    expect(resources.every(({ apiVersion }) => apiVersion === API_VERSION)).toBe(true);
    expect(connection.spec.connectorRef.generation).toBe(2);
    expect(connector.metadata.generation).toBe(2);
  });

  it('round trips optional metadata and status without flattening them', () => {
    const decoded = JSON.parse(JSON.stringify(connection)) as Connection;
    expect(decoded).toEqual(connection);
    expect(decoded.metadata.labels).toEqual({ team: 'platform' });
    expect(decoded.status.configuration.schema).toEqual(
      expect.objectContaining({ type: 'object' }),
    );
    expect(decoded).not.toHaveProperty('state');
  });

  it('keeps actor signing keys write-only and key provider data redacted', () => {
    const createActor = {
      apiVersion: API_VERSION,
      kind: 'Actor',
      metadata: { namespace: 'root.acme' },
      spec: {
        externalId: 'subject-1',
        signingKey: { raw: 'write-only' },
      },
    } satisfies CreateActorRequest;

    expect(JSON.parse(JSON.stringify(createActor)).spec.signingKey).toEqual({ raw: 'write-only' });
    expect(JSON.parse(JSON.stringify(actor)).spec).not.toHaveProperty('signingKey');
    expect(JSON.parse(JSON.stringify(key)).spec.keyData.value).toBe('****************');
    expectTypeOf<ActorSpec>().not.toHaveProperty('signingKey');
  });

  it('builds the restricted Actor resource allowed in JWT claims', () => {
    const claim = actorResourceToClaim(actor);
    const created = createActorClaim({
      externalId: 'subject-2',
      namespace: 'root.acme',
      permissions: [
        { namespace: 'root.acme', resources: ['connections'], verbs: ['get'] },
      ],
    });

    expect(claim).toEqual({
      apiVersion: API_VERSION,
      kind: 'Actor',
      metadata: { name: 'service', namespace: 'root.acme' },
      spec: { externalId: 'subject-1', permissions: [] },
    });
    expect(claim).not.toHaveProperty('metadata.id');
    expect(claim).not.toHaveProperty('metadata.createdAt');
    expect(claim).not.toHaveProperty('status');
    expect(created.spec.externalId).toBe('subject-2');
  });
});

const notification = {
  apiVersion: API_VERSION,
  kind: 'Notification',
  metadata: {
    id: 'ntf_test',
    namespace: 'root.acme',
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:05:06Z',
  },
  spec: {
    key: 'connection:reauth',
    level: NotificationLevel.WARNING,
    resourceRef: objectReference('Connection', { id: 'cxn_test' }),
    title: 'Reauthenticate',
    message: 'Credentials need attention.',
  },
  status: { state: NotificationState.ACTIVE, viewed: false },
} satisfies Notification;

const requestEvent = {
  apiVersion: API_VERSION,
  kind: 'RequestEvent',
  metadata: {
    id: 'req_test',
    namespace: 'root.acme',
    createdAt: '2026-01-02T03:04:05Z',
  },
  spec: {
    requestType: RequestType.PROXY,
    durationMilliseconds: 12,
    namespaceRef: objectReference('Namespace', { id: 'root.acme' }),
    request: { method: 'GET', host: 'api.example.com', scheme: 'https', path: '/v1' },
    response: { statusCode: 200 },
    captureAvailable: false,
  },
} satisfies RequestEvent;

const task = {
  apiVersion: API_VERSION,
  kind: 'Task',
  metadata: { id: 'task-token' },
  spec: { type: 'sync' },
  status: { state: TaskState.COMPLETED },
} satisfies Task;

const taskQueue = {
  apiVersion: API_VERSION,
  kind: 'TaskQueue',
  metadata: { id: 'default' },
  spec: {},
  status: {
    memoryUsage: 1,
    latencySeconds: 0,
    size: 1,
    groups: 0,
    pending: 1,
    active: 0,
    scheduled: 0,
    retry: 0,
    archived: 0,
    completed: 0,
    aggregating: 0,
    processed: 0,
    failed: 0,
    processedTotal: 0,
    failedTotal: 0,
    paused: false,
  },
} satisfies TaskQueue;

const taskExecution = {
  apiVersion: API_VERSION,
  kind: 'TaskExecution',
  metadata: { id: 'task-1' },
  spec: { queue: 'default', type: 'sync', payload: '{}', maxRetry: 3 },
  status: { state: 'pending', retried: 0 },
} satisfies TaskExecution;

const taskQueueHistory = {
  apiVersion: API_VERSION,
  kind: 'TaskQueueHistory',
  metadata: { id: 'default' },
  spec: { days: 30 },
  status: { items: [{ date: '2026-01-02', processed: 2, failed: 0 }] },
} satisfies TaskQueueHistory;

const taskServer = {
  apiVersion: API_VERSION,
  kind: 'TaskServer',
  metadata: { id: 'server-1' },
  spec: { host: 'worker', pid: 42, concurrency: 10, queues: { default: 1 }, strictPriority: false },
  status: { state: 'active', activeWorkers: [] },
} satisfies TaskServer;

const taskSchedule = {
  apiVersion: API_VERSION,
  kind: 'TaskSchedule',
  metadata: { id: 'schedule-1' },
  spec: { schedule: '@every 1m', taskType: 'sync' },
  status: { nextRunAt: '2026-01-02T03:04:05Z' },
} satisfies TaskSchedule;

const historyEvent = {
  apiVersion: API_VERSION,
  kind: 'WorkflowHistoryEvent',
  metadata: { id: 'event-1', createdAt: '2026-01-02T03:04:05Z' },
  spec: { sequenceId: 1, type: 'WorkflowExecutionStarted' },
} satisfies WorkflowHistoryEvent;

const workflow = {
  apiVersion: API_VERSION,
  kind: 'WorkflowInstance',
  metadata: { id: 'execution-1' },
  spec: { instanceId: 'workflow-1', queue: 'default' },
  status: { state: 'active', history: [historyEvent] },
} satisfies WorkflowInstance;

const searchResults = {
  apiVersion: API_VERSION,
  kind: 'SearchResultList',
  metadata: { truncatedKinds: ['Connection'], incompleteKinds: [] },
  items: [
    {
      resourceRef: objectReference('Connection', { id: 'cxn_test', name: 'production', namespace: 'root.acme' }),
      labels: {},
      matchedLabels: [],
      updatedAt: '2026-01-02T03:05:06Z',
    },
  ],
} satisfies SearchResultList;

describe('v1alpha1 read-only projections', () => {
  it('keeps projection identity, observations, and list metadata nested', () => {
    const projections = [
      notification,
      requestEvent,
      task,
      taskQueue,
      taskExecution,
      taskQueueHistory,
      taskServer,
      taskSchedule,
      workflow,
      historyEvent,
    ];

    expect(projections.map(({ kind }) => kind)).toEqual([
      'Notification',
      'RequestEvent',
      'Task',
      'TaskQueue',
      'TaskExecution',
      'TaskQueueHistory',
      'TaskServer',
      'TaskSchedule',
      'WorkflowInstance',
      'WorkflowHistoryEvent',
    ]);
    expect(searchResults.metadata.truncatedKinds).toEqual(['Connection']);
    expect(searchResults.items[0].resourceRef.kind).toBe('Connection');
  });

  it('discriminates setup action status under status', () => {
    const response = {
      apiVersion: API_VERSION,
      kind: 'ConnectionSetup',
      metadata: { target: objectReference('Connection', { id: 'cxn_test' }) },
      spec: {},
      status: { type: ConnectionSetupResponseType.REDIRECT, redirectUrl: 'https://example.com' },
    } satisfies ConnectionSetupResponse;

    expect(response.status.type).toBe(ConnectionSetupResponseType.REDIRECT);
    expect(response).not.toHaveProperty('type');
  });
});
