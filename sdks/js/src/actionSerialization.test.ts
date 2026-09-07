import { beforeEach, describe, expect, it, vi } from 'vitest';

const postMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());
const getMock = vi.hoisted(() => vi.fn());
const patchMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  client: { get: getMock, patch: patchMock, post: postMock, put: putMock },
}));

import { API_VERSION, objectReference } from './common';
import {
  archiveConnector,
  ConnectorReleaseState,
  createConnectorGeneration,
  disconnectAllConnectorConnections,
  forceConnectorGenerationState,
  getConnectorGeneration,
  listConnectorGenerations,
  updateConnectorGeneration,
} from './connectors';
import {
  abortConnection,
  ConnectionState,
  disconnectConnection,
  forceConnectionState,
  initiateConnection,
  migrateConnectionVersion,
  submitConnection,
} from './connections';
import { markNotificationViewed, markNotificationsViewed } from './notifications';
import { dryRunRateLimit } from './rateLimits';

describe('v1alpha1 action serialization', () => {
  beforeEach(() => {
    getMock.mockReset();
    patchMock.mockReset();
    postMock.mockReset();
    putMock.mockReset();
  });

  it('uses generation paths for connector generation operations', () => {
    listConnectorGenerations('cxr_test', {limit: 10});
    getConnectorGeneration('cxr_test', 3);
    createConnectorGeneration('cxr_test');
    updateConnectorGeneration('cxr_test', 3, {
      apiVersion: API_VERSION,
      kind: 'Connector',
      metadata: {labels: {environment: 'production'}},
      spec: {},
    });

    expect(getMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/connectors/cxr_test/generations',
      {params: {limit: 10}},
    );
    expect(getMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/connectors/cxr_test/generations/3',
    );
    expect(postMock).toHaveBeenCalledWith(
      '/api/v1/connectors/cxr_test/generations',
      undefined,
    );
    expect(patchMock).toHaveBeenCalledWith(
      '/api/v1/connectors/cxr_test/generations/3',
      {
        apiVersion: API_VERSION,
        kind: 'Connector',
        metadata: {labels: {environment: 'production'}},
        spec: {},
      },
    );
  });

  it('targets a connector reference when initiating a connection', () => {
    initiateConnection(objectReference('Connector', { id: 'cxr_test' }), {
      intoNamespace: 'root.acme',
      name: 'production',
      returnToUrl: 'https://example.com/callback',
    });

    expect(postMock).toHaveBeenCalledWith('/api/v1/connections/_initiate', {
      apiVersion: API_VERSION,
      kind: 'ConnectionInitiate',
      metadata: {
        target: { apiVersion: API_VERSION, kind: 'Connector', id: 'cxr_test' },
      },
      spec: {
        intoNamespace: 'root.acme',
        name: 'production',
        returnToUrl: 'https://example.com/callback',
      },
    });
  });

  it('builds connection-targeted setup and lifecycle actions', () => {
    submitConnection('cxn_test', {
      stepId: 'preconnect:0',
      data: { workspace: 'ws-123' },
    });
    disconnectConnection('cxn_test', { timeoutSeconds: 600 });
    abortConnection('cxn_test');

    const target = {
      apiVersion: API_VERSION,
      kind: 'Connection',
      id: 'cxn_test',
    };
    expect(postMock).toHaveBeenNthCalledWith(1, '/api/v1/connections/cxn_test/_submit', {
      apiVersion: API_VERSION,
      kind: 'ConnectionSetupSubmit',
      metadata: { target },
      spec: { stepId: 'preconnect:0', data: { workspace: 'ws-123' } },
    });
    expect(postMock).toHaveBeenNthCalledWith(2, '/api/v1/connections/cxn_test/_disconnect', {
      apiVersion: API_VERSION,
      kind: 'ConnectionDisconnect',
      metadata: { target },
      spec: { timeoutSeconds: 600 },
    });
    expect(postMock).toHaveBeenNthCalledWith(3, '/api/v1/connections/cxn_test/_abort', {
      apiVersion: API_VERSION,
      kind: 'ConnectionSetupAbort',
      metadata: { target },
      spec: {},
    });
  });

  it('requires an exact connector generation for migration actions', () => {
    migrateConnectionVersion('cxn_test', {
      connectorRef: objectReference('Connector', { id: 'cxr_test', generation: 3 }),
      timeoutSeconds: 600,
    });
    forceConnectionState('cxn_test', ConnectionState.CONFIGURED);

    expect(postMock).toHaveBeenCalledWith(
      '/api/v1/connections/cxn_test/_migrateVersion',
      expect.objectContaining({
        apiVersion: API_VERSION,
        kind: 'ConnectionVersionMigration',
        spec: {
          connectorRef: {
            apiVersion: API_VERSION,
            kind: 'Connector',
            id: 'cxr_test',
            generation: 3,
          },
          timeoutSeconds: 600,
        },
      }),
    );
    expect(putMock).toHaveBeenCalledWith(
      '/api/v1/connections/cxn_test/_forceState',
      expect.objectContaining({
        apiVersion: API_VERSION,
        kind: 'ConnectionForceState',
        spec: { state: ConnectionState.CONFIGURED },
      }),
    );
  });

  it('serializes single and batch notification targets as references', () => {
    markNotificationViewed('ntf_one');
    markNotificationsViewed(['ntf_one', 'ntf_two']);

    expect(postMock).toHaveBeenNthCalledWith(1, '/api/v1/notifications/ntf_one/_viewed', {
      apiVersion: API_VERSION,
      kind: 'NotificationView',
      metadata: {
        target: { apiVersion: API_VERSION, kind: 'Notification', id: 'ntf_one' },
      },
      spec: {},
    });
    expect(postMock).toHaveBeenNthCalledWith(2, '/api/v1/notifications/_viewed', {
      apiVersion: API_VERSION,
      kind: 'NotificationBatchView',
      metadata: {
        targets: [
          { apiVersion: API_VERSION, kind: 'Notification', id: 'ntf_one' },
          { apiVersion: API_VERSION, kind: 'Notification', id: 'ntf_two' },
        ],
      },
      spec: {},
    });
  });

  it('builds logical and generation-specific connector actions', () => {
    disconnectAllConnectorConnections('cxr_test', { timeoutSeconds: 600 });
    archiveConnector('cxr_test');
    forceConnectorGenerationState('cxr_test', 3, ConnectorReleaseState.PRIMARY);

    expect(postMock).toHaveBeenNthCalledWith(1, '/api/v1/connectors/cxr_test/_disconnectAll', {
      apiVersion: API_VERSION,
      kind: 'ConnectorDisconnectAll',
      metadata: {
        target: { apiVersion: API_VERSION, kind: 'Connector', id: 'cxr_test' },
      },
      spec: { timeoutSeconds: 600 },
    });
    expect(postMock).toHaveBeenNthCalledWith(2, '/api/v1/connectors/cxr_test/_archive', {
      apiVersion: API_VERSION,
      kind: 'ConnectorArchive',
      metadata: {
        target: { apiVersion: API_VERSION, kind: 'Connector', id: 'cxr_test' },
      },
      spec: {},
    });
    expect(putMock).toHaveBeenCalledWith(
      '/api/v1/connectors/cxr_test/generations/3/_forceState',
      {
        apiVersion: API_VERSION,
        kind: 'ConnectorForceState',
        metadata: {
          target: {
            apiVersion: API_VERSION,
            kind: 'Connector',
            id: 'cxr_test',
            generation: 3,
          },
        },
        spec: { state: ConnectorReleaseState.PRIMARY },
      },
    );
  });

  it('builds a rate-limit dry-run action from ergonomic context', () => {
    dryRunRateLimit({
      request: { method: 'GET', url: 'https://api.example.com/v1/things' },
      requestType: 'proxy',
      context: { namespace: 'root.acme', actorId: 'act_test' },
    });

    expect(postMock).toHaveBeenCalledWith('/api/v1/rate-limits/_dryRun', {
      apiVersion: API_VERSION,
      kind: 'RateLimitDryRun',
      metadata: {
        target: { apiVersion: API_VERSION, kind: 'Namespace', id: 'root.acme' },
      },
      spec: {
        request: { method: 'GET', url: 'https://api.example.com/v1/things' },
        requestType: 'proxy',
        actorRef: { apiVersion: API_VERSION, kind: 'Actor', id: 'act_test' },
      },
    });
  });
});
