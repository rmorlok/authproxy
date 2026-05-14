import type { Meta, StoryObj } from '@storybook/react';
import React, { useState } from 'react';
import Box from '@mui/material/Box';
import RequestForm, {
    EMPTY_REQUEST_VALUE,
    RequestFormValue,
} from '../components/RequestForm';

// Stateful wrapper — Storybook stories don't carry their own state by
// default, so we wrap the form in a tiny container that owns it. Renders
// the live JSON next to the form for visual smoke-checking.
function ControlledRequestForm({ initial }: { initial: RequestFormValue }) {
    const [value, setValue] = useState<RequestFormValue>(initial);
    return (
        <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 4, p: 3 }}>
            <RequestForm value={value} onChange={setValue} />
            <Box>
                <h3>Current value</h3>
                <pre style={{ fontSize: 12, background: '#f4f4f4', padding: 12 }}>
                    {JSON.stringify(value, null, 2)}
                </pre>
            </Box>
        </Box>
    );
}

const meta = {
    title: 'Components/RequestForm',
    component: ControlledRequestForm,
    parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof ControlledRequestForm>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {
    args: { initial: EMPTY_REQUEST_VALUE },
};

// Pre-filled to make manual smoke-checking faster: headers + labels +
// JSON body all populated, advanced overrides hidden so the default
// connection-first flow is what's visible.
export const Prefilled: Story = {
    args: {
        initial: {
            request: {
                method: 'POST',
                url: 'https://api.example.com/v1/things',
                headers: { 'X-Source': 'admin-ui' },
                labels: { team: 'acme' },
                body_json: { name: 'example' },
            },
            context: {},
        },
    },
};

// Surfaces the Advanced section open with manual actor / namespace
// overrides, so the affordance is obvious in the gallery.
export const WithOverrides: Story = {
    args: {
        initial: {
            request: { method: 'GET', url: 'https://api.example.com/v1/me', headers: {}, labels: {} },
            context: { actorId: 'act_demo', namespace: 'root.acme' },
        },
    },
};
