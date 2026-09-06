import { client } from './client';
import {
  ActionRequest,
  ActionResponse,
  MutableResourceMetadata,
  ObjectMetadata,
  ObjectReference,
  ResourceList,
  TypeMeta,
  VersionedConnectorReference,
  actionRequest,
  objectReference,
} from './common';

export const CONNECTION_KIND = 'Connection' as const;

export enum ConnectionState {
  SETUP = 'setup',
  CONFIGURED = 'configured',
  DISABLED = 'disabled',
  DISCONNECTING = 'disconnecting',
  DISCONNECTED = 'disconnected',
}

export enum ConnectionHealthState {
  HEALTHY = 'healthy',
  UNHEALTHY = 'unhealthy',
}

export interface ConnectionMetadata extends ObjectMetadata {
  id: string;
  name: string;
  namespace: string;
  createdAt: string;
  updatedAt: string;
}

export interface ConnectionSpec {
  connectorRef: VersionedConnectorReference;
  /** Connector-defined desired configuration. Auth credentials are stored separately. */
  configuration?: Record<string, unknown>;
}

export interface ConnectionStatus {
  lifecycle: {
    state: ConnectionState;
  };
  health: {
    state: ConnectionHealthState;
  };
  setup?: {
    stepId?: string;
    error?: string;
  };
  configuration: {
    configured: boolean;
    /** JSON Schema derived from the exact connector generation. */
    schema: Record<string, unknown>;
  };
}

export interface Connection extends TypeMeta<typeof CONNECTION_KIND> {
  metadata: ConnectionMetadata;
  spec: ConnectionSpec;
  status: ConnectionStatus;
}

export interface UpdateConnectionRequest extends TypeMeta<typeof CONNECTION_KIND> {
  metadata: MutableResourceMetadata;
  /** Connection bindings/configuration are changed through typed actions. */
  spec: Record<string, never>;
}

export type ConnectionList = ResourceList<Connection>;

export function canBeDisconnected(connection: Connection): boolean {
  return (
    connection.status.lifecycle.state !== ConnectionState.DISCONNECTING &&
    connection.status.lifecycle.state !== ConnectionState.DISCONNECTED
  );
}

export const CONNECTION_INITIATE_KIND = 'ConnectionInitiate' as const;
export const CONNECTION_SETUP_KIND = 'ConnectionSetup' as const;
export const CONNECTION_SETUP_SUBMIT_KIND = 'ConnectionSetupSubmit' as const;
export const CONNECTION_SETUP_ABORT_KIND = 'ConnectionSetupAbort' as const;
export const CONNECTION_RECONFIGURE_KIND = 'ConnectionReconfigure' as const;
export const CONNECTION_SETUP_CANCEL_KIND = 'ConnectionSetupCancel' as const;
export const CONNECTION_SETUP_RETRY_KIND = 'ConnectionSetupRetry' as const;
export const CONNECTION_REAUTHENTICATE_KIND = 'ConnectionReauthenticate' as const;
export const CONNECTION_DISCONNECT_KIND = 'ConnectionDisconnect' as const;
export const CONNECTION_VERSION_MIGRATION_KIND = 'ConnectionVersionMigration' as const;
export const CONNECTION_FORCE_STATE_KIND = 'ConnectionForceState' as const;

export interface ConnectionInitiateSpec {
  intoNamespace?: string;
  name?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  returnToUrl: string;
}

export type ConnectionInitiateRequest = ActionRequest<
  typeof CONNECTION_INITIATE_KIND,
  'Connector',
  ConnectionInitiateSpec
>;

export enum ConnectionSetupResponseType {
  REDIRECT = 'redirect',
  FORM = 'form',
  COMPLETE = 'complete',
  VERIFYING = 'verifying',
  ERROR = 'error',
}

export interface ConnectionSetupRedirectStatus {
  type: ConnectionSetupResponseType.REDIRECT;
  redirectUrl: string;
}

export interface ConnectionSetupFormStatus {
  type: ConnectionSetupResponseType.FORM;
  stepId: string;
  stepTitle?: string;
  stepDescription?: string;
  jsonSchema: Record<string, unknown>;
  uiSchema: Record<string, unknown>;
  /** Previously submitted values are always returned irreversibly redacted. */
  data?: Record<string, unknown>;
}

