import {
  API_VERSION,
  CONNECTION_KIND,
  CONNECTION_SETUP_KIND,
  CONNECTOR_KIND,
  NOTIFICATION_KIND,
  Connection,
  ConnectionHealthState,
  ConnectionSetupCompleteResponse,
  ConnectionSetupResponseType,
  ConnectionState,
  Connector,
  ConnectorReleaseState,
  Notification,
  NotificationLevel,
  NotificationState,
  objectReference,
} from '@authproxy/api';

interface ConnectorFixtureOptions {
  id?: string;
  name?: string;
  namespace?: string;
  generation?: number;
  displayName?: string;
  description?: string;
  highlight?: string;
  logo?: unknown;
  hasConfigure?: boolean;
}

export const connectorFixture = (options: ConnectorFixtureOptions = {}): Connector => {
  const id = options.id ?? 'google-calendar';
  return {
    apiVersion: API_VERSION,
    kind: CONNECTOR_KIND,
    metadata: {
      id,
      name: options.name ?? id,
      namespace: options.namespace ?? 'root',
      generation: options.generation ?? 1,
      createdAt: '2023-04-01T12:00:00Z',
      updatedAt: '2023-04-01T12:00:00Z',
    },
    spec: {
      definition: {
        displayName: options.displayName ?? 'Google Calendar',
        description: options.description ?? 'Calendar app',
        ...(options.highlight === undefined ? {} : { highlight: options.highlight }),
        logo: options.logo ?? { publicUrl: 'https://example.com/logo.png' },
        auth: { type: 'no-auth' },
        ...(options.hasConfigure ? {
          setupFlow: {
            configure: {
              steps: [{ id: 'configure', type: 'form' }],
            },
          },
        } : {}),
      },
    },
    status: { release: { state: ConnectorReleaseState.ACTIVE } },
  };
};

interface ConnectionFixtureOptions {
  id?: string;
  name?: string;
  namespace?: string;
  connector?: Connector;
  state?: ConnectionState;
  healthState?: ConnectionHealthState;
  setupStepId?: string;
}

export const connectionFixture = (options: ConnectionFixtureOptions = {}): Connection => {
  const connector = options.connector ?? connectorFixture();
  return {
    apiVersion: API_VERSION,
    kind: CONNECTION_KIND,
    metadata: {
      id: options.id ?? 'c-1',
      name: options.name ?? 'primary-calendar',
      namespace: options.namespace ?? 'root',
      createdAt: '2023-04-01T12:00:00Z',
      updatedAt: '2023-04-01T12:00:00Z',
    },
    spec: {
      connectorRef: objectReference(CONNECTOR_KIND, {
        id: connector.metadata.id,
        generation: connector.metadata.generation,
      }),
    },
    status: {
      lifecycle: { state: options.state ?? ConnectionState.CONFIGURED },
      health: { state: options.healthState ?? ConnectionHealthState.HEALTHY },
      ...(options.setupStepId ? { setup: { stepId: options.setupStepId } } : {}),
      configuration: { configured: true, schema: {} },
    },
  };
};

export const completeSetupResponseFixture = (
  connectionId: string,
): ConnectionSetupCompleteResponse => ({
  apiVersion: API_VERSION,
  kind: CONNECTION_SETUP_KIND,
  metadata: { target: objectReference(CONNECTION_KIND, { id: connectionId }) },
  spec: {},
  status: { type: ConnectionSetupResponseType.COMPLETE },
});

interface NotificationFixtureOptions {
  id?: string;
  viewed?: boolean;
  actionUrl?: string;
}

export const notificationFixture = (options: NotificationFixtureOptions = {}): Notification => ({
  apiVersion: API_VERSION,
  kind: NOTIFICATION_KIND,
  metadata: {
    id: options.id ?? 'ntf_1',
    namespace: 'root',
    createdAt: '2026-07-12T12:00:00Z',
    updatedAt: '2026-07-12T12:00:00Z',
  },
  spec: {
    key: 'connection:cxn1:auth_required',
    level: NotificationLevel.WARNING,
    resourceRef: objectReference(CONNECTION_KIND, { id: 'cxn_1' }),
    title: 'Connection requires re-authentication',
    message: 'Reconnect this connection to continue using it.',
  },
  status: {
    state: NotificationState.ACTIVE,
    viewed: options.viewed ?? false,
    ...(options.actionUrl ? { action: { url: options.actionUrl } } : {}),
  },
});
