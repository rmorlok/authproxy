import {beforeEach, describe, expect, it, vi} from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const patchMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
    client: {get: getMock, post: postMock, patch: patchMock},
}));

import {createActor, updateActor} from './actors';
import {initiateConnection, updateConnection} from './connections';
import {updateConnector} from './connectors';
import {createKey, listKeys, updateKey} from './keys';
import {listNamespaces} from './namespaces';
import {
    createRateLimit,
    RATE_LIMIT_API_VERSION,
    RATE_LIMIT_KIND,
    RateLimitMode,
    updateRateLimit,
} from './rateLimits';

describe('resource name contracts', () => {
    beforeEach(() => {
        getMock.mockReset();
        postMock.mockReset();
        patchMock.mockReset();
    });

    it('sends optional names on create requests', () => {
        createActor({namespace: 'root', externalId: 'customer-1', name: 'customer'});
        initiateConnection('cxr_test', '/return', {env: 'prod'}, 'production-crm');
        createKey({namespace: 'root', name: 'primary-key'});
        createRateLimit({
            apiVersion: RATE_LIMIT_API_VERSION,
            kind: RATE_LIMIT_KIND,
            metadata: {namespace: 'root', name: 'public-api'},
            spec: {
                mode: RateLimitMode.ENFORCE,
                selector: {},
                bucket: {},
                algorithm: {fixedWindow: {window: '1m', limit: 10}},
            },
        });

        expect(postMock).toHaveBeenCalledWith('/api/v1/actors', expect.objectContaining({name: 'customer'}));
        expect(postMock).toHaveBeenCalledWith('/api/v1/connections/_initiate', expect.objectContaining({name: 'production-crm'}));
        expect(postMock).toHaveBeenCalledWith('/api/v1/keys', expect.objectContaining({name: 'primary-key'}));
        expect(postMock).toHaveBeenCalledWith('/api/v1/rate-limits', expect.objectContaining({
            apiVersion: RATE_LIMIT_API_VERSION,
            kind: RATE_LIMIT_KIND,
            metadata: expect.objectContaining({name: 'public-api'}),
        }));
    });

    it('renames resources by immutable id', () => {
        updateActor('act_test', {name: 'actor-name'});
        updateConnection('cxn_test', {name: 'connection-name'});
        updateConnector('cxr_test', {name: 'connector-name'});
        updateKey('key_test', {name: 'key-name'});
        updateRateLimit('rl_test', {
            apiVersion: RATE_LIMIT_API_VERSION,
            kind: RATE_LIMIT_KIND,
            metadata: {name: 'limit-name'},
            spec: {},
        });

        expect(patchMock).toHaveBeenCalledWith('/api/v1/actors/act_test', {name: 'actor-name'});
        expect(patchMock).toHaveBeenCalledWith('/api/v1/connections/cxn_test', {name: 'connection-name'});
        expect(patchMock).toHaveBeenCalledWith('/api/v1/connectors/cxr_test', {name: 'connector-name'});
        expect(patchMock).toHaveBeenCalledWith('/api/v1/keys/key_test', {name: 'key-name'});
        expect(patchMock).toHaveBeenCalledWith('/api/v1/rate-limits/rl_test', {
            apiVersion: RATE_LIMIT_API_VERSION,
            kind: RATE_LIMIT_KIND,
            metadata: {name: 'limit-name'},
            spec: {},
        });
    });

    it('passes exact-name list filters without replacing ids', () => {
        listKeys({name: 'shared', namespace: 'root.**'});
        listNamespaces({name: 'team'});

        expect(getMock).toHaveBeenCalledWith('/api/v1/keys', {params: {name: 'shared', namespace: 'root.**'}});
        expect(getMock).toHaveBeenCalledWith('/api/v1/namespaces', {params: {name: 'team'}});
    });
});
