import {describe, expect, it} from 'vitest';
import {adminNavigationItems, matchingNavigationItems} from './navigation';

describe('Admin navigation catalog', () => {
    it('keeps the existing Settings sidebar item out of palette results', () => {
        expect(adminNavigationItems.some((item) => item.label === 'Settings')).toBe(true);
        expect(matchingNavigationItems('').some((item) => item.label === 'Settings')).toBe(false);
        expect(matchingNavigationItems('settings')).toEqual([]);
    });

    it('finds commands by their local keywords', () => {
        expect(matchingNavigationItems('jobs').map((item) => item.label)).toEqual(['Tasks']);
        expect(matchingNavigationItems('integrations').map((item) => item.label)).toEqual(['Connectors']);
    });
});
