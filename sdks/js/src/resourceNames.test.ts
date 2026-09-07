import {beforeEach, describe, expect, it, vi} from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const patchMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
    client: {get: getMock, post: postMock, patch: patchMock},
}));

import {ACTOR_KIND, createActor, updateActor} from './actors';
import {API_VERSION, objectReference} from './common';
import {initiateConnection, updateConnection} from './connections';
import {CONNECTOR_KIND, updateConnector} from './connectors';
import {createKey, KEY_KIND, KeyState, listKeys, updateKey} from './keys';
import {clearNamespaceKey, listNamespaces, setNamespaceKey} from './namespaces';
import {
    createRateLimit,
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
        createActor({
            apiVersion: API_VERSION,
            kind: ACTOR_KIND,
            metadata: {namespace: 'root', name: 'customer'},
            spec: {externalId: 'customer-1'},
        });
        initiateConnection(objectReference(CONNECTOR_KIND, {id: 'cxr_test'}), {
            returnToUrl: '/return',
            labels: {env: 'prod'},
            name: 'production-crm',
        });
        createKey({
            apiVersion: API_VERSION,
            kind: 'Key',
            metadata: {namespace: 'root', name: 'primary-key'},
            spec: {keyData: {numBytes: 32}},
        });
        createRateLimit({
            apiVersion: API_VERSION,
            kind: RATE_LIMIT_KIND,
            metadata: {namespace: 'root', name: 'public-api'},
            spec: {
                mode: RateLimitMode.ENFORCE,
                selector: {},
                bucket: {},
                algorithm: {fixedWindow: {window: '1m', limit: 10}},
            },
        });

        expect(postMock).toHaveBeenCalledWith('/api/v1/actors', expect.objectContaining({
            apiVersion: API_VERSION,
            kind: ACTOR_KIND,
            metadata: expect.objectContaining({name: 'customer'}),
        }));
        expect(postMock).toHaveBeenCalledWith('/api/v1/connections/_initiate', expect.objectContaining({
            apiVersion: API_VERSION,
            kind: 'ConnectionInitiate',
            spec: expect.objectContaining({name: 'production-crm'}),
        }));
        expect(postMock).toHaveBeenCalledWith('/api/v1/keys', expect.objectContaining({
            metadata: expect.objectContaining({name: 'primary-key'}),
        }));
        expect(postMock).toHaveBeenCalledWith('/api/v1/rate-limits', expect.objectContaining({
            apiVersion: API_VERSION,
            kind: RATE_LIMIT_KIND,
            metadata: expect.objectContaining({name: 'public-api'}),
        }));
    });

    it('renames resources by immutable id', () => {
        updateActor('act_test', {
            apiVersion: API_VERSION,
            kind: ACTOR_KIND,
            metadata: {name: 'actor-name'},
            spec: {},
        });
        updateConnection('cxn_test', {
            apiVersion: API_VERSION,
            kind: 'Connection',
            metadata: {name: 'connection-name'},
            spec: {},
        });
        updateConnector('cxr_test', {
            apiVersion: API_VERSION,
            kind: CONNECTOR_KIND,
            metadata: {name: 'connector-name'},
            spec: {},
        });
        updateKey('key_test', {
            apiVersion: API_VERSION,
            kind: 'Key',
            metadata: {name: 'key-name'},
            spec: {desiredState: KeyState.ACTIVE},
        });
        updateRateLimit('rl_test', {
            apiVersion: API_VERSION,
            kind: RATE_LIMIT_KIND,
            metadata: {name: 'limit-name'},
            spec: {},
        });

        expect(patchMock).toHaveBeenCalledWith('/api/v1/actors/act_test', {
            apiVersion: API_VERSION,
            kind: ACTOR_KIND,
            metadata: {name: 'actor-name'},
            spec: {},
        });
        expect(patchMock).toHaveBeenCalledWith('/api/v1/connections/cxn_test', expect.objectContaining({
            apiVersion: API_VERSION,
            kind: 'Connection',
            metadata: {name: 'connection-name'},
            spec: {},
        }));
        expect(patchMock).toHaveBeenCalledWith('/api/v1/connectors/cxr_test', expect.objectContaining({
            apiVersion: API_VERSION,
            kind: CONNECTOR_KIND,
            metadata: {name: 'connector-name'},
            spec: {},
        }));
        expect(patchMock).toHaveBeenCalledWith('/api/v1/keys/key_test', expect.objectContaining({
            apiVersion: API_VERSION,
            kind: 'Key',
            metadata: {name: 'key-name'},
        }));
        expect(patchMock).toHaveBeenCalledWith('/api/v1/rate-limits/rl_test', {
            apiVersion: API_VERSION,
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

    it('updates namespace key assignment through the canonical resource patch', () => {
        setNamespaceKey('root.acme', objectReference(KEY_KIND, {id: 'key_test'}));
        clearNamespaceKey('root.acme');

        expect(patchMock).toHaveBeenNthCalledWith(1, '/api/v1/namespaces/root.acme', {
            apiVersion: API_VERSION,
            kind: 'Namespace',
            metadata: {},
            spec: {
                encryptionKeyRef: {
                    apiVersion: API_VERSION,
                    kind: KEY_KIND,
                    id: 'key_test',
                },
            },
        });
        expect(patchMock).toHaveBeenNthCalledWith(2, '/api/v1/namespaces/root.acme', {
            apiVersion: API_VERSION,
            kind: 'Namespace',
            metadata: {},
            spec: {encryptionKeyRef: null},
        });
    });
});
