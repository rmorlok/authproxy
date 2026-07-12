import type {SearchResourceSummary, SearchResourceType} from '@authproxy/api';
import {labelSelectorUsesSystemLabels, type ParsedSearchQuery} from './query';

export const SEARCH_CACHE_TTL_MS = 5 * 60 * 1000;
export const SEARCH_CACHE_MAX_ENTRIES = 500;
export const SEARCH_RESULT_LIMIT = 50;

interface CacheEntry {
    item: SearchResourceSummary;
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

    put(scopeKey: string, items: SearchResourceSummary[], now = Date.now()): void {
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

    list(scopeKey: string, now = Date.now()): SearchResourceSummary[] {
        this.prune(now);
        const prefix = `${scopeKey}|`;
        const result: SearchResourceSummary[] = [];
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

    private key(scopeKey: string, item: SearchResourceSummary): string {
        return `${scopeKey}|${item.resource_type}|${item.resource_id}`;
    }
}

export function filterCachedResources(
    items: SearchResourceSummary[],
    parsed: ParsedSearchQuery,
): SearchResourceSummary[] {
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
        if (types.size > 0 && !types.has(item.resource_type)) {
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
        .sort((a, b) => b.score - a.score || Date.parse(b.item.updated_at) - Date.parse(a.item.updated_at) ||
            a.item.resource_type.localeCompare(b.item.resource_type) ||
            a.item.resource_id.localeCompare(b.item.resource_id))
        .map(({item}) => item)
        .slice(0, SEARCH_RESULT_LIMIT);
}

export function mergeSearchResults(
    local: SearchResourceSummary[],
    remote: SearchResourceSummary[],
    retainLocalTypes?: SearchResourceType[],
): SearchResourceSummary[] {
    const seen = new Set<string>();
    const result: SearchResourceSummary[] = [];
    // Once the server responds, its cross-type rank is authoritative. Cached
    // entries that were not returned remotely remain useful as a best-effort
    // tail when a type was truncated or incomplete.
    const retainedTypes = retainLocalTypes === undefined ? null : new Set(retainLocalTypes);
    const retainedLocal = retainedTypes === null
        ? local
        : local.filter((item) => retainedTypes.has(item.resource_type));
    for (const item of [...remote, ...retainedLocal]) {
        const key = `${item.resource_type}|${item.resource_id}`;
        if (seen.has(key)) continue;
        seen.add(key);
        result.push(item);
        if (result.length === SEARCH_RESULT_LIMIT) break;
    }
    return result;
}

function searchableValues(item: SearchResourceSummary): string[] {
    return Object.values(item.labels);
}

function localScore(item: SearchResourceSummary, needle: string): number {
    if (!needle) return 0;
    let score = 0;
    for (const value of searchableValues(item).map((value) => value.toLowerCase())) {
        if (value === needle) score = Math.max(score, 3);
        else if (value.startsWith(needle)) score = Math.max(score, 2);
        else if (value.includes(needle)) score = Math.max(score, 1);
    }
    return score;
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
