import React from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { InlineField, Input, SecretInput } from '@grafana/ui';

import { AuthProxyDataSourceOptions, AuthProxySecureJsonData } from './types';

type Props = DataSourcePluginOptionsEditorProps<AuthProxyDataSourceOptions, AuthProxySecureJsonData>;

export function ConfigEditor({ options, onOptionsChange }: Props) {
  const jsonData = options.jsonData ?? {};
  const secureJsonData = options.secureJsonData ?? {};
  const secureJsonFields = options.secureJsonFields ?? {};

  return (
    <>
      <InlineField label="AuthProxy URL" labelWidth={24} grow>
        <Input
          value={jsonData.baseUrl ?? ''}
          placeholder="http://authproxy-api:8081"
          onChange={(event) =>
            onOptionsChange({
              ...options,
              jsonData: { ...jsonData, baseUrl: event.currentTarget.value },
            })
          }
        />
      </InlineField>
      <InlineField label="JWT" labelWidth={24} grow>
        <SecretInput
          value={secureJsonData.jwt ?? ''}
          isConfigured={Boolean(secureJsonFields.jwt)}
          onChange={(event) =>
            onOptionsChange({
              ...options,
              secureJsonData: { ...secureJsonData, jwt: event.currentTarget.value },
            })
          }
          onReset={() =>
            onOptionsChange({
              ...options,
              secureJsonFields: { ...secureJsonFields, jwt: false },
              secureJsonData: { ...secureJsonData, jwt: '' },
            })
          }
        />
      </InlineField>
    </>
  );
}
