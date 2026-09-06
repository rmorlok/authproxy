import { client } from './client';
import {
  API_VERSION,
  MutableResourceMetadata,
  NamespacedCreateMetadata,
  ObjectMetadata,
  ResourceList,
  TypeMeta,
} from './common';

// Actor models mirror the canonical authproxy.net/v1alpha1 resource.

export const ACTOR_KIND = 'Actor' as const;

export interface ActorPermission {
  namespace: string;
  resources: string[];
  resourceIds?: string[];
  verbs: string[];
}

export type ActorSigningKey = Record<string, unknown>;

export interface ActorMetadata extends ObjectMetadata {
  id: string;
  name: string;
  namespace: string;
  createdAt: string;
  updatedAt: string;
}

export interface ActorSpec {
  externalId: string;
  permissions?: ActorPermission[];
}

export interface ActorStatus {
  signingKeyConfigured: boolean;
}

export interface Actor extends TypeMeta<typeof ACTOR_KIND> {
  metadata: ActorMetadata;
  spec: ActorSpec;
  status: ActorStatus;
}

export interface CreateActorRequest extends TypeMeta<typeof ACTOR_KIND> {
  metadata: NamespacedCreateMetadata;
  spec: ActorSpec & {
    /** Write-only signing material; it is never returned on Actor resources. */
    signingKey?: ActorSigningKey;
  };
}

export interface UpdateActorRequest extends TypeMeta<typeof ACTOR_KIND> {
  metadata: MutableResourceMetadata;
  spec: {
    externalId?: string;
    permissions?: ActorPermission[];
    /** Explicit null removes actor-specific signing material. */
    signingKey?: ActorSigningKey | null;
  };
}

export type ActorList = ResourceList<Actor>;

/** Restricted Actor resource permitted in an AuthProxy JWT actor claim. */
export interface ActorClaim extends TypeMeta<typeof ACTOR_KIND> {
  metadata: {
    name?: string;
    namespace: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    id?: never;
    generation?: never;
    createdAt?: never;
    updatedAt?: never;
  };
  spec: ActorSpec;
  status?: never;
}

export interface ActorClaimInput {
  externalId: string;
  namespace: string;
  name?: string;
  permissions?: ActorPermission[];
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

/** Builds the restricted resource shape accepted in a JWT's actor claim. */
export const createActorClaim = (input: ActorClaimInput): ActorClaim => ({
  apiVersion: API_VERSION,
  kind: ACTOR_KIND,
  metadata: {
    ...(input.name ? { name: input.name } : {}),
    namespace: input.namespace,
    ...(input.labels ? { labels: { ...input.labels } } : {}),
    ...(input.annotations ? { annotations: { ...input.annotations } } : {}),
  },
  spec: {
    externalId: input.externalId,
    ...(input.permissions
      ? {
          permissions: input.permissions.map((permission) => ({
            ...permission,
            resources: [...permission.resources],
            ...(permission.resourceIds ? { resourceIds: [...permission.resourceIds] } : {}),
            verbs: [...permission.verbs],
          })),
        }
      : {}),
  },
});

/** Removes database identity, timestamps, status, and signing data from an Actor. */
export const actorResourceToClaim = (actor: Actor): ActorClaim =>
  createActorClaim({
    externalId: actor.spec.externalId,
    namespace: actor.metadata.namespace,
    name: actor.metadata.name,
    permissions: actor.spec.permissions,
    labels: actor.metadata.labels,
    annotations: actor.metadata.annotations,
  });

export interface PutActorLabelRequest {
  value: string;
}

export interface ActorLabel {
  key: string;
  value: string;
}

export interface PutActorAnnotationRequest {
  value: string;
}

export interface ActorAnnotation {
  key: string;
  value: string;
}

/**
 * Parameters used for listing actors.
 */
export interface ListActorsParams {
  name?: string;
  externalId?: string;
  namespace?: string;
  labelSelector?: string;
  cursor?: string;
  limit?: number;
  orderBy?: string;
}

/**
 * Get a list of all actors
 * @param params The parameters for filtering and pagination
 */
export const listActors = (params?: ListActorsParams) => {
  return client.get<ActorList>('/api/v1/actors', { params });
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

/**
 * Get all annotations for a specific actor by ID (uuid)
 */
export const getActorAnnotations = (id: string) => {
  return client.get<Record<string, string>>(`/api/v1/actors/${id}/annotations`);
};

/**
 * Get a specific annotation for an actor by ID (uuid) and annotation key
 */
export const getActorAnnotation = (id: string, annotationKey: string) => {
  return client.get<ActorAnnotation>(`/api/v1/actors/${id}/annotations/${annotationKey}`);
};

/**
 * Set a specific annotation for an actor by ID (uuid) and annotation key
 */
export const putActorAnnotation = (id: string, annotationKey: string, value: string) => {
  return client.put<ActorAnnotation>(`/api/v1/actors/${id}/annotations/${annotationKey}`, { value });
};

/**
 * Delete a specific annotation for an actor by ID (uuid) and annotation key
 */
export const deleteActorAnnotation = (id: string, annotationKey: string) => {
  return client.delete(`/api/v1/actors/${id}/annotations/${annotationKey}`);
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
  getAnnotations: getActorAnnotations,
  getAnnotation: getActorAnnotation,
  putAnnotation: putActorAnnotation,
  deleteAnnotation: deleteActorAnnotation,
};
