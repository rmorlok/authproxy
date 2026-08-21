import {describe, expect, it, vi} from 'vitest';

vi.mock('@mui/x-data-grid', () => ({
    DataGrid: () => null,
}));

import {columns} from './Actors';

describe('Actors table columns', () => {
    it('shows current actor fields', () => {
        expect(columns.map(({field}) => field)).toEqual([
            'name',
            'id',
            'externalId',
            'namespace',
            'createdAt',
            'updatedAt',
        ]);
    });
});
