import {AxiosRequestConfig} from 'axios';
import {client} from './client';

export type MetricsAggregation = 'count' | 'avg' | 'p95';
export type RequestEventMetricsMetric = 'request_events' | 'request_events.errors' | 'request_events.duration_ms';
export type ResourceMetricsMetric =
    | 'resources.connections'
    | 'resources.actors'
    | 'resources.connectors'
    | 'resources.connector_versions'
    | 'resources.namespaces'
    | 'resources.rate_limits';
export type MetricsMetric = RequestEventMetricsMetric | ResourceMetricsMetric;

export type RequestEventMetricsGroupBy =
    | 'type'
    | 'method'
    | 'response_status_code'
    | 'response_source'
    | 'connector_id';
export type ResourceMetricsGroupBy =
    | 'state'
    | 'health_state'
    | 'connector_id'
    | 'connector_version'
    | 'namespace'
    | 'mode';
export type MetricsGroupBy = RequestEventMetricsGroupBy | ResourceMetricsGroupBy;

export interface MetricsRange {
    start: string;
    end: string;
    step: string;
}

export interface MetricsQueryRef {
    ref_id: string;
    metric: MetricsMetric;
    aggregation: MetricsAggregation;
    group_by?: MetricsGroupBy[];
}

export interface MetricsQueryRequest {
    range: MetricsRange;
    namespace?: string;
    label_selector?: string;
    queries: MetricsQueryRef[];
}

export interface MetricsPoint {
    timestamp: string;
    value: number;
}

export interface MetricsSeries {
    ref_id: string;
    metric: string;
    aggregation: string;
    labels?: Record<string, string>;
    points: MetricsPoint[];
}

export interface MetricsQueryResponse {
    series: MetricsSeries[];
}

export const queryMetrics = (request: MetricsQueryRequest, config?: AxiosRequestConfig) => {
    return client.post<MetricsQueryResponse>('/api/v1/metrics/query', request, config);
};

export const metrics = {
    query: queryMetrics,
};
