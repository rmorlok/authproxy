import React, { useState, useCallback } from 'react';
import { JsonForms } from '@jsonforms/react';
import { materialCells, materialRenderers } from '@jsonforms/material-renderers';
import type { JsonFormsCore, UISchemaElement } from '@jsonforms/core';
import type { JsonSchema } from '@jsonforms/core';
import { Box, Button, CircularProgress } from '@mui/material';

export interface ConnectionFormStepProps {
    connectionId: string;
    jsonSchema: Record<string, unknown>;
    uiSchema: Record<string, unknown>;
    onSubmit: (connectionId: string, data: unknown) => void;
    onCancel: () => void;
    isSubmitting: boolean;
}

const ConnectionFormStep: React.FC<ConnectionFormStepProps> = ({
    connectionId,
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

    return (
        <Box sx={{ p: 2 }}>
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
