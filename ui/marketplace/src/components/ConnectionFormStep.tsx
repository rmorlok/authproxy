import React, { useState, useCallback, useEffect, useMemo } from 'react';
import { JsonForms } from '@jsonforms/react';
import { materialCells, materialRenderers } from '@jsonforms/material-renderers';
import type { JsonFormsCore, UISchemaElement } from '@jsonforms/core';
import type { JsonSchema } from '@jsonforms/core';
import { Box, Button, CircularProgress, Typography, LinearProgress } from '@mui/material';
import { connections, DataSourceOption } from '@authproxy/api';

export interface ConnectionFormStepProps {
    connectionId: string;
    stepTitle?: string;
    stepDescription?: string;
    currentStep: number;
    totalSteps: number;
    jsonSchema: Record<string, unknown>;
    uiSchema: Record<string, unknown>;
    onSubmit: (connectionId: string, data: unknown) => void;
    onCancel: () => void;
    isSubmitting: boolean;
}

/**
 * Scan JSON schema properties for x-data-source annotations and return
 * a map of property name -> data source ID.
 */
function findDataSources(schema: Record<string, unknown>): Record<string, string> {
    const result: Record<string, string> = {};
    const properties = schema.properties as Record<string, Record<string, unknown>> | undefined;
    if (!properties) return result;

    for (const [propName, propSchema] of Object.entries(properties)) {
        const sourceId = propSchema['x-data-source'];
        if (typeof sourceId === 'string') {
            result[propName] = sourceId;
        }
    }
    return result;
}

/**
 * Return a copy of the schema with x-data-source properties replaced by
 * oneOf enums so JsonForms renders them as select dropdowns.
 */
function applyDataSourcesToSchema(
    schema: Record<string, unknown>,
    optionsMap: Record<string, DataSourceOption[]>,
): Record<string, unknown> {
    const properties = schema.properties as Record<string, Record<string, unknown>> | undefined;
    if (!properties) return schema;

    const newProperties: Record<string, Record<string, unknown>> = {};
    let changed = false;

    for (const [propName, propSchema] of Object.entries(properties)) {
        const options = optionsMap[propName];
        if (options) {
            changed = true;
            const { 'x-data-source': _, ...rest } = propSchema;
            newProperties[propName] = {
                ...rest,
                oneOf: options.map((opt) => ({
                    const: opt.value,
                    title: opt.label,
                })),
            };
        } else {
            newProperties[propName] = propSchema;
        }
    }

    if (!changed) return schema;
    return { ...schema, properties: newProperties };
}

const ConnectionFormStep: React.FC<ConnectionFormStepProps> = ({
    connectionId,
    stepTitle,
    stepDescription,
    currentStep,
    totalSteps,
    jsonSchema,
    uiSchema,
    onSubmit,
    onCancel,
    isSubmitting,
}) => {
    const [data, setData] = useState<unknown>({});
    const [hasErrors, setHasErrors] = useState(false);
    const [dataSourceOptions, setDataSourceOptions] = useState<Record<string, DataSourceOption[]>>({});
    const [loadingDataSources, setLoadingDataSources] = useState(false);

    const dataSources = useMemo(() => findDataSources(jsonSchema), [jsonSchema]);
    const hasDataSources = Object.keys(dataSources).length > 0;

    // Fetch options for all x-data-source properties
    useEffect(() => {
        const entries = Object.entries(dataSources);
        if (entries.length === 0) return;

        setLoadingDataSources(true);
        Promise.all(
            entries.map(([propName, sourceId]) =>
                connections.getDataSource(connectionId, sourceId).then((resp) => [propName, resp.data] as const)
            )
        )
            .then((results) => {
                const opts: Record<string, DataSourceOption[]> = {};
                for (const [propName, options] of results) {
                    opts[propName] = options;
                }
                setDataSourceOptions(opts);
            })
            .finally(() => setLoadingDataSources(false));
    }, [connectionId, dataSources]);

    const resolvedSchema = useMemo(
        () => (hasDataSources ? applyDataSourcesToSchema(jsonSchema, dataSourceOptions) : jsonSchema),
        [jsonSchema, dataSourceOptions, hasDataSources],
    );

    const handleChange = useCallback((state: Pick<JsonFormsCore, 'data' | 'errors'>) => {
        setData(state.data);
        setHasErrors((state.errors?.length ?? 0) > 0);
    }, []);

    const handleSubmit = useCallback(() => {
        onSubmit(connectionId, data);
    }, [connectionId, data, onSubmit]);

    const progress = totalSteps > 0 ? ((currentStep + 1) / totalSteps) * 100 : 0;

    return (
        <Box sx={{ p: 2 }}>
            {totalSteps > 1 && (
                <Box sx={{ mb: 2 }}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                        <Typography variant="caption" color="text.secondary">
                            Step {currentStep + 1} of {totalSteps}
                        </Typography>
                    </Box>
                    <LinearProgress variant="determinate" value={progress} />
                </Box>
            )}
            {stepTitle && (
                <Typography variant="h6" gutterBottom>
                    {stepTitle}
                </Typography>
            )}
            {stepDescription && (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    {stepDescription}
                </Typography>
            )}
            {loadingDataSources ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                    <CircularProgress />
                </Box>
            ) : (
                <JsonForms
                    schema={resolvedSchema as JsonSchema}
                    uischema={uiSchema as unknown as UISchemaElement}
                    data={data}
                    renderers={materialRenderers}
                    cells={materialCells}
                    onChange={handleChange}
                />
            )}
            <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1, mt: 3 }}>
                <Button
                    variant="outlined"
                    onClick={onCancel}
                    disabled={isSubmitting}
                >
                    Cancel
                </Button>
                <Button
                    variant="contained"
                    onClick={handleSubmit}
                    disabled={isSubmitting || hasErrors || loadingDataSources}
                    startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
                >
                    {isSubmitting ? 'Submitting...' : 'Submit'}
                </Button>
            </Box>
        </Box>
    );
};

export default ConnectionFormStep;
