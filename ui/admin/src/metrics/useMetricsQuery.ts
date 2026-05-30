import * as React from 'react';
import {
    MetricsQueryRef,
    MetricsQueryResponse,
    MetricsRange,
    namespaceAndChildren,
    queryMetrics,
} from '@authproxy/api';
import {useSelector} from 'react-redux';
import {selectCurrentNamespacePath} from '../store/namespacesSlice';
import {metricsResponseIsEmpty, metricsSeriesByRef, summarizeMetricsResponse} from './timeSeries';

export interface UseMetricsQueryOptions {
    range: MetricsRange;
    queries: MetricsQueryRef[];
    namespace?: string | null;
    labelSelector?: string;
    enabled?: boolean;
}

export interface UseMetricsQueryResult {
    data: MetricsQueryResponse | null;
    loading: boolean;
    error: string | null;
    empty: boolean;
    summaries: ReturnType<typeof summarizeMetricsResponse>;
    seriesByRef: ReturnType<typeof metricsSeriesByRef>;
    reload: () => void;
}

export const useMetricsQuery = ({
    range,
    queries,
    namespace,
    labelSelector,
    enabled = true,
}: UseMetricsQueryOptions): UseMetricsQueryResult => {
    const currentNamespace = useSelector(selectCurrentNamespacePath);
    const [data, setData] = React.useState<MetricsQueryResponse | null>(null);
    const [loading, setLoading] = React.useState(false);
    const [error, setError] = React.useState<string | null>(null);
    const [reloadNonce, setReloadNonce] = React.useState(0);

    const effectiveNamespace = React.useMemo(
        () => namespace ?? namespaceAndChildren(currentNamespace),
        [currentNamespace, namespace],
    );
    React.useEffect(() => {
        if (!enabled || queries.length === 0) {
            setLoading(false);
            setError(null);
            setData(null);
            return;
        }

        const controller = new AbortController();
        setLoading(true);
        setError(null);

        void queryMetrics(
            {
                range,
                namespace: effectiveNamespace,
                label_selector: labelSelector || undefined,
                queries,
            },
            {signal: controller.signal},
        )
            .then((response) => {
                if (!controller.signal.aborted) {
                    setData(response.data);
                }
            })
            .catch((err: unknown) => {
                if (!controller.signal.aborted) {
                    setData(null);
                    setError(metricsErrorMessage(err));
                }
            })
            .finally(() => {
                if (!controller.signal.aborted) {
                    setLoading(false);
                }
            });

        return () => controller.abort();
    }, [effectiveNamespace, enabled, labelSelector, queries, range, reloadNonce]);

    const summaries = React.useMemo(() => summarizeMetricsResponse(data), [data]);
    const seriesByRef = React.useMemo(() => metricsSeriesByRef(data), [data]);

    return {
        data,
        loading,
        error,
        empty: metricsResponseIsEmpty(data),
        summaries,
        seriesByRef,
        reload: () => setReloadNonce((value) => value + 1),
    };
};

const metricsErrorMessage = (err: unknown): string => {
    if (typeof err === 'object' && err !== null) {
        const maybeAxios = err as {
            message?: string;
            response?: {data?: {error?: string; message?: string}};
        };
        return maybeAxios.response?.data?.error ||
            maybeAxios.response?.data?.message ||
            maybeAxios.message ||
            'Failed to load metrics';
    }
    return 'Failed to load metrics';
};
