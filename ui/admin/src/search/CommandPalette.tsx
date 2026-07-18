import * as React from 'react';
import {Command} from 'cmdk';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import InputAdornment from '@mui/material/InputAdornment';
import SearchRoundedIcon from '@mui/icons-material/SearchRounded';
import {alpha, styled} from '@mui/material/styles';
import Typography from '@mui/material/Typography';
import type {SearchResourceSummary, SearchResourceType} from '@authproxy/api';
import {namespaceAndChildren, searchResources} from '@authproxy/api';
import {useDispatch, useSelector} from 'react-redux';
import {useNavigate} from 'react-router-dom';
import type {AppDispatch, RootState} from '../store';
import {selectCurrentNamespacePath, setCurrentNamespace} from '../store/namespacesSlice';
import {
    filterCachedResources,
    mergeSearchResults,
    SEARCH_RESULT_LIMIT,
    SearchResourceCache,
} from './cache';
import {matchingNavigationItems} from './navigation';
import {labelSelectorUsesSystemLabels, parseSearchQuery} from './query';

const ALL_RESOURCE_TYPES: SearchResourceType[] = [
    'actor',
    'connection',
    'connector',
    'namespace',
    'key',
    'rate_limit',
];

const sessionCache = new SearchResourceCache();

interface CommandPaletteContextValue {
    open: (initialQuery?: string) => void;
    close: () => void;
}

const CommandPaletteContext = React.createContext<CommandPaletteContextValue | null>(null);

export function useCommandPalette(): CommandPaletteContextValue {
    const context = React.useContext(CommandPaletteContext);
    if (!context) {
        throw new Error('useCommandPalette must be used within CommandPaletteProvider');
    }
    return context;
}

interface RemoteState {
    queryKey: string;
    items: SearchResourceSummary[];
    truncatedTypes: SearchResourceType[];
    incompleteTypes: SearchResourceType[];
    loading: boolean;
    error: boolean;
    completed: boolean;
}

const EMPTY_REMOTE_STATE: RemoteState = {
    queryKey: '',
    items: [],
    truncatedTypes: [],
    incompleteTypes: [],
    loading: false,
    error: false,
    completed: false,
};

const CommandRoot = styled(Command)(({theme}) => ({
    color: theme.palette.text.primary,
    background: theme.palette.background.paper,
    width: '100%',
    '.command-palette-input-wrapper': {
        alignItems: 'center',
        borderBottom: `1px solid ${theme.palette.divider}`,
        display: 'flex',
        padding: theme.spacing(1.5, 2),
    },
    '[cmdk-input]': {
        background: 'transparent',
        border: 0,
        color: 'inherit',
        flex: 1,
        font: 'inherit',
        fontSize: theme.typography.h6.fontSize,
        minWidth: 0,
        outline: 'none',
    },
    '[cmdk-list]': {
        maxHeight: 'min(60vh, 520px)',
        overflowY: 'auto',
        overscrollBehavior: 'contain',
        padding: theme.spacing(1),
    },
    '[cmdk-group-heading]': {
        color: theme.palette.text.secondary,
        fontSize: theme.typography.caption.fontSize,
        fontWeight: 700,
        letterSpacing: '0.08em',
        padding: theme.spacing(1, 1.25, 0.5),
        textTransform: 'uppercase',
    },
    '[cmdk-item]': {
        alignItems: 'center',
        borderRadius: theme.shape.borderRadius,
        cursor: 'pointer',
        display: 'flex',
        gap: theme.spacing(1.5),
        minHeight: 48,
        padding: theme.spacing(1, 1.25),
        userSelect: 'none',
    },
    '[cmdk-item][data-selected="true"]': {
        background: alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.22 : 0.1),
    },
}));

interface CommandPaletteProviderProps extends React.PropsWithChildren {
    cache?: SearchResourceCache;
    debounceMs?: number;
    search?: typeof searchResources;
}

