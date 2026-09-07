import type {ManagedResourceKind, SearchResourceType, SearchResult} from '@authproxy/api';
import {labelSelectorUsesSystemLabels, type ParsedSearchQuery} from './query';

export const SEARCH_CACHE_TTL_MS = 5 * 60 * 1000;
export const SEARCH_CACHE_MAX_ENTRIES = 500;
export const SEARCH_RESULT_LIMIT = 50;

interface CacheEntry {
    item: SearchResult;
    scopeKey: string;
    expiresAt: number;
}

interface SeedState {
    expiresAt: number;
    truncatedTypes: Set<SearchResourceType>;
}

export class SearchResourceCache {
    private entries = new Map<string, CacheEntry>();
    private seeds = new Map<string, SeedState>();

    put(scopeKey: string, items: SearchResult[], now = Date.now()): void {
        for (const item of items) {
            const key = this.key(scopeKey, item);
            this.entries.delete(key);
            this.entries.set(key, {item, scopeKey, expiresAt: now + SEARCH_CACHE_TTL_MS});
        }
        this.prune(now);
    }

    markSeeded(scopeKey: string, truncatedTypes: SearchResourceType[], now = Date.now()): void {
        this.seeds.delete(scopeKey);
        this.seeds.set(scopeKey, {
            expiresAt: now + SEARCH_CACHE_TTL_MS,
            truncatedTypes: new Set(truncatedTypes),
        });
        while (this.seeds.size > SEARCH_CACHE_MAX_ENTRIES) {
            const oldest = this.seeds.keys().next().value as string | undefined;
            if (!oldest) break;
            this.seeds.delete(oldest);
        }
    }

    needsSeed(scopeKey: string, now = Date.now()): boolean {
        const state = this.seeds.get(scopeKey);
        if (state && state.expiresAt <= now) {
            this.seeds.delete(scopeKey);
            return true;
        }
        return !state;
    }

    isComplete(scopeKey: string, types: SearchResourceType[], now = Date.now()): boolean {
        const state = this.seeds.get(scopeKey);
        if (!state || state.expiresAt <= now) {
            if (state) this.seeds.delete(scopeKey);
            return false;
        }
        return types.every((type) => !state.truncatedTypes.has(type));
    }

    list(scopeKey: string, now = Date.now()): SearchResult[] {
        this.prune(now);
        const prefix = `${scopeKey}|`;
        const result: SearchResult[] = [];
        const touched: Array<[string, CacheEntry]> = [];
        for (const [key, entry] of this.entries) {
            if (key.startsWith(prefix)) {
                result.push(entry.item);
                touched.push([key, entry]);
            }
        }
        for (const [key, entry] of touched) {
            this.entries.delete(key);
            this.entries.set(key, entry);
        }
        return result;
    }

    clear(): void {
        this.entries.clear();
        this.seeds.clear();
    }

    get size(): number {
        return this.entries.size;
    }

    private prune(now: number): void {
        for (const [key, entry] of this.entries) {
            if (entry.expiresAt <= now) {
                this.entries.delete(key);
                this.seeds.delete(entry.scopeKey);
            }
        }
        while (this.entries.size > SEARCH_CACHE_MAX_ENTRIES) {
            const oldest = this.entries.keys().next().value as string | undefined;
            if (!oldest) break;
            const evicted = this.entries.get(oldest);
            this.entries.delete(oldest);
            if (evicted) this.seeds.delete(evicted.scopeKey);
        }
    }

    private key(scopeKey: string, item: SearchResult): string {
        return `${scopeKey}|${item.resourceRef.kind}|${searchResultId(item)}`;
    }
}

