import { client } from './client';
import {
  MutableResourceMetadata,
  NamespacedCreateMetadata,
  ObjectMetadata,
  ResourceList,
  TypeMeta,
} from './common';

export const KEY_KIND = 'Key' as const;

export enum KeyUsage {
  DATA_ENCRYPTION = 'data_encryption',
}

export enum KeyMaterialType {
  SYMMETRIC = 'symmetric',
  PUBLIC = 'public',
  PRIVATE = 'private',
  EXTERNAL = 'external',
}

export enum KeyState {
  ACTIVE = 'active',
  DISABLED = 'disabled',
}

export type KeyData = Record<string, unknown>;

export interface KeyMetadata extends ObjectMetadata {
  id: string;
  name: string;
  namespace: string;
  createdAt: string;
  updatedAt: string;
}

export interface KeySpec {
  usage: KeyUsage;
  materialType: KeyMaterialType;
  desiredState: KeyState;
  /** Provider configuration is always returned with secret fields redacted. */
  keyData?: KeyData;
}

export interface KeyStatus {
  state: KeyState;
  keyDataConfigured: boolean;
}

export interface Key extends TypeMeta<typeof KEY_KIND> {
  metadata: KeyMetadata;
  spec: KeySpec;
  status: KeyStatus;
}

export interface CreateKeyRequest extends TypeMeta<typeof KEY_KIND> {
  metadata: NamespacedCreateMetadata;
  spec: {
    usage?: KeyUsage;
    materialType?: KeyMaterialType;
    desiredState?: KeyState;
    /** Write-only provider configuration; responses contain only redacted values. */
    keyData: KeyData;
  };
}

export interface UpdateKeyRequest extends TypeMeta<typeof KEY_KIND> {
  metadata: MutableResourceMetadata;
  spec: {
    usage?: KeyUsage;
    materialType?: KeyMaterialType;
    desiredState?: KeyState;
    keyData?: KeyData;
  };
}

export type KeyList = ResourceList<Key>;

export interface ListKeysParams {
  name?: string;
  cursor?: string;
  limit?: number;
  state?: KeyState;
  namespace?: string;
  labelSelector?: string;
  orderBy?: string;
}

export interface KeyLabel {
  key: string;
  value: string;
}

export interface KeyAnnotation {
  key: string;
  value: string;
}

export const listKeys = (params?: ListKeysParams) =>
  client.get<KeyList>('/api/v1/keys', { params });

export const createKey = (request: CreateKeyRequest) =>
  client.post<Key>('/api/v1/keys', request);

export const getKey = (id: string) => client.get<Key>(`/api/v1/keys/${id}`);

export const updateKey = (id: string, request: UpdateKeyRequest) =>
  client.patch<Key>(`/api/v1/keys/${id}`, request);

export const deleteKey = (id: string) => client.delete(`/api/v1/keys/${id}`);

export const getKeyLabels = (id: string) =>
  client.get<Record<string, string>>(`/api/v1/keys/${id}/labels`);

export const getKeyLabel = (id: string, labelKey: string) =>
  client.get<KeyLabel>(`/api/v1/keys/${id}/labels/${labelKey}`);

export const putKeyLabel = (id: string, labelKey: string, value: string) =>
  client.put<KeyLabel>(`/api/v1/keys/${id}/labels/${labelKey}`, { value });

export const deleteKeyLabel = (id: string, labelKey: string) =>
  client.delete(`/api/v1/keys/${id}/labels/${labelKey}`);

export const getKeyAnnotations = (id: string) =>
  client.get<Record<string, string>>(`/api/v1/keys/${id}/annotations`);

export const getKeyAnnotation = (id: string, annotationKey: string) =>
  client.get<KeyAnnotation>(`/api/v1/keys/${id}/annotations/${annotationKey}`);

export const putKeyAnnotation = (id: string, annotationKey: string, value: string) =>
  client.put<KeyAnnotation>(`/api/v1/keys/${id}/annotations/${annotationKey}`, { value });

export const deleteKeyAnnotation = (id: string, annotationKey: string) =>
  client.delete(`/api/v1/keys/${id}/annotations/${annotationKey}`);

export const keys = {
  list: listKeys,
  create: createKey,
  get: getKey,
  update: updateKey,
  delete: deleteKey,
  getLabels: getKeyLabels,
  getLabel: getKeyLabel,
  putLabel: putKeyLabel,
  deleteLabel: deleteKeyLabel,
  getAnnotations: getKeyAnnotations,
  getAnnotation: getKeyAnnotation,
  putAnnotation: putKeyAnnotation,
  deleteAnnotation: deleteKeyAnnotation,
};
