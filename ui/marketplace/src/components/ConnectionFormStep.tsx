import React, { useState, useCallback, useEffect, useMemo, useRef } from 'react';
import { JsonForms } from '@jsonforms/react';
import { materialCells, materialRenderers } from '@jsonforms/material-renderers';
import type { JsonFormsCore, UISchemaElement } from '@jsonforms/core';
import type { JsonSchema } from '@jsonforms/core';
import { Box, Button, CircularProgress, Typography } from '@mui/material';
import { connections, DataSourceOption } from '@authproxy/api';
import { marketplaceTokens } from '../theme';

export interface ConnectionFormStepProps {
    connectionId: string;
    stepTitle?: string;
    stepDescription?: string;
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

function hasMeaningfulValue(value: unknown): boolean {
    if (value == null) return false;
    if (typeof value === 'string') return value.length > 0;
    if (Array.isArray(value)) return value.some(hasMeaningfulValue);
    if (typeof value === 'object') return Object.values(value).some(hasMeaningfulValue);
    return true;
}

const ConnectionFormStep: React.FC<ConnectionFormStepProps> = ({
    connectionId,
    stepTitle,
    stepDescription,
    jsonSchema,
    uiSchema,
    onSubmit,
    onCancel,
    isSubmitting,
}) => {
    const [data, setData] = useState<unknown>({});
    const [hasErrors, setHasErrors] = useState(true);
    const [shouldShowValidation, setShouldShowValidation] = useState(false);
    const [dataSourceOptions, setDataSourceOptions] = useState<Record<string, DataSourceOption[]>>({});
    const [loadingDataSources, setLoadingDataSources] = useState(false);
    const hasUserInteracted = useRef(false);
    const hasEnteredValue = useRef(false);

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
        const hasCurrentErrors = (state.errors?.length ?? 0) > 0;
        setData(state.data);
        setHasErrors(hasCurrentErrors);
        if (hasUserInteracted.current && hasMeaningfulValue(state.data)) {
            hasEnteredValue.current = true;
        }
        if (hasUserInteracted.current && hasEnteredValue.current && hasCurrentErrors) {
            setShouldShowValidation(true);
        }
    }, []);

    const handleUserInteraction = useCallback(() => {
        hasUserInteracted.current = true;
    }, []);

    const handleBlur = useCallback(() => {
        if (hasUserInteracted.current) {
            setShouldShowValidation(true);
        }
    }, []);

    const handleSubmit = useCallback(() => {
        onSubmit(connectionId, data);
    }, [connectionId, data, onSubmit]);

    return (
        <Box sx={{ pt: 1 }}>
            {stepTitle && (
                <Typography variant="subtitle1" gutterBottom>
                    {stepTitle}
                </Typography>
            )}
            {stepDescription && (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    {stepDescription}
                </Typography>
            )}
            {loadingDataSources ? (
                <Box
                    sx={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        gap: 1.5,
                        py: marketplaceTokens.spacing.pageY,
                    }}
                >
                    <CircularProgress />
                    <Typography variant="body2" color="text.secondary">
                        Loading setup options...
                    </Typography>
                </Box>
            ) : (
                <Box
                    onInputCapture={handleUserInteraction}
                    onChangeCapture={handleUserInteraction}
                    onBlurCapture={handleBlur}
                >
                    <JsonForms
                        schema={resolvedSchema as JsonSchema}
                        uischema={uiSchema as unknown as UISchemaElement}
                        data={data}
                        renderers={materialRenderers}
                        cells={materialCells}
                        validationMode={shouldShowValidation ? 'ValidateAndShow' : 'ValidateAndHide'}
                        onChange={handleChange}
                    />
                </Box>
            )}
            <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1, mt: marketplaceTokens.spacing.headerGap + 1 }}>
                <Button
                    variant="text"
                    onClick={onCancel}
                    disabled={isSubmitting}
                >
                    Cancel setup
                </Button>
                <Button
                    variant="contained"
                    onClick={handleSubmit}
                    disabled={isSubmitting || hasErrors || loadingDataSources}
                    startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
                >
                    {isSubmitting ? 'Saving setup...' : 'Save and verify'}
                </Button>
            </Box>
        </Box>
    );
};

export default ConnectionFormStep;
