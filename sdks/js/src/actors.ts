import { client } from './client';
import { ListResponse } from './common';

// Actor models

export interface Actor {
  id: string;
  external_id: string;
  email: string;
  admin: boolean;
  super_admin: boolean;
  created_at: string;
  updated_at: string;
}

/**
 * Parameters used for listing actors.
 */
export interface ListActorsParams {
  external_id?: string;
  email?: string;
  admin?: boolean;
  super_admin?: boolean;
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

export const actors = {
  list: listActors,
  getById: getActorById,
  getByExternalId: getActorByExternalId,
  getByMe: getMe,
  deleteById: deleteActorById,
  deleteByExternalId: deleteActorByExternalId,
};
