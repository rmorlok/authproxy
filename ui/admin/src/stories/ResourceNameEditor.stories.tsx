import React from 'react';
import type {Meta, StoryObj} from '@storybook/react';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import ResourceNameEditor from '../components/ResourceNameEditor';

const meta = {
    title: 'Resources/Resource Name',
    component: ResourceNameEditor,
    parameters: {layout: 'centered'},
} satisfies Meta<typeof ResourceNameEditor>;

export default meta;
type Story = StoryObj<typeof meta>;

function OverviewStory() {
    const [name, setName] = React.useState('production-crm');
    return (
        <Stack direction={{xs: 'column', sm: 'row'}} spacing={2} sx={{minWidth: 680}}>
            <Paper variant="outlined" sx={{p: 3, flex: 1}}>
                <Typography variant="overline" color="text.secondary">Connection</Typography>
                <ResourceNameEditor
                    name={name}
                    resourceType="Connection"
                    onRename={async (nextName) => setName(nextName)}
                />
                <Typography variant="caption" color="text.secondary">
                    ID: cxn_01J8V8N8K8J2G5J9X1Q4
                </Typography>
            </Paper>
            <Paper variant="outlined" sx={{p: 3, flex: 1}}>
                <Typography variant="overline" color="text.secondary">Namespace</Typography>
                <ResourceNameEditor name="payments" resourceType="Namespace"/>
                <Typography variant="caption" color="text.secondary">
                    Path: root.acme.payments
                </Typography>
            </Paper>
        </Stack>
    );
}

export const Overview: Story = {
    render: () => <OverviewStory/>,
    args: {name: 'production-crm', resourceType: 'Connection'},
};
