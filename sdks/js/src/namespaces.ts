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

export interface UpdateNamespaceRequest {
    labels?: Record<string, string>;
}

export interface PutNamespaceLabelRequest {
    value: string;
}

export interface NamespaceLabel {
    key: string;
    value: string;
}

export interface Namespace {
  path: string;
  state: NamespaceState;
  labels?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface CreateNamespaceRequest {
    path: string;
    labels?: Record<string, string>;
}

/**
 * Parameters used for listing namespaces.
 */
export interface ListNamespaceParams {
  state?: NamespaceState;
  namespace?: string;
  label_selector?: string;
  cursor?: string;
  limit?: number;
  order_by?: string;
  children_of?: string;
}

/**
 * Returns a matcher that will match for the specified namespace path and all its children. This value can be used
 * in the namespace filter param for listing resources. If no path is specified, the matcher will match for all namespaces.
 */
export const namespaceAndChildren = (path: string | null | undefined): string => {
    if( !path ) {
        return ROOT_NAMESPACE_PATH + NAMESPACE_PATH_SEPARATOR +  "**";
    }

    if (path.endsWith("**")) {
        return path;
    } else {
        return path + NAMESPACE_PATH_SEPARATOR + "**";
    }
}

/**
 * Get a list of all namespaces
 * @param params The parameters for filtering and pagination
 */
export const listNamespaces = (params: ListNamespaceParams) => {
  return client.get<ListResponse<Namespace>>('/api/v1/namespaces', { params });
};

/**
 * Create a new namespace
 * @param request The namespace to create
 */
export const createNamespace = (request: CreateNamespaceRequest) => {
    return client.post<Namespace>('/api/v1/namespaces', request);
};

/**
 * Get a specific namespace by path
 */
export const getNamespaceByPath = (path: string) => {
  return client.get<Namespace>(`/api/v1/namespaces/${path}`);
};

/**
 * Update a namespace's labels
 */
export const updateNamespace = (path: string, request: UpdateNamespaceRequest) => {
  return client.patch<Namespace>(`/api/v1/namespaces/${path}`, request);
};

/**
 * Get all labels for a specific namespace by path
 */
export const getNamespaceLabels = (path: string) => {
  return client.get<Record<string, string>>(`/api/v1/namespaces/${path}/labels`);
};

/**
 * Get a specific label for a namespace by path and label key
 */
export const getNamespaceLabel = (path: string, labelKey: string) => {
  return client.get<NamespaceLabel>(`/api/v1/namespaces/${path}/labels/${labelKey}`);
};

/**
 * Set a specific label for a namespace by path and label key
 */
export const putNamespaceLabel = (path: string, labelKey: string, value: string) => {
  return client.put<NamespaceLabel>(`/api/v1/namespaces/${path}/labels/${labelKey}`, { value });
};

/**
 * Delete a specific label for a namespace by path and label key
 */
export const deleteNamespaceLabel = (path: string, labelKey: string) => {
  return client.delete(`/api/v1/namespaces/${path}/labels/${labelKey}`);
};

export const namespaces = {
  list: listNamespaces,
  create: createNamespace,
  getByPath: getNamespaceByPath,
  update: updateNamespace,
  getLabels: getNamespaceLabels,
  getLabel: getNamespaceLabel,
  putLabel: putNamespaceLabel,
  deleteLabel: deleteNamespaceLabel,
};