export interface ConnectionSetupCompleteStatus {
  type: ConnectionSetupResponseType.COMPLETE;
}

export interface ConnectionSetupVerifyingStatus {
  type: ConnectionSetupResponseType.VERIFYING;
}

export interface ConnectionSetupErrorStatus {
  type: ConnectionSetupResponseType.ERROR;
  error: string;
  canRetry?: boolean;
}

export type ConnectionSetupStatus =
  | ConnectionSetupRedirectStatus
  | ConnectionSetupFormStatus
  | ConnectionSetupCompleteStatus
  | ConnectionSetupVerifyingStatus
  | ConnectionSetupErrorStatus;

export type ConnectionSetupResponse = ActionResponse<
  typeof CONNECTION_SETUP_KIND,
  typeof CONNECTION_KIND,
  Record<string, never>,
  ConnectionSetupStatus
>;

export type ConnectionSetupRedirectResponse = ConnectionSetupResponse & {
  status: ConnectionSetupRedirectStatus;
};
export type ConnectionSetupFormResponse = ConnectionSetupResponse & {
  status: ConnectionSetupFormStatus;
};
export type ConnectionSetupCompleteResponse = ConnectionSetupResponse & {
  status: ConnectionSetupCompleteStatus;
};
export type ConnectionSetupVerifyingResponse = ConnectionSetupResponse & {
  status: ConnectionSetupVerifyingStatus;
};
export type ConnectionSetupErrorResponse = ConnectionSetupResponse & {
  status: ConnectionSetupErrorStatus;
};

export function isRedirectResponse(
  response: ConnectionSetupResponse,
): response is ConnectionSetupRedirectResponse {
  return response.status.type === ConnectionSetupResponseType.REDIRECT;
}

export function isFormResponse(
  response: ConnectionSetupResponse,
): response is ConnectionSetupFormResponse {
  return response.status.type === ConnectionSetupResponseType.FORM;
}

export function isCompleteResponse(
  response: ConnectionSetupResponse,
): response is ConnectionSetupCompleteResponse {
  return response.status.type === ConnectionSetupResponseType.COMPLETE;
}

export function isVerifyingResponse(
  response: ConnectionSetupResponse,
): response is ConnectionSetupVerifyingResponse {
  return response.status.type === ConnectionSetupResponseType.VERIFYING;
}

export function isErrorResponse(
  response: ConnectionSetupResponse,
): response is ConnectionSetupErrorResponse {
  return response.status.type === ConnectionSetupResponseType.ERROR;
}

export interface ConnectionSetupSubmitSpec {
  stepId: string;
  data: unknown;
  returnToUrl?: string;
}

export type ConnectionSetupSubmitRequest = ActionRequest<
  typeof CONNECTION_SETUP_SUBMIT_KIND,
  typeof CONNECTION_KIND,
  ConnectionSetupSubmitSpec
>;

export interface ConnectionSetupControlSpec {
  returnToUrl?: string;
}

export type ConnectionSetupRetryRequest = ActionRequest<
  typeof CONNECTION_SETUP_RETRY_KIND,
  typeof CONNECTION_KIND,
  ConnectionSetupControlSpec
>;

export type ConnectionReauthenticateRequest = ActionRequest<
  typeof CONNECTION_REAUTHENTICATE_KIND,
  typeof CONNECTION_KIND,
  ConnectionSetupControlSpec
>;

export type EmptyConnectionActionKind =
  | typeof CONNECTION_SETUP_ABORT_KIND
  | typeof CONNECTION_RECONFIGURE_KIND
  | typeof CONNECTION_SETUP_CANCEL_KIND;

export type EmptyConnectionActionRequest<K extends EmptyConnectionActionKind> = ActionRequest<
  K,
  typeof CONNECTION_KIND,
  Record<string, never>
>;

export interface ConnectionDisconnectSpec {
  timeoutSeconds?: number;
}

export interface ConnectionDisconnectStatus {
  taskId: string;
  connection: Connection;
}

export type ConnectionDisconnectRequest = ActionRequest<
  typeof CONNECTION_DISCONNECT_KIND,
  typeof CONNECTION_KIND,
  ConnectionDisconnectSpec
