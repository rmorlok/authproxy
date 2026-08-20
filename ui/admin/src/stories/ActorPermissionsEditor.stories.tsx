import React from 'react';
import type {Meta, StoryObj} from '@storybook/react';
import Paper from '@mui/material/Paper';
import ActorPermissionsEditor from '../components/ActorPermissionsEditor';
import type {Permission} from '@authproxy/api';

const meta = {
  title: 'Actors/Actor Permissions',
  component: ActorPermissionsEditor,
  parameters: {layout: 'centered'},
} satisfies Meta<typeof ActorPermissionsEditor>;

export default meta;
type Story = StoryObj<typeof meta>;

const initialPermissions: Permission[] = [
  {
    namespace: 'root.acme.**',
    resources: ['connectors', 'connections'],
    verbs: ['get', 'list', 'create'],
  },
  {
    namespace: 'root.acme.sales',
    resources: ['connections'],
    resourceIds: ['cxn_salesforce'],
    verbs: ['proxy'],
  },
];

function PermissionsStory() {
  const [permissions, setPermissions] = React.useState(initialPermissions);
  return (
    <Paper variant="outlined" sx={{p: 3, width: 760}}>
      <ActorPermissionsEditor permissions={permissions} onSave={async next => setPermissions(next)}/>
    </Paper>
  );
}

export const Populated: Story = {
  render: () => <PermissionsStory/>,
  args: {permissions: initialPermissions, onSave: async () => {}},
};
