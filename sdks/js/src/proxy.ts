// Proxy request / response shapes. Mirrors internal/core/iface/proxy.go.
//
// Owned by the SDK rather than a single consumer because two places need
// to author one: the rate-limit dry-run admin page and (next) a "send
// this for real" button that posts to /connections/{id}/_proxy. Anything
// that builds an HTTP request to send through an AuthProxy connection
// should produce one of these.

export interface ProxyRequest {
    url: string;
    method: string;
    headers?: Record<string, string>;
    /**
     * Per-request labels merged into the request's label snapshot. Request
     * labels override connection labels with the same key. Keys under
     * `apxy/` are reserved and silently dropped by the server.
     */
    labels?: Record<string, string>;
    /**
     * Raw body bytes, base64-encoded for transport. Exactly one of
     * body_raw / body_json may be set; for GET/HEAD/etc., neither.
     */
    body_raw?: string;
    /** JSON body. Alternative to body_raw. */
    body_json?: unknown;
}

export interface ProxyResponse {
    status_code: number;
    headers: Record<string, string>;
    body_raw?: string;
    body_json?: unknown;
}

// Valid HTTP methods accepted by the proxy. Mirrors the server's
// validation set in internal/core/iface/proxy.go.
export const PROXY_METHODS = [
    'GET',
    'HEAD',
    'POST',
    'PUT',
    'PATCH',
    'DELETE',
    'OPTIONS',
    'CONNECT',
    'TRACE',
] as const;
export type ProxyMethod = typeof PROXY_METHODS[number];
