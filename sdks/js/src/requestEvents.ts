import { client } from './client';
import {
  ListMetadata,
  ObjectMetadata,
  ObjectReference,
  ResourceList,
  TypeMeta,
} from './common';
import { RateLimitMode } from './rateLimits';

export const REQUEST_EVENT_KIND = 'RequestEvent' as const;

export enum RequestType {
  GLOBAL = 'global',
  PROXY = 'proxy',
  OAUTH = 'oauth',
  PUBLIC = 'public',
  PROBE = 'probe',
}

export enum ResponseSource {
  UPSTREAM = 'upstream',
  CONNECTOR_RATE_LIMITER = 'connector_rate_limiter',
  RATE_LIMIT = 'rate_limit',
}

export interface RequestEventMetadata extends ObjectMetadata {
  id: string;
  namespace: string;
  createdAt: string;
}

export interface RequestEventRequest {
  method: string;
  host: string;
  scheme: string;
  path: string;
  httpVersion?: string;
  sizeBytes?: number;
  mimeType?: string;
  bodySkipped?: string;
}

export interface RequestEventResponse {
  statusCode?: number;
  error?: string;
  httpVersion?: string;
  sizeBytes?: number;
  mimeType?: string;
  bodySkipped?: string;
  source?: ResponseSource;
}

export interface RequestEventCapture {
  request: {
    url: string;
    headers: Record<string, string[]>;
    /** Captured bytes encoded as base64. Subject to API secret-replay policy. */
    body?: string;
  };
  response: {
    headers: Record<string, string[]>;
    /** Captured bytes encoded as base64. Subject to API secret-replay policy. */
    body?: string;
  };
}

export interface RequestEventRateLimit {
  rateLimitRef: ObjectReference<'RateLimit'>;
  mode: RateLimitMode;
  bucket?: Record<string, string>;
}

export interface RequestEventSpec {
  requestType: RequestType;
  correlationId?: string;
  durationMilliseconds: number;
  namespaceRef: ObjectReference<'Namespace'>;
  actorRef?: ObjectReference<'Actor'>;
  connectionRef?: ObjectReference<'Connection'>;
  connectorRef?: ObjectReference<'Connector'>;
  request: RequestEventRequest;
  response: RequestEventResponse;
  captureAvailable: boolean;
  capture?: RequestEventCapture;
  rateLimit?: RequestEventRateLimit;
  rateLimitsMatched?: RequestEventRateLimit[];
  internalTimeout?: boolean;
  requestCancelled?: boolean;
}

/** Immutable projection used by both list and get request-event endpoints. */
export interface RequestEvent extends TypeMeta<typeof REQUEST_EVENT_KIND> {
  metadata: RequestEventMetadata;
  spec: RequestEventSpec;
}

export type RequestEventList = ResourceList<RequestEvent> & {
  metadata: ListMetadata & {
    total?: number;
  };
};

export interface ListRequestEventsParams {
  cursor?: string;
  limit?: number;
  orderBy?: string;
  namespace?: string;
  requestType?: RequestType;
  labelSelector?: string;
  correlationId?: string;
  connectionId?: string;
  connectorType?: string;
  connectorId?: string;
  connectorGeneration?: number;
  method?: string;
  statusCode?: number;
  statusCodeRange?: string;
  timestampRange?: string;
  path?: string;
  pathRegex?: string;
  responseSource?: ResponseSource;
  rateLimitId?: string;
}

export const listRequestEvents = (params?: ListRequestEventsParams) =>
  client.get<RequestEventList>('/api/v1/metrics/request-events', { params });

export const getRequestEvent = (id: string) =>
  client.get<RequestEvent>(`/api/v1/metrics/request-events/${id}`);

export const requestEvents = {
  list: listRequestEvents,
  get: getRequestEvent,
};
