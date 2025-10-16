import {client} from './client';
import {ListResponse} from "./common";

// Request log models

export enum RequestType {
    GLOBAL = 'global',
    PROXY = 'proxy',
    OAUTH = 'oauth',
    PUBLIC = 'public',
}

// RequestEntryRecord is the log data recorded for every request. It does not contain header and body data,
// which is only conditionally recorded.
export interface RequestEntryRecord {
    type: RequestType;              // The type of request (global, proxy, oauth, public)
    request_id: string;             // The ID of the request (randomly generated UUID)
    correlation_id: string;         // Correlation ID for the request as supplied by the proxy caller
    timestamp: string;              // Timestamp of the request
    duration: number;               // Duration of the request in milliseconds
    connection_id: string;          // The ID of the connection that handled the request, if applicable
    connector_id: string;           // The ID of the connector that handled the request, if applicable
    connector_version: number;      // The version of the connector that handled the request, if applicable
    method: string;                 // The HTTP method of the request
    host: string;                   // The host of the request
    scheme: string;                 // The scheme of the request (http, https)
    path: string;                   // The path of the request
    request_http_version?: string;  // The HTTP version of the request
    request_size_bytes?: number;    // The size of the request in bytes
    request_mime_type?: string;     // The MIME type of the request
    response_status_code?: number;  // The HTTP status code of the response
    response_error?: string;        // The error message if the response was an error (could not make HTTP call)
    response_http_version?: string; // The HTTP version of the response
    response_size_bytes?: number;   // The size of the response in bytes
    response_mime_type?: string;    // The MIME type of the response
    internal_timeout?: boolean;     // If there was an internal timeout while capture full response size/body
    request_cancelled?: boolean;    // If the caller cancelled the request before the full body was consumed
    full_request_recorded?: boolean;// If the full request body was recorded; This means you may be able to get the full request
}

// RequestEntry is the full data for a single request. It contains header and body data.
export interface RequestEntry {
    id: string;                      // Request ID
    cid: string;                     // Correlation ID
    ts: string;                      // Timestamp
    dur: number;                     // Duration
    full: boolean;                   // Full data present
    req: {                           // -- Request data --
        u: string;                   // URL
        v: string;                   // HTTP version
        m: string;                   // Method
        h: Record<string, string[]>; // Headers
        cl?: number;                 // Content length
        b: string;                   // Body; base64 encoded string
    },                               // Request
    res: {                           // -- Response data --
        v: string;                   // HTTP version
        sc: number;                  // HTTP status code
        h: Record<string, string[]>; // Headers
        b?: string;                  // Body; base64 encoded string
        cl?: number;                 // Content length
        err?: string;                // Error message
    }                                // Response
}

/**
 * Parameters used for listing requests.
 * This interface defines the criteria and options available for querying request log data.
 *
 * @interface
 */
export interface ListRequestsParams {
    type?: RequestType;
    cursor?: string;
    limit?: number;
    order_by?: string;
}

/**
 * Get a list of all actors
 * @param params The parameters for filtering and pagination
 * @returns Promise with the list of actors
 */
export const listRequests = (params: ListRequestsParams) => {
    return client.get<ListResponse<RequestEntryRecord>>('/api/v1/request-log', {params});
};

/**
 * Get a specific actor by ID (uuid)
 * @param id The ID of the actor to get
 * @returns Promise with the actor details
 */
export const getRequest = (id: string) => {
    return client.get<RequestEntry>(`/api/v1/request-log/${id}`);
};

export const actors = {
    list: listRequests,
    get: getRequest,
};
