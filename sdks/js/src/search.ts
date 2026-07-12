import {AxiosRequestConfig} from 'axios';
import {client} from './client';

export type SearchResourceType =
    | 'actor'
    | 'connection'
    | 'connector'
    | 'namespace'
    | 'key'
    | 'rate_limit';

export type SearchMode = 'query' | 'seed';

export interface SearchLabelMatch {
    key: string;
    value: string;
}

export interface SearchResourceSummary {
    resource_type: SearchResourceType;
    resource_id: string;
    namespace: string;
    labels: Record<string, string>;
    matched_labels: SearchLabelMatch[];
    updated_at: string;
}

export interface SearchResourcesResponse {
    items: SearchResourceSummary[];
    truncated_types: SearchResourceType[];
    incomplete_types: SearchResourceType[];
}

export interface SearchResourcesParams {
    mode?: SearchMode;
    resource_type?: SearchResourceType[];
    q?: string;
    label_selector?: string;
    namespace?: string;
    limit?: number;
}

export const searchResources = (
    params: SearchResourcesParams,
    config?: AxiosRequestConfig,
) => {
    return client.get<SearchResourcesResponse>('/api/v1/search/resources', {
        ...config,
        params,
        paramsSerializer: config?.paramsSerializer ?? {indexes: null},
    });
};

export const resourceSearch = {
    search: searchResources,
};
