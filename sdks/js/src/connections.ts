import {client} from './client';
import {Connector} from './connectors';
import {ListResponse} from './common';

// Connection models
//
// SETUP: connection is persisted and one or more setup steps are in progress.
// CONFIGURED: setup is complete. The connection may or may not be currently
// usable; the orthogonal ConnectionHealthState axis carries that signal.
export enum ConnectionState {
    SETUP = 'setup',
    CONFIGURED = 'configured',
    DISABLED = 'disabled',
    DISCONNECTING = 'disconnecting',
    DISCONNECTED = 'disconnected',
}

// Operational health signal for a connection. Distinct from ConnectionState:
// a Configured connection whose credentials have stopped working flips to
// UNHEALTHY without leaving the Configured lifecycle state. UIs surface this
// to drive the unified re-authentication action.
export enum ConnectionHealthState {
    HEALTHY = 'healthy',
    UNHEALTHY = 'unhealthy',
}

export interface UpdateConnectionRequest {
    name?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
}

export interface PutConnectionLabelRequest {
    value: string;
}

export interface ConnectionLabel {
    key: string;
    value: string;
}

export interface PutConnectionAnnotationRequest {
    value: string;
}

export interface ConnectionAnnotation {
    key: string;
    value: string;
}

export interface Connection {
    id: string;
    name: string;
    namespace: string;
    connector: Connector;
    state: ConnectionState;
    healthState: ConnectionHealthState;
    setupStepId?: string;
    setupError?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    createdAt: string;
    updatedAt: string;
}

export function canBeDisconnected(connection: Connection): boolean {
    return (
        connection.state !== ConnectionState.DISCONNECTING &&
        connection.state !== ConnectionState.DISCONNECTED
    );
}

// Request models
export interface InitiateConnectionRequest {
    connectorId: string;
    name?: string;
    returnToUrl: string;
    labels?: Record<string, string>;
}

export enum ConnectionSetupResponseType {
    REDIRECT = 'redirect',
    FORM = 'form',
    COMPLETE = 'complete',
    VERIFYING = 'verifying',
    ERROR = 'error',
}

export interface ConnectionSetupResponse {
    id: string;
    type: ConnectionSetupResponseType;
}

export interface ConnectionSetupRedirectResponse extends ConnectionSetupResponse {
    type: ConnectionSetupResponseType.REDIRECT;
    redirectUrl: string;
}

export interface ConnectionSetupFormResponse extends ConnectionSetupResponse {
    type: ConnectionSetupResponseType.FORM;
    stepId: string;
    stepTitle?: string;
    stepDescription?: string;
    jsonSchema: Record<string, unknown>;
    uiSchema: Record<string, unknown>;
}

export interface ConnectionSetupCompleteResponse extends ConnectionSetupResponse {
    type: ConnectionSetupResponseType.COMPLETE;
}

export interface ConnectionSetupVerifyingResponse extends ConnectionSetupResponse {
    type: ConnectionSetupResponseType.VERIFYING;
}

export interface ConnectionSetupErrorResponse extends ConnectionSetupResponse {
    type: ConnectionSetupResponseType.ERROR;
    error: string;
    canRetry: boolean;
}

export function isRedirectResponse(response: ConnectionSetupResponse): response is ConnectionSetupRedirectResponse {
    return response.type === ConnectionSetupResponseType.REDIRECT;
}

export function isFormResponse(response: ConnectionSetupResponse): response is ConnectionSetupFormResponse {
    return response.type === ConnectionSetupResponseType.FORM;
}

export function isCompleteResponse(response: ConnectionSetupResponse): response is ConnectionSetupCompleteResponse {
    return response.type === ConnectionSetupResponseType.COMPLETE;
}

export function isVerifyingResponse(response: ConnectionSetupResponse): response is ConnectionSetupVerifyingResponse {
    return response.type === ConnectionSetupResponseType.VERIFYING;
}

export function isErrorResponse(response: ConnectionSetupResponse): response is ConnectionSetupErrorResponse {
    return response.type === ConnectionSetupResponseType.ERROR;
}

