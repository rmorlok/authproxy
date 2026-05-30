import { DataSourcePlugin } from '@grafana/data';
import { getTemplateSrv } from '@grafana/runtime';

import { ConfigEditor } from './ConfigEditor';
import { DataSource } from './DataSource';
import { QueryEditor } from './QueryEditor';
import { VariableQueryEditor } from './VariableQueryEditor';
import { AuthProxyDataSourceOptions, AuthProxyQuery } from './types';

export const plugin = new DataSourcePlugin<DataSource, AuthProxyQuery, AuthProxyDataSourceOptions>(
  (settings) => new DataSource(settings, getTemplateSrv())
)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor)
  .setVariableQueryEditor(VariableQueryEditor);
