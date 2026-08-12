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
    resourceType: SearchResourceType;
    resourceId: string;
    name: string;
    namespace: string;
    labels: Record<string, string>;
    matchedLabels: SearchLabelMatch[];
    updatedAt: string;
}

export interface SearchResourcesResponse {
    items: SearchResourceSummary[];
    truncatedTypes: SearchResourceType[];
    incompleteTypes: SearchResourceType[];
}

export interface SearchResourcesParams {
    mode?: SearchMode;
    resourceType?: SearchResourceType[];
    q?: string;
    labelSelector?: string;
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
