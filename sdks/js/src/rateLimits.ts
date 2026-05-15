import { client } from './client';
import { ListResponse } from './common';
import { ProxyRequest } from './proxy';

// Rate-limit models. Mirror the server's routes.RateLimitJson shape and
// the internal rate_limit schema package — kept here verbatim so SDK
// consumers can author definitions in TypeScript with full type safety.

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
    label_selector?: string;
    methods?: string[];
    path_match?: RateLimitPathMatch;
    /**
     * When omitted, defaults to ['proxy', 'probe']. An explicit empty list is
     * rejected at validation.
     */
    request_types?: string[];
}

export interface RateLimitBucket {
    /**
     * Reserved names: actor, connection, connector, connector_version,
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
    refill_rate: number;
}

/**
 * Tagged union — exactly one variant must be set. The server (and the
 * Terraform provider) validate this at write time.
 */
export interface RateLimitAlgorithm {
    fixed_window?: RateLimitFixedWindow;
    sliding_window?: RateLimitSlidingWindow;
    token_bucket?: RateLimitTokenBucket;
}

export interface RateLimitDefinition {
    mode?: RateLimitMode;
    selector: RateLimitSelector;
    bucket: RateLimitBucket;
    algorithm: RateLimitAlgorithm;
}

export interface RateLimit {
    id: string;
    namespace: string;
    definition: RateLimitDefinition;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    created_at: string;
    updated_at: string;
}

export interface CreateRateLimitRequest {
    namespace: string;
    definition: RateLimitDefinition;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
}

export interface UpdateRateLimitRequest {
    definition?: RateLimitDefinition;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
}

export interface ListRateLimitsParams {
    cursor?: string;
    limit?: number;
    namespace?: string;
    label_selector?: string;
    order_by?: string;
}

/**
 * List rate limits with optional filtering and pagination.
 */
export const listRateLimits = (params: ListRateLimitsParams) => {
    return client.get<ListResponse<RateLimit>>('/api/v1/rate-limits', { params });
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
 * Update a rate limit's definition, labels, or annotations. Pass only the
 * fields you want to change; omitted fields are left untouched.
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
    request_type: string;
    context: {
        connection_id?: string;
        actor_id?: string;
        namespace?: string;
    };
}

export interface DryRunRateLimitMatch {
    rate_limit_id: string;
    namespace: string;
    effective_mode: string;
    bucket_key: string;
    algorithm_summary: string;
    would_allow: boolean;
    remaining: number;
    retry_after_ms: number;
    /**
     * True when the runtime fail-opened on a Redis error during Peek.
     * The UI should surface this as "couldn't read counter; runtime
     * would fail-open" so operators know the would_allow isn't
     * trustworthy.
     */
    peek_failed: boolean;
}

export interface DryRunRateLimitNotMatched {
    rate_limit_id: string;
    namespace: string;
    reason: string;
}

export interface DryRunRateLimitResponse {
    request_label_snapshot: Record<string, string>;
    matched: DryRunRateLimitMatch[];
    not_matched: DryRunRateLimitNotMatched[];
}

/**
 * Evaluate which rate limits would apply to a synthesized request, and
 * whether each would limit it. Counters are NOT incremented — the
 * server's Limiter.Peek reads counter state without writing. Useful for
 * validating selectors / buckets / algorithms without sending real
 * traffic.
 */
export const dryRunRateLimit = (req: DryRunRateLimitRequest) => {
    return client.post<DryRunRateLimitResponse>('/api/v1/rate-limits/_dry_run', req);
};

// --- Label & annotation sub-resources, identical shape to encryption keys. ---

export interface RateLimitLabel {
    key: string;
    value: string;
}

export interface RateLimitAnnotation {
    key: string;
    value: string;
}

export const getRateLimitLabels = (id: string) =>
    client.get<Record<string, string>>(`/api/v1/rate-limits/${id}/labels`);

export const getRateLimitLabel = (id: string, labelKey: string) =>
    client.get<RateLimitLabel>(`/api/v1/rate-limits/${id}/labels/${labelKey}`);

export const putRateLimitLabel = (id: string, labelKey: string, value: string) =>
    client.put<RateLimitLabel>(`/api/v1/rate-limits/${id}/labels/${labelKey}`, { value });

export const deleteRateLimitLabel = (id: string, labelKey: string) =>
    client.delete(`/api/v1/rate-limits/${id}/labels/${labelKey}`);

export const getRateLimitAnnotations = (id: string) =>
    client.get<Record<string, string>>(`/api/v1/rate-limits/${id}/annotations`);

export const getRateLimitAnnotation = (id: string, annotationKey: string) =>
    client.get<RateLimitAnnotation>(`/api/v1/rate-limits/${id}/annotations/${annotationKey}`);

export const putRateLimitAnnotation = (id: string, annotationKey: string, value: string) =>
    client.put<RateLimitAnnotation>(`/api/v1/rate-limits/${id}/annotations/${annotationKey}`, { value });

export const deleteRateLimitAnnotation = (id: string, annotationKey: string) =>
    client.delete(`/api/v1/rate-limits/${id}/annotations/${annotationKey}`);

export const rateLimits = {
    list: listRateLimits,
    create: createRateLimit,
    get: getRateLimit,
    update: updateRateLimit,
    delete: deleteRateLimit,
    dryRun: dryRunRateLimit,
    getLabels: getRateLimitLabels,
    getLabel: getRateLimitLabel,
    putLabel: putRateLimitLabel,
    deleteLabel: deleteRateLimitLabel,
    getAnnotations: getRateLimitAnnotations,
    getAnnotation: getRateLimitAnnotation,
    putAnnotation: putRateLimitAnnotation,
    deleteAnnotation: deleteRateLimitAnnotation,
};
