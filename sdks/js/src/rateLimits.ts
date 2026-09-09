import { client } from './client';
import {
    ActionRequest,
    ActionResponse,
    GenerationlessObjectReference,
    MutableResourceMetadata,
    NamespacedCreateMetadata,
    ObjectMetadata,
    ResourceList,
    TypeMeta,
    actionRequest,
    objectReference,
} from './common';
import { ProxyRequest } from './proxy';

// Rate-limit models mirror the canonical authproxy.net/v1alpha1 resource.

export const RATE_LIMIT_KIND = 'RateLimit' as const;

export enum RateLimitMode {
    ENFORCE = 'enforce',
    OBSERVE = 'observe',
}

export enum PathMatchKind {
    PREFIX = 'prefix',
    GLOB = 'glob',
    REGEX = 'regex',
}

export enum SlidingWindowMode {
    LOG = 'log',
    COUNTER = 'counter',
}

export interface RateLimitPathMatch {
    kind: PathMatchKind;
    value: string;
}

export interface RateLimitSelector {
    labelSelector?: string;
    methods?: string[];
    pathMatch?: RateLimitPathMatch;
    /**
     * When omitted, defaults to ['proxy', 'probe']. An explicit empty list is
     * rejected at validation.
     */
    requestTypes?: string[];
}

export interface RateLimitBucket {
    /**
     * Reserved names: actor, connection, connector, connector_generation,
     * namespace, method. Labels: `labels/<key>`. Empty / omitted = single
     * global bucket per rule.
     */
    dimensions?: string[];
}

export interface RateLimitFixedWindow {
    /** Human-duration string (e.g. '1m', '5m'). */
    window: string;
    limit: number;
}

export interface RateLimitSlidingWindow {
    window: string;
    limit: number;
    mode: SlidingWindowMode;
}

export interface RateLimitTokenBucket {
    capacity: number;
    /** Tokens per second; may be fractional (e.g. 0.5). */
    refillRate: number;
}

/**
 * Tagged union — exactly one variant must be set. The server (and the
 * Terraform provider) validate this at write time.
 */
export type RateLimitAlgorithm =
    | {fixedWindow: RateLimitFixedWindow; slidingWindow?: never; tokenBucket?: never}
    | {slidingWindow: RateLimitSlidingWindow; fixedWindow?: never; tokenBucket?: never}
    | {tokenBucket: RateLimitTokenBucket; fixedWindow?: never; slidingWindow?: never};

export type RateLimitConnectorReference = GenerationlessObjectReference<'Connector'>;

export type RateLimitConnectionReference = GenerationlessObjectReference<'Connection'>;

export type RateLimitScope =
    | {namespaceMatcher: string; connectorRef?: never; connectionRef?: never}
    | {connectorRef: RateLimitConnectorReference; connectionRef?: never; namespaceMatcher?: never}
    | {connectionRef: RateLimitConnectionReference; connectorRef?: never; namespaceMatcher?: never};

export interface RateLimitSpec {
    /** Omit to apply across metadata.namespace and all descendants. */
    scope?: RateLimitScope;
    mode?: RateLimitMode;
    selector: RateLimitSelector;
    bucket: RateLimitBucket;
    algorithm: RateLimitAlgorithm;
}

export interface RateLimitMetadata extends ObjectMetadata {
    id: string;
    name: string;
    namespace: string;
    createdAt: string;
    updatedAt: string;
}

export interface RateLimitStatus {
    effectiveMode: RateLimitMode;
}

export interface RateLimit extends TypeMeta<typeof RATE_LIMIT_KIND> {
    metadata: RateLimitMetadata;
    spec: RateLimitSpec;
    status: RateLimitStatus;
}

export interface CreateRateLimitRequest extends TypeMeta<typeof RATE_LIMIT_KIND> {
    metadata: NamespacedCreateMetadata;
    spec: RateLimitSpec;
}

export interface UpdateRateLimitRequest extends TypeMeta<typeof RATE_LIMIT_KIND> {
    metadata: MutableResourceMetadata;
    spec: {
        scope?: RateLimitScope | null;
        mode?: RateLimitMode;
        selector?: RateLimitSelector;
        bucket?: RateLimitBucket;
        algorithm?: RateLimitAlgorithm;
    };
}

export type RateLimitList = ResourceList<RateLimit>;

export interface ListRateLimitsParams {
    name?: string;
    cursor?: string;
    limit?: number;
    namespace?: string;
    labelSelector?: string;
    orderBy?: string;
}

