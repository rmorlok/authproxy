import React from 'react';
import { SelectableValue } from '@grafana/data';
import { InlineField, Input, Select } from '@grafana/ui';

import { AuthProxyVariableQuery, AuthProxyVariableType } from './types';

interface Props {
  query: AuthProxyVariableQuery;
  onChange: (query: AuthProxyVariableQuery) => void;
}

const variableTypes: Array<SelectableValue<AuthProxyVariableType>> = [
  { label: 'Namespaces', value: 'namespaces' },
  { label: 'Connectors', value: 'connectors' },
  { label: 'Connections', value: 'connections' },
  { label: 'Actors', value: 'actors' },
  { label: 'Rate limits', value: 'rate_limits' },
];

export function VariableQueryEditor({ query, onChange }: Props) {
  const current = query ?? { type: 'namespaces' };
  const update = (patch: Partial<AuthProxyVariableQuery>) => onChange({ ...current, ...patch });

  return (
    <>
      <InlineField label="List" labelWidth={16}>
        <Select
          width={28}
          options={variableTypes}
          value={variableTypes.find((option) => option.value === current.type)}
          onChange={(option) => update({ type: option.value ?? 'namespaces' })}
        />
      </InlineField>
      <InlineField label="Namespace" labelWidth={16} grow>
        <Input value={current.namespace ?? ''} onChange={(event) => update({ namespace: event.currentTarget.value })} />
      </InlineField>
      <InlineField label="Labels" labelWidth={16} grow>
        <Input
          value={current.labelSelector ?? ''}
          placeholder="env=prod,team=platform"
          onChange={(event) => update({ labelSelector: event.currentTarget.value })}
        />
      </InlineField>
      {current.type === 'connections' && (
        <InlineField label="Connector" labelWidth={16}>
          <Input
            width={28}
            value={current.connectorId ?? ''}
            onChange={(event) => update({ connectorId: event.currentTarget.value })}
          />
        </InlineField>
      )}
    </>
  );
}
