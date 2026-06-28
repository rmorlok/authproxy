import {
  DataQueryRequest,
  DataQueryResponse,
  DataSourceInstanceSettings,
  dateTime,
  MetricFindValue,
  toDataFrame,
} from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';

import { AuthProxyDataSourceOptions, AuthProxyQuery, AuthProxyVariableQuery } from './types';

export class DataSource extends DataSourceWithBackend<AuthProxyQuery, AuthProxyDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<AuthProxyDataSourceOptions>) {
    super(instanceSettings);
  }

  applyTemplateVariables(query: AuthProxyQuery): AuthProxyQuery {
    const replace = (value?: string) => getTemplateSrv().replace(value ?? '');
    return {
      ...query,
      namespace: replace(query.namespace),
      labelSelector: replace(query.labelSelector),
      requestFilters: query.requestFilters
        ? {
            ...query.requestFilters,
            namespace: replace(query.requestFilters.namespace),
            requestType: replace(query.requestFilters.requestType),
            correlationId: replace(query.requestFilters.correlationId),
            connectionId: replace(query.requestFilters.connectionId),
            connectorType: replace(query.requestFilters.connectorType),
            connectorId: replace(query.requestFilters.connectorId),
            method: replace(query.requestFilters.method),
            statusCodeRange: replace(query.requestFilters.statusCodeRange),
            timestampRange: replace(query.requestFilters.timestampRange),
            path: replace(query.requestFilters.path),
            pathRegex: replace(query.requestFilters.pathRegex),
            labelSelector: replace(query.requestFilters.labelSelector),
            responseSource: replace(query.requestFilters.responseSource),
            rateLimitId: replace(query.requestFilters.rateLimitId),
          }
        : undefined,
    };
  }

  async metricFindQuery(query: AuthProxyVariableQuery): Promise<MetricFindValue[]> {
    const now = dateTime();
    const replace = (value?: string) => getTemplateSrv().replace(value ?? '');
    const variableQuery: AuthProxyVariableQuery = {
      ...query,
      namespace: replace(query.namespace),
      labelSelector: replace(query.labelSelector),
      connectorId: replace(query.connectorId),
    };
    const request = {
      app: 'dashboard',
      requestId: 'authproxy-variable-query',
      interval: '15m',
      intervalMs: 15 * 60 * 1000,
      maxDataPoints: 500,
      range: {
        from: dateTime(now).subtract(15, 'minutes'),
        to: now,
        raw: { from: 'now-15m', to: 'now' },
      },
      scopedVars: {},
      targets: [
        {
          refId: 'VariableQuery',
          queryType: 'variable',
          variable: variableQuery,
        },
      ],
      timezone: 'browser',
    } as DataQueryRequest<AuthProxyQuery>;

    const response = await new Promise<DataQueryResponse>((resolve, reject) => {
      const subscription = this.query(request).subscribe({
        next: resolve,
        error: reject,
      });
      return () => subscription.unsubscribe();
    });
    const frame = response.data?.[0] ? toDataFrame(response.data[0]) : undefined;
    if (!frame) {
      return [];
    }
    const text = frame.fields.find((field) => field.name === 'text');
    const value = frame.fields.find((field) => field.name === 'value');
    if (!text || !value) {
      return [];
    }
    const values: MetricFindValue[] = [];
    for (let idx = 0; idx < frame.length; idx += 1) {
      values.push({
        text: String(text.values[idx] ?? ''),
        value: String(value.values[idx] ?? ''),
      });
    }
    return values;
  }
}
