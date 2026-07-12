import {describe, expect, it} from 'vitest';
import type {SearchResourceSummary, SearchResourceType} from '@authproxy/api';
import {
    filterCachedResources,
    mergeSearchResults,
    SEARCH_CACHE_MAX_ENTRIES,
    SEARCH_CACHE_TTL_MS,
    SearchResourceCache,
} from './cache';
import {parseSearchQuery} from './query';

function resource(
    id: string,
    type: SearchResourceType = 'connection',
    labels: Record<string, string> = {},
): SearchResourceSummary {
    return {
        resource_type: type,
        resource_id: id,
        namespace: 'root.team',
        labels,
        matched_labels: [],
        updated_at: '2026-07-12T12:00:00Z',
    };
}

describe('SearchResourceCache', () => {
    it('expires entries and seed completeness', () => {
        const cache = new SearchResourceCache();
        cache.put('current:root', [resource('cxn_one')], 100);
        cache.markSeeded('current:root', [], 100);
        expect(cache.list('current:root', 100)).toHaveLength(1);
        expect(cache.isComplete('current:root', ['connection'], 100)).toBe(true);
        expect(cache.list('current:root', 100 + SEARCH_CACHE_TTL_MS)).toHaveLength(0);
        expect(cache.needsSeed('current:root', 100 + SEARCH_CACHE_TTL_MS)).toBe(true);
    });

    it('uses a hard LRU bound', () => {
        const cache = new SearchResourceCache();
        for (let i = 0; i <= SEARCH_CACHE_MAX_ENTRIES; i += 1) {
            cache.put('all', [resource('cxn_' + i)], 100 + i);
        }
        expect(cache.size).toBe(SEARCH_CACHE_MAX_ENTRIES);
        expect(cache.list('all', 1000).some((item) => item.resource_id === 'cxn_0')).toBe(false);
    });

    it('refreshes entries when their scope is read', () => {
        const cache = new SearchResourceCache();
        cache.put('keep', [resource('cxn_keep')], 100);
        for (let i = 0; i < SEARCH_CACHE_MAX_ENTRIES - 1; i += 1) {
            cache.put('other', [resource('cxn_' + i)], 100 + i);
        }

        expect(cache.list('keep', 1000)).toHaveLength(1);
        cache.put('other', [resource('cxn_new')], 1000);

        expect(cache.list('keep', 1000).map((item) => item.resource_id)).toEqual(['cxn_keep']);
    });

    it('invalidates seed completeness when LRU eviction removes scope entries', () => {
        const cache = new SearchResourceCache();
        const firstScope = Array.from({length: 300}, (_, index) => resource('cxn_first_' + index));
        const secondScope = Array.from({length: 300}, (_, index) => resource('cxn_second_' + index));
        cache.put('first', firstScope, 100);
        cache.markSeeded('first', [], 100);
        expect(cache.isComplete('first', ['connection'], 100)).toBe(true);

        cache.put('second', secondScope, 100);

        expect(cache.isComplete('first', ['connection'], 100)).toBe(false);
    });
});

describe('local search', () => {
    it('filters type, label selector, and text with stable scoring', () => {
        const items = [
            resource('cxn_exact', 'connection', {name: 'payments', env: 'prod'}),
            resource('cxn_prefix', 'connection', {name: 'payments-api', env: 'prod'}),
            resource('act_other', 'actor', {name: 'payments', env: 'prod'}),
        ];
        const parsed = parseSearchQuery('type:connection label:env=prod payments');
        expect(filterCachedResources(items, parsed).map((item) => item.resource_id))
            .toEqual(['cxn_exact', 'cxn_prefix']);
    });

    it('deduplicates local and remote results', () => {
        const local = [resource('cxn_one'), resource('cxn_two')];
        const remote = [resource('cxn_two'), resource('cxn_three')];
        expect(mergeSearchResults(local, remote).map((item) => item.resource_id))
            .toEqual(['cxn_two', 'cxn_three', 'cxn_one']);
    });

    it('drops stale cached tails after a complete remote response', () => {
        const local = [resource('cxn_stale'), resource('act_partial', 'actor')];
        const remote = [resource('cxn_current')];

        expect(mergeSearchResults(local, remote, []).map((item) => item.resource_id))
            .toEqual(['cxn_current']);
        expect(mergeSearchResults(local, remote, ['actor']).map((item) => item.resource_id))
            .toEqual(['cxn_current', 'act_partial']);
    });

    it('does not evaluate system-label selectors against user-only cache data', () => {
        const items = [resource('cxn_one', 'connection', {name: 'payments'})];

        expect(filterCachedResources(items, parseSearchQuery('label:!apxy/cxn/-/ns'))).toEqual([]);
        expect(filterCachedResources(items, parseSearchQuery('label:apxy/cxn/-/ns!=root'))).toEqual([]);
    });

    it('caps merged results at 50 entries', () => {
        const local = Array.from({length: 75}, (_, index) => resource('cxn_' + index));
        expect(mergeSearchResults(local, [])).toHaveLength(50);
    });
});
