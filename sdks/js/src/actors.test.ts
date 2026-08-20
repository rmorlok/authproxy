import {beforeEach, describe, expect, it, vi} from 'vitest';

const patchMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  client: {patch: patchMock},
}));

import {updateActor, type Permission} from './actors';

describe('actor permission contracts', () => {
  beforeEach(() => {
    patchMock.mockReset();
  });

  it('sends permissions when updating actors', () => {
    const permissions: Permission[] = [{
      namespace: 'root.tenant.**',
      resources: ['connections'],
      resourceIds: ['cxn_example'],
      verbs: ['get', 'proxy'],
    }];

    updateActor('act_test', {permissions});

    expect(patchMock).toHaveBeenCalledWith('/api/v1/actors/act_test', {permissions});
  });
});