export function CommandPaletteProvider({
    children,
    cache = sessionCache,
    debounceMs = 300,
    search = searchResources,
}: CommandPaletteProviderProps) {
    const navigate = useNavigate();
    const dispatch = useDispatch<AppDispatch>();
    const currentNamespace = useSelector(selectCurrentNamespacePath);
    const actorId = useSelector((state: RootState) => state.auth?.actor_id ?? null);
    const [isOpen, setIsOpen] = React.useState(false);
    const [query, setQuery] = React.useState('');
    const [cacheVersion, setCacheVersion] = React.useState(0);
    const [seedLoading, setSeedLoading] = React.useState(false);
    const [seedError, setSeedError] = React.useState(false);
    const [remote, setRemote] = React.useState<RemoteState>(EMPTY_REMOTE_STATE);
    const remoteSequence = React.useRef(0);

    const open = React.useCallback((initialQuery = '') => {
        setQuery(initialQuery);
        setIsOpen(true);
    }, []);
    const close = React.useCallback(() => setIsOpen(false), []);

    React.useEffect(() => {
        const handleKeyDown = (event: KeyboardEvent) => {
            if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
                event.preventDefault();
                setIsOpen((value) => !value);
            }
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, []);

    const parsed = React.useMemo(() => parseSearchQuery(query), [query]);
    const searchTextLength = Array.from(parsed.text).length;
    const requestedTypes = parsed.resourceTypes.length > 0 ? parsed.resourceTypes : ALL_RESOURCE_TYPES;
    const sessionScope = `actor:${actorId ?? 'pending'}`;
    const scopeKey = parsed.scope === 'all'
        ? `${sessionScope}|all`
        : `${sessionScope}|current:${currentNamespace}`;
    const queryKey = `${scopeKey}|${query}`;

    React.useEffect(() => {
        if (!isOpen) return;
        const seedScopeKey = `${sessionScope}|current:${currentNamespace}`;
        if (!cache.needsSeed(seedScopeKey)) return;

        const controller = new AbortController();
        setSeedLoading(true);
        setSeedError(false);
        void search({
            mode: 'seed',
            resource_type: ALL_RESOURCE_TYPES,
            namespace: namespaceAndChildren(currentNamespace),
            limit: 50,
        }, {signal: controller.signal}).then((response) => {
            if (controller.signal.aborted) return;
            cache.put(seedScopeKey, response.data.items);
            cache.markSeeded(seedScopeKey, uniqueTypes([
                ...response.data.truncated_types,
                ...response.data.incomplete_types,
            ]));
            setCacheVersion((value) => value + 1);
            setSeedError(false);
        }).catch(() => {
            if (!controller.signal.aborted) setSeedError(true);
        }).finally(() => {
            if (!controller.signal.aborted) setSeedLoading(false);
        });

        return () => controller.abort();
    }, [cache, currentNamespace, isOpen, search, sessionScope]);

    const remoteEligible = !parsed.error && !parsed.direct && (
        parsed.labelSelector.length > 0 || searchTextLength >= 3
    );
    const cacheComplete = parsed.scope === 'current' &&
        !labelSelectorUsesSystemLabels(parsed.labelSelector) &&
        cache.isComplete(scopeKey, requestedTypes);

    React.useEffect(() => {
        const sequence = ++remoteSequence.current;
        setRemote({...EMPTY_REMOTE_STATE, queryKey});
        if (!isOpen || !remoteEligible || cacheComplete) return;

        const controller = new AbortController();
        const timer = window.setTimeout(() => {
            setRemote((state) => state.queryKey === queryKey ? {...state, loading: true} : state);
            void search({
                mode: 'query',
                resource_type: parsed.resourceTypes.length > 0 ? parsed.resourceTypes : undefined,
                q: parsed.text || undefined,
                label_selector: parsed.labelSelector || undefined,
                namespace: parsed.scope === 'current' ? namespaceAndChildren(currentNamespace) : undefined,
                limit: SEARCH_RESULT_LIMIT,
            }, {signal: controller.signal}).then((response) => {
                if (controller.signal.aborted || sequence !== remoteSequence.current) return;
                cache.put(scopeKey, response.data.items);
                setCacheVersion((value) => value + 1);
                setSeedError(false);
                setRemote({
                    queryKey,
                    items: response.data.items,
                    truncatedTypes: response.data.truncated_types,
                    incompleteTypes: response.data.incomplete_types,
                    loading: false,
                    error: false,
                    completed: true,
                });
            }).catch(() => {
                if (controller.signal.aborted || sequence !== remoteSequence.current) return;
                setRemote({
                    queryKey,
                    items: [],
                    truncatedTypes: [],
                    incompleteTypes: [],
                    loading: false,
                    error: true,
                    completed: false,
                });
            });
        }, debounceMs);

        return () => {
            window.clearTimeout(timer);
            controller.abort();
        };
    }, [cache, cacheComplete, currentNamespace, debounceMs, isOpen, parsed, queryKey, remoteEligible, scopeKey, search]);

    const cachedItems = React.useMemo(
        () => filterCachedResources(cache.list(scopeKey), parsed),
        [cache, cacheVersion, parsed, scopeKey],
    );
    const activeRemote = remote.queryKey === queryKey ? remote : EMPTY_REMOTE_STATE;
    const partialRemoteTypes = uniqueTypes([
        ...activeRemote.truncatedTypes,
        ...activeRemote.incompleteTypes,
    ]);
    const resourceItems = mergeSearchResults(
        cachedItems,
        activeRemote.items,
        activeRemote.completed ? partialRemoteTypes : undefined,
    );
    const hasResourceQualifier = parsed.resourceTypes.length > 0 ||
        parsed.labelSelector.length > 0 || parsed.scope !== 'current';
    const navigationItems = parsed.error || parsed.direct || hasResourceQualifier
        ? []
        : matchingNavigationItems(parsed.text);
    const visibleResources = resourceItems.slice(0, Math.max(0, SEARCH_RESULT_LIMIT - navigationItems.length));

    const selectPath = React.useCallback((path: string, external = false) => {
        setIsOpen(false);
        if (external) {
            window.open(path, '_blank', 'noopener,noreferrer');
        } else {
            navigate(path);
        }
    }, [navigate]);

    const selectResource = React.useCallback((item: SearchResourceSummary) => {
        setIsOpen(false);
        if (item.resource_type === 'namespace') {
            dispatch(setCurrentNamespace(item.resource_id));
            navigate('/namespace');
            return;
        }
        navigate(resourcePath(item));
    }, [dispatch, navigate]);

    const selectDirect = React.useCallback(() => {
        if (!parsed.direct) return;
        setIsOpen(false);
        if (parsed.direct.kind === 'namespace' && parsed.direct.namespace) {
            dispatch(setCurrentNamespace(parsed.direct.namespace));
        }
        navigate(parsed.direct.path);
    }, [dispatch, navigate, parsed.direct]);

    const context = React.useMemo(() => ({open, close}), [close, open]);

    return (
        <CommandPaletteContext.Provider value={context}>
            {children}
            <Dialog
                open={isOpen}
                onClose={close}
                fullWidth
                maxWidth="sm"
                aria-labelledby="admin-command-palette-title"
                PaperProps={{
                    sx: {
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 2,
                        boxShadow: 24,
                        m: 2,
                        overflow: 'hidden',
                        position: 'fixed',
                        top: {xs: 32, sm: 72},
                    },
                }}
            >
                <DialogTitle
                    id="admin-command-palette-title"
                    sx={{position: 'absolute', width: 1, height: 1, p: 0, overflow: 'hidden', clip: 'rect(0 0 0 0)'}}
                >
                    Search Admin UI
                </DialogTitle>
                <CommandRoot label="Search Admin UI" shouldFilter={false} loop>
                    <Box className="command-palette-input-wrapper">
                        <InputAdornment position="start" sx={{color: 'text.secondary', mr: 1}}>
                            <SearchRoundedIcon />
                        </InputAdornment>
                        <Command.Input
                            autoFocus
                            aria-label="Search resources and Admin UI"
                            placeholder="Search resources or enter a command…"
                            value={query}
                            onValueChange={setQuery}
                            onKeyDown={(event) => {
                                if (event.key === 'Enter' && parsed.direct) {
                                    event.preventDefault();
                                    event.stopPropagation();
                                    selectDirect();
                                }
                            }}
                        />
                        <Chip label="Esc" size="small" variant="outlined" sx={{ml: 1}} />
                    </Box>
                    <Command.List aria-label="Search results">
                        {navigationItems.length > 0 && (
                            <Command.Group heading="Admin">
                                {navigationItems.map((item) => {
                                    const Icon = item.icon;
                                    return (
                                        <Command.Item
                                            key={`navigation:${item.path}`}
                                            value={`navigation:${item.path}`}
                                            onSelect={() => selectPath(item.path, item.external)}
                                        >
                                            <Icon color="action" />
                                            <Box sx={{minWidth: 0}}>
                                                <Typography variant="body2" fontWeight={600}>{item.label}</Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    {item.external ? 'Opens in a new tab' : item.path}
                                                </Typography>
                                            </Box>
                                        </Command.Item>
                                    );
                                })}
                            </Command.Group>
                        )}
                        {visibleResources.length > 0 && (
                            <Command.Group heading="Resources">
                                {visibleResources.map((item) => (
                                    <Command.Item
                                        key={`${item.resource_type}:${item.resource_id}`}
                                        value={`${item.resource_type}:${item.resource_id}`}
                                        onSelect={() => selectResource(item)}
                                    >
                                        <ResourceTypeBadge type={item.resource_type} />
                                        <Box sx={{minWidth: 0, flex: 1}}>
                                            <Typography variant="body2" fontWeight={600} noWrap>
                                                {resourceTitle(item, parsed.text)}
                                            </Typography>
                                            <Typography variant="caption" color="text.secondary" noWrap component="div">
                                                {item.resource_id}
                                                {item.namespace && item.resource_type !== 'namespace' ? ` · ${item.namespace}` : ''}
                                            </Typography>
                                        </Box>
                                    </Command.Item>
                                ))}
                            </Command.Group>
                        )}
                        <PaletteStatus
                            parsedError={parsed.error}
                            direct={Boolean(parsed.direct)}
                            query={query}
                            searchTextLength={searchTextLength}
                            qualifierOnly={!parsed.text && !parsed.labelSelector && hasResourceQualifier}
                            navigationCount={navigationItems.length}
                            resourceCount={visibleResources.length}
                            remoteEligible={remoteEligible}
                            seedLoading={seedLoading}
                            loading={activeRemote.loading}
                            serverError={activeRemote.error || seedError}
                            truncatedTypes={activeRemote.truncatedTypes}
                            incompleteTypes={activeRemote.incompleteTypes}
                        />
                    </Command.List>
                </CommandRoot>
            </Dialog>
        </CommandPaletteContext.Provider>
    );
}

function ResourceTypeBadge({type}: {type: SearchResourceType}) {
    return (
        <Chip
            label={type.replace('_', ' ')}
            size="small"
            color="primary"
            variant="outlined"
            sx={{minWidth: 88, textTransform: 'capitalize'}}
        />
    );
}

interface PaletteStatusProps {
    parsedError: string | null;
    direct: boolean;
    query: string;
    searchTextLength: number;
    qualifierOnly: boolean;
    navigationCount: number;
    resourceCount: number;
    remoteEligible: boolean;
    seedLoading: boolean;
    loading: boolean;
    serverError: boolean;
    truncatedTypes: SearchResourceType[];
    incompleteTypes: SearchResourceType[];
}

function PaletteStatus(props: PaletteStatusProps) {
    if (props.parsedError) return <StatusRow color="error.main">{props.parsedError}</StatusRow>;
    if (props.direct) return <StatusRow>Press Enter to open this resource.</StatusRow>;
    if (props.loading || props.seedLoading) {
        return (
            <StatusRow>
                <CircularProgress size={14} sx={{mr: 1}} />
                Searching…
            </StatusRow>
        );
    }
    if (props.serverError) {
        return <StatusRow color="warning.main">Server search is unavailable. Cached results and commands still work.</StatusRow>;
    }
    if (props.incompleteTypes.length > 0 || props.truncatedTypes.length > 0) {
        return (
            <StatusRow color="warning.main">
                {props.incompleteTypes.length > 0 && (
                    <>Some resource types did not finish: {formatTypes(props.incompleteTypes)}. </>
                )}
                {props.truncatedTypes.length > 0 && (
                    <>Results are partial for {formatTypes(props.truncatedTypes)}. Refine your query.</>
                )}
            </StatusRow>
        );
    }
    if (props.qualifierOnly && !props.remoteEligible) {
        return <StatusRow color="text.secondary">Showing cached matches only. Add a label selector or at least 3 characters to search the server.</StatusRow>;
    }
    if (props.searchTextLength > 0 && !props.remoteEligible && props.searchTextLength < 3) {
        return <StatusRow color="text.secondary">Showing local matches. Enter at least 3 characters for server search.</StatusRow>;
    }
    if (props.navigationCount === 0 && props.resourceCount === 0 && props.query.trim()) {
        return <StatusRow color="text.secondary">No matching commands or resources.</StatusRow>;
    }
    return null;
}

function StatusRow({children, color = 'text.secondary'}: React.PropsWithChildren<{color?: string}>) {
    return (
        <Box role="status" sx={{alignItems: 'center', color, display: 'flex', px: 1.25, py: 1.5}}>
            <Typography variant="caption" component="div">{children}</Typography>
        </Box>
    );
}

function resourceTitle(item: SearchResourceSummary, searchText: string): string {
    const needle = searchText.toLowerCase();
    const candidates = [
        ...item.matched_labels,
        ...Object.entries(item.labels).map(([key, value]) => ({key, value})),
    ].filter((candidate, index, all) =>
        Boolean(candidate.value) &&
        all.findIndex((other) => other.key === candidate.key && other.value === candidate.value) === index,
    );
    if (needle) {
        const queryMatch = candidates
            .filter((candidate) => candidate.value.toLowerCase().includes(needle))
            .sort((left, right) =>
                titleMatchRank(left.value, needle) - titleMatchRank(right.value, needle) ||
                left.key.localeCompare(right.key),
            )[0];
        if (queryMatch) return queryMatch.value;
    }
    const matched = item.matched_labels.find((label) => label.value)?.value;
    if (matched) return matched;
    if (item.labels.name) return item.labels.name;
    const label = Object.entries(item.labels).sort(([left], [right]) => left.localeCompare(right))[0]?.[1];
    return label || item.resource_id;
}

function titleMatchRank(value: string, needle: string): number {
    const normalized = value.toLowerCase();
    if (normalized === needle) return 0;
    if (normalized.startsWith(needle)) return 1;
    return 2;
}

function resourcePath(item: SearchResourceSummary): string {
    if (item.resource_type === 'namespace') return '/namespace';
    const routes: Record<Exclude<SearchResourceType, 'namespace'>, string> = {
        actor: '/actors/',
        connection: '/connections/',
        connector: '/connectors/',
        key: '/keys/',
        rate_limit: '/rate-limits/',
    };
    return routes[item.resource_type] + encodeURIComponent(item.resource_id);
}

function uniqueTypes(types: SearchResourceType[]): SearchResourceType[] {
    return Array.from(new Set(types));
}

function formatTypes(types: SearchResourceType[]): string {
    return types.map((type) => type.replace('_', ' ')).join(', ');
}
