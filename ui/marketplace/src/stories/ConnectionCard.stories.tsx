import * as React from 'react';
import {Meta, StoryObj} from '@storybook/react';
import ConnectionCard, {ConnectionCardSkeleton} from '../components/ConnectionCard';
import {Connection, ConnectionState, ConnectionHealthState, ConnectorVersionState} from '@authproxy/api';
import {Provider} from 'react-redux';
import {configureStore} from '@reduxjs/toolkit';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';

const logoDataUri = (label: string, background: string) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${label} logo"><rect width="280" height="140" rx="8" fill="${background}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
};

const googleCalendarLogo = logoDataUri('Google Calendar', '#1a73e8');

// Create a mock store with connectors and connections
const mockStore = configureStore({
  reducer: {
    connectors: connectorsReducer,
    connections: connectionsReducer,
  },
  preloadedState: {
    connectors: {
      items: [
        {
          id: 'google-calendar',
          display_name: 'Google Calendar',
          description: 'Connect to your Google Calendar to manage events and appointments.',
          logo: googleCalendarLogo,
        },
      ],
      status: 'succeeded',
      error: null,
    },
    connections: {
      items: [],
      status: 'idle',
      error: null,
      initiatingConnection: false,
      initiationError: null,
      disconnectingConnection: false,
      disconnectionError: null,
      currentTaskId: null
    },
  },
});

const meta: Meta<typeof ConnectionCard> = {
  title: 'Components/ConnectionCard',
  component: ConnectionCard,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <Provider store={mockStore}>
        <Story />
      </Provider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConnectionCard>;

const mockConnection: Connection = {
  id: '123e4567-e89b-12d3-a456-426614174000',
  connector: {
      type: 'google-calendar',
      versions: 0,
      states: [ConnectorVersionState.PRIMARY],
      id: "923e4567-e89b-12d3-a456-426614174009",
      version: 0,
      state: ConnectorVersionState.PRIMARY,
      display_name: "Google Calendar",
      description: "A google calendar connector",
      logo: googleCalendarLogo
  },
  state: ConnectionState.CONFIGURED,
  health_state: ConnectionHealthState.HEALTHY,
  created_at: '2023-04-01T12:00:00Z',
  updated_at: '2023-04-01T12:00:00Z',
};

export const Connected: Story = {
  args: {
    connection: mockConnection,
  },
};

export const NewlyConnected: Story = {
  args: {
    connection: mockConnection,
    highlightNew: true,
  },
};

export const ConnectedConfigurable: Story = {
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true,
      },
    },
  },
};

export const Unhealthy: Story = {
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true,
      },
      health_state: ConnectionHealthState.UNHEALTHY,
    },
  },
};

export const Created: Story = {
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.SETUP,
    },
  },
};

export const Failed: Story = {
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISABLED,
    },
  },
};

export const Disconnecting: Story = {
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING,
    },
  },
};

export const Disconnected: Story = {
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTED,
    },
  },
};

export const UnknownConnector: Story = {
  args: {
    connection: {
      ...mockConnection,
      connector_id: 'unknown-connector',
    },
  },
};

export const WithTaskInProgress: Story = {
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING,
    },
  },
  decorators: [
    (Story) => {
      const store = configureStore({
        reducer: {
          connectors: connectorsReducer,
          connections: connectionsReducer,
        },
        preloadedState: {
          connectors: {
            items: [
              {
                id: 'google-calendar',
                display_name: 'Google Calendar',
                description: 'Connect to your Google Calendar to manage events and appointments.',
                logo: googleCalendarLogo,
              },
            ],
            status: 'succeeded',
            error: null,
          },
          connections: {
            items: [],
            status: 'idle',
            error: null,
            initiatingConnection: false,
            initiationError: null,
            disconnectingConnection: true,
            disconnectionError: null,
            currentTaskId: 'task-123'
          },
        },
      });

      return (
        <Provider store={store}>
          <Story />
        </Provider>
      );
    },
  ],
};

export const Skeleton: Story = {
  render: () => <ConnectionCardSkeleton />,
};
