import { client } from './client';
import { ListResponse } from './common';

// Key models

export enum KeyState {
    ACTIVE = 'active',
    DISABLED = 'disabled',
}

export interface Key {
  id: string;
  namespace: string;
  state: KeyState;
  key_data?: KeyData;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export type KeyData = Record<string, unknown>;

export interface CreateKeyRequest {
    namespace: string;
    key_data?: KeyData;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
}

export interface UpdateKeyRequest {
    state?: KeyState;
    key_data?: KeyData;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
}

export interface ListKeysParams {
  cursor?: string;
  limit?: number;
  state?: KeyState;
  namespace?: string;
  label_selector?: string;
  order_by?: string;
}

export interface KeyLabel {
  key: string;
  value: string;
}

export interface PutKeyLabelRequest {
  value: string;
}

export interface PutKeyAnnotationRequest {
  value: string;
}

export interface KeyAnnotation {
  key: string;
  value: string;
}

/**
 * List keys with optional filtering and pagination
 */
export const listKeys = (params: ListKeysParams) => {
  return client.get<ListResponse<Key>>('/api/v1/keys', { params });
};

/**
 * Create a new key
 */
export const createKey = (request: CreateKeyRequest) => {
    return client.post<Key>('/api/v1/keys', request);
};

/**
 * Get a specific key by ID
 */
export const getKey = (id: string) => {
  return client.get<Key>(`/api/v1/keys/${id}`);
};

/**
 * Update a key's state and/or labels
 */
export const updateKey = (id: string, request: UpdateKeyRequest) => {
  return client.patch<Key>(`/api/v1/keys/${id}`, request);
};

/**
 * Delete a key (soft delete)
 */
export const deleteKey = (id: string) => {
  return client.delete(`/api/v1/keys/${id}`);
};

/**
 * Get all labels for a specific key
 */
export const getKeyLabels = (id: string) => {
  return client.get<Record<string, string>>(`/api/v1/keys/${id}/labels`);
};

/**
 * Get a specific label for a key
 */
export const getKeyLabel = (id: string, labelKey: string) => {
  return client.get<KeyLabel>(`/api/v1/keys/${id}/labels/${labelKey}`);
};

/**
 * Set a specific label for a key
 */
export const putKeyLabel = (id: string, labelKey: string, value: string) => {
  return client.put<KeyLabel>(`/api/v1/keys/${id}/labels/${labelKey}`, { value });
};

/**
 * Delete a specific label from a key
 */
export const deleteKeyLabel = (id: string, labelKey: string) => {
  return client.delete(`/api/v1/keys/${id}/labels/${labelKey}`);
};

/**
 * Get all annotations for a specific key
 */
export const getKeyAnnotations = (id: string) => {
  return client.get<Record<string, string>>(`/api/v1/keys/${id}/annotations`);
};

/**
 * Get a specific annotation for a key
 */
export const getKeyAnnotation = (id: string, annotationKey: string) => {
  return client.get<KeyAnnotation>(`/api/v1/keys/${id}/annotations/${annotationKey}`);
};

/**
 * Set a specific annotation for a key
 */
export const putKeyAnnotation = (id: string, annotationKey: string, value: string) => {
  return client.put<KeyAnnotation>(`/api/v1/keys/${id}/annotations/${annotationKey}`, { value });
};

/**
 * Delete a specific annotation from a key
 */
export const deleteKeyAnnotation = (id: string, annotationKey: string) => {
  return client.delete(`/api/v1/keys/${id}/annotations/${annotationKey}`);
};

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
