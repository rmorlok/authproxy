/** The schema version used by every v1 API resource and transport object. */
export const API_VERSION = 'authproxy.net/v1alpha1' as const;

export type ApiVersion = typeof API_VERSION;

/** Kinds that identify durable, client-managed AuthProxy resources. */
export type ManagedResourceKind =
  | 'Actor'
  | 'Connection'
  | 'Connector'
  | 'Key'
  | 'Namespace'
  | 'RateLimit';

/** Read-only resource-shaped projections returned by operational APIs. */
export type ProjectionKind =
  | 'Notification'
  | 'RequestEvent'
  | 'Task'
  | 'TaskExecution'
  | 'TaskQueue'
  | 'TaskQueueHistory'
  | 'TaskSchedule'
  | 'TaskServer'
  | 'WorkflowHistoryEvent'
  | 'WorkflowInstance';

export type ResourceKind = ManagedResourceKind | ProjectionKind;

/** Identifies the concrete schema used by a resource or transport object. */
export interface TypeMeta<K extends string = string> {
  apiVersion: ApiVersion;
  kind: K;
}

/** Common resource metadata. Individual resources narrow required fields. */
export interface ObjectMetadata {
  id?: string;
  name?: string;
  namespace?: string;
  generation?: number;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  createdAt?: string;
  updatedAt?: string;
}

export enum ConditionStatus {
  TRUE = 'True',
  FALSE = 'False',
  UNKNOWN = 'Unknown',
}

export interface Condition {
  type: string;
  status: ConditionStatus;
  observedGeneration?: number;
  lastTransitionTime: string;
  reason?: string;
  message?: string;
}

/** Common metadata fields accepted by mutable, namespaced resources. */
export interface NamespacedCreateMetadata {
  name?: string;
  namespace: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

/** Common metadata fields accepted by resource merge patches. */
export interface MutableResourceMetadata {
  name?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

/**
 * Stable resource identity. References use either id or namespace/name.
 * Connector references may additionally select a generation.
 */
export interface ObjectReference<K extends string = ResourceKind> extends TypeMeta<K> {
  id?: string;
  name?: string;
  namespace?: string;
  generation?: number;
}

export type ConnectorGenerationReference = ObjectReference<'Connector'> & {
  generation: number;
};

export type GenerationlessObjectReference<K extends string> = Omit<
  ObjectReference<K>,
  'generation'
> & {
  generation?: never;
};

export interface ListMetadata {
  resourceVersion?: string;
  continue?: string;
  remainingItemCount?: number;
}

/** Kubernetes-style list transport for a homogeneous resource kind. */
export type ResourceList<T extends TypeMeta<string>> = TypeMeta<`${T['kind']}List`> & {
  metadata: ListMetadata;
  items: T[];
};

export interface ActionMetadata<TTargetKind extends string = ResourceKind> {
  target: ObjectReference<TTargetKind>;
}

/** Imperative action sent to the API. Status is always server-owned. */
export type ActionRequest<
  K extends string,
  TTargetKind extends string,
  TSpec,
> = TypeMeta<K> & {
  metadata: ActionMetadata<TTargetKind>;
  spec: TSpec;
  status?: never;
};

/** Typed action result returned by the API. */
export type ActionResponse<
  K extends string,
  TTargetKind extends string,
  TSpec,
  TStatus,
> = TypeMeta<K> & {
  metadata: ActionMetadata<TTargetKind>;
  spec: TSpec;
  status: TStatus;
};

export const objectReference = <
  K extends string,
  I extends Omit<ObjectReference<K>, 'apiVersion' | 'kind'>,
>(
  kind: K,
  identity: I,
): TypeMeta<K> & I => ({
  apiVersion: API_VERSION,
  kind,
  ...identity,
});

export const actionRequest = <K extends string, TTargetKind extends string, TSpec>(
  kind: K,
  target: ObjectReference<TTargetKind>,
  spec: TSpec,
): ActionRequest<K, TTargetKind, TSpec> => ({
  apiVersion: API_VERSION,
  kind,
  metadata: { target },
  spec,
});