export interface SubmitConnectionRequest {
    stepId: string;
    data: unknown;
    returnToUrl?: string;
}

export interface RetryConnectionRequest {
    returnToUrl?: string;
}

export interface ReauthConnectionRequest {
    returnToUrl?: string;
}

export interface DataSourceOption {
    value: string;
    label: string;
}

// Disconnect models
export interface DisconnectConnectionRequest {
    timeoutSeconds?: number;
}

export interface DisconnectResponseJson {
    taskId: string;
    connection: Connection;
}

export interface MigrateConnectionVersionRequest {
    targetVersion: number;
    timeoutSeconds?: number;
}

export interface MigrateConnectionVersionResponseJson {
    taskId: string;
    connectionId: string;
    sourceVersion: number;
    targetVersion: number;
}

export interface ForceConnectionStateRequest {
    state: ConnectionState;
}

export type ForceConnectionStateResponse = Connection;

/**
 * Parameters used for listing connections.
 */
export interface ListConnectionsParams {
    name?: string;
    state?: ConnectionState;
    namespace?: string;
    labelSelector?: string;
    cursor?: string;
    limit?: number;
    orderBy?: string;
}

/**
 * Get a list of all connections
 */
export const listConnections = (params: ListConnectionsParams) => {
    return client.get<ListResponse<Connection>>('/api/v1/connections', {params});
};

/**
 * Get a specific connection by ID
 */
export const getConnection = (id: string) => {
    return client.get<Connection>(`/api/v1/connections/${id}`);
};

/**
 * Initiate a new connection
 */
export const initiateConnection = (
    connectorId: string,
    returnToUrl: string,
    labels?: Record<string, string>,
    name?: string,
) => {
    const request: InitiateConnectionRequest = {
        connectorId: connectorId,
        name,
        returnToUrl: returnToUrl,
        labels,
    };

    return client.post<ConnectionSetupResponse>(
        '/api/v1/connections/_initiate',
        request
    );
};

/**
 * Submit form data for a connection setup step
 */
export const submitConnection = (
    connectionId: string,
    stepId: string,
    data: unknown,
    returnToUrl?: string
) => {
    const request: SubmitConnectionRequest = {
        stepId: stepId,
        data,
        returnToUrl: returnToUrl,
    };

    return client.post<ConnectionSetupResponse>(
        `/api/v1/connections/${connectionId}/_submit`,
        request
    );
};

/**
 * Disconnect a connection
 */
export const disconnectConnection = (id: string, request?: DisconnectConnectionRequest) => {
    return client.post<DisconnectResponseJson>(
        `/api/v1/connections/${id}/_disconnect`,
        request
    );
};

/**
 * Migrate a connection to another version of the same connector.
 */
export const migrateConnectionVersion = (id: string, request: MigrateConnectionVersionRequest) => {
    return client.post<MigrateConnectionVersionResponseJson>(
        `/api/v1/connections/${id}/_migrateVersion`,
        request
    );
};

/**
 * Force the state of a connection. Requires admin permissions.
 */
export const forceConnectionState = (id: string, state: ConnectionState) => {
    const request: ForceConnectionStateRequest = {
        state: state,
    };
    return client.put<ForceConnectionStateResponse>(
        `/api/v1/connections/${id}/_forceState`,
        request
    );
};

/**
 * Update a connection's labels
 */
export const updateConnection = (id: string, request: UpdateConnectionRequest) => {
    return client.patch<Connection>(`/api/v1/connections/${id}`, request);
};

/**
 * Get all labels for a specific connection by ID (uuid)
 */
export const getConnectionLabels = (id: string) => {
    return client.get<Record<string, string>>(`/api/v1/connections/${id}/labels`);
};

/**
 * Get a specific label for a connection by ID (uuid) and label key
 */
export const getConnectionLabel = (id: string, labelKey: string) => {
    return client.get<ConnectionLabel>(`/api/v1/connections/${id}/labels/${labelKey}`);
};

/**
 * Set a specific label for a connection by ID (uuid) and label key
 */
