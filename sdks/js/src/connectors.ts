import { client } from './client';
import {
  ActionRequest,
  ActionResponse,
  MutableResourceMetadata,
  NamespacedCreateMetadata,
  ObjectMetadata,
  ObjectReference,
  ResourceList,
  TypeMeta,
  actionRequest,
  objectReference,
} from './common';

export const CONNECTOR_KIND = 'Connector' as const;

export enum ConnectorReleaseState {
  DRAFT = 'draft',
  PRIMARY = 'primary',
  ACTIVE = 'active',
  ARCHIVED = 'archived',
}

/**
 * Connector definitions are open, connector-authored documents validated by
 * the server's connector schema. The SDK preserves every definition field.
 */
export type ConnectorDefinition = Record<string, unknown>;

export interface ConnectorMetadata extends ObjectMetadata {
  id: string;
  name: string;
  namespace: string;
  generation: number;
  createdAt: string;
  updatedAt: string;
}

export interface ConnectorReleaseSpec {
  desiredState?: ConnectorReleaseState.DRAFT | ConnectorReleaseState.PRIMARY;
}

export interface ConnectorSpec {
  release?: ConnectorReleaseSpec;
  definition: ConnectorDefinition;
}

export interface ConnectorStatus {
  release: {
    state: ConnectorReleaseState;
  };
}

/**
 * Every connector generation is a Connector resource. The logical connector
 * is metadata.id; metadata.generation selects a specific generation.
 */
export interface Connector extends TypeMeta<typeof CONNECTOR_KIND> {
  metadata: ConnectorMetadata;
  spec: ConnectorSpec;
  status: ConnectorStatus;
}

export interface CreateConnectorRequest extends TypeMeta<typeof CONNECTOR_KIND> {
  metadata: NamespacedCreateMetadata;
  spec: ConnectorSpec;
}

export interface UpdateConnectorRequest extends TypeMeta<typeof CONNECTOR_KIND> {
  metadata: MutableResourceMetadata;
  spec: {
    release?: {
      desiredState?: ConnectorReleaseState.DRAFT | ConnectorReleaseState.PRIMARY;
    };
    definition?: ConnectorDefinition;
  };
}

export type ConnectorList = ResourceList<Connector>;

export const CONNECTOR_DISCONNECT_ALL_KIND = 'ConnectorDisconnectAll' as const;
export const CONNECTOR_ARCHIVE_KIND = 'ConnectorArchive' as const;
export const CONNECTOR_FORCE_STATE_KIND = 'ConnectorForceState' as const;

export interface ConnectorLifecycleSpec {
  timeoutSeconds?: number;
}

export interface ConnectorLifecycleStatus {
  taskId: string;
}

export type ConnectorLifecycleRequest<K extends typeof CONNECTOR_DISCONNECT_ALL_KIND | typeof CONNECTOR_ARCHIVE_KIND> =
  ActionRequest<K, typeof CONNECTOR_KIND, ConnectorLifecycleSpec>;

export type ConnectorLifecycleResponse<K extends typeof CONNECTOR_DISCONNECT_ALL_KIND | typeof CONNECTOR_ARCHIVE_KIND> =
  ActionResponse<K, typeof CONNECTOR_KIND, ConnectorLifecycleSpec, ConnectorLifecycleStatus>;

export interface ConnectorForceStateSpec {
  state: ConnectorReleaseState;
}

export type ConnectorForceStateRequest = ActionRequest<
  typeof CONNECTOR_FORCE_STATE_KIND,
  typeof CONNECTOR_KIND,
  ConnectorForceStateSpec
>;

export interface ListConnectorsParams {
  name?: string;
  state?: ConnectorReleaseState;
  namespace?: string;
  labelSelector?: string;
  cursor?: string;
  limit?: number;
  orderBy?: string;
}

export interface ListConnectorGenerationsParams {
  state?: ConnectorReleaseState;
  namespace?: string;
  labelSelector?: string;
  cursor?: string;
  limit?: number;
  orderBy?: string;
}

export const listConnectors = (params?: ListConnectorsParams) =>
  client.get<ConnectorList>('/api/v1/connectors', { params });

export const createConnector = (request: CreateConnectorRequest) =>
  client.post<Connector>('/api/v1/connectors', request);

export const getConnector = (id: string) =>
  client.get<Connector>(`/api/v1/connectors/${id}`);

/** Updates connector-level metadata and the current draft generation. */
export const updateConnector = (id: string, request: UpdateConnectorRequest) =>
  client.patch<Connector>(`/api/v1/connectors/${id}`, request);

export const listConnectorGenerations = (
  id: string,
  params?: ListConnectorGenerationsParams,
) =>
  client.get<ConnectorList>(`/api/v1/connectors/${id}/generations`, { params });

export const createConnectorGeneration = (
  id: string,
  request?: CreateConnectorRequest,
) => client.post<Connector>(`/api/v1/connectors/${id}/generations`, request);

export const getConnectorGeneration = (id: string, generation: number) =>
  client.get<Connector>(`/api/v1/connectors/${id}/generations/${generation}`);

const connectorTarget = (
  id: string,
  generation?: number,
): ObjectReference<typeof CONNECTOR_KIND> =>
  objectReference(CONNECTOR_KIND, generation === undefined ? {id} : {id, generation});

export const updateConnectorGeneration = (
  id: string,
  generation: number,
  request: UpdateConnectorRequest,
) => client.patch<Connector>(`/api/v1/connectors/${id}/generations/${generation}`, request);

export const forceConnectorGenerationState = (
  id: string,
  generation: number,
  state: ConnectorReleaseState,
) => {
  const request: ConnectorForceStateRequest = actionRequest(
    CONNECTOR_FORCE_STATE_KIND,
    connectorTarget(id, generation),
    {state},
  );
  return client.put<Connector>(
    `/api/v1/connectors/${id}/generations/${generation}/_forceState`,
    request,
  );
};

export const disconnectAllConnectorConnections = (
  id: string,
  spec: ConnectorLifecycleSpec = {},
) => {
  const request: ConnectorLifecycleRequest<typeof CONNECTOR_DISCONNECT_ALL_KIND> = actionRequest(
    CONNECTOR_DISCONNECT_ALL_KIND,
    connectorTarget(id),
    spec,
  );
  return client.post<ConnectorLifecycleResponse<typeof CONNECTOR_DISCONNECT_ALL_KIND>>(
    `/api/v1/connectors/${id}/_disconnectAll`,
    request,
  );
};

export const archiveConnector = (id: string, spec: ConnectorLifecycleSpec = {}) => {
  const request: ConnectorLifecycleRequest<typeof CONNECTOR_ARCHIVE_KIND> = actionRequest(
    CONNECTOR_ARCHIVE_KIND,
    connectorTarget(id),
    spec,
  );
  return client.post<ConnectorLifecycleResponse<typeof CONNECTOR_ARCHIVE_KIND>>(
    `/api/v1/connectors/${id}/_archive`,
    request,
  );
};

export const connectors = {
  list: listConnectors,
  create: createConnector,
  get: getConnector,
  update: updateConnector,
  listGenerations: listConnectorGenerations,
  createGeneration: createConnectorGeneration,
  getGeneration: getConnectorGeneration,
  updateGeneration: updateConnectorGeneration,
  forceGenerationState: forceConnectorGenerationState,
  disconnectAll: disconnectAllConnectorConnections,
  archive: archiveConnector,
};
