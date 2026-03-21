import {client} from './client';
import {ListResponse} from './common';

// Connector models
export interface ConnectorVersion {
    id: string;
    version: number;
    namespace: string;
    state: ConnectorVersionState;
    definition: Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    created_at: string;
    updated_at: string;
}

export interface Connector {
    id: string;
    version: number;
    namespace: string;
    state: ConnectorVersionState;
    display_name: string;
    description: string;
    highlight?: string;
    status_page_url?: string;
    logo: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    created_at: string;
    updated_at: string;
    versions: number;
    states: ConnectorVersionState[];
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
    state?: ConnectorVersionState;
    namespace?: string;
    label_selector?: string;
    cursor?: string;
    limit?: number;
    order_by?: string;
}

export interface ListConnectorVersionsParams {
    state?: ConnectorVersionState;
    namespace?: string;
    label_selector?: string;
    cursor?: string;
    limit?: number;
    order_by?: string;
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
        `/api/v1/connectors/${id}/versions/${version}/_force_state`,
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
    listVersions: listConnectorVersions,
    getVersion: getConnectorVersion,
    force_version_state: forceConnectorVersionState,
    getAnnotations: getConnectorAnnotations,
    getAnnotation: getConnectorAnnotation,
    putAnnotation: putConnectorAnnotation,
    deleteAnnotation: deleteConnectorAnnotation,
    getVersionAnnotations: getConnectorVersionAnnotations,
    getVersionAnnotation: getConnectorVersionAnnotation,
    putVersionAnnotation: putConnectorVersionAnnotation,
    deleteVersionAnnotation: deleteConnectorVersionAnnotation,
};