export function filterCachedResources(
    items: SearchResult[],
    parsed: ParsedSearchQuery,
): SearchResult[] {
    if (parsed.error || parsed.direct) {
        return [];
    }
    // Seed responses intentionally omit apxy/* labels, so positive and
    // negative system-label selectors cannot be evaluated from cache without
    // false positives or false negatives. Wait for the authoritative server
    // result instead.
    if (labelSelectorUsesSystemLabels(parsed.labelSelector)) {
        return [];
    }
    const types = new Set(parsed.resourceTypes);
    const needle = parsed.text.toLowerCase();
    const filtered = items.filter((item) => {
        if (types.size > 0 && !types.has(searchResultType(item))) {
            return false;
        }
        if (parsed.labelSelector && !matchesLabelSelector(item.labels, parsed.labelSelector)) {
            return false;
        }
        if (!needle) {
            return true;
        }
        return searchableValues(item).some((value) => value.toLowerCase().includes(needle));
    });

    return filtered
        .map((item) => ({item, score: localScore(item, needle)}))
        .sort((a, b) => b.score - a.score || Date.parse(b.item.updatedAt) - Date.parse(a.item.updatedAt) ||
            searchResultType(a.item).localeCompare(searchResultType(b.item)) ||
            searchResultId(a.item).localeCompare(searchResultId(b.item)))
        .map(({item}) => item)
        .slice(0, SEARCH_RESULT_LIMIT);
}

export function mergeSearchResults(
    local: SearchResult[],
    remote: SearchResult[],
    retainLocalTypes?: SearchResourceType[],
): SearchResult[] {
    const seen = new Set<string>();
    const result: SearchResult[] = [];
    // Once the server responds, its cross-type rank is authoritative. Cached
    // entries that were not returned remotely remain useful as a best-effort
    // tail when a type was truncated or incomplete.
    const retainedTypes = retainLocalTypes === undefined ? null : new Set(retainLocalTypes);
    const retainedLocal = retainedTypes === null
        ? local
        : local.filter((item) => retainedTypes.has(searchResultType(item)));
    for (const item of [...remote, ...retainedLocal]) {
        const key = `${item.resourceRef.kind}|${searchResultId(item)}`;
        if (seen.has(key)) continue;
        seen.add(key);
        result.push(item);
        if (result.length === SEARCH_RESULT_LIMIT) break;
    }
    return result;
}

function searchableValues(item: SearchResult): string[] {
    return [searchResultName(item), ...Object.values(item.labels)];
}

function localScore(item: SearchResult, needle: string): number {
    if (!needle) return 0;
    const name = searchResultName(item).toLowerCase();
    if (name === needle) return 3;
    if (name.startsWith(needle)) return 2;
    if (name.includes(needle)) return 1;
    for (const value of Object.values(item.labels).map((value) => value.toLowerCase())) {
        if (value.includes(needle)) return 1;
    }
    return 0;
}

const RESOURCE_TYPES_BY_KIND: Record<ManagedResourceKind, SearchResourceType> = {
    Actor: 'actor',
    Connection: 'connection',
    Connector: 'connector',
    Namespace: 'namespace',
    Key: 'key',
    RateLimit: 'rate_limit',
};

export function searchResourceTypeFromKind(kind: ManagedResourceKind): SearchResourceType {
    return RESOURCE_TYPES_BY_KIND[kind];
}

export function searchResultType(item: SearchResult): SearchResourceType {
    return searchResourceTypeFromKind(item.resourceRef.kind);
}

export function searchResultId(item: SearchResult): string {
    return item.resourceRef.id || item.resourceRef.name || '';
}

export function searchResultName(item: SearchResult): string {
    return item.resourceRef.name || searchResultId(item);
}

export function searchResultNamespace(item: SearchResult): string | undefined {
    return item.resourceRef.namespace;
}

function matchesLabelSelector(labels: Record<string, string>, selector: string): boolean {
    return selector.split(',').every((fragment) => {
        if (fragment.startsWith('!')) {
            return !(fragment.slice(1) in labels);
        }
        for (const operator of ['!=', '==', '=']) {
            const index = fragment.indexOf(operator);
            if (index >= 0) {
                const key = fragment.slice(0, index);
                const expected = fragment.slice(index + operator.length);
                if (operator === '!=') return labels[key] !== expected;
                return labels[key] === expected;
            }
        }
        return fragment in labels;
    });
}
