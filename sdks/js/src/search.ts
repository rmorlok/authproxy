import { AxiosRequestConfig } from 'axios';
import { client } from './client';
import { ManagedResourceKind, ObjectReference, TypeMeta } from './common';

export const SEARCH_RESULT_LIST_KIND = 'SearchResultList' as const;

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

export interface SearchResult {
  resourceRef: ObjectReference<ManagedResourceKind>;
  labels: Record<string, string>;
  matchedLabels: SearchLabelMatch[];
  updatedAt: string;
}

export interface SearchResultList extends TypeMeta<typeof SEARCH_RESULT_LIST_KIND> {
  metadata: {
    truncatedKinds: ManagedResourceKind[];
    incompleteKinds: ManagedResourceKind[];
  };
  items: SearchResult[];
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
) =>
  client.get<SearchResultList>('/api/v1/search/resources', {
    ...config,
    params,
    paramsSerializer: config?.paramsSerializer ?? { indexes: null },
  });

export const resourceSearch = {
  search: searchResources,
};
