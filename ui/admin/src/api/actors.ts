import {client} from './client';
import {ListResponse} from "./common";

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
 * This interface defines the criteria and options available for querying actor data.
 *
 * @interface
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
 * @returns Promise with the list of actors
 */
export const listActors = (params: ListActorsParams) => {
    return client.get<ListResponse<Actor>>('/api/v1/actors', {params});
};

/**
 * Get a specific actor by ID (uuid)
 * @param id The ID of the actor to get
 * @returns Promise with the actor details
 */
export const getActorById = (id: string) => {
    return client.get<Actor>(`/api/v1/actors/${id}`);
};

/**
 * Get a specific actor by external id.
 * @param externalId The ID of the actor to get
 * @returns Promise with the actor details
 */
export const getActorByExternalId = (externalId: string) => {
    return client.get<Actor>(`/api/v1/actors/external-id/${externalId}`);
};

/**
 * Get the currently authenticated actor
 * @returns Promise with the actor details for the authenticated actor
 */
export const getMe = () => {
    return getActorById('me');
};

/**
 * Delete an actor by id (uuid)
 * @param id The ID of the actor to delete
 * @returns Promise with the delete response. No data will be returned for success, just 204 status code.
 */
export const deleteActorById = (id: string) => {
    return client.delete(`/api/v1/actors/${id}`);
};

/**
 * Delete an actor by external id
 * @param externalId The ID of the actor to delete
 * @returns Promise with the delete response. No data will be returned for success, just 204 status code.
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
