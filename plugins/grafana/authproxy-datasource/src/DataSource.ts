import {
  DataQueryRequest,
  DataQueryResponse,
  DataSourceInstanceSettings,
  dateTime,
  MetricFindValue,
  toDataFrame,
} from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';

import { AuthProxyDataSourceOptions, AuthProxyQuery, AuthProxyVariableQuery } from './types';

export class DataSource extends DataSourceWithBackend<AuthProxyQuery, AuthProxyDataSourceOptions> {
  constructor(
    instanceSettings: DataSourceInstanceSettings<AuthProxyDataSourceOptions>,
    private readonly templateSrv?: { replace(value?: string): string }
  ) {
    super(instanceSettings);
  }

  applyTemplateVariables(query: AuthProxyQuery): AuthProxyQuery {
    const replace = this.templateSrv?.replace.bind(this.templateSrv) ?? ((value?: string) => value ?? '');
    return {
      ...query,
      namespace: replace(query.namespace),
      labelSelector: replace(query.labelSelector),
      requestFilters: query.requestFilters
        ? {
            ...query.requestFilters,
            namespace: replace(query.requestFilters.namespace),
            labelSelector: replace(query.requestFilters.labelSelector),
            connectionId: replace(query.requestFilters.connectionId),
            connectorId: replace(query.requestFilters.connectorId),
            rateLimitId: replace(query.requestFilters.rateLimitId),
          }
        : undefined,
    };
  }

  async metricFindQuery(query: AuthProxyVariableQuery): Promise<MetricFindValue[]> {
    const now = dateTime();
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
          variable: query,
        },
      ],
      timezone: 'browser',
    } as DataQueryRequest<AuthProxyQuery>;

    const response = (await lastValueFrom(this.query(request))) as DataQueryResponse;
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
