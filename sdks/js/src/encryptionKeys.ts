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
  created_at: string;
  updated_at: string;
}

export interface CreateEncryptionKeyRequest {
    namespace: string;
    key_data?: Record<string, unknown>;
    labels?: Record<string, string>;
}

export interface UpdateEncryptionKeyRequest {
    state?: EncryptionKeyState;
    labels?: Record<string, string>;
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
};
