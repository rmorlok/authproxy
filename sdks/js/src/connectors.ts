import { client } from './client';
import {
  MutableResourceMetadata,
  NamespacedCreateMetadata,
  ObjectMetadata,
  ResourceList,
  TypeMeta,
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
 * is metadata.id; metadata.generation selects a specific version.
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

export interface ConnectorLifecycleRequest {
  timeoutSeconds?: number;
}

export interface ConnectorLifecycleResponse {
  taskId: string;
  connectorId: string;
}

export interface ForceConnectorGenerationStateRequest {
  state: ConnectorReleaseState;
}

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

export interface ConnectorLabel {
  key: string;
  value: string;
}

export interface ConnectorAnnotation {
  key: string;
  value: string;
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
  client.get<ConnectorList>(`/api/v1/connectors/${id}/versions`, { params });

export const createConnectorGeneration = (
  id: string,
  request?: CreateConnectorRequest,
) => client.post<Connector>(`/api/v1/connectors/${id}/versions`, request);

export const getConnectorGeneration = (id: string, generation: number) =>
  client.get<Connector>(`/api/v1/connectors/${id}/versions/${generation}`);

export const updateConnectorGeneration = (
  id: string,
  generation: number,
  request: UpdateConnectorRequest,
) => client.patch<Connector>(`/api/v1/connectors/${id}/versions/${generation}`, request);

export const forceConnectorGenerationState = (
  id: string,
  generation: number,
  state: ConnectorReleaseState,
) => {
  const request: ForceConnectorGenerationStateRequest = { state };
  return client.put<Connector>(
    `/api/v1/connectors/${id}/versions/${generation}/_forceState`,
    request,
  );
};

export const disconnectAllConnectorConnections = (
  id: string,
  request?: ConnectorLifecycleRequest,
) => client.post<ConnectorLifecycleResponse>(`/api/v1/connectors/${id}/_disconnectAll`, request);

export const archiveConnector = (id: string, request?: ConnectorLifecycleRequest) =>
  client.post<ConnectorLifecycleResponse>(`/api/v1/connectors/${id}/_archive`, request);

export const getConnectorLabels = (id: string) =>
  client.get<Record<string, string>>(`/api/v1/connectors/${id}/labels`);

export const getConnectorLabel = (id: string, labelKey: string) =>
  client.get<ConnectorLabel>(`/api/v1/connectors/${id}/labels/${labelKey}`);

export const putConnectorLabel = (id: string, labelKey: string, value: string) =>
  client.put<ConnectorLabel>(`/api/v1/connectors/${id}/labels/${labelKey}`, { value });

export const deleteConnectorLabel = (id: string, labelKey: string) =>
  client.delete(`/api/v1/connectors/${id}/labels/${labelKey}`);

export const getConnectorAnnotations = (id: string) =>
  client.get<Record<string, string>>(`/api/v1/connectors/${id}/annotations`);

export const getConnectorAnnotation = (id: string, annotationKey: string) =>
  client.get<ConnectorAnnotation>(`/api/v1/connectors/${id}/annotations/${annotationKey}`);

export const putConnectorAnnotation = (id: string, annotationKey: string, value: string) =>
  client.put<ConnectorAnnotation>(`/api/v1/connectors/${id}/annotations/${annotationKey}`, {
    value,
  });

export const deleteConnectorAnnotation = (id: string, annotationKey: string) =>
  client.delete(`/api/v1/connectors/${id}/annotations/${annotationKey}`);

export const getConnectorGenerationLabels = (id: string, generation: number) =>
  client.get<Record<string, string>>(
    `/api/v1/connectors/${id}/versions/${generation}/labels`,
  );

export const getConnectorGenerationLabel = (
  id: string,
  generation: number,
  labelKey: string,
) =>
  client.get<ConnectorLabel>(
    `/api/v1/connectors/${id}/versions/${generation}/labels/${labelKey}`,
  );

export const putConnectorGenerationLabel = (
  id: string,
  generation: number,
  labelKey: string,
  value: string,
) =>
  client.put<ConnectorLabel>(
    `/api/v1/connectors/${id}/versions/${generation}/labels/${labelKey}`,
    { value },
  );

export const deleteConnectorGenerationLabel = (
  id: string,
  generation: number,
  labelKey: string,
) => client.delete(`/api/v1/connectors/${id}/versions/${generation}/labels/${labelKey}`);

export const getConnectorGenerationAnnotations = (id: string, generation: number) =>
  client.get<Record<string, string>>(
    `/api/v1/connectors/${id}/versions/${generation}/annotations`,
  );

export const getConnectorGenerationAnnotation = (
  id: string,
  generation: number,
  annotationKey: string,
) =>
  client.get<ConnectorAnnotation>(
    `/api/v1/connectors/${id}/versions/${generation}/annotations/${annotationKey}`,
  );

export const putConnectorGenerationAnnotation = (
  id: string,
  generation: number,
  annotationKey: string,
  value: string,
) =>
  client.put<ConnectorAnnotation>(
    `/api/v1/connectors/${id}/versions/${generation}/annotations/${annotationKey}`,
    { value },
  );

export const deleteConnectorGenerationAnnotation = (
  id: string,
  generation: number,
  annotationKey: string,
) => client.delete(`/api/v1/connectors/${id}/versions/${generation}/annotations/${annotationKey}`);

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
  getLabels: getConnectorLabels,
  getLabel: getConnectorLabel,
  putLabel: putConnectorLabel,
  deleteLabel: deleteConnectorLabel,
  getAnnotations: getConnectorAnnotations,
  getAnnotation: getConnectorAnnotation,
  putAnnotation: putConnectorAnnotation,
  deleteAnnotation: deleteConnectorAnnotation,
  getGenerationLabels: getConnectorGenerationLabels,
  getGenerationLabel: getConnectorGenerationLabel,
  putGenerationLabel: putConnectorGenerationLabel,
  deleteGenerationLabel: deleteConnectorGenerationLabel,
  getGenerationAnnotations: getConnectorGenerationAnnotations,
  getGenerationAnnotation: getConnectorGenerationAnnotation,
  putGenerationAnnotation: putConnectorGenerationAnnotation,
  deleteGenerationAnnotation: deleteConnectorGenerationAnnotation,
};
