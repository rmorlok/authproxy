import { client } from './client';
import { ListResponse } from './common';

// Actor models

export interface UpdateActorRequest {
    labels?: Record<string, string>;
}

export interface PutActorLabelRequest {
  value: string;
}

export interface ActorLabel {
  key: string;
  value: string;
}

export interface Actor {
  id: string;
  namespace: string;
  labels?: Record<string, string>;
  external_id: string;
  created_at: string;
  updated_at: string;
}

export interface CreateActorRequest {
    namespace: string;
    external_id: string;
    labels?: Record<string, string>;
}

/**
 * Parameters used for listing actors.
 */
export interface ListActorsParams {
  external_id?: string;
  namespace?: string;
  label_selector?: string;
  cursor?: string;
  limit?: number;
  order_by?: string;
}

/**
 * Get a list of all actors
 * @param params The parameters for filtering and pagination
 */
export const listActors = (params: ListActorsParams) => {
  return client.get<ListResponse<Actor>>('/api/v1/actors', { params });
};

/**
 * Create a new actor
 * @param request The actor to create
 */
export const createActor = (request: CreateActorRequest) => {
    return client.post<Actor>('/api/v1/actors', request);
};

/**
 * Get a specific actor by ID (uuid)
 */
export const getActorById = (id: string) => {
  return client.get<Actor>(`/api/v1/actors/${id}`);
};

/**
 * Get a specific actor by external id.
 */
export const getActorByExternalId = (externalId: string) => {
  return client.get<Actor>(`/api/v1/actors/external-id/${externalId}`);
};

/**
 * Get the currently authenticated actor
 */
export const getMe = () => {
  return getActorById('me');
};

/**
 * Delete an actor by id (uuid)
 */
export const deleteActorById = (id: string) => {
  return client.delete(`/api/v1/actors/${id}`);
};

/**
 * Delete an actor by external id
 */
export const deleteActorByExternalId = (externalId: string) => {
  return client.delete(`/api/v1/actors/external-id/${externalId}`);
};

/**
 * Update an actor by ID (uuid)
 */
export const updateActor = (id: string, request: UpdateActorRequest) => {
  return client.patch<Actor>(`/api/v1/actors/${id}`, request);
};

/**
 * Update an actor by external ID
 */
export const updateActorByExternalId = (
  externalId: string,
  namespace: string | undefined,
  request: UpdateActorRequest
) => {
  return client.patch<Actor>(`/api/v1/actors/external-id/${externalId}`, request, {
    params: { namespace },
  });
};

/**
 * Get all labels for a specific actor by ID (uuid)
 */
export const getActorLabels = (id: string) => {
  return client.get<Record<string, string>>(`/api/v1/actors/${id}/labels`);
};

/**
 * Get a specific label for an actor by ID (uuid) and label key
 */
export const getActorLabel = (id: string, labelKey: string) => {
  return client.get<ActorLabel>(`/api/v1/actors/${id}/labels/${labelKey}`);
};

/**
 * Set a specific label for an actor by ID (uuid) and label key
 */
export const putActorLabel = (id: string, labelKey: string, value: string) => {
  return client.put<ActorLabel>(`/api/v1/actors/${id}/labels/${labelKey}`, { value });
};

/**
 * Delete a specific label for an actor by ID (uuid) and label key
 */
export const deleteActorLabel = (id: string, labelKey: string) => {
  return client.delete(`/api/v1/actors/${id}/labels/${labelKey}`);
};

export const actors = {
  list: listActors,
  create: createActor,
  getById: getActorById,
  getByExternalId: getActorByExternalId,
  getByMe: getMe,
  deleteById: deleteActorById,
  deleteByExternalId: deleteActorByExternalId,
  update: updateActor,
  updateByExternalId: updateActorByExternalId,
  getLabels: getActorLabels,
  getLabel: getActorLabel,
  putLabel: putActorLabel,
  deleteLabel: deleteActorLabel,
};
