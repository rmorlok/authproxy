import {client} from './client';
import {ListResponse} from './common';

// Connector models
export interface ConnectorVersion {
    id: string;
    name: string;
    version: number;
    namespace: string;
    state: ConnectorVersionState;
    definition: Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    createdAt: string;
    updatedAt: string;
}

export interface Connector {
    id: string;
    name: string;
    version: number;
    namespace: string;
    state: ConnectorVersionState;
    displayName: string;
    description: string;
    highlight?: string;
    statusPageUrl?: string;
    logo: string;
    hasConfigure: boolean;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    createdAt: string;
    updatedAt: string;
}

export interface PutConnectorAnnotationRequest {
    value: string;
}

export interface ConnectorAnnotation {
    key: string;
    value: string;
}

export enum ConnectorVersionState {
    DRAFT = 'draft',
    PRIMARY = 'primary',
    ACTIVE = 'active',
    ARCHIVED = 'archived',
}

export interface ListConnectorsParams {
    name?: string;
    state?: ConnectorVersionState;
    namespace?: string;
    labelSelector?: string;
    cursor?: string;
    limit?: number;
    orderBy?: string;
}

export interface ListConnectorVersionsParams {
    state?: ConnectorVersionState;
    namespace?: string;
    labelSelector?: string;
    cursor?: string;
    limit?: number;
    orderBy?: string;
}

export interface UpdateConnectorRequest {
    name?: string;
}

export interface ConnectorLifecycleRequest {
    timeoutSeconds?: number;
}

export interface ConnectorLifecycleResponse {
    taskId: string;
    connectorId: string;
}

/**
 * Get a list of all available connectors
 */
export const listConnectors = (params: ListConnectorsParams) => {
    return client.get<ListResponse<Connector>>('/api/v1/connectors', {params});
};

/**
 * Get a specific connector by ID
 */
export const getConnector = (id: string) => {
    return client.get<Connector>(`/api/v1/connectors/${id}`);
};

/**
 * Update connector-level fields shared by every definition version.
 */
export const updateConnector = (id: string, request: UpdateConnectorRequest) => {
    return client.patch<Connector>(`/api/v1/connectors/${id}`, request);
};

/**
 * Get versions for a specific connector by ID
 */
export const listConnectorVersions = (
    id: string,
    params: ListConnectorVersionsParams
) => {
    return client.get<ListResponse<ConnectorVersion>>(
        `/api/v1/connectors/${id}/versions`,
        {params}
    );
};

/**
 * Get a specific connector version by ID and version number
 */
export const getConnectorVersion = (id: string, version: number) => {
    return client.get<ConnectorVersion>(`/api/v1/connectors/${id}/versions/${version}`);
};

export interface ForceConnectorVersionStateRequest {
    state: ConnectorVersionState;
}

export type ForceConnectorVersionStateResponse = ConnectorVersion;

/**
 * Force a connector version into a specific state (admin operation)
 */
export const forceConnectorVersionState = (id: string, version: number, state: ConnectorVersionState) => {
    const request: ForceConnectorVersionStateRequest = { state };
    return client.put<ForceConnectorVersionStateResponse>(
        `/api/v1/connectors/${id}/versions/${version}/_forceState`,
        request
    );
};

/**
 * Disconnect all connections for a connector.
 */
export const disconnectAllConnectorConnections = (id: string, request?: ConnectorLifecycleRequest) => {
    return client.post<ConnectorLifecycleResponse>(
        `/api/v1/connectors/${id}/_disconnectAll`,
        request
    );
};

/**
 * Archive a connector after disconnecting its connections.
 */
export const archiveConnector = (id: string, request?: ConnectorLifecycleRequest) => {
    return client.post<ConnectorLifecycleResponse>(
        `/api/v1/connectors/${id}/_archive`,
        request
    );
};

/**
 * Get all annotations for a specific connector
 */
export const getConnectorAnnotations = (id: string) => {
    return client.get<Record<string, string>>(`/api/v1/connectors/${id}/annotations`);
};

/**
 * Get a specific annotation for a connector
 */
export const getConnectorAnnotation = (id: string, annotationKey: string) => {
    return client.get<ConnectorAnnotation>(`/api/v1/connectors/${id}/annotations/${annotationKey}`);
};

/**
 * Set a specific annotation for a connector
 */
export const putConnectorAnnotation = (id: string, annotationKey: string, value: string) => {
    return client.put<ConnectorAnnotation>(`/api/v1/connectors/${id}/annotations/${annotationKey}`, { value });
};

/**
 * Delete a specific annotation from a connector
 */
export const deleteConnectorAnnotation = (id: string, annotationKey: string) => {
    return client.delete(`/api/v1/connectors/${id}/annotations/${annotationKey}`);
};

/**
 * Get all annotations for a specific connector version
 */
export const getConnectorVersionAnnotations = (id: string, version: number) => {
    return client.get<Record<string, string>>(`/api/v1/connectors/${id}/versions/${version}/annotations`);
};

/**
 * Get a specific annotation for a connector version
 */
export const getConnectorVersionAnnotation = (id: string, version: number, annotationKey: string) => {
    return client.get<ConnectorAnnotation>(`/api/v1/connectors/${id}/versions/${version}/annotations/${annotationKey}`);
};

/**
 * Set a specific annotation for a connector version
 */
export const putConnectorVersionAnnotation = (id: string, version: number, annotationKey: string, value: string) => {
    return client.put<ConnectorAnnotation>(`/api/v1/connectors/${id}/versions/${version}/annotations/${annotationKey}`, { value });
};

/**
 * Delete a specific annotation from a connector version
 */
export const deleteConnectorVersionAnnotation = (id: string, version: number, annotationKey: string) => {
    return client.delete(`/api/v1/connectors/${id}/versions/${version}/annotations/${annotationKey}`);
};

export const connectors = {
    list: listConnectors,
    get: getConnector,
    update: updateConnector,
    listVersions: listConnectorVersions,
    getVersion: getConnectorVersion,
    forceVersionState: forceConnectorVersionState,
    disconnectAll: disconnectAllConnectorConnections,
    archive: archiveConnector,
    getAnnotations: getConnectorAnnotations,
    getAnnotation: getConnectorAnnotation,
    putAnnotation: putConnectorAnnotation,
    deleteAnnotation: deleteConnectorAnnotation,
    getVersionAnnotations: getConnectorVersionAnnotations,
    getVersionAnnotation: getConnectorVersionAnnotation,
    putVersionAnnotation: putConnectorVersionAnnotation,
    deleteVersionAnnotation: deleteConnectorVersionAnnotation,
};
