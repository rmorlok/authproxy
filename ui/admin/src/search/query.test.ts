import {describe, expect, it} from 'vitest';
import {directSearchDestination, parseSearchQuery} from './query';

describe('search query parser', () => {
    it.each([
        ['act_test', '/actors/act_test'],
        ['cxn_test', '/connections/cxn_test'],
        ['cxr_test', '/connectors/cxr_test'],
        ['req_test', '/requests/req_test'],
        ['key_global', '/keys/key_global'],
        ['rl_test', '/rate-limits/rl_test'],
    ])('routes direct id %s', (input, path) => {
        expect(directSearchDestination(input)).toEqual({kind: 'resource', path});
    });

    it('routes namespace paths directly', () => {
        expect(directSearchDestination('root.acme_team')).toEqual({
            kind: 'namespace',
            path: '/namespace',
            namespace: 'root.acme_team',
        });
    });

    it('parses and normalizes qualifiers', () => {
        expect(parseSearchQuery(
            'type:connections type:connection type:rate-limit label:env=prod label:team!=old scope:all payments',
        )).toMatchObject({
            text: 'payments',
            resourceTypes: ['connection', 'rate_limit'],
            labelSelector: 'env=prod,team!=old',
            scope: 'all',
            error: null,
        });
    });

    it.each([
        ['type:unknown', 'Unknown resource type'],
        ['scope:nearby', 'Unknown scope'],
        ['scope:all scope:current', 'Conflicting scope'],
        ['label:', 'requires a selector'],
        ['label:bad$key=value', 'Invalid label key'],
        ['label:bad_prefix.example/name=value', 'Invalid label key'],
        ['label:apxy/name=value', 'Invalid label key'],
        ['label:' + 'a'.repeat(64) + '=value', 'Invalid label key'],
        ['owner:me', 'Unknown qualifier'],
    ])('rejects invalid query %s', (input, message) => {
        expect(parseSearchQuery(input).error).toContain(message);
    });

    it('uses the extended system-label value limit', () => {
        const namespacePath = 'root.' + 'a'.repeat(70);
        expect(parseSearchQuery('label:apxy/cxn/-/ns=' + namespacePath).error).toBeNull();
        expect(parseSearchQuery('label:team=' + namespacePath).error).toContain('Invalid label value');
    });
});
