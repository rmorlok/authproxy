import React from 'react';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { InlineField, Input, Select } from '@grafana/ui';

import { DataSource } from './DataSource';
import { AuthProxyDataSourceOptions, AuthProxyQuery, AuthProxyQueryType } from './types';

type Props = QueryEditorProps<DataSource, AuthProxyQuery, AuthProxyDataSourceOptions>;

const queryTypes: Array<SelectableValue<AuthProxyQueryType>> = [
  { label: 'Metrics', value: 'metrics' },
  { label: 'Request events', value: 'request_events' },
];

const aggregations: Array<SelectableValue<string>> = [
  { label: 'Sum', value: 'sum' },
  { label: 'Count', value: 'count' },
  { label: 'Average', value: 'avg' },
  { label: 'Minimum', value: 'min' },
  { label: 'Maximum', value: 'max' },
  { label: 'p50', value: 'p50' },
  { label: 'p95', value: 'p95' },
  { label: 'p99', value: 'p99' },
];

const statusRanges: Array<SelectableValue<string>> = [
  { label: 'Any', value: '' },
  { label: '2xx', value: '2xx' },
  { label: '3xx', value: '3xx' },
  { label: '4xx', value: '4xx' },
  { label: '5xx', value: '5xx' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const update = (patch: Partial<AuthProxyQuery>) => {
    onChange({ ...query, ...patch });
    onRunQuery();
  };
  const queryType = query.queryType ?? 'metrics';

  return (
    <>
      <InlineField label="Mode" labelWidth={16}>
        <Select
          width={28}
          options={queryTypes}
          value={queryTypes.find((option) => option.value === queryType)}
          onChange={(option) => update({ queryType: option.value ?? 'metrics' })}
        />
      </InlineField>
      {queryType === 'request_events' ? (
        <RequestEventsEditor query={query} update={update} />
      ) : (
        <MetricsEditor query={query} update={update} />
      )}
    </>
  );
}

function MetricsEditor({ query, update }: { query: AuthProxyQuery; update: (patch: Partial<AuthProxyQuery>) => void }) {
  return (
    <>
      <InlineField label="Metric" labelWidth={16} grow>
        <Input value={query.metric ?? ''} onChange={(event) => update({ metric: event.currentTarget.value })} />
      </InlineField>
      <InlineField label="Aggregation" labelWidth={16}>
        <Select
          width={24}
          options={aggregations}
          value={aggregations.find((option) => option.value === query.aggregation)}
          onChange={(option) => update({ aggregation: option.value ?? '' })}
        />
      </InlineField>
      <InlineField label="Group by" labelWidth={16} grow>
        <Input
          value={(query.groupBy ?? []).join(', ')}
          placeholder="connector_id, status_code"
          onChange={(event) =>
            update({
              groupBy: event.currentTarget.value
                .split(',')
                .map((item) => item.trim())
                .filter(Boolean),
            })
          }
        />
      </InlineField>
      <CommonFilters query={query} update={update} />
    </>
  );
}

function RequestEventsEditor({
  query,
  update,
}: {
  query: AuthProxyQuery;
  update: (patch: Partial<AuthProxyQuery>) => void;
}) {
  const filters = query.requestFilters ?? {};
  const updateFilters = (patch: Partial<NonNullable<AuthProxyQuery['requestFilters']>>) =>
    update({ requestFilters: { ...filters, ...patch } });

  return (
    <>
      <CommonFilters query={query} update={update} />
      <InlineField label="Method" labelWidth={16}>
        <Input width={16} value={filters.method ?? ''} onChange={(event) => updateFilters({ method: event.currentTarget.value })} />
      </InlineField>
      <InlineField label="Status" labelWidth={16}>
        <Select
          width={16}
          options={statusRanges}
          value={statusRanges.find((option) => option.value === filters.statusCodeRange)}
          onChange={(option) => updateFilters({ statusCodeRange: option.value ?? '' })}
        />
      </InlineField>
      <InlineField label="Connection" labelWidth={16}>
        <Input
          width={28}
          value={filters.connectionId ?? ''}
          onChange={(event) => updateFilters({ connectionId: event.currentTarget.value })}
        />
      </InlineField>
      <InlineField label="Connector" labelWidth={16}>
        <Input
          width={28}
          value={filters.connectorId ?? ''}
          onChange={(event) => updateFilters({ connectorId: event.currentTarget.value })}
        />
      </InlineField>
      <InlineField label="Rate limit" labelWidth={16}>
        <Input
          width={28}
          value={filters.rateLimitId ?? ''}
          onChange={(event) => updateFilters({ rateLimitId: event.currentTarget.value })}
        />
      </InlineField>
      <InlineField label="Path regex" labelWidth={16} grow>
        <Input value={filters.pathRegex ?? ''} onChange={(event) => updateFilters({ pathRegex: event.currentTarget.value })} />
      </InlineField>
    </>
  );
}

function CommonFilters({ query, update }: { query: AuthProxyQuery; update: (patch: Partial<AuthProxyQuery>) => void }) {
  return (
    <>
      <InlineField label="Namespace" labelWidth={16} grow>
        <Input value={query.namespace ?? ''} onChange={(event) => update({ namespace: event.currentTarget.value })} />
      </InlineField>
      <InlineField label="Labels" labelWidth={16} grow>
        <Input
          value={query.labelSelector ?? ''}
          placeholder="env=prod,team=platform"
          onChange={(event) => update({ labelSelector: event.currentTarget.value })}
        />
      </InlineField>
    </>
  );
}
