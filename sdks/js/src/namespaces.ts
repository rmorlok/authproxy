import { client } from './client';
import { ListResponse } from './common';

// Namespace models

// The predefined root namespace path
export const ROOT_NAMESPACE_PATH = 'root';
export const NAMESPACE_PATH_SEPARATOR = '.';

export enum NamespaceState {
    ACTIVE = 'active',
    DISCONNECTING = 'disconnecting',
    DISCONNECTED = 'disconnected',
}

export interface Namespace {
  path: string;
  state: NamespaceState;
  created_at: string;
  updated_at: string;
}

/**
 * Parameters used for listing namespaces.
 */
export interface ListNamespaceParams {
  state?: NamespaceState;
  cursor?: string;
  limit?: number;
  order_by?: string;
  children_of?: string;
}

/**
 * Get a list of all namespaces
 * @param params The parameters for filtering and pagination
 */
export const listNamespaces = (params: ListNamespaceParams) => {
  return client.get<ListResponse<Namespace>>('/api/v1/namespaces', { params });
};

/**
 * Get a specific namespace by path
 */
export const getNamespaceByPath = (path: string) => {
  return client.get<Namespace>(`/api/v1/namespaces/${path}`);
};

export const namespaces = {
  list: listNamespaces,
  getByPath: getNamespaceByPath,
};
