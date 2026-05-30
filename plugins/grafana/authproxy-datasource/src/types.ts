import { DataQuery, DataSourceJsonData } from '@grafana/data';

export type AuthProxyQueryType = 'metrics' | 'request_events' | 'variable';
export type AuthProxyVariableType = 'namespaces' | 'connectors' | 'connections' | 'actors' | 'rate_limits';

export interface AuthProxyDataSourceOptions extends DataSourceJsonData {
  baseUrl?: string;
}

export interface AuthProxySecureJsonData {
  jwt?: string;
}

export interface AuthProxyRequestFilters {
  namespace?: string;
  requestType?: string;
  correlationId?: string;
  connectionId?: string;
  connectorType?: string;
  connectorId?: string;
  method?: string;
  statusCode?: number;
  statusCodeRange?: string;
  timestampRange?: string;
  path?: string;
  pathRegex?: string;
  labelSelector?: string;
  responseSource?: string;
  rateLimitId?: string;
}

export interface AuthProxyVariableQuery {
  type: AuthProxyVariableType;
  namespace?: string;
  labelSelector?: string;
  connectorId?: string;
}

export interface AuthProxyQuery extends DataQuery {
  queryType?: AuthProxyQueryType;
  metric?: string;
  aggregation?: string;
  groupBy?: string[];
  namespace?: string;
  labelSelector?: string;
  requestFilters?: AuthProxyRequestFilters;
  variable?: AuthProxyVariableQuery;
}
