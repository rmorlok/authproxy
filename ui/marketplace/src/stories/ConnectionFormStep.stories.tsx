import { Meta, StoryObj } from '@storybook/react';
import ConnectionFormStep from '../components/ConnectionFormStep';
import { fn } from '@storybook/test';

const meta: Meta<typeof ConnectionFormStep> = {
    title: 'Components/ConnectionFormStep',
    component: ConnectionFormStep,
    parameters: {
        layout: 'centered',
    },
    tags: ['autodocs'],
    args: {
        onSubmit: fn(),
        onCancel: fn(),
        isSubmitting: false,
        connectionId: 'cxn_test550e8400abcde',
    },
};

export default meta;
type Story = StoryObj<typeof ConnectionFormStep>;

export const SimpleTextForm: Story = {
    args: {
        jsonSchema: {
            type: 'object',
            properties: {
                apiKey: {
                    type: 'string',
                    title: 'API Key',
                    description: 'Your API key for the service',
                },
                region: {
                    type: 'string',
                    title: 'Region',
                    enum: ['us-east-1', 'us-west-2', 'eu-west-1', 'ap-southeast-1'],
                },
            },
            required: ['apiKey', 'region'],
        },
        uiSchema: {
            type: 'VerticalLayout',
            elements: [
                { type: 'Control', scope: '#/properties/apiKey' },
                { type: 'Control', scope: '#/properties/region' },
            ],
        },
    },
};

export const ComplexForm: Story = {
    args: {
        jsonSchema: {
            type: 'object',
            properties: {
                name: {
                    type: 'string',
                    title: 'Connection Name',
                    minLength: 3,
                    maxLength: 50,
                },
                environment: {
                    type: 'string',
                    title: 'Environment',
                    enum: ['production', 'staging', 'development'],
                    default: 'development',
                },
                credentials: {
                    type: 'object',
                    title: 'Credentials',
                    properties: {
                        username: {
                            type: 'string',
                            title: 'Username',
                        },
                        password: {
                            type: 'string',
                            title: 'Password',
                        },
                    },
                    required: ['username', 'password'],
                },
                enableLogging: {
                    type: 'boolean',
                    title: 'Enable Request Logging',
                    default: true,
                },
                maxRetries: {
                    type: 'integer',
                    title: 'Max Retries',
                    minimum: 0,
                    maximum: 10,
                    default: 3,
                },
            },
            required: ['name', 'environment', 'credentials'],
        },
        uiSchema: {
            type: 'VerticalLayout',
            elements: [
                { type: 'Control', scope: '#/properties/name' },
                { type: 'Control', scope: '#/properties/environment' },
                {
                    type: 'Group',
                    label: 'Authentication',
                    elements: [
                        { type: 'Control', scope: '#/properties/credentials/properties/username' },
                        {
                            type: 'Control',
                            scope: '#/properties/credentials/properties/password',
                            options: { format: 'password' },
                        },
                    ],
                },
                {
                    type: 'HorizontalLayout',
                    elements: [
                        { type: 'Control', scope: '#/properties/enableLogging' },
                        { type: 'Control', scope: '#/properties/maxRetries' },
                    ],
                },
            ],
        },
    },
};

export const Submitting: Story = {
    args: {
        ...SimpleTextForm.args,
        isSubmitting: true,
    },
};

export const ReconfigureWithExistingSelections: Story = {
    args: {
        connectionId: 'cxn_google_calendar',
        stepTitle: 'Select a Calendar',
        stepDescription: 'Your current selection is shown. Save it unchanged or choose another calendar.',
        jsonSchema: {
            type: 'object',
            properties: {
                calendar: {
                    type: 'string',
                    title: 'Calendar',
                    enum: ['primary', 'product', 'support'],
                },
                syncDirection: {
                    type: 'string',
                    title: 'Sync direction',
                    enum: ['two-way', 'import-only', 'export-only'],
                },
            },
            required: ['calendar', 'syncDirection'],
        },
        uiSchema: {
            type: 'VerticalLayout',
            elements: [
                { type: 'Control', scope: '#/properties/calendar' },
                { type: 'Control', scope: '#/properties/syncDirection' },
            ],
        },
        initialData: {
            calendar: 'product',
            syncDirection: 'two-way',
        },
    },
};

export const ReconfigureWithoutExistingSelections: Story = {
    args: {
        ...ReconfigureWithExistingSelections.args,
        stepDescription: 'Before the fix, reconfigure opened without the stored selections.',
        initialData: undefined,
    },
};

export const MinimalForm: Story = {
    args: {
        jsonSchema: {
            type: 'object',
            properties: {
                token: {
                    type: 'string',
                    title: 'Access Token',
                    description: 'Paste your personal access token here',
                },
            },
            required: ['token'],
        },
        uiSchema: {
            type: 'VerticalLayout',
            elements: [
                { type: 'Control', scope: '#/properties/token' },
            ],
        },
    },
};