>;
export type ConnectionDisconnectResponse = ActionResponse<
  typeof CONNECTION_DISCONNECT_KIND,
  typeof CONNECTION_KIND,
  ConnectionDisconnectSpec,
  ConnectionDisconnectStatus
>;

export interface ConnectionVersionMigrationSpec {
  connectorRef: VersionedConnectorReference;
  timeoutSeconds?: number;
}

export interface ConnectionVersionMigrationStatus {
  taskId: string;
  sourceConnectorRef: VersionedConnectorReference;
  targetConnectorRef: VersionedConnectorReference;
}

export type ConnectionVersionMigrationRequest = ActionRequest<
  typeof CONNECTION_VERSION_MIGRATION_KIND,
  typeof CONNECTION_KIND,
  ConnectionVersionMigrationSpec
>;
export type ConnectionVersionMigrationResponse = ActionResponse<
  typeof CONNECTION_VERSION_MIGRATION_KIND,
  typeof CONNECTION_KIND,
  ConnectionVersionMigrationSpec,
  ConnectionVersionMigrationStatus
>;

export interface ConnectionForceStateSpec {
  state: ConnectionState;
}

export type ConnectionForceStateRequest = ActionRequest<
  typeof CONNECTION_FORCE_STATE_KIND,
  typeof CONNECTION_KIND,
  ConnectionForceStateSpec
>;
export type ConnectionForceStateResponse = ActionResponse<
  typeof CONNECTION_FORCE_STATE_KIND,
  typeof CONNECTION_KIND,
  ConnectionForceStateSpec,
  { connection: Connection }
>;

export interface DataSourceOption {
  value: string;
  label: string;
}

export interface ListConnectionsParams {
  name?: string;
  state?: ConnectionState;
  namespace?: string;
  labelSelector?: string;
  cursor?: string;
  limit?: number;
  orderBy?: string;
}

const connectionTarget = (id: string): ObjectReference<typeof CONNECTION_KIND> =>
  objectReference(CONNECTION_KIND, { id });

export const listConnections = (params?: ListConnectionsParams) =>
  client.get<ConnectionList>('/api/v1/connections', { params });

export const getConnection = (id: string) =>
  client.get<Connection>(`/api/v1/connections/${id}`);

export const initiateConnection = (
  connectorRef: ObjectReference<'Connector'>,
  spec: ConnectionInitiateSpec,
) => {
  const request: ConnectionInitiateRequest = actionRequest(
    CONNECTION_INITIATE_KIND,
    connectorRef,
    spec,
  );
  return client.post<ConnectionSetupResponse>('/api/v1/connections/_initiate', request);
};

export const submitConnection = (connectionId: string, spec: ConnectionSetupSubmitSpec) => {
  const request: ConnectionSetupSubmitRequest = actionRequest(
    CONNECTION_SETUP_SUBMIT_KIND,
    connectionTarget(connectionId),
    spec,
  );
  return client.post<ConnectionSetupResponse>(
    `/api/v1/connections/${connectionId}/_submit`,
    request,
  );
};

export const disconnectConnection = (id: string, spec: ConnectionDisconnectSpec = {}) => {
  const request: ConnectionDisconnectRequest = actionRequest(
    CONNECTION_DISCONNECT_KIND,
    connectionTarget(id),
    spec,
  );
  return client.post<ConnectionDisconnectResponse>(
    `/api/v1/connections/${id}/_disconnect`,
    request,
  );
};

export const migrateConnectionVersion = (id: string, spec: ConnectionVersionMigrationSpec) => {
  const request: ConnectionVersionMigrationRequest = actionRequest(
    CONNECTION_VERSION_MIGRATION_KIND,
    connectionTarget(id),
    spec,
  );
  return client.post<ConnectionVersionMigrationResponse>(
    `/api/v1/connections/${id}/_migrateVersion`,
    request,
  );
};

export const forceConnectionState = (id: string, state: ConnectionState) => {
  const request: ConnectionForceStateRequest = actionRequest(
    CONNECTION_FORCE_STATE_KIND,
    connectionTarget(id),
    { state },
  );
  return client.put<ConnectionForceStateResponse>(
    `/api/v1/connections/${id}/_forceState`,
    request,
  );
};

export const updateConnection = (id: string, request: UpdateConnectionRequest) =>
  client.patch<Connection>(`/api/v1/connections/${id}`, request);

