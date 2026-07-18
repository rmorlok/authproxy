import {beforeEach, describe, expect, it, vi} from 'vitest';

const getMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
    client: {get: getMock},
}));

import {searchResources} from './search';

describe('searchResources', () => {
    beforeEach(() => getMock.mockReset());

    it('serializes repeated resource types and forwards abort configuration', () => {
        const controller = new AbortController();
        searchResources({
            mode: 'query',
            resource_type: ['connection', 'namespace'],
            q: 'payments',
        }, {signal: controller.signal});

        expect(getMock).toHaveBeenCalledWith('/api/v1/search/resources', {
            signal: controller.signal,
            params: {
                mode: 'query',
                resource_type: ['connection', 'namespace'],
                q: 'payments',
            },
            paramsSerializer: {indexes: null},
        });
    });

    it('preserves a caller-provided params serializer', () => {
        const paramsSerializer = vi.fn(() => 'resource_type=connection');
        searchResources({resource_type: ['connection']}, {paramsSerializer});

        expect(getMock).toHaveBeenCalledWith('/api/v1/search/resources', expect.objectContaining({
            paramsSerializer,
        }));
    });
});
