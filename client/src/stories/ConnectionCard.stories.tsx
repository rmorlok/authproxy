import React from 'react';
import { Meta, StoryObj } from '@storybook/react';
import ConnectionCard, { ConnectionCardSkeleton } from '../components/ConnectionCard';
import { Connection, ConnectionState } from '../models';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import connectorsReducer from '../store/connectorsSlice';

// Create a mock store with connectors
const mockStore = configureStore({
  reducer: {
    connectors: connectorsReducer,
  },
  preloadedState: {
    connectors: {
      items: [
        {
          id: 'google-calendar',
          display_name: 'Google Calendar',
          description: 'Connect to your Google Calendar to manage events and appointments.',
          logo: 'https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg',
        },
      ],
      status: 'succeeded',
      error: null,
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
  connector_id: 'google-calendar',
  state: ConnectionState.CONNECTED,
  created_at: '2023-04-01T12:00:00Z',
  updated_at: '2023-04-01T12:00:00Z',
};

export const Connected: Story = {
  args: {
    connection: mockConnection,
  },
};

export const Created: Story = {
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.CREATED,
    },
  },
};

export const Failed: Story = {
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.FAILED,
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

export const Skeleton: Story = {
  render: () => <ConnectionCardSkeleton />,
};