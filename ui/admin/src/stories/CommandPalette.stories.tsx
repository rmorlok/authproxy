import * as React from 'react';
import type {Meta, StoryObj} from '@storybook/react';
import {Provider} from 'react-redux';
import {configureStore} from '@reduxjs/toolkit';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import type {
    SearchResourceSummary,
    SearchResourcesParams,
    SearchResourcesResponse,
} from '@authproxy/api';
import namespaceReducer from '../store/namespacesSlice';
import {CommandPaletteProvider, useCommandPalette} from '../search/CommandPalette';
import {SearchResourceCache} from '../search/cache';

const connection = resource('connection', 'cxn_payments', 'payments-production', {env: 'prod', team: 'billing'});
const connector = resource('connector', 'cxr_stripe', 'Stripe', {category: 'payments'});
const actor = resource('actor', 'act_finance', 'finance-service', {team: 'billing'});
const namespace = resource('namespace', 'root.platform', 'Platform', {team: 'platform'}, 'root.platform');

interface PaletteStoryProps {
    initialQuery: string;
    seed: SearchResourcesResponse;
    query?: SearchResourcesResponse;
}

function PaletteStory({initialQuery, seed, query = emptyResponse()}: PaletteStoryProps) {
    const store = React.useMemo(() => configureStore({
        reducer: {namespaces: namespaceReducer},
        preloadedState: {
            namespaces: {
                currentPath: 'root',
                hasInitialized: true,
                current: null,
                status: 'succeeded' as const,
                error: null,
                children: [],
                childrenHasMore: false,
                childrenStatus: 'succeeded' as const,
                childrenError: null,
            },
        },
    }), []);
    const cache = React.useMemo(() => new SearchResourceCache(), []);
    const search = React.useCallback((params: SearchResourcesParams) => Promise.resolve({
        status: 200,
        statusText: 'OK',
        headers: {},
        config: {},
        data: params.mode === 'seed' ? seed : query,
    } as any), [query, seed]);

    return (
        <Provider store={store}>
            <CommandPaletteProvider cache={cache} debounceMs={0} search={search}>
                <StoryBackdrop />
                <OpenOnMount query={initialQuery} />
            </CommandPaletteProvider>
        </Provider>
    );
}

function StoryBackdrop() {
    return (
        <Box sx={{bgcolor: 'background.default', minHeight: '100vh', p: 4}}>
            <Typography variant="h4">AuthProxy Admin</Typography>
            <Typography color="text.secondary">Command palette visual fixture</Typography>
        </Box>
    );
}

function OpenOnMount({query}: {query: string}) {
    const palette = useCommandPalette();
    React.useEffect(() => palette.open(query), [palette, query]);
    return null;
}

const meta = {
    title: 'Admin/Command Palette',
    component: PaletteStory,
    parameters: {layout: 'fullscreen'},
} satisfies Meta<typeof PaletteStory>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {
    args: {
        initialQuery: 'nothing-matches',
        seed: emptyResponse(),
    },
};

export const Populated: Story = {
    args: {
        initialQuery: 'pay',
        seed: response([connection, connector, actor, namespace]),
    },
};

export const StructuredQuery: Story = {
    args: {
        initialQuery: 'type:connection label:env=prod payments',
        seed: response([connection, connector, actor, namespace]),
    },
};

export const TruncatedAndIncomplete: Story = {
    args: {
        initialQuery: 'payments',
        seed: response([connection], ['connection']),
        query: response([connection, connector], ['connection'], ['connector']),
    },
};

function resource(
    resourceType: SearchResourceSummary['resource_type'],
    resourceId: string,
    name: string,
    labels: Record<string, string>,
    resourceNamespace = 'root.acme',
): SearchResourceSummary {
    return {
        resource_type: resourceType,
        resource_id: resourceId,
        namespace: resourceNamespace,
        labels: {name, ...labels},
        matched_labels: [],
        updated_at: '2026-07-12T12:00:00Z',
    };
}

function response(
    items: SearchResourceSummary[],
    truncatedTypes: SearchResourcesResponse['truncated_types'] = [],
    incompleteTypes: SearchResourcesResponse['incomplete_types'] = [],
): SearchResourcesResponse {
    return {
        items,
        truncated_types: truncatedTypes,
        incomplete_types: incompleteTypes,
    };
}

function emptyResponse(): SearchResourcesResponse {
    return response([]);
}
