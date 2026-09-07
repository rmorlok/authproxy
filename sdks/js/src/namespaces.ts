import { client } from './client';
import {
  API_VERSION,
  GenerationlessObjectReference,
  ObjectMetadata,
  ResourceList,
  TypeMeta,
} from './common';

export const NAMESPACE_KIND = 'Namespace' as const;
export const ROOT_NAMESPACE_PATH = 'root';
export const NAMESPACE_PATH_SEPARATOR = '.';

export enum NamespaceState {
  ACTIVE = 'active',
  DESTROYING = 'destroying',
  DESTROYED = 'destroyed',
}

export interface NamespaceMetadata extends ObjectMetadata {
  /** Immutable canonical namespace path. */
  id: string;
  /** Final path segment. */
  name: string;
  /** Parent path; omitted only for root. */
  namespace?: string;
  createdAt: string;
  updatedAt: string;
}

export interface NamespaceSpec {
  encryptionKeyRef?: GenerationlessObjectReference<'Key'>;
}

export interface NamespaceStatus {
  state: NamespaceState;
}

export interface Namespace extends TypeMeta<typeof NAMESPACE_KIND> {
  metadata: NamespaceMetadata;
  spec: NamespaceSpec;
  status: NamespaceStatus;
}

export interface CreateNamespaceRequest extends TypeMeta<typeof NAMESPACE_KIND> {
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: NamespaceSpec;
}

export interface UpdateNamespaceRequest extends TypeMeta<typeof NAMESPACE_KIND> {
  metadata: {
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: {
    /** Null clears the key; omission leaves the current value unchanged. */
    encryptionKeyRef?: GenerationlessObjectReference<'Key'> | null;
  };
}

export type NamespaceList = ResourceList<Namespace>;

export interface ListNamespaceParams {
  name?: string;
  state?: NamespaceState;
  namespace?: string;
  labelSelector?: string;
  cursor?: string;
  limit?: number;
  orderBy?: string;
  childrenOf?: string;
}

/** Matches a namespace path and every descendant in list filters. */
export const namespaceAndChildren = (path?: string | null): string => {
  if (!path) {
    return `${ROOT_NAMESPACE_PATH}${NAMESPACE_PATH_SEPARATOR}**`;
  }
  return path.endsWith('**') ? path : `${path}${NAMESPACE_PATH_SEPARATOR}**`;
};

export const listNamespaces = (params?: ListNamespaceParams) =>
  client.get<NamespaceList>('/api/v1/namespaces', { params });

export const createNamespace = (request: CreateNamespaceRequest) =>
  client.post<Namespace>('/api/v1/namespaces', request);

export const getNamespaceByPath = (path: string) =>
  client.get<Namespace>(`/api/v1/namespaces/${path}`);

export const updateNamespace = (path: string, request: UpdateNamespaceRequest) =>
  client.patch<Namespace>(`/api/v1/namespaces/${path}`, request);

/** Returns the Namespace resource whose spec contains the assigned key. */
export const getNamespaceKey = (path: string) =>
  getNamespaceByPath(path);

export const setNamespaceKey = (
  path: string,
  encryptionKeyRef: GenerationlessObjectReference<'Key'>,
) => {
  const request: UpdateNamespaceRequest = {
    apiVersion: API_VERSION,
    kind: NAMESPACE_KIND,
    metadata: {},
    spec: { encryptionKeyRef },
  };
  return updateNamespace(path, request);
};

export const clearNamespaceKey = (path: string) => {
  const request: UpdateNamespaceRequest = {
    apiVersion: API_VERSION,
    kind: NAMESPACE_KIND,
    metadata: {},
    spec: { encryptionKeyRef: null },
  };
  return updateNamespace(path, request);
};

export const namespaces = {
  list: listNamespaces,
  create: createNamespace,
  getByPath: getNamespaceByPath,
  update: updateNamespace,
  getKey: getNamespaceKey,
  setKey: setNamespaceKey,
  clearKey: clearNamespaceKey,
};
