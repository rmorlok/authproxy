import React, { useState, useCallback } from 'react';
import { JsonForms } from '@jsonforms/react';
import { materialCells, materialRenderers } from '@jsonforms/material-renderers';
import type { JsonFormsCore, UISchemaElement } from '@jsonforms/core';
import type { JsonSchema } from '@jsonforms/core';
import { Box, Button, CircularProgress, Typography, LinearProgress } from '@mui/material';

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
            <JsonForms
                schema={jsonSchema as JsonSchema}
                uischema={uiSchema as unknown as UISchemaElement}
                data={data}
                renderers={materialRenderers}
                cells={materialCells}
                onChange={handleChange}
            />
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
                    disabled={isSubmitting || hasErrors}
                    startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
                >
                    {isSubmitting ? 'Submitting...' : 'Submit'}
                </Button>
            </Box>
        </Box>
    );
};

export default ConnectionFormStep;
