import type {SearchResourceType} from '@authproxy/api';

export type SearchScope = 'current' | 'all';

export interface DirectSearchDestination {
    kind: 'resource' | 'namespace';
    path: string;
    namespace?: string;
}

export interface ParsedSearchQuery {
    raw: string;
    text: string;
    resourceTypes: SearchResourceType[];
    labelSelector: string;
    scope: SearchScope;
    direct: DirectSearchDestination | null;
    error: string | null;
}

const typeAliases: Record<string, SearchResourceType> = {
    actor: 'actor',
    actors: 'actor',
    connection: 'connection',
    connections: 'connection',
    connector: 'connector',
    connectors: 'connector',
    namespace: 'namespace',
    namespaces: 'namespace',
    key: 'key',
    keys: 'key',
    'rate-limit': 'rate_limit',
    'rate-limits': 'rate_limit',
    rate_limit: 'rate_limit',
    rate_limits: 'rate_limit',
};

const directResourceRoutes: Array<[string, string]> = [
    ['act_', '/actors/'],
    ['cxn_', '/connections/'],
    ['cxr_', '/connectors/'],
    ['req_', '/requests/'],
    ['key_', '/keys/'],
    ['rl_', '/rate-limits/'],
];

const namespacePathPattern = /^root(?:\.[A-Za-z0-9_][A-Za-z0-9_-]*)*$/;
const labelNamePattern = /^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,61}[A-Za-z0-9])?$/;
const dnsLabelPattern = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/;
const labelValuePattern = /^$|^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/;

export function directSearchDestination(raw: string): DirectSearchDestination | null {
    const value = raw.trim();
    if (!value || /\s/.test(value)) {
        return null;
    }
    if (namespacePathPattern.test(value)) {
        return {kind: 'namespace', path: '/namespace', namespace: value};
    }
    for (const [prefix, route] of directResourceRoutes) {
        if (value.startsWith(prefix) && value.length > prefix.length) {
            return {kind: 'resource', path: route + encodeURIComponent(value)};
        }
    }
    return null;
}

function validateLabelFragment(fragment: string): string | null {
    if (!fragment) {
        return 'label: requires a selector fragment';
    }
    let key = fragment;
    let value: string | undefined;
    for (const operator of ['!=', '==', '=']) {
        const index = fragment.indexOf(operator);
        if (index >= 0) {
            key = fragment.slice(0, index);
            value = fragment.slice(index + operator.length);
            break;
        }
    }
    if (key.startsWith('!') && value === undefined) {
        key = key.slice(1);
    }
    if (!isValidLabelKey(key)) {
        return 'Invalid label key in "' + fragment + '"';
    }
    if (value !== undefined) {
        const maxLength = key.startsWith('apxy/') ? 253 : 63;
        if (value.length > maxLength || !labelValuePattern.test(value)) {
            return 'Invalid label value in "' + fragment + '"';
        }
    }
    return null;
}

function isValidLabelKey(key: string): boolean {
    if (!key) return false;

    const separator = key.lastIndexOf('/');
    const prefix = separator >= 0 ? key.slice(0, separator) : '';
    const name = separator >= 0 ? key.slice(separator + 1) : key;
    if (!name || name.length > 63 || !labelNamePattern.test(name)) return false;

    if (key.startsWith('apxy/')) {
        if (!prefix || prefix.length > 253) return false;
        const innerPrefix = prefix.slice('apxy/'.length);
        if (!innerPrefix) return false;
        return innerPrefix.split('/').every((segment) =>
            segment === '-' || dnsLabelPattern.test(segment),
        );
    }

    if (separator < 0) return true;
    if (!prefix || prefix.length > 253) return false;
    return prefix.split('.').every((segment) => dnsLabelPattern.test(segment));
}

export function parseSearchQuery(raw: string): ParsedSearchQuery {
    const trimmed = raw.trim();
    const direct = directSearchDestination(trimmed);
    if (direct) {
        return {
            raw,
            text: '',
            resourceTypes: [],
            labelSelector: '',
            scope: 'current',
            direct,
            error: null,
        };
    }

    const resourceTypes: SearchResourceType[] = [];
    const seenTypes = new Set<SearchResourceType>();
    const labelFragments: string[] = [];
    const textTokens: string[] = [];
    let scope: SearchScope = 'current';
    let explicitScope: SearchScope | null = null;

    for (const token of trimmed ? trimmed.split(/\s+/) : []) {
        if (token.startsWith('type:')) {
            const alias = token.slice('type:'.length).toLowerCase();
            const resourceType = typeAliases[alias];
            if (!resourceType) {
                return invalid(raw, 'Unknown resource type "' + alias + '"');
            }
            if (!seenTypes.has(resourceType)) {
                seenTypes.add(resourceType);
                resourceTypes.push(resourceType);
            }
            continue;
        }
        if (token.startsWith('label:')) {
            const fragment = token.slice('label:'.length);
            const validationError = validateLabelFragment(fragment);
            if (validationError) {
                return invalid(raw, validationError);
            }
            labelFragments.push(fragment);
            continue;
        }
        if (token.startsWith('scope:')) {
            const value = token.slice('scope:'.length) as SearchScope;
            if (value !== 'current' && value !== 'all') {
                return invalid(raw, 'Unknown scope "' + value + '"');
            }
            if (explicitScope && explicitScope !== value) {
                return invalid(raw, 'Conflicting scope qualifiers');
            }
            explicitScope = value;
            scope = value;
            continue;
        }
        if (token.includes(':')) {
            return invalid(raw, 'Unknown qualifier "' + token.slice(0, token.indexOf(':')) + '"');
        }
        textTokens.push(token);
    }

    return {
        raw,
        text: textTokens.join(' ').trim(),
        resourceTypes,
        labelSelector: labelFragments.join(','),
        scope,
        direct: null,
        error: null,
    };
}

export function labelSelectorUsesSystemLabels(selector: string): boolean {
    return selector.split(',').some((fragment) => {
        const withoutNegation = fragment.startsWith('!') ? fragment.slice(1) : fragment;
        const operatorIndex = withoutNegation.search(/[!=]/);
        const key = operatorIndex >= 0 ? withoutNegation.slice(0, operatorIndex) : withoutNegation;
        return key.startsWith('apxy/');
    });
}

function invalid(raw: string, error: string): ParsedSearchQuery {
    return {
        raw,
        text: '',
        resourceTypes: [],
        labelSelector: '',
        scope: 'current',
        direct: null,
        error,
    };
}
