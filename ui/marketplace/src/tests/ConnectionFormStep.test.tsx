import * as React from 'react';
import {render, screen} from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import ConnectionFormStep from '../components/ConnectionFormStep';

const requiredTextStep = {
    connectionId: 'cxn_preconnect',
    stepTitle: 'Choose a pretend tenant',
    stepDescription: 'This pre-connect step runs before OAuth.',
    jsonSchema: {
        type: 'object',
        required: ['tenant'],
        properties: {
            tenant: {
                type: 'string',
                title: 'Demo tenant',
            },
        },
    },
    uiSchema: {
        type: 'VerticalLayout',
        elements: [{type: 'Control', scope: '#/properties/tenant'}],
    },
};

describe('ConnectionFormStep', () => {
    test('hides required validation until the user changes the field', async () => {
        const user = userEvent.setup();

        render(
            <ConnectionFormStep
                {...requiredTextStep}
                onSubmit={vi.fn()}
                onCancel={vi.fn()}
                isSubmitting={false}
            />,
        );

        expect(screen.getByLabelText(/Demo tenant/i)).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Save and verify/i})).toBeDisabled();
        expect(screen.queryByText(/is a required property/i)).not.toBeInTheDocument();

        await user.type(screen.getByLabelText(/Demo tenant/i), 'acme');
        expect(screen.queryByText(/is a required property/i)).not.toBeInTheDocument();

        await user.clear(screen.getByLabelText(/Demo tenant/i));
        expect(screen.queryByText(/is a required property/i)).not.toBeInTheDocument();

        await user.tab();
        expect(screen.getByText(/is a required property/i)).toBeInTheDocument();
        expect(screen.getByRole('button', {name: /Save and verify/i})).toBeDisabled();
    });
});
