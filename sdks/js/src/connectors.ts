import {client} from './client';
import {ListResponse} from './common';

// Connector models
export interface ConnectorVersion {
    id: string;
    version: number;
    state: ConnectorVersionState;
    type: string;
    definition: Record<string, any>;
    created_at: string;
    updated_at: string;
}

export interface Connector {
    id: string;
    version: number;
    state: ConnectorVersionState;
    type: string;
    display_name: string;
    description: string;
    highlight?: string;
    logo: string;
    created_at: string;
    updated_at: string;
    versions: number;
    states: ConnectorVersionState[];
}

export enum ConnectorVersionState {
    DRAFT = 'draft',
    PRIMARY = 'primary',
    ACTIVE = 'active',
    ARCHIVED = 'archived',
}

export interface ListConnectorsParams {
    state?: ConnectorVersionState;
    cursor?: string;
    limit?: number;
    order_by?: string;
}

export interface ListConnectorVersionsParams {
    state?: ConnectorVersionState;
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

export const connectors = {
    list: listConnectors,
    get: getConnector,
    listVersions: listConnectorVersions,
    getVersion: getConnectorVersion,
};