/**
 * List rate limits with optional filtering and pagination.
 */
export const listRateLimits = (params?: ListRateLimitsParams) => {
    return client.get<RateLimitList>('/api/v1/rate-limits', { params });
};

/**
 * Create a new rate limit.
 */
export const createRateLimit = (request: CreateRateLimitRequest) => {
    return client.post<RateLimit>('/api/v1/rate-limits', request);
};

/**
 * Get a specific rate limit by ID.
 */
export const getRateLimit = (id: string) => {
    return client.get<RateLimit>(`/api/v1/rate-limits/${id}`);
};

/**
 * Update a rate limit's spec or metadata. Fields omitted inside metadata and
 * spec are left untouched; scope may be null to restore namespace scope.
 */
export const updateRateLimit = (id: string, request: UpdateRateLimitRequest) => {
    return client.patch<RateLimit>(`/api/v1/rate-limits/${id}`, request);
};

/**
 * Delete a rate limit (soft delete).
 */
export const deleteRateLimit = (id: string) => {
    return client.delete(`/api/v1/rate-limits/${id}`);
};

// --- Dry run ---

/**
 * DryRunRateLimitRequest mirrors the server's input. Reuses ProxyRequest
 * for the request half, matching the shape /connections/{id}/_proxy
 * already accepts.
 */
export interface DryRunRateLimitRequest {
    request: ProxyRequest;
    requestType: string;
    context: (
        | {connectionId: string; namespace?: never}
        | {namespace: string; connectionId?: never}
    ) & {
        actorId?: string;
    };
}

export const RATE_LIMIT_DRY_RUN_KIND = 'RateLimitDryRun' as const;

export interface DryRunRateLimitSpec {
    request: ProxyRequest;
    requestType: string;
    actorRef?: GenerationlessObjectReference<'Actor'>;
}

export interface DryRunRateLimitMatch {
    rateLimitId: string;
    namespace: string;
    effectiveMode: string;
    bucketKey: string;
    algorithmSummary: string;
    wouldAllow: boolean;
    remaining: number;
    retryAfterMs: number;
    /**
     * True when the runtime fail-opened on a Redis error during Peek.
     * The UI should surface this as "couldn't read counter; runtime
     * would fail-open" so operators know the would_allow isn't
     * trustworthy.
     */
    peekFailed: boolean;
}

export interface DryRunRateLimitNotMatched {
    rateLimitId: string;
    namespace: string;
    reason: string;
}

export interface DryRunRateLimitStatus {
    requestLabelSnapshot: Record<string, string>;
    matched: DryRunRateLimitMatch[];
    notMatched: DryRunRateLimitNotMatched[];
}

export type DryRunRateLimitActionRequest = ActionRequest<
    typeof RATE_LIMIT_DRY_RUN_KIND,
    'Connection' | 'Namespace',
    DryRunRateLimitSpec
>;

export type DryRunRateLimitResponse = ActionResponse<
    typeof RATE_LIMIT_DRY_RUN_KIND,
    'Connection' | 'Namespace',
    DryRunRateLimitSpec,
    DryRunRateLimitStatus
>;

/**
 * Evaluate which rate limits would apply to a synthesized request, and
 * whether each would limit it. Counters are NOT incremented — the
 * server's Limiter.Peek reads counter state without writing. Useful for
 * validating selectors / buckets / algorithms without sending real
 * traffic.
 */
export const dryRunRateLimit = (req: DryRunRateLimitRequest) => {
    const target = req.context.connectionId
        ? objectReference('Connection', {id: req.context.connectionId})
        : objectReference('Namespace', {id: req.context.namespace});
    const spec: DryRunRateLimitSpec = {
        request: req.request,
        requestType: req.requestType,
        ...(req.context.actorId
            ? {actorRef: objectReference('Actor', {id: req.context.actorId})}
            : {}),
    };
    const action: DryRunRateLimitActionRequest = actionRequest(
        RATE_LIMIT_DRY_RUN_KIND,
        target,
        spec,
    );
    return client.post<DryRunRateLimitResponse>('/api/v1/rate-limits/_dryRun', action);
};

export const rateLimits = {
    list: listRateLimits,
    create: createRateLimit,
    get: getRateLimit,
    update: updateRateLimit,
    delete: deleteRateLimit,
    dryRun: dryRunRateLimit,
};
