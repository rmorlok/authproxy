import {describe, expect, it} from 'vitest';
import {
  editableReservedKeys,
  mapToRows,
  rowsToMap,
  SYSTEM_LABEL_PREFIX,
} from './KeyValueRowsEditor';

describe('KeyValueRowsEditor helpers', () => {
  it('marks system-managed labels readonly and omits them from user payloads', () => {
    const rows = mapToRows({
      environment: 'dev',
      'apxy/key/-/id': 'key_test',
    }, {readonlyKeyPrefix: SYSTEM_LABEL_PREFIX});

    expect(rows.find(row => row.key === 'environment')?.readonly).toBe(false);
    expect(rows.find(row => row.key === 'apxy/key/-/id')?.readonly).toBe(true);
    expect(rowsToMap(rows, {includeReadonly: false})).toEqual({
      environment: 'dev',
    });
  });

  it('reports only editable reserved labels', () => {
    const rows = mapToRows({
      environment: 'dev',
      'apxy/key/-/id': 'key_test',
    }, {readonlyKeyPrefix: SYSTEM_LABEL_PREFIX});
    rows.push({
      id: 'new-row',
      key: 'apxy/key/manual',
      value: 'bad',
    });

    expect(editableReservedKeys(rows, SYSTEM_LABEL_PREFIX)).toEqual(['apxy/key/manual']);
  });
});
