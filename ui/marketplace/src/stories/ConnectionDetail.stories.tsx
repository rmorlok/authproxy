import * as React from 'react';
import { Meta, StoryObj } from '@storybook/react';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import { Connection, ConnectionHealthState, ConnectionState } from '@authproxy/api';
import ConnectionDetail from '../components/ConnectionDetail';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import toastsReducer from '../store/toastsSlice';
import { connectionFixture, connectorFixture } from '../testing/resources';

const gmailLogo = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="88" height="88" viewBox="0 0 88 88"%3E%3Cpath fill="%234285f4" d="M14 24v42h14V35z"/%3E%3Cpath fill="%2334a853" d="M60 35v31h14V24z"/%3E%3Cpath fill="%23fbbc04" d="M60 24v11l14-11z"/%3E%3Cpath fill="%23ea4335" d="M14 24l30 22 30-22v-2c0-6-7-9-12-5L44 30 26 17c-5-4-12-1-12 5z"/%3E%3C/svg%3E';

const gmailConnector = connectorFixture({
  id: 'gmail',
  displayName: 'GMail',
  description: 'Have the agent respond to your emails without you needing to be involved. Like magic.',
  highlight: 'Respond to email automatically.',
  logo: {publicUrl: gmailLogo},
  hasConfigure: true,
});

const baseConnection: Connection = connectionFixture({
  id: 'c-gmail',
  connector: gmailConnector,
  state: ConnectionState.CONFIGURED,
  healthState: ConnectionHealthState.HEALTHY,
});

const makeStore = (connection: Connection) => configureStore({
  reducer: {
    connectors: connectorsReducer,
    connections: connectionsReducer,
    toasts: toastsReducer,
  },
  preloadedState: {
    connectors: {
      items: [gmailConnector],
      status: 'succeeded',
      error: null,
    },
    connections: {
      items: [connection],
      status: 'succeeded',
      error: null,
      initiatingConnection: false,
      initiationError: null,
      disconnectingConnection: false,
      disconnectionError: null,
      currentTaskId: null,
      currentFormStep: null,
      submittingForm: false,
      formSubmitError: null,
      verifyingConnectionId: null,
      verifyError: null,
      retryingConnection: false,
      recentlyCompletedConnectionId: null,
    },
    toasts: { items: [] },
  },
});

const meta: Meta<typeof ConnectionDetail> = {
  title: 'Components/ConnectionDetail',
  component: ConnectionDetail,
  parameters: {
    layout: 'fullscreen',
    viewport: {
      defaultViewport: 'marketplaceDesktop',
    },
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof ConnectionDetail>;

const renderWithConnection = (connection: Connection) => (
  <Provider store={makeStore(connection)}>
    <ConnectionDetail connectionId={connection.metadata.id} />
  </Provider>
);

export const Configured: Story = {
  render: () => renderWithConnection(baseConnection),
};

export const RequiresSetup: Story = {
  render: () => renderWithConnection(connectionFixture({
    id: baseConnection.metadata.id,
    connector: gmailConnector,
    state: ConnectionState.SETUP,
  })),
};

export const RequiresReconnection: Story = {
  render: () => renderWithConnection(connectionFixture({
    id: baseConnection.metadata.id,
    connector: gmailConnector,
    healthState: ConnectionHealthState.UNHEALTHY,
  })),
};