export const getConnectionLabels = (id: string) =>
  client.get<Record<string, string>>(`/api/v1/connections/${id}/labels`);

export const getConnectionLabel = (id: string, labelKey: string) =>
  client.get<{ key: string; value: string }>(`/api/v1/connections/${id}/labels/${labelKey}`);

export const putConnectionLabel = (id: string, labelKey: string, value: string) =>
  client.put<{ key: string; value: string }>(
    `/api/v1/connections/${id}/labels/${labelKey}`,
    { value },
  );

export const deleteConnectionLabel = (id: string, labelKey: string) =>
  client.delete(`/api/v1/connections/${id}/labels/${labelKey}`);

export const getConnectionAnnotations = (id: string) =>
  client.get<Record<string, string>>(`/api/v1/connections/${id}/annotations`);

export const getConnectionAnnotation = (id: string, annotationKey: string) =>
  client.get<{ key: string; value: string }>(
    `/api/v1/connections/${id}/annotations/${annotationKey}`,
  );

export const putConnectionAnnotation = (id: string, annotationKey: string, value: string) =>
  client.put<{ key: string; value: string }>(
    `/api/v1/connections/${id}/annotations/${annotationKey}`,
    { value },
  );

export const deleteConnectionAnnotation = (id: string, annotationKey: string) =>
  client.delete(`/api/v1/connections/${id}/annotations/${annotationKey}`);

const emptyConnectionAction = <K extends EmptyConnectionActionKind>(id: string, kind: K) =>
  actionRequest(kind, connectionTarget(id), {});

export const abortConnection = (id: string) =>
  client.post<void>(
    `/api/v1/connections/${id}/_abort`,
    emptyConnectionAction(id, CONNECTION_SETUP_ABORT_KIND),
  );

export const getSetupStep = (connectionId: string, returnToUrl?: string) =>
  client.get<ConnectionSetupResponse>(
    `/api/v1/connections/${connectionId}/_setupStep`,
    returnToUrl ? { params: { returnToUrl } } : undefined,
  );

export const getDataSource = (connectionId: string, sourceId: string) =>
  client.get<DataSourceOption[]>(
    `/api/v1/connections/${connectionId}/_dataSource/${sourceId}`,
  );

export const reconfigureConnection = (id: string) =>
  client.post<ConnectionSetupResponse>(
    `/api/v1/connections/${id}/_reconfigure`,
    emptyConnectionAction(id, CONNECTION_RECONFIGURE_KIND),
  );

export const cancelSetupConnection = (id: string) =>
  client.post<void>(
    `/api/v1/connections/${id}/_cancelSetup`,
    emptyConnectionAction(id, CONNECTION_SETUP_CANCEL_KIND),
  );

export const retryConnection = (id: string, spec: ConnectionSetupControlSpec = {}) => {
  const request: ConnectionSetupRetryRequest = actionRequest(
    CONNECTION_SETUP_RETRY_KIND,
    connectionTarget(id),
    spec,
  );
  return client.post<ConnectionSetupResponse>(`/api/v1/connections/${id}/_retry`, request);
};

export const reauthConnection = (id: string, spec: ConnectionSetupControlSpec = {}) => {
  const request: ConnectionReauthenticateRequest = actionRequest(
    CONNECTION_REAUTHENTICATE_KIND,
    connectionTarget(id),
    spec,
  );
  return client.post<ConnectionSetupResponse>(`/api/v1/connections/${id}/_reauth`, request);
};

export const connections = {
  list: listConnections,
  get: getConnection,
  initiate: initiateConnection,
  submit: submitConnection,
  disconnect: disconnectConnection,
  migrateVersion: migrateConnectionVersion,
  abort: abortConnection,
  forceState: forceConnectionState,
  update: updateConnection,
  getSetupStep,
  getDataSource,
  reconfigure: reconfigureConnection,
  cancelSetup: cancelSetupConnection,
  retry: retryConnection,
  reauth: reauthConnection,
  getLabels: getConnectionLabels,
  getLabel: getConnectionLabel,
  putLabel: putConnectionLabel,
  deleteLabel: deleteConnectionLabel,
  getAnnotations: getConnectionAnnotations,
  getAnnotation: getConnectionAnnotation,
  putAnnotation: putConnectionAnnotation,
  deleteAnnotation: deleteConnectionAnnotation,
};
