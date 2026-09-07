import {beforeEach, describe, expect, it, vi} from 'vitest';

const patchMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  client: {patch: patchMock},
}));

import {ACTOR_KIND, updateActor, type Permission, type UpdateActorRequest} from './actors';

const actorPatch = (permissions: Permission[]): UpdateActorRequest => ({
  apiVersion: 'authproxy.net/v1alpha1',
  kind: ACTOR_KIND,
  metadata: {},
  spec: {permissions},
});

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

    const request = actorPatch(permissions);
    updateActor('act_test', request);

    expect(patchMock).toHaveBeenCalledWith('/api/v1/actors/act_test', request);
  });
});
