import {describe, expect, it} from 'vitest';
import {adminNavigationItems, matchingNavigationItems} from './navigation';

describe('Admin navigation catalog', () => {
    it('keeps the existing Settings sidebar item out of palette results', () => {
        expect(adminNavigationItems.some((item) => item.label === 'Settings')).toBe(true);
        expect(matchingNavigationItems('').some((item) => item.label === 'Settings')).toBe(false);
        expect(matchingNavigationItems('settings')).toEqual([]);
    });

    it('finds commands by their local keywords', () => {
        expect(matchingNavigationItems('jobs').map((item) => item.label)).toEqual(['Internal Tasks']);
        expect(matchingNavigationItems('integrations').map((item) => item.label)).toEqual(['Connectors']);
        expect(matchingNavigationItems('namespace').map((item) => item.label)).toEqual(['Namespaces']);
    });

    it('keeps internal tasks and workflows at the bottom of the main navigation', () => {
        expect(adminNavigationItems.filter((item) => item.section === 'main').slice(-2).map((item) => [
            item.label,
            item.path,
        ])).toEqual([
            ['Internal Tasks', '/internal-tasks'],
            ['Internal Workflows', '/internal-workflows'],
        ]);
    });
});
