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

export const getNamespaceLabels = (path: string) =>
  client.get<Record<string, string>>(`/api/v1/namespaces/${path}/labels`);

export const getNamespaceLabel = (path: string, labelKey: string) =>
  client.get<{ key: string; value: string }>(`/api/v1/namespaces/${path}/labels/${labelKey}`);

export const putNamespaceLabel = (path: string, labelKey: string, value: string) =>
  client.put<{ key: string; value: string }>(`/api/v1/namespaces/${path}/labels/${labelKey}`, {
    value,
  });

export const deleteNamespaceLabel = (path: string, labelKey: string) =>
  client.delete(`/api/v1/namespaces/${path}/labels/${labelKey}`);

/** Returns the Namespace resource whose spec contains the assigned key. */
export const getNamespaceKey = (path: string) =>
  client.get<Namespace>(`/api/v1/namespaces/${path}/key`);

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
  return client.put<Namespace>(`/api/v1/namespaces/${path}/key`, request);
};

export const clearNamespaceKey = (path: string) =>
  client.delete(`/api/v1/namespaces/${path}/key`);

export const getNamespaceAnnotations = (path: string) =>
  client.get<Record<string, string>>(`/api/v1/namespaces/${path}/annotations`);

export const getNamespaceAnnotation = (path: string, annotationKey: string) =>
  client.get<{ key: string; value: string }>(
    `/api/v1/namespaces/${path}/annotations/${annotationKey}`,
  );

export const putNamespaceAnnotation = (path: string, annotationKey: string, value: string) =>
  client.put<{ key: string; value: string }>(
    `/api/v1/namespaces/${path}/annotations/${annotationKey}`,
    { value },
  );

export const deleteNamespaceAnnotation = (path: string, annotationKey: string) =>
  client.delete(`/api/v1/namespaces/${path}/annotations/${annotationKey}`);

export const namespaces = {
  list: listNamespaces,
  create: createNamespace,
  getByPath: getNamespaceByPath,
  update: updateNamespace,
  getLabels: getNamespaceLabels,
  getLabel: getNamespaceLabel,
  putLabel: putNamespaceLabel,
  deleteLabel: deleteNamespaceLabel,
  getAnnotations: getNamespaceAnnotations,
  getAnnotation: getNamespaceAnnotation,
  putAnnotation: putNamespaceAnnotation,
  deleteAnnotation: deleteNamespaceAnnotation,
  getKey: getNamespaceKey,
  setKey: setNamespaceKey,
  clearKey: clearNamespaceKey,
};