export const putConnectionLabel = (id: string, labelKey: string, value: string) => {
    return client.put<ConnectionLabel>(`/api/v1/connections/${id}/labels/${labelKey}`, { value });
};

/**
 * Delete a specific label for a connection by ID (uuid) and label key
 */
export const deleteConnectionLabel = (id: string, labelKey: string) => {
    return client.delete(`/api/v1/connections/${id}/labels/${labelKey}`);
};

/**
 * Get all annotations for a specific connection by ID (uuid)
 */
export const getConnectionAnnotations = (id: string) => {
    return client.get<Record<string, string>>(`/api/v1/connections/${id}/annotations`);
};

/**
 * Get a specific annotation for a connection by ID (uuid) and annotation key
 */
export const getConnectionAnnotation = (id: string, annotationKey: string) => {
    return client.get<ConnectionAnnotation>(`/api/v1/connections/${id}/annotations/${annotationKey}`);
};

/**
 * Set a specific annotation for a connection by ID (uuid) and annotation key
 */
export const putConnectionAnnotation = (id: string, annotationKey: string, value: string) => {
    return client.put<ConnectionAnnotation>(`/api/v1/connections/${id}/annotations/${annotationKey}`, { value });
};

/**
 * Delete a specific annotation for a connection by ID (uuid) and annotation key
 */
export const deleteConnectionAnnotation = (id: string, annotationKey: string) => {
    return client.delete(`/api/v1/connections/${id}/annotations/${annotationKey}`);
};

/**
 * Abort a connection that is still in setup
 */
export const abortConnection = (id: string) => {
    return client.post<void>(`/api/v1/connections/${id}/_abort`);
};

/**
 * Get the current setup step for a connection
 */
export const getSetupStep = (connectionId: string, returnToUrl?: string) => {
    return client.get<ConnectionSetupResponse>(
        `/api/v1/connections/${connectionId}/_setupStep`,
        returnToUrl ? {params: {returnToUrl: returnToUrl}} : undefined
    );
};

/**
 * Get data source options for a connection setup step
 */
export const getDataSource = (connectionId: string, sourceId: string) => {
    return client.get<DataSourceOption[]>(`/api/v1/connections/${connectionId}/_dataSource/${sourceId}`);
};

/**
 * Reconfigure a completed connection by restarting its configure phase
 */
export const reconfigureConnection = (id: string) => {
    return client.post<ConnectionSetupResponse>(`/api/v1/connections/${id}/_reconfigure`);
};

/**
 * Cancel an in-flight reconfigure on a ready connection by clearing setup_step_id and setup_error.
 * The connection remains ready and its previously stored configuration continues to apply.
 */
export const cancelSetupConnection = (id: string) => {
    return client.post<void>(`/api/v1/connections/${id}/_cancelSetup`);
};

/**
 * Retry a connection that failed during setup, including auth-phase failures and probe
 * verification failures. For connectors with preconnect steps, returns to preconnect:0 so the
 * user can correct inputs. For connectors without preconnect steps, re-initiates OAuth
 * (returnToUrl is required in that case).
 */
export const retryConnection = (id: string, returnToUrl?: string) => {
    const request: RetryConnectionRequest = { returnToUrl: returnToUrl };
    return client.post<ConnectionSetupResponse>(
        `/api/v1/connections/${id}/_retry`,
        request
    );
};

/**
 * Re-authenticate a Ready connection. Used both for user-driven credential rotation and as the
 * recovery action when a connection has flipped to unhealthy. For api-key connectors, returns the
 * credentials form (no prior values pre-filled); on submit the existing credential is rotated
 * atomically. For OAuth2 connectors, restarts at preconnect:0 if defined, otherwise re-initiates
 * the OAuth redirect (returnToUrl is required in that case).
 */
export const reauthConnection = (id: string, returnToUrl?: string) => {
    const request: ReauthConnectionRequest = { returnToUrl: returnToUrl };
    return client.post<ConnectionSetupResponse>(
        `/api/v1/connections/${id}/_reauth`,
        request
    );
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
    getSetupStep: getSetupStep,
    getDataSource: getDataSource,
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
