import {client} from './client';
import {Connector} from './connectors';
import {ListResponse} from './common';

// Connection models
export enum ConnectionState {
    CREATED = 'created',
    READY = 'ready',
    DISABLED = 'disabled',
    DISCONNECTING = 'disconnecting',
    DISCONNECTED = 'disconnected',
}

export interface UpdateConnectionRequest {
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
    namespace: string;
    connector: Connector;
    state: ConnectionState;
    setup_step?: string;
    setup_error?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    created_at: string;
    updated_at: string;
}

export function canBeDisconnected(connection: Connection): boolean {
    return (
        connection.state !== ConnectionState.DISCONNECTING &&
        connection.state !== ConnectionState.DISCONNECTED
    );
}

// Request models
export interface InitiateConnectionRequest {
    connector_id: string;
    return_to_url: string;
    labels?: Record<string, string>;
}

export enum InitiateConnectionResponseType {
    REDIRECT = 'redirect',
    FORM = 'form',
    COMPLETE = 'complete',
    VERIFYING = 'verifying',
    ERROR = 'error',
}

export interface InitiateConnectionResponse {
    id: string;
    type: InitiateConnectionResponseType;
}

export interface InitiateConnectionRedirectResponse extends InitiateConnectionResponse {
    type: InitiateConnectionResponseType.REDIRECT;
    redirect_url: string;
}

export interface InitiateConnectionFormResponse extends InitiateConnectionResponse {
    type: InitiateConnectionResponseType.FORM;
    step_id: string;
    step_title?: string;
    step_description?: string;
    current_step: number;
    total_steps: number;
    json_schema: Record<string, unknown>;
    ui_schema: Record<string, unknown>;
}

export interface InitiateConnectionCompleteResponse extends InitiateConnectionResponse {
    type: InitiateConnectionResponseType.COMPLETE;
}

export interface InitiateConnectionVerifyingResponse extends InitiateConnectionResponse {
    type: InitiateConnectionResponseType.VERIFYING;
}

export interface InitiateConnectionErrorResponse extends InitiateConnectionResponse {
    type: InitiateConnectionResponseType.ERROR;
    error: string;
    can_retry: boolean;
}

export function isRedirectResponse(response: InitiateConnectionResponse): response is InitiateConnectionRedirectResponse {
    return response.type === InitiateConnectionResponseType.REDIRECT;
}

export function isFormResponse(response: InitiateConnectionResponse): response is InitiateConnectionFormResponse {
    return response.type === InitiateConnectionResponseType.FORM;
}

export function isCompleteResponse(response: InitiateConnectionResponse): response is InitiateConnectionCompleteResponse {
    return response.type === InitiateConnectionResponseType.COMPLETE;
}

export function isVerifyingResponse(response: InitiateConnectionResponse): response is InitiateConnectionVerifyingResponse {
    return response.type === InitiateConnectionResponseType.VERIFYING;
}

export function isErrorResponse(response: InitiateConnectionResponse): response is InitiateConnectionErrorResponse {
    return response.type === InitiateConnectionResponseType.ERROR;
}

export interface SubmitConnectionRequest {
    step_id: string;
    data: unknown;
}

export interface RetryConnectionRequest {
    return_to_url?: string;
}

export interface DataSourceOption {
    value: string;
    label: string;
}

// Disconnect models
export interface DisconnectResponseJson {
    task_id: string;
    connection: Connection;
}

export interface ForceConnectionStateRequest {
    state: ConnectionState;
}

export type ForceConnectionStateResponse = Connection;

/**
 * Parameters used for listing connections.
 */
export interface ListConnectionsParams {
    state?: ConnectionState;
    namespace?: string;
    label_selector?: string;
    cursor?: string;
    limit?: number;
    order_by?: string;
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
    labels?: Record<string, string>
) => {
    const request: InitiateConnectionRequest = {
        connector_id: connectorId,
        return_to_url: returnToUrl,
        labels,
    };

    return client.post<InitiateConnectionResponse>(
        '/api/v1/connections/_initiate',
        request
    );
};

/**
 * Submit form data for a connection setup step
 */
export const submitConnection = (connectionId: string, stepId: string, data: unknown) => {
    const request: SubmitConnectionRequest = { step_id: stepId, data };

    return client.post<InitiateConnectionResponse>(
        `/api/v1/connections/${connectionId}/_submit`,
        request
    );
};

/**
 * Disconnect a connection
 */
export const disconnectConnection = (id: string) => {
    return client.post<DisconnectResponseJson>(`/api/v1/connections/${id}/_disconnect`);
};

/**
 * Force the state of a connection. Requires admin permissions.
 */
export const forceConnectionState = (id: string, state: ConnectionState) => {
    const request: ForceConnectionStateRequest = {
        state: state,
    };
    return client.put<ForceConnectionStateResponse>(
        `/api/v1/connections/${id}/_force_state`,
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
export const getSetupStep = (connectionId: string) => {
    return client.get<InitiateConnectionResponse>(`/api/v1/connections/${connectionId}/_setup_step`);
};

/**
 * Get data source options for a connection setup step
 */
export const getDataSource = (connectionId: string, sourceId: string) => {
    return client.get<DataSourceOption[]>(`/api/v1/connections/${connectionId}/_data_source/${sourceId}`);
};

/**
 * Reconfigure a completed connection by restarting its configure phase
 */
export const reconfigureConnection = (id: string) => {
    return client.post<InitiateConnectionResponse>(`/api/v1/connections/${id}/_reconfigure`);
};

/**
 * Cancel an in-flight reconfigure on a ready connection by clearing setup_step and setup_error.
 * The connection remains ready and its previously stored configuration continues to apply.
 */
export const cancelSetupConnection = (id: string) => {
    return client.post<void>(`/api/v1/connections/${id}/_cancel_setup`);
};

/**
 * Retry a connection that failed during probe verification. For connectors with preconnect steps,
 * returns to preconnect:0 so the user can correct inputs. For connectors without preconnect steps,
 * re-initiates OAuth (return_to_url is required in that case).
 */
export const retryConnection = (id: string, returnToUrl?: string) => {
    const request: RetryConnectionRequest = { return_to_url: returnToUrl };
    return client.post<InitiateConnectionResponse>(
        `/api/v1/connections/${id}/_retry`,
        request
    );
};

export const connections = {
    list: listConnections,
    get: getConnection,
    initiate: initiateConnection,
    submit: submitConnection,
    disconnect: disconnectConnection,
    abort: abortConnection,
    force_state: forceConnectionState,
    update: updateConnection,
    getSetupStep: getSetupStep,
    getDataSource: getDataSource,
    reconfigure: reconfigureConnection,
    cancelSetup: cancelSetupConnection,
    retry: retryConnection,
    getLabels: getConnectionLabels,
    getLabel: getConnectionLabel,
    putLabel: putConnectionLabel,
    deleteLabel: deleteConnectionLabel,
    getAnnotations: getConnectionAnnotations,
    getAnnotation: getConnectionAnnotation,
    putAnnotation: putConnectionAnnotation,
    deleteAnnotation: deleteConnectionAnnotation,
};
