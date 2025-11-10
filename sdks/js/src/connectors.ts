import { client } from './client';
import { ListResponse } from './common';

// Connector models
export interface ConnectorVersion {
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
}

export interface Connector extends ConnectorVersion {
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

/**
 * Get a list of all available connectors
 */
export const listConnectors = (params: ListConnectorsParams) => {
  return client.get<ListResponse<Connector>>('/api/v1/connectors', { params });
};

/**
 * Get a specific connector by ID
 */
export const getConnector = (id: string) => {
  return client.get<Connector>(`/api/v1/connectors/${id}`);
};

export const connectors = {
  list: listConnectors,
  get: getConnector,
};
