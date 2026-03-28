import {client} from './client';
import {Connector} from './connectors';
import {ListResponse} from './common';

// Connection models
export enum ConnectionState {
    CREATED = 'created',
    CONNECTED = 'connected',
    FAILED = 'failed',
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
    json_schema: Record<string, unknown>;
    ui_schema: Record<string, unknown>;
}

export interface InitiateConnectionCompleteResponse extends InitiateConnectionResponse {
    type: InitiateConnectionResponseType.COMPLETE;
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

export interface SubmitConnectionRequest {
    data: unknown;
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
export const submitConnection = (connectionId: string, data: unknown) => {
    const request: SubmitConnectionRequest = { data };

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

export const connections = {
    list: listConnections,
    get: getConnection,
    initiate: initiateConnection,
    submit: submitConnection,
    disconnect: disconnectConnection,
    force_state: forceConnectionState,
    update: updateConnection,
    getLabels: getConnectionLabels,
    getLabel: getConnectionLabel,
    putLabel: putConnectionLabel,
    deleteLabel: deleteConnectionLabel,
    getAnnotations: getConnectionAnnotations,
    getAnnotation: getConnectionAnnotation,
    putAnnotation: putConnectionAnnotation,
    deleteAnnotation: deleteConnectionAnnotation,
};
