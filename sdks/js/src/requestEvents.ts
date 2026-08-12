import {client} from './client';
import {ListResponse} from './common';

// Request events models

export enum RequestType {
    GLOBAL = 'global',
    PROXY = 'proxy',
    OAUTH = 'oauth',
    PUBLIC = 'public',
}

// Identifies who produced the response captured by a RequestEventRecord.
// "upstream" is the historical default and means the response (including
// any 429) came from the 3rd-party. "connector_rate_limiter" means the
// connector-level reactive 429 limiter short-circuited the request before
// reaching the 3rd party. "rate_limit" means a proxy-side RateLimit
// resource matched and rejected the request.
export enum ResponseSource {
    UPSTREAM = 'upstream',
    CONNECTOR_RATE_LIMITER = 'connector_rate_limiter',
    RATE_LIMIT = 'rate_limit',
}

// A single rate-limit rule that matched a request. The full set of
// matches is stored on RequestEventRecord.rateLimitMatched so observers
// can see every rule that contributed to the decision, not just the one
// that ultimately rejected the request.
export interface RateLimitMatch {
    id: string; // The ID of the rate-limit resource that matched
    mode: string; // 'enforce' or 'observe'
    bucket?: Record<string, string>; // Resolved bucket dimensions (dimension name → value)
}

// RequestEventRecord is the log data recorded for every request event. It does not contain header and body data,
// which is only conditionally recorded.
export interface RequestEventRecord {
    namespace: string; // The namespace of the request
    type: RequestType; // The type of request (global, proxy, oauth, public)
    requestId: string; // The ID of the request (randomly generated UUID)
    correlationId: string; // Correlation ID for the request as supplied by the proxy caller
    timestamp: string; // Timestamp of the request
    duration: number; // Duration of the request in milliseconds
    connectionId: string; // The ID of the connection that handled the request, if applicable
    connectorId: string; // The ID of the connector that handled the request, if applicable
    connectorVersion: number; // The version of the connector that handled the request, if applicable
    method: string; // The HTTP method of the request
    host: string; // The host of the request
    scheme: string; // The scheme of the request (http, https)
    path: string; // The path of the request
    requestHttpVersion?: string; // The HTTP version of the request
    requestSizeBytes?: number; // The size of the request in bytes
    requestMimeType?: string; // The MIME type of the request
    responseStatusCode?: number; // The HTTP status code of the response
    responseError?: string; // The error message if the response was an error (could not make HTTP call)
    responseHttpVersion?: string; // The HTTP version of the response
    responseSizeBytes?: number; // The size of the response in bytes
    responseMimeType?: string; // The MIME type of the response
    internalTimeout?: boolean; // If there was an internal timeout while capture full response size/body
    requestCancelled?: boolean; // If the caller cancelled the request before the full body was consumed
    fullRequestRecorded?: boolean; // If the full request body was recorded; This means you may be able to get the full request
    labels?: Record<string, string>; // Labels associated with the request (merged from connection and per-request labels)

    // Rate-limit attribution. Defaults to ResponseSource.UPSTREAM for any
    // request that was not short-circuited by a rate limiter. The
    // rate_limit_* fields are populated when a proxy-side RateLimit
    // resource matched the request — they remain empty for upstream and
    // connector_rate_limiter responses.
    responseSource?: ResponseSource;
    rateLimitId?: string; // ID of the RateLimit resource that fired (when response_source = rate_limit)
    rateLimitMode?: string; // 'enforce' or 'observe' (when response_source = rate_limit)
    rateLimitBucket?: Record<string, string>; // Resolved bucket dimensions for the firing rule
    rateLimitMatched?: RateLimitMatch[]; // Full set of rate-limit rules that matched this request
}

// RequestEvent is the full data for a single request event. It contains header and body data.
export interface RequestEvent {
    id: string; // Request ID
    ns: string; // Namespace
    cid: string; // Correlation ID
    ts: string; // Timestamp
    dur: number; // Duration
    full: boolean; // Full data present
    req: {
        // -- Request data --
        u: string; // URL
        v: string; // HTTP version
        m: string; // Method
        h: Record<string, string[]>; // Headers
        cl?: number; // Content length
        b: string; // Body; base64 encoded string
    }; // Request
    res: {
        // -- Response data --
        v: string; // HTTP version
        sc: number; // HTTP status code
        h: Record<string, string[]>; // Headers
        b?: string; // Body; base64 encoded string
        cl?: number; // Content length
        err?: string; // Error message
    }; // Response
}

/**
 * Parameters used for listing request events.
 */
export interface ListRequestEventsParams {
    cursor?: string;
    limit?: number;
    orderBy?: string;

    /*
     * Filters
     */

    namespace?: string;
    requestType?: RequestType;
    labelSelector?: string;
    correlationId?: string;
    connectionId?: string;
    connectorType?: string;
    connectorId?: string;
    connectorVersion?: number;
    method?: string;
    statusCode?: number;
    statusCodeRange?: string; // Changed to string to match Go's format (e.g., "200-299")
    timestampRange?: string;
    path?: string;
    pathRegex?: string; // Changed to string to match Go's format
    responseSource?: ResponseSource; // Filter by who produced the response
    rateLimitId?: string; // Filter for entries that fired a specific RateLimit resource
}

/**
 * Get a list of request events
 */
export const listRequestEvents = (params: ListRequestEventsParams) => {
    return client.get<ListResponse<RequestEventRecord>>('/api/v1/metrics/request-events', {params});
};

/**
 * Get a specific request event by ID (uuid)
 */
export const getRequestEvent = (id: string) => {
    return client.get<RequestEvent>(`/api/v1/metrics/request-events/${id}`);
};

export const requestEvents = {
    list: listRequestEvents,
    get: getRequestEvent,
};
