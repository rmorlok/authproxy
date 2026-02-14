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
}

export interface PutConnectionLabelRequest {
    value: string;
}

export interface ConnectionLabel {
    key: string;
    value: string;
}

export interface Connection {
    id: string;
    namespace: string;
    connector: Connector;
    state: ConnectionState;
    labels?: Record<string, string>;
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
}

export interface InitiateConnectionResponse {
    id: string;
    type: InitiateConnectionResponseType;
}

export interface InitiateConnectionRedirectResponse extends InitiateConnectionResponse {
    redirect_url: string;
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

    return client.post<InitiateConnectionRedirectResponse>(
        '/api/v1/connections/_initiate',
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

export const connections = {
    list: listConnections,
    get: getConnection,
    initiate: initiateConnection,
    disconnect: disconnectConnection,
    force_state: forceConnectionState,
    update: updateConnection,
    getLabels: getConnectionLabels,
    getLabel: getConnectionLabel,
    putLabel: putConnectionLabel,
    deleteLabel: deleteConnectionLabel,
};
