import { client } from './client';
import { ListResponse } from './common';

// Encryption Key models

export enum EncryptionKeyState {
    ACTIVE = 'active',
    DISABLED = 'disabled',
}

export interface EncryptionKey {
  id: string;
  namespace: string;
  state: EncryptionKeyState;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface CreateEncryptionKeyRequest {
    namespace: string;
    key_data?: Record<string, unknown>;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
}

export interface UpdateEncryptionKeyRequest {
    state?: EncryptionKeyState;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
}

export interface ListEncryptionKeysParams {
  cursor?: string;
  limit?: number;
  state?: EncryptionKeyState;
  namespace?: string;
  label_selector?: string;
  order_by?: string;
}

export interface EncryptionKeyLabel {
  key: string;
  value: string;
}

export interface PutEncryptionKeyLabelRequest {
  value: string;
}

export interface PutEncryptionKeyAnnotationRequest {
  value: string;
}

export interface EncryptionKeyAnnotation {
  key: string;
  value: string;
}

/**
 * List encryption keys with optional filtering and pagination
 */
export const listEncryptionKeys = (params: ListEncryptionKeysParams) => {
  return client.get<ListResponse<EncryptionKey>>('/api/v1/encryption-keys', { params });
};

/**
 * Create a new encryption key
 */
export const createEncryptionKey = (request: CreateEncryptionKeyRequest) => {
    return client.post<EncryptionKey>('/api/v1/encryption-keys', request);
};

/**
 * Get a specific encryption key by ID
 */
export const getEncryptionKey = (id: string) => {
  return client.get<EncryptionKey>(`/api/v1/encryption-keys/${id}`);
};

/**
 * Update an encryption key's state and/or labels
 */
export const updateEncryptionKey = (id: string, request: UpdateEncryptionKeyRequest) => {
  return client.patch<EncryptionKey>(`/api/v1/encryption-keys/${id}`, request);
};

/**
 * Delete an encryption key (soft delete)
 */
export const deleteEncryptionKey = (id: string) => {
  return client.delete(`/api/v1/encryption-keys/${id}`);
};

/**
 * Get all labels for a specific encryption key
 */
export const getEncryptionKeyLabels = (id: string) => {
  return client.get<Record<string, string>>(`/api/v1/encryption-keys/${id}/labels`);
};

/**
 * Get a specific label for an encryption key
 */
export const getEncryptionKeyLabel = (id: string, labelKey: string) => {
  return client.get<EncryptionKeyLabel>(`/api/v1/encryption-keys/${id}/labels/${labelKey}`);
};

/**
 * Set a specific label for an encryption key
 */
export const putEncryptionKeyLabel = (id: string, labelKey: string, value: string) => {
  return client.put<EncryptionKeyLabel>(`/api/v1/encryption-keys/${id}/labels/${labelKey}`, { value });
};

/**
 * Delete a specific label from an encryption key
 */
export const deleteEncryptionKeyLabel = (id: string, labelKey: string) => {
  return client.delete(`/api/v1/encryption-keys/${id}/labels/${labelKey}`);
};

/**
 * Get all annotations for a specific encryption key
 */
export const getEncryptionKeyAnnotations = (id: string) => {
  return client.get<Record<string, string>>(`/api/v1/encryption-keys/${id}/annotations`);
};

/**
 * Get a specific annotation for an encryption key
 */
export const getEncryptionKeyAnnotation = (id: string, annotationKey: string) => {
  return client.get<EncryptionKeyAnnotation>(`/api/v1/encryption-keys/${id}/annotations/${annotationKey}`);
};

/**
 * Set a specific annotation for an encryption key
 */
export const putEncryptionKeyAnnotation = (id: string, annotationKey: string, value: string) => {
  return client.put<EncryptionKeyAnnotation>(`/api/v1/encryption-keys/${id}/annotations/${annotationKey}`, { value });
};

/**
 * Delete a specific annotation from an encryption key
 */
export const deleteEncryptionKeyAnnotation = (id: string, annotationKey: string) => {
  return client.delete(`/api/v1/encryption-keys/${id}/annotations/${annotationKey}`);
};

export const encryptionKeys = {
  list: listEncryptionKeys,
  create: createEncryptionKey,
  get: getEncryptionKey,
  update: updateEncryptionKey,
  delete: deleteEncryptionKey,
  getLabels: getEncryptionKeyLabels,
  getLabel: getEncryptionKeyLabel,
  putLabel: putEncryptionKeyLabel,
  deleteLabel: deleteEncryptionKeyLabel,
  getAnnotations: getEncryptionKeyAnnotations,
  getAnnotation: getEncryptionKeyAnnotation,
  putAnnotation: putEncryptionKeyAnnotation,
  deleteAnnotation: deleteEncryptionKeyAnnotation,
};
