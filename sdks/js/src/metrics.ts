import {client} from './client';

export type MetricsAggregation = 'count' | 'avg' | 'p95';
export type MetricsMetric = 'request_events' | 'request_events.errors' | 'request_events.duration_ms';
export type MetricsGroupBy = 'type' | 'method' | 'response_status_code' | 'response_source' | 'connector_id';

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

export const queryMetrics = (request: MetricsQueryRequest) => {
    return client.post<MetricsQueryResponse>('/api/v1/metrics/query', request);
};

export const metrics = {
    query: queryMetrics,
};
